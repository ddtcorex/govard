package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	ProjectExtensionsDir       = ".govard"
	ProjectCommandsDir         = ".govard/commands"
	ProjectHooksDir            = ".govard/hooks"
	ProjectLocalConfigPath     = ".govard/govard.local.yml"
	ProjectComposeOverridePath = ".govard/docker-compose.override.yml"
	GlobalCommandsDirEnvVar    = "GOVARD_GLOBAL_COMMANDS_DIR"
)

type ProjectCommand struct {
	Name string
	Path string
}

var validProjectCommandName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func EnsureExtensionContract(root string, force bool) ([]string, error) {
	cleanRoot := filepath.Clean(root)
	if cleanRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}

	dirs := []string{
		filepath.Join(cleanRoot, ProjectExtensionsDir),
		filepath.Join(cleanRoot, ProjectCommandsDir),
		filepath.Join(cleanRoot, ProjectHooksDir),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	files := []scaffoldFile{
		{
			Path: filepath.Join(cleanRoot, ProjectLocalConfigPath),
			Mode: 0644,
			Content: `# Project-local Govard overrides (recommended to keep uncommitted).
# This file is loaded after govard.yml.
#
# Example:
# stack:
#   php_version: "8.4"
#
# hooks:
#   pre_up:
#     - name: "Project pre-up"
#       run: "bash .govard/hooks/pre_up.sh"
`,
		},
		{
			Path: filepath.Join(cleanRoot, ProjectComposeOverridePath),
			Mode: 0644,
			Content: `# Project-specific compose overrides merged after framework blueprints.
#
# Example:
# services:
#   php:
#     environment:
#       APP_ENV: local
`,
		},
		{
			Path: filepath.Join(cleanRoot, ProjectHooksDir, "pre_up.sh"),
			Mode: 0755,
			Content: `#!/usr/bin/env bash
set -euo pipefail

echo "[govard hook] pre_up: project-specific setup"
`,
		},
		{
			Path: filepath.Join(cleanRoot, ProjectCommandsDir, "hello"),
			Mode: 0755,
			Content: `#!/usr/bin/env bash
set -euo pipefail

echo "Hello from .govard/commands/hello"
echo "Args: $*"
`,
		},
	}

	changed := make([]string, 0, len(files))
	for _, file := range files {
		written, err := writeScaffoldFile(file, force)
		if err != nil {
			return nil, err
		}
		if written {
			rel, relErr := filepath.Rel(cleanRoot, file.Path)
			if relErr != nil {
				changed = append(changed, file.Path)
			} else {
				changed = append(changed, rel)
			}
		}
	}

	sort.Strings(changed)
	return changed, nil
}

func DiscoverProjectCommands(root string) ([]ProjectCommand, error) {
	commandsDir := filepath.Join(filepath.Clean(root), ProjectCommandsDir)
	return discoverCommandsFromDir(commandsDir)
}

func DiscoverGlobalCommands() ([]ProjectCommand, error) {
	commandsDir := strings.TrimSpace(os.Getenv(GlobalCommandsDirEnvVar))
	if commandsDir == "" {
		commandsDir = filepath.Join(GovardHomeDir(), "commands")
	}
	return discoverCommandsFromDir(commandsDir)
}

func DiscoverMergedCommands(root string) ([]ProjectCommand, error) {
	projectCommands, err := DiscoverProjectCommands(root)
	if err != nil {
		return nil, err
	}

	globalCommands, err := DiscoverGlobalCommands()
	if err != nil {
		return nil, err
	}

	resolved := map[string]ProjectCommand{}
	for _, command := range projectCommands {
		resolved[command.Name] = command
	}
	for _, command := range globalCommands {
		if _, exists := resolved[command.Name]; exists {
			continue
		}
		resolved[command.Name] = command
	}

	names := make([]string, 0, len(resolved))
	for name := range resolved {
		names = append(names, name)
	}
	sort.Strings(names)

	commands := make([]ProjectCommand, 0, len(names))
	for _, name := range names {
		commands = append(commands, resolved[name])
	}
	return commands, nil
}

func discoverCommandsFromDir(commandsDir string) ([]ProjectCommand, error) {
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ProjectCommand{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", commandsDir, err)
	}

	resolved := map[string]ProjectCommand{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := normalizeProjectCommandName(entry.Name())
		if name == "" {
			continue
		}
		if _, exists := resolved[name]; exists {
			continue
		}

		resolved[name] = ProjectCommand{
			Name: name,
			Path: filepath.Join(commandsDir, entry.Name()),
		}
	}

	names := make([]string, 0, len(resolved))
	for name := range resolved {
		names = append(names, name)
	}
	sort.Strings(names)

	commands := make([]ProjectCommand, 0, len(names))
	for _, name := range names {
		commands = append(commands, resolved[name])
	}
	return commands, nil
}

type scaffoldFile struct {
	Path    string
	Content string
	Mode    os.FileMode
}

func writeScaffoldFile(file scaffoldFile, force bool) (bool, error) {
	current, err := os.ReadFile(file.Path)
	if err == nil && !force {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", file.Path, err)
	}
	if err == nil && string(current) == file.Content {
		if chmodErr := os.Chmod(file.Path, file.Mode); chmodErr != nil {
			return false, fmt.Errorf("chmod %s: %w", file.Path, chmodErr)
		}
		return false, nil
	}

	if writeErr := os.WriteFile(file.Path, []byte(file.Content), file.Mode); writeErr != nil {
		return false, fmt.Errorf("write %s: %w", file.Path, writeErr)
	}
	return true, nil
}

func normalizeProjectCommandName(fileName string) string {
	base := strings.TrimSpace(fileName)
	if base == "" || strings.HasPrefix(base, ".") {
		return ""
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ToLower(strings.TrimSpace(base))
	if !validProjectCommandName.MatchString(base) {
		return ""
	}
	return base
}
