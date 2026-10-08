// Package projectstores tells the engine which verify, audit and lint-cache
// directories a project owns under the Govard home. Those stores are keyed by
// ids derived from the project path by the verify and audit packages, which the
// engine cannot import, so every entry point that deletes projects (the CLI and
// the desktop app) calls Register once at start-up.
package projectstores

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"govard/internal/audit"
	"govard/internal/engine"
	"govard/internal/frameworks/types"
	"govard/internal/verify"
)

var once sync.Once

// Register installs the resolver with the engine. It is idempotent.
func Register() {
	once.Do(func() { engine.RegisterProjectStoreResolver(Resolve) })
}

// RepositoryIdentity is the identity the audit store mixes into a project id:
// the git origin when there is one, else the composer package name.
func RepositoryIdentity(root, origin string) string {
	if strings.TrimSpace(origin) != "" {
		return strings.TrimSpace(origin)
	}
	contents, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return ""
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return ""
	}
	return strings.TrimSpace(manifest.Name)
}

func gitOutput(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Resolve lists the candidate store directories of the project at root.
//
// Lint cache namespaces of project-mode audits are derivable (the target is the
// project root itself). Namespaces of module_in_project and standalone audits
// are keyed by arbitrary module paths and cannot be attributed to a project
// from its root, so they are deliberately left alone rather than deleted by
// guess; they are small, rebuildable caches.
func Resolve(root string) []string {
	canonical := root
	if abs, err := filepath.Abs(root); err == nil {
		canonical = abs
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			canonical = resolved
		}
	}
	home := engine.GovardHomeDir()
	dirs := []string{filepath.Join(home, "verify-runs", verify.ProjectID(root))}
	origin, _ := gitOutput(canonical, "config", "--get", "remote.origin.url")
	identities := []string{"", RepositoryIdentity(canonical, origin)}
	for _, identity := range identities {
		dirs = append(dirs, filepath.Join(home, "audit", audit.ProjectID(canonical, identity)))
	}
	lintRoot := audit.DefaultLintCacheRoot(home)
	paths := []string{canonical}
	if root != canonical {
		paths = append(paths, root)
	}
	for _, identity := range identities {
		projectID := audit.ProjectID(canonical, identity)
		for _, targetPath := range paths {
			dirs = append(dirs, filepath.Join(lintRoot, audit.LintTargetID(projectID, types.AuditTargetProject, targetPath)))
		}
	}
	return dirs
}
