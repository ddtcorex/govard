package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// composeProjectLabel is the label Docker Compose stamps on every container,
// network and volume it creates. Filtering on label=<key>=<value> is an exact
// match, so a project is never matched by prefix or substring.
const composeProjectLabel = "com.docker.compose.project"

// reservedProjectNames are compose projects Govard itself owns; deleting a
// project must never reach them.
var reservedProjectNames = map[string]bool{"proxy": true, "warden": true}

// projectHomeDirs are the per-project directories Govard renders under its
// home, each keyed by the exact project name.
var projectHomeDirs = []string{"varnish", "rabbitmq", "nginx", "apache"}

// ProjectArtifacts is everything a project leaves under the Govard home: its
// rendered compose files (and their .hash companions), its per-project asset
// directories and its entry in active-projects.json.
type ProjectArtifacts struct {
	Name        string
	Root        string
	Files       []string
	Dirs        []string
	ActiveEntry bool
	// Docker is true when containers, networks or volumes labelled with the
	// project (or its -frontend companion) exist.
	Docker bool
}

// Empty reports whether the project left nothing behind.
func (a ProjectArtifacts) Empty() bool {
	return len(a.Files) == 0 && len(a.Dirs) == 0 && !a.ActiveEntry && !a.Docker
}

// Lines describes the artifacts for a confirmation prompt or a report.
func (a ProjectArtifacts) Lines() []string {
	var lines []string
	if a.Docker {
		lines = append(lines, fmt.Sprintf("Docker containers, networks and volumes of %q and %q", a.Name, a.Name+"-frontend"))
	}
	lines = append(lines, a.Files...)
	for _, d := range a.Dirs {
		lines = append(lines, d+string(filepath.Separator))
	}
	if a.ActiveEntry {
		lines = append(lines, fmt.Sprintf("%s (entry %q)", activeProjectsPath(), a.Name))
	}
	return lines
}

