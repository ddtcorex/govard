// Package auditmagento embeds the Govard-owned Magento lint image context so
// the audit toolchain can be built locally when the published image is not
// reachable. The context is Govard's own work: runner, reporter, locked
// analyzer manifests, contract tests, and Dockerfile.
package auditmagento

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

//go:embed Dockerfile bin toolchains tests
var contextFiles embed.FS

// ContextFS exposes the embedded Docker build context.
var ContextFS fs.FS = contextFiles

// EntrypointPath is the runner path inside the built image.
const EntrypointPath = "/usr/local/bin/magelint"

// ReportSchemaVersion is the lint report schema the image entrypoint writes.
const ReportSchemaVersion = 2

// PHPVersions lists the exact PHP policy the image supports, in order.
// Project and module-in-project targets may select any of them; standalone
// modules follow the narrower default matrix declared by the framework.
var phpVersions = []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}

// PHPVersions returns a copy of the PHP versions the image provides.
func PHPVersions() []string {
	return append([]string(nil), phpVersions...)
}

// FileMode returns the mode a context file must have once materialized.
// embed.FS reports every file as read-only, so executables are restored from
// their context path; the same rule feeds ContextDigest.
func FileMode(path string) fs.FileMode {
	if strings.HasPrefix(path, "bin/") && path != "bin/report.php" {
		return 0o755
	}
	if strings.HasPrefix(path, "tests/") && strings.HasSuffix(path, ".sh") {
		return 0o755
	}
	return 0o644
}

var (
	contextDigestOnce  sync.Once
	contextDigestValue string
	contextDigestErr   error
)

// ContextDigest returns the SHA-256 digest of the embedded build context. It
// covers every context path, its materialized mode, and its bytes, in sorted
// path order, so an unchanged context always yields an identical digest.
func ContextDigest() (string, error) {
	contextDigestOnce.Do(func() {
		contextDigestValue, contextDigestErr = computeContextDigest()
	})
	return contextDigestValue, contextDigestErr
}

func computeContextDigest() (string, error) {
	paths, err := ContextPaths()
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, path := range paths {
		content, err := fs.ReadFile(ContextFS, path)
		if err != nil {
			return "", fmt.Errorf("read audit lint context %q: %w", path, err)
		}
		fmt.Fprintf(hash, "%s\n%04o\n%d\n", path, FileMode(path).Perm(), len(content))
		hash.Write(content)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// ContextPaths returns every embedded context file path in sorted order.
func ContextPaths() ([]string, error) {
	var paths []string
	err := fs.WalkDir(ContextFS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path == "." {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk audit lint context: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}
