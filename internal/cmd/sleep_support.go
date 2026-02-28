package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/engine"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/pterm/pterm"
)

const (
	SleepStatePathEnvVar = "GOVARD_SLEEP_STATE_PATH"
	sleepStateVersion    = 1
)

type sleepProjectState struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type sleepStateDocument struct {
	Version     int                 `json:"version"`
	GeneratedAt string              `json:"generated_at"`
	Projects    []sleepProjectState `json:"projects"`
}

type sleepProjectTarget struct {
	Name string
	Path string
}

var discoverRunningGovardProjectsForSleep = discoverRunningGovardProjects
var readProjectRegistryEntriesForSleep = engine.ReadProjectRegistryEntries
var runProjectGovardCommandForSleep = runGovardProjectCommand

func sleepStatePath() string {
	if override := strings.TrimSpace(os.Getenv(SleepStatePathEnvVar)); override != "" {
		return filepath.Clean(override)
	}
	return filepath.Join(engine.GovardHomeDir(), "sleep-state.json")
}

func readSleepState() (sleepStateDocument, error) {
	path := sleepStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sleepStateDocument{}, nil
		}
		return sleepStateDocument{}, fmt.Errorf("read sleep state %s: %w", path, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return sleepStateDocument{}, nil
	}

	state := sleepStateDocument{}
	if err := json.Unmarshal(data, &state); err != nil {
		return sleepStateDocument{}, fmt.Errorf("parse sleep state %s: %w", path, err)
	}
	filtered := make([]sleepProjectState, 0, len(state.Projects))
	for _, project := range state.Projects {
		name := strings.TrimSpace(project.Name)
		projectPath := filepath.Clean(strings.TrimSpace(project.Path))
		if name == "" || projectPath == "" || projectPath == "." {
			continue
		}
		filtered = append(filtered, sleepProjectState{Name: name, Path: projectPath})
	}
	state.Projects = filtered
	if state.Version <= 0 {
		state.Version = sleepStateVersion
	}
	return state, nil
}

func writeSleepState(state sleepStateDocument) error {
	state.Version = sleepStateVersion
	if strings.TrimSpace(state.GeneratedAt) == "" {
		state.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}

	normalized := make([]sleepProjectState, 0, len(state.Projects))
	for _, project := range state.Projects {
		name := strings.TrimSpace(project.Name)
		projectPath := filepath.Clean(strings.TrimSpace(project.Path))
		if name == "" || projectPath == "" || projectPath == "." {
			continue
		}
		normalized = append(normalized, sleepProjectState{Name: name, Path: projectPath})
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Name == normalized[j].Name {
			return normalized[i].Path < normalized[j].Path
		}
		return normalized[i].Name < normalized[j].Name
	})
	state.Projects = normalized

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sleep state: %w", err)
	}
	payload = append(payload, '\n')

	path := sleepStatePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create sleep state dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "sleep-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create sleep state temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	defer cleanup()

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write sleep state temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close sleep state temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod sleep state temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace sleep state file %s: %w", path, err)
	}
	return nil
}

func clearSleepState() error {
	path := sleepStatePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove sleep state %s: %w", path, err)
	}
	return nil
}

func discoverRunningGovardProjects() ([]string, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connect Docker: %w", err)
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list running containers: %w", err)
	}

	projects := map[string]bool{}
	for _, item := range containers {
		projectName := strings.TrimSpace(item.Labels["com.docker.compose.project"])
		if projectName == "" {
			projectName = parseProjectFromContainerNames(item.Names)
		}
		if projectName == "" {
			continue
		}
		if projectName == "proxy" || projectName == "warden" {
			continue
		}
		projects[projectName] = true
	}

	names := make([]string, 0, len(projects))
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func parseProjectFromContainerNames(names []string) string {
	for _, rawName := range names {
		cleanName := strings.TrimPrefix(strings.TrimSpace(rawName), "/")
		if cleanName == "" {
			continue
		}
		parts := strings.Split(cleanName, "-")
		if len(parts) < 3 {
			continue
		}
		return strings.Join(parts[:len(parts)-2], "-")
	}
	return ""
}

