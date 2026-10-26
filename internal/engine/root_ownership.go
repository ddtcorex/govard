package engine

import (
	"fmt"
	"govard/internal/conventions"
	"os"
	"path/filepath"
	"strings"
)

// RootOwnershipRisk reports whether a command running with euid would create
// root-owned files in a project owned by a regular user: true only when
// running as root on a project whose owner is not root. A root-owned project
// (CI containers) never warns.
func RootOwnershipRisk(projectRoot string, euid int, ownerUID func(string) (int, error)) (bool, error) {
	if euid != 0 {
		return false, nil
	}
	owner, err := ownerUID(projectRoot)
	if err != nil {
		return false, err
	}
	return owner != 0, nil
}

// GovardCustomMount describes the web server custom config bind mount of a
// project that supports the audit profiler: the host directory, the path it
// is mounted at, and the container that mounts it.
type GovardCustomMount struct {
	HostDir       string
	ContainerPath string
	Container     string
}

// CustomConfigMount returns the active web server's custom config mount, or
// false when the framework or web server has none.
func CustomConfigMount(root string, config Config) (GovardCustomMount, bool) {
	if !FrameworkSupportsAuditProfiler(config.Framework) {
		return GovardCustomMount{}, false
	}
	project := strings.TrimSpace(config.ProjectName)
	switch strings.ToLower(strings.TrimSpace(config.Stack.Services.WebServer)) {
	case "nginx":
		return GovardCustomMount{filepath.Join(root, ProjectNginxCustomDir), "/etc/nginx/custom", project + conventions.WebSuffix}, true
	case "apache":
		return GovardCustomMount{filepath.Join(root, ProjectApacheCustomDir), "/usr/local/apache2/conf/custom", project + conventions.WebSuffix}, true
	case "hybrid":
		// Hybrid terminates FastCGI in Apache.
		return GovardCustomMount{filepath.Join(root, ProjectApacheCustomDir), "/usr/local/apache2/conf/custom", project + "-apache" + conventions.ReplicaSuffix}, true
	}
	return GovardCustomMount{}, false
}

// PrepareGovardBindSources creates, as the invoking user, the Govard-owned
// bind mount sources a rendered compose file may reference. A compose up on a
// previously rendered file otherwise lets the Docker daemon recreate a missing
// source as root, which leaves .govard/ root owned.
func PrepareGovardBindSources(root string, config Config) error {
	mount, ok := CustomConfigMount(root, config)
	if !ok {
		return nil
	}
	if err := os.MkdirAll(mount.HostDir, 0o755); err != nil {
		return fmt.Errorf("prepare audit profiler custom config directory: %w", err)
	}
	return nil
}