// SafeProjectArtifactName reports whether name can safely key a path under the
// Govard home and a Docker label filter.
func SafeProjectArtifactName(name string) bool {
	if name == "" || reservedProjectNames[strings.ToLower(name)] {
		return false
	}
	if name != filepath.Base(name) || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`+"\x00")
}

func activeProjectsPath() string {
	return filepath.Join(GovardHomeDir(), "active-projects.json")
}

// CollectProjectArtifacts lists the artifacts of one project by exact name.
// With a known root the compose files are the exact paths Govard renders for it
// (per profile); without one only the profile-less `<name>-<hash>.yml` shape is
// matched, because a profile suffix cannot be told from a longer project name.
func CollectProjectArtifacts(name, root string, profiles []string) ProjectArtifacts {
	art := ProjectArtifacts{Name: name, Root: root}
	if !SafeProjectArtifactName(name) {
		return art
	}

	seen := map[string]bool{}
	add := func(path string) {
		for _, candidate := range []string{path, path + ".hash"} {
			if seen[candidate] {
				continue
			}
			if info, err := os.Lstat(candidate); err == nil && !info.IsDir() {
				seen[candidate] = true
				art.Files = append(art.Files, candidate)
			}
		}
	}

	if strings.TrimSpace(root) != "" {
		set := append([]string{""}, profiles...)
		for _, profile := range set {
			profile = strings.TrimSpace(profile)
			add(ComposeFilePathWithProfile(root, name, profile))
			add(FrontendComposeFilePath(root, name, profile))
		}
	} else {
		composeDir := filepath.Join(GovardHomeDir(), "compose")
		for _, dir := range []string{composeDir, filepath.Join(composeDir, "frontend")} {
			base := name
			if dir != composeDir {
				base = name + "-frontend"
			}
			pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(sanitizeComposeProjectName(base)) + `-[0-9a-f]{12}\.yml$`)
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() && pattern.MatchString(entry.Name()) {
					add(filepath.Join(dir, entry.Name()))
				}
			}
		}
	}

	home := GovardHomeDir()
	for _, sub := range projectHomeDirs {
		dir := filepath.Join(home, sub, name)
		if info, err := os.Lstat(dir); err == nil && info.IsDir() {
			art.Dirs = append(art.Dirs, dir)
		}
	}
	art.ActiveEntry = activeProjectListed(name)
	return art
}

func readActiveProjects() ([]string, bool) {
	data, err := os.ReadFile(activeProjectsPath())
	if err != nil {
		return nil, false
	}
	doc := pmaActiveProjectsDocument{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false
	}
	return doc.Projects, true
}

func activeProjectListed(name string) bool {
	projects, ok := readActiveProjects()
	if !ok {
		return false
	}
	for _, p := range projects {
		if p == name {
			return true
		}
	}
	return false
}

// RemoveActiveProject drops one exact name from active-projects.json.
func RemoveActiveProject(name string) error {
	projects, ok := readActiveProjects()
	if !ok {
		return nil
	}
	kept := make([]string, 0, len(projects))
	found := false
	for _, p := range projects {
		if p == name {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return nil
	}
	return writePMAActiveProjectsFile(activeProjectsPath(), kept)
}

func withinGovardHome(path string) bool {
	home := filepath.Clean(GovardHomeDir())
	rel, err := filepath.Rel(home, filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// RemoveProjectArtifacts deletes the collected files and directories and the
// active-projects entry. It only ever touches paths inside the Govard home.
// It returns the paths it removed; failures are reported to stderr and do not
// stop the sweep.
func RemoveProjectArtifacts(a ProjectArtifacts, stderr io.Writer) []string {
	var removed []string
	for _, path := range append(append([]string{}, a.Files...), a.Dirs...) {
		if !withinGovardHome(path) {
			fmt.Fprintf(stderr, "Warning: refusing to remove %s: outside the Govard home\n", path)
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			fmt.Fprintf(stderr, "Warning: could not remove %s: %v\n", path, err)
			continue
		}
		removed = append(removed, path)
	}
	if a.ActiveEntry && SafeProjectArtifactName(a.Name) {
		if err := RemoveActiveProject(a.Name); err != nil {
			fmt.Fprintf(stderr, "Warning: could not update active-projects.json: %v\n", err)
		} else {
			removed = append(removed, activeProjectsPath())
		}
	}
	return removed
}

func dockerListByLabel(ctx context.Context, args []string, project string) []string {
	full := append(append([]string{}, args...), "--filter", "label="+composeProjectLabel+"="+project)
	out, err := exec.CommandContext(ctx, "docker", full...).Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

// ComposeProjectResourcesExist reports whether any container, network or
// volume carries the exact compose project label for name or name-frontend.
func ComposeProjectResourcesExist(ctx context.Context, name string) bool {
	if !SafeProjectArtifactName(name) {
		return false
	}
	for _, project := range []string{name, name + "-frontend"} {
		for _, args := range composeListArgs {
			if len(dockerListByLabel(ctx, args, project)) > 0 {
				return true
			}
		}
	}
	return false
}

var composeListArgs = [][]string{
	{"ps", "-aq"},
	{"network", "ls", "-q"},
	{"volume", "ls", "-q"},
}

// RemoveComposeProjectResources removes whatever `compose down` left behind for
// the project and its -frontend companion: containers, networks and volumes
// matched by the exact compose project label.
func RemoveComposeProjectResources(ctx context.Context, name string, stdout, stderr io.Writer) error {
	if !SafeProjectArtifactName(name) {
		return fmt.Errorf("refusing to remove Docker resources of project name %q", name)
	}
	var firstErr error
	for _, project := range []string{name, name + "-frontend"} {
		steps := []struct {
			list   []string
			remove []string
		}{
			{[]string{"ps", "-aq"}, []string{"rm", "-f"}},
			{[]string{"network", "ls", "-q"}, []string{"network", "rm"}},
			{[]string{"volume", "ls", "-q"}, []string{"volume", "rm"}},
		}
		for _, step := range steps {
			ids := dockerListByLabel(ctx, step.list, project)
			if len(ids) == 0 {
				continue
			}
			cmd := exec.CommandContext(ctx, "docker", append(append([]string{}, step.remove...), ids...)...)
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(stderr, "Warning: docker %s: %v\n", strings.Join(step.remove, " "), err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}

// DiscoverProjectByQuery resolves a delete query that is not (or no longer)
// in the registry: an existing project directory, or an exact project name.
// It reports found only when the project left artifacts behind.
func DiscoverProjectByQuery(ctx context.Context, query string) (ProjectArtifacts, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return ProjectArtifacts{}, false
	}

	root, name := "", ""
	if strings.ContainsRune(query, filepath.Separator) || strings.HasPrefix(query, ".") || strings.HasPrefix(query, "~") {
		path := query
		if strings.HasPrefix(path, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, strings.TrimPrefix(path, "~"))
			}
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return ProjectArtifacts{}, false
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return ProjectArtifacts{}, false
		}
		root = abs
		name = NormalizeProjectName(filepath.Base(abs))
		if cfg, _, err := LoadConfigFromDir(abs, true); err == nil && strings.TrimSpace(cfg.ProjectName) != "" {
			name = cfg.ProjectName
		}
	} else {
		name = NormalizeProjectName(query)
	}
	if !SafeProjectArtifactName(name) {
		return ProjectArtifacts{}, false
	}

	var profiles []string
	if root != "" {
		if cfg, _, err := LoadConfigFromDir(root, true); err == nil && cfg.Profile != "" {
			profiles = append(profiles, cfg.Profile)
		}
	}
	art := CollectProjectArtifacts(name, root, profiles)
	art.Docker = ComposeProjectResourcesExist(ctx, name)
	return art, !art.Empty()
}

// DeleteProjectByName removes a project that has no registry entry: it brings
// the compose project down by exact name, sweeps leftover Docker resources and
// deletes the artifacts. With a known root the full DeleteProject path is used
// instead, so hooks and proxy routes are handled too.
func DeleteProjectByName(ctx context.Context, a ProjectArtifacts, stdout, stderr io.Writer) error {
	if !SafeProjectArtifactName(a.Name) {
		return fmt.Errorf("refusing to delete project name %q", a.Name)
	}
	if strings.TrimSpace(a.Root) != "" {
		return DeleteProject(ctx, a.Root, stdout, stderr)
	}
	cleanupProjectResidue(ctx, a.Name, "", nil, stdout, stderr)
	return nil
}

// cleanupProjectResidue is the part of a delete that works from the project
// name alone: the frontend compose project, leftover Docker resources and the
// artifacts under the Govard home.
func cleanupProjectResidue(ctx context.Context, name, root string, profiles []string, stdout, stderr io.Writer) {
	if !SafeProjectArtifactName(name) {
		return
	}
	for _, project := range []string{name, name + "-frontend"} {
		args := []string{"compose", "-p", project, "down", "-v", "--remove-orphans"}
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(stderr, "Warning: docker compose -p %s down -v: %v\n", project, err)
		}
	}
	_ = RemoveComposeProjectResources(ctx, name, stdout, stderr)
	RemoveProjectArtifacts(CollectProjectArtifacts(name, root, profiles), stderr)
	_ = RemoveActiveProject(name)
}
