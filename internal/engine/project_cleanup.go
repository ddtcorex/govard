package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/proxy"
)

// DeleteProject performs a full cleanup of a project's Govard-managed resources.
// It stops containers, removes volumes, unregisters domains from proxy and hosts,
// runs lifecycle hooks, and removes the project from the registry.
func DeleteProject(ctx context.Context, projectPath string, stdout, stderr io.Writer) error {
	projectPath = filepath.Clean(strings.TrimSpace(projectPath))
	if projectPath == "" {
		return fmt.Errorf("project path is required")
	}

	// 1. Load config (best effort)
	config, _, loadErr := LoadConfigFromDir(projectPath, true)

	// Derive project name: from config if available, otherwise from directory name
	projectName := filepath.Base(projectPath)
	var profiles []string
	if entry, ok := GetProjectRegistryEntry(projectPath); ok {
		// The registry remembers the name and profiles even when the project
		// directory or its config is gone.
		if entry.ProjectName != "" {
			projectName = entry.ProjectName
		}
		profiles = append(profiles, entry.Profile, entry.PreviousProfile)
	}
	if loadErr == nil && config.ProjectName != "" {
		projectName = config.ProjectName
	}
	if loadErr == nil && config.Profile != "" {
		profiles = append(profiles, config.Profile)
	}

	// 2. Pre-delete hooks (only if config exists)
	if loadErr == nil {
		if err := RunHooks(config, HookPreDelete, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "Warning: pre-delete hooks failed: %v\n", err)
		}
	}

	// 3. Stop containers and remove volumes (down -v)
	// We use the project name explicitly to allow cleanup even if compose file is missing
	composePath := ""
	if loadErr == nil {
		path := ComposeFilePathWithProfile(projectPath, projectName, config.Profile)
		if _, err := os.Stat(path); err == nil {
			composePath = path
		}
	}

	stderr = quietStderr(stderr)
	err := RunCompose(ctx, ComposeOptions{
		ProjectDir:  projectPath,
		ProjectName: projectName,
		ComposeFile: composePath,
		Args:        []string{"down", "-v", "--remove-orphans"},
		Stdout:      stdout,
		Stderr:      stderr,
		Stdin:       os.Stdin,
	})
	if err != nil {
		// During deletion, we treat docker failures as warnings to ensure
		// the project can still be removed from the registry even if Docker is stuck.
		fmt.Fprintf(stderr, "Warning: docker compose down -v: %v\n", err)
	}

	// 3b. The frontend compose project, leftover containers/networks/volumes
	// and the rendered files under the Govard home. Matched by exact name only.
	cleanupProjectResidue(ctx, projectName, projectPath, profiles, stdout, stderr)

	// 4. Unregister domains from proxy and hosts (only if config exists)
	if loadErr == nil {
		for _, domain := range config.AllDomains() {
			if err := proxy.UnregisterDomain(domain); err != nil {
				fmt.Fprintf(stderr, "Warning: Could not remove proxy route for %s: %v\n", domain, err)
			}
			if err := RemoveHostsEntry(domain); err != nil {
				fmt.Fprintf(stderr, "Warning: Could not remove hosts entry for %s: %v\n", domain, err)
			}
		}
	}

	// 5. Post-delete hooks (only if config exists)
	if loadErr == nil {
		if err := RunHooks(config, HookPostDelete, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "Warning: post-delete hooks failed: %v\n", err)
		}
	}

	// 6. Remove from registry (always)
	if err := DeleteProjectRegistryEntry(projectPath); err != nil {
		return fmt.Errorf("failed to remove project from registry: %w", err)
	}

	return nil
}
