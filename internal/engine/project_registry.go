package engine

import (
	"encoding/json"
	"fmt"
	"govard/internal/conventions"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

const (
	ProjectRegistryPathEnvVar = "GOVARD_PROJECT_REGISTRY_PATH"
	projectRegistryVersion    = 1
)

type ProjectRegistryEntry struct {
	Path             string    `json:"path"`
	ProjectName      string    `json:"project_name"`
	Domain           string    `json:"domain,omitempty"`
	ExtraDomains     []string  `json:"extra_domains,omitempty"`
	Framework        string    `json:"framework,omitempty"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	LastCommand      string    `json:"last_command,omitempty"`
	PHPVersion       string    `json:"php_version,omitempty"`
	Profile          string    `json:"profile,omitempty"`
	PreviousProfile  string    `json:"previous_profile,omitempty"`
	FrameworkVersion string    `json:"framework_version,omitempty"`
}

type projectRegistryDocument struct {
	Version  int                    `json:"version"`
	Projects []ProjectRegistryEntry `json:"projects"`
}

func ProjectRegistryPath() string {
	if override := strings.TrimSpace(os.Getenv(ProjectRegistryPathEnvVar)); override != "" {
		return filepath.Clean(override)
	}

	return filepath.Join(GovardHomeDir(), "projects.json")
}

// StaleProjectRegistryEntries returns the registered project paths whose
// directory no longer exists. A trial workspace removed by hand, or a checkout
// deleted outside Govard, leaves one behind; doctor reports them instead of
// letting `project list` show a project that cannot be opened.
func StaleProjectRegistryEntries() ([]string, error) {
	entries, err := ReadProjectRegistryEntries()
	if err != nil {
		return nil, err
	}

	stale := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := strings.TrimSpace(entry.Path)
		if path == "" {
			continue
		}
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			stale = append(stale, path)
		}
	}
	sort.Strings(stale)

	return stale, nil
}

func ReadProjectRegistryEntries() ([]ProjectRegistryEntry, error) {
	path := ProjectRegistryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ProjectRegistryEntry{}, nil
		}
		return nil, fmt.Errorf("read project registry %s: %w", path, err)
	}

	if strings.TrimSpace(string(data)) == "" {
		return []ProjectRegistryEntry{}, nil
	}

	doc := projectRegistryDocument{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse project registry %s: %w", path, err)
	}

	entries := make([]ProjectRegistryEntry, 0, len(doc.Projects))
	for _, entry := range doc.Projects {
		normalized, ok := normalizeProjectRegistryEntry(entry)
		if !ok {
			continue
		}
		entries = append(entries, normalized)
	}
	sortProjectRegistryEntries(entries)
	return entries, nil
}

func UpsertProjectRegistryEntry(entry ProjectRegistryEntry) error {
	normalized, ok := normalizeProjectRegistryEntry(entry)
	if !ok {
		return fmt.Errorf("project path is required")
	}

	entries, err := ReadProjectRegistryEntries()
	if err != nil {
		return err
	}

	replaced := false
	for index := range entries {
		if entries[index].Path == normalized.Path {
			entries[index] = normalized
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, normalized)
	}

	sortProjectRegistryEntries(entries)
	if err := writeProjectRegistryEntries(ProjectRegistryPath(), entries); err != nil {
		return err
	}
	return nil
}

func GetProjectRegistryEntry(path string) (ProjectRegistryEntry, bool) {
	path = normalizeProjectRegistryPath(path)
	if path == "" {
		return ProjectRegistryEntry{}, false
	}

	entries, err := ReadProjectRegistryEntries()
	if err != nil {
		return ProjectRegistryEntry{}, false
	}

	for _, entry := range entries {
		if entry.Path == path {
			return entry, true
		}
	}

	return ProjectRegistryEntry{}, false
}

// ClearPreviousProfile clears the previous_profile field for a project in the registry.
func ClearPreviousProfile(path string) error {
	path = normalizeProjectRegistryPath(path)
	if path == "" {
		return fmt.Errorf("project path is required")
	}

	entry, ok := GetProjectRegistryEntry(path)
	if !ok {
		return nil // Nothing to clear
	}

	entry.PreviousProfile = ""
	return UpsertProjectRegistryEntry(entry)
}

func DeleteProjectRegistryEntry(path string) error {
	path = normalizeProjectRegistryPath(path)
	if path == "" {
		return fmt.Errorf("project path is required")
	}

	entries, err := ReadProjectRegistryEntries()
	if err != nil {
		return err
	}

	filtered := make([]ProjectRegistryEntry, 0, len(entries))
	found := false
	for _, entry := range entries {
		if entry.Path == path {
			found = true
			continue
		}
		filtered = append(filtered, entry)
	}

	if !found {
		return nil
	}

	return writeProjectRegistryEntries(ProjectRegistryPath(), filtered)
}

func ValidateProjectIdentityUniqueness(projectPath string, config Config) error {
	entries, err := ReadProjectRegistryEntries()
	if err != nil {
		return fmt.Errorf("read project registry: %w", err)
	}

	cleanPath := normalizeProjectRegistryPath(projectPath)
	projectName := strings.TrimSpace(strings.ToLower(config.ProjectName))
	if projectName == "" && cleanPath != "." && cleanPath != "" {
		projectName = NormalizeProjectName(filepath.Base(cleanPath))
	}

	requestedDomains := map[string]struct{}{}
	for _, domain := range config.AllDomains() {
		normalized := strings.TrimSpace(strings.ToLower(domain))
		if normalized == "" {
			continue
		}
		requestedDomains[normalized] = struct{}{}
	}

	for _, entry := range entries {
		entryPath := filepath.Clean(strings.TrimSpace(entry.Path))
		if cleanPath != "" && entryPath == cleanPath {
			continue
		}

		if projectName != "" && strings.EqualFold(strings.TrimSpace(entry.ProjectName), projectName) {
			return fmt.Errorf("project_name %s is already used by %s", projectName, entryPath)
		}

		for _, domain := range append([]string{entry.Domain}, entry.ExtraDomains...) {
			normalized := strings.TrimSpace(strings.ToLower(domain))
			if normalized == "" {
				continue
			}
			if _, exists := requestedDomains[normalized]; !exists {
				continue
			}

			projectLabel := strings.TrimSpace(entry.ProjectName)
			if projectLabel == "" {
				projectLabel = entryPath
			}
			return fmt.Errorf("domain %s is already used by project %s (%s)", normalized, projectLabel, entryPath)
		}
	}

	return nil
}

func normalizeProjectRegistryEntry(entry ProjectRegistryEntry) (ProjectRegistryEntry, bool) {
	entry.Path = normalizeProjectRegistryPath(entry.Path)
	if entry.Path == "" {
		return ProjectRegistryEntry{}, false
	}

	if os.Getenv(ProjectRegistryPathEnvVar) == "" {
		tempDir := normalizeProjectRegistryPath(os.TempDir())
		if strings.HasPrefix(entry.Path, "/tmp/") || strings.HasPrefix(entry.Path, tempDir) || strings.Contains(entry.Path, "govard/tests") {
			return ProjectRegistryEntry{}, false
		}
	}

	entry.ProjectName = strings.TrimSpace(entry.ProjectName)
	entry.Domain = strings.TrimSpace(entry.Domain)
	for i, d := range entry.ExtraDomains {
		entry.ExtraDomains[i] = strings.TrimSpace(d)
	}
	entry.Framework = strings.TrimSpace(strings.ToLower(entry.Framework))
	entry.LastCommand = strings.TrimSpace(strings.ToLower(entry.LastCommand))
	entry.PHPVersion = strings.TrimSpace(entry.PHPVersion)
	entry.Profile = strings.TrimSpace(entry.Profile)
	entry.FrameworkVersion = strings.TrimSpace(entry.FrameworkVersion)
	if entry.LastSeenAt.IsZero() {
		entry.LastSeenAt = time.Now().UTC()
	} else {
		entry.LastSeenAt = entry.LastSeenAt.UTC()
	}
	if entry.ProjectName == "" {
		entry.ProjectName = filepath.Base(entry.Path)
	}
	return entry, true
}

func normalizeProjectRegistryPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if path == "." {
		return path
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return path
}

func sortProjectRegistryEntries(entries []ProjectRegistryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].LastSeenAt.Equal(entries[j].LastSeenAt) {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].LastSeenAt.After(entries[j].LastSeenAt)
	})
}

func writeProjectRegistryEntries(path string, entries []ProjectRegistryEntry) error {
	doc := projectRegistryDocument{
		Version:  projectRegistryVersion,
		Projects: entries,
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project registry: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create project registry dir %s: %w", dir, err)
	}

	// Defensive check: if the target path exists as a directory (common corruption symptom), remove it.
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		pterm.Warning.Printf("Registry path %s is a directory, attempting removal to restore registry file.\n", path)
		_ = os.Remove(path) // Try non-recursive removal first for safety
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			// If still exists, try recursive remove if it's the specific projects.json corruption
			if strings.HasSuffix(path, "projects.json") {
				_ = os.RemoveAll(path)
			}
		}
	}

	tmpFile, err := os.CreateTemp(dir, "projects-*.tmp")
	if err != nil {
		return fmt.Errorf("create project registry temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	defer cleanup()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write project registry temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close project registry temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("chmod project registry temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace project registry file %s: %w", path, err)
	}
	return nil
}