func resolveSleepTargets(runningProjects []string, entries []engine.ProjectRegistryEntry) ([]sleepProjectTarget, []string) {
	entryByProject := map[string]engine.ProjectRegistryEntry{}
	entryByBase := map[string]engine.ProjectRegistryEntry{}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.ProjectName)
		if name != "" {
			if _, exists := entryByProject[name]; !exists {
				entryByProject[name] = entry
			}
		}
		base := filepath.Base(strings.TrimSpace(entry.Path))
		if base != "" {
			if _, exists := entryByBase[base]; !exists {
				entryByBase[base] = entry
			}
		}
	}

	seen := map[string]bool{}
	targets := make([]sleepProjectTarget, 0, len(runningProjects))
	missing := make([]string, 0)

	for _, projectName := range runningProjects {
		name := strings.TrimSpace(projectName)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		entry, ok := entryByProject[name]
		if !ok {
			entry, ok = entryByBase[name]
		}
		if !ok {
			missing = append(missing, name)
			continue
		}

		projectPath := filepath.Clean(strings.TrimSpace(entry.Path))
		if projectPath == "" || projectPath == "." {
			missing = append(missing, name)
			continue
		}
		targets = append(targets, sleepProjectTarget{Name: name, Path: projectPath})
	}

	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Name == targets[j].Name {
			return targets[i].Path < targets[j].Path
		}
		return targets[i].Name < targets[j].Name
	})
	sort.Strings(missing)
	return targets, missing
}

func runGovardProjectCommand(projectPath string, args ...string) error {
	path := filepath.Clean(strings.TrimSpace(projectPath))
	if path == "" || path == "." {
		return fmt.Errorf("project path is required")
	}
	if len(args) == 0 {
		return fmt.Errorf("govard subcommand args are required")
	}

	binary, err := os.Executable()
	if err != nil || strings.TrimSpace(binary) == "" {
		binary = "govard"
	}

	command := exec.Command(binary, args...)
	command.Dir = path
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	if err := command.Run(); err != nil {
		return fmt.Errorf("run govard %s in %s: %w", strings.Join(args, " "), path, err)
	}
	return nil
}

func runSleep() error {
	startedAt := time.Now()
	operationStatus := engine.OperationStatusFailure
	operationMessage := ""
	defer func() {
		writeOperationEventBestEffort(
			"sleep.run",
			operationStatus,
			engine.Config{},
			"",
			"",
			operationMessage,
			"",
			time.Since(startedAt),
		)
	}()

	runningProjects, err := discoverRunningGovardProjectsForSleep()
	if err != nil {
		operationMessage = err.Error()
		return err
	}
	if len(runningProjects) == 0 {
		_ = clearSleepState()
		pterm.Info.Println("No running projects found. Nothing to sleep.")
		operationStatus = engine.OperationStatusSuccess
		operationMessage = "no running projects found"
		return nil
	}

	entries, err := readProjectRegistryEntriesForSleep()
	if err != nil {
		operationMessage = err.Error()
		return err
	}

	targets, missing := resolveSleepTargets(runningProjects, entries)
	for _, projectName := range missing {
		pterm.Warning.Printf("Skipping %s: not found in project registry.\n", projectName)
	}
	if len(targets) == 0 {
		_ = clearSleepState()
		pterm.Warning.Println("No running projects matched the project registry. Nothing to sleep.")
		operationStatus = engine.OperationStatusSuccess
		operationMessage = "no running projects matched project registry"
		return nil
	}

	stopped := make([]sleepProjectState, 0, len(targets))
	failures := make([]string, 0)

	for _, target := range targets {
		pterm.Info.Printf("Sleeping %s...\n", target.Name)
		if err := runProjectGovardCommandForSleep(target.Path, "stop"); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", target.Name, err))
			pterm.Error.Printf("Failed to sleep %s: %v\n", target.Name, err)
			continue
		}
		stopped = append(stopped, sleepProjectState(target))
	}

	if len(stopped) > 0 {
		state := sleepStateDocument{
			Version:     sleepStateVersion,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Projects:    stopped,
		}
		if err := writeSleepState(state); err != nil {
			operationMessage = err.Error()
			return err
		}
		pterm.Success.Printf("Saved sleep state for %d project(s): %s\n", len(stopped), sleepStatePath())
	} else {
		_ = clearSleepState()
	}

	if len(failures) > 0 {
		operationMessage = strings.Join(failures, "; ")
		return fmt.Errorf("failed to sleep %d project(s): %s", len(failures), operationMessage)
	}

	operationStatus = engine.OperationStatusSuccess
	operationMessage = fmt.Sprintf("slept %d project(s)", len(stopped))
	return nil
}

func runWake() error {
	startedAt := time.Now()
	operationStatus := engine.OperationStatusFailure
	operationMessage := ""
	defer func() {
		writeOperationEventBestEffort(
			"wake.run",
			operationStatus,
			engine.Config{},
			"",
			"",
			operationMessage,
			"",
			time.Since(startedAt),
		)
	}()

	state, err := readSleepState()
	if err != nil {
		operationMessage = err.Error()
		return err
	}
	if len(state.Projects) == 0 {
		pterm.Info.Println("No sleep state found. Nothing to wake.")
		operationStatus = engine.OperationStatusSuccess
		operationMessage = "no sleep state"
		return nil
	}

	remaining := make([]sleepProjectState, 0)
	failures := make([]string, 0)
	wokeCount := 0

	for _, project := range state.Projects {
		pterm.Info.Printf("Waking %s...\n", project.Name)
		if err := runProjectGovardCommandForSleep(project.Path, "up"); err != nil {
			remaining = append(remaining, project)
			failures = append(failures, fmt.Sprintf("%s: %v", project.Name, err))
			pterm.Error.Printf("Failed to wake %s: %v\n", project.Name, err)
			continue
		}
		wokeCount++
	}

	if len(remaining) > 0 {
		state.Projects = remaining
		state.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeSleepState(state); err != nil {
			operationMessage = err.Error()
			return err
		}
	} else if err := clearSleepState(); err != nil {
		operationMessage = err.Error()
		return err
	}

	if len(failures) > 0 {
		operationMessage = strings.Join(failures, "; ")
		return fmt.Errorf("failed to wake %d project(s): %s", len(failures), operationMessage)
	}

	pterm.Success.Printf("Woke %d project(s).\n", wokeCount)
	operationStatus = engine.OperationStatusSuccess
	operationMessage = fmt.Sprintf("woke %d project(s)", wokeCount)
	return nil
}

// RunSleepForTest exposes sleep execution for tests.
func RunSleepForTest() error {
	return runSleep()
}

// RunWakeForTest exposes wake execution for tests.
func RunWakeForTest() error {
	return runWake()
}

// SetDiscoverRunningGovardProjectsForSleepForTest overrides running-project discovery for tests.
func SetDiscoverRunningGovardProjectsForSleepForTest(fn func() ([]string, error)) func() {
	previous := discoverRunningGovardProjectsForSleep
	if fn == nil {
		discoverRunningGovardProjectsForSleep = discoverRunningGovardProjects
	} else {
		discoverRunningGovardProjectsForSleep = fn
	}
	return func() {
		discoverRunningGovardProjectsForSleep = previous
	}
}

// SetReadProjectRegistryEntriesForSleepForTest overrides registry reader for tests.
func SetReadProjectRegistryEntriesForSleepForTest(fn func() ([]engine.ProjectRegistryEntry, error)) func() {
	previous := readProjectRegistryEntriesForSleep
	if fn == nil {
		readProjectRegistryEntriesForSleep = engine.ReadProjectRegistryEntries
	} else {
		readProjectRegistryEntriesForSleep = fn
	}
	return func() {
		readProjectRegistryEntriesForSleep = previous
	}
}

// SetRunProjectGovardCommandForSleepForTest overrides project command runner for tests.
func SetRunProjectGovardCommandForSleepForTest(fn func(projectPath string, args ...string) error) func() {
	previous := runProjectGovardCommandForSleep
	if fn == nil {
		runProjectGovardCommandForSleep = runGovardProjectCommand
	} else {
		runProjectGovardCommandForSleep = fn
	}
	return func() {
		runProjectGovardCommandForSleep = previous
	}
}

// SleepStatePathForTest exposes resolved sleep-state path for tests.
func SleepStatePathForTest() string {
	return sleepStatePath()
}
