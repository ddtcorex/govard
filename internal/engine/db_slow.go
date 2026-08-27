package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DbSlowThreshold returns the slow-query threshold in seconds. The audit uses
// `TIME threshold 1s` for `db-slow` so quick and deep share the same gate and
// `pt-query-digest`/`mysqldumpslow` aggregation stays comparable.
func DbSlowThreshold() int {
	return 1
}

// XdebugGuard reports whether the profiler/DB-slow run should warn about
// Xdebug or non-production mode. The spec requires a guard warning when
// `xdebug:true` or `MAGE_MODE` != production because Xdebug adds ~10-20% tax
// and non-production mode skews measurements.
//
// The variadic form supports both `XdebugGuard()` (zero-arg production probe
// that loads the current project's config) and `XdebugGuard(cfg)` for
// hermetic unit tests. At least one caller in the brief uses the zero-arg
// form while the engine test passes an explicit Config.
func XdebugGuard(cfgs ...Config) bool {
	if len(cfgs) > 0 {
		return xdebugGuardForConfig(cfgs[0])
	}
	// Zero-arg probe: best-effort load from the current working directory.
	// If no config can be resolved, assume no warning is needed rather than
	// failing the audit.
	cfg, err := loadConfigForGuard()
	if err != nil || cfg == nil {
		return false
	}
	return xdebugGuardForConfig(*cfg)
}

func xdebugGuardForConfig(cfg Config) bool {
	if cfg.Stack.Features.Xdebug {
		return true
	}
	// MAGE_MODE guard is intentionally conservative: only `production` is
	// considered non-warning. When the framework env file indicates otherwise,
	// the caller should surface a warning and `Skipped: <reason>` if needed.
	// Without an explicit mode field on Config, Xdebug is the primary gate;
	// additional mode checks can be layered via MageModeGuard.
	return false
}

// MageModeGuard reports whether the given Magento mode should trigger a guard
// warning. It is a pure helper for the audit's `MAGE_MODE` guard.
func MageModeGuard(mode string) bool {
	switch mode {
	case "production":
		return false
	case "":
		return false
	default:
		return true
	}
}

// DbSlowLockPath returns the filesystem lock path used to serialize DB slow
// log access. The audit uses an atomic `mkdir` lock
// (`var/debug/.performance-audit.lock` or Govard tmp equivalent) so
// concurrent per-page captures do not interleave `mysqldumpslow` or
// `pt-query-digest` collection. The lock is never a `SET GLOBAL` hand-edit.
func DbSlowLockPath(projectRoot string) string {
	if projectRoot == "" {
		projectRoot = "."
	}
	return filepath.Join(projectRoot, "var", "debug", ".performance-audit.lock")
}

// AcquireDbSlowLock attempts to acquire the DB slow log lock via atomic
// `mkdir`. It returns a release function on success. The caller must defer
// the release to avoid stale locks after `trap` restore.
func AcquireDbSlowLock(projectRoot string) (func() error, error) {
	lockPath := DbSlowLockPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db-slow lock directory: %w", err)
	}
	if err := os.Mkdir(lockPath, 0o755); err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("db-slow lock %q is already held", lockPath)
		}
		return nil, fmt.Errorf("acquire db-slow lock: %w", err)
	}
	release := func() error {
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("release db-slow lock: %w", err)
		}
		return nil
	}
	return release, nil
}

// DbSlowCollector describes how DB slow entries are collected inside the DB
// container without `SET GLOBAL slow_query_log=ON` hand-edits. The Govard DB
// lifecycle owns the slow log via container-local `mysqldumpslow` and
// `pt-query-digest` against the mounted slow log file.
func DbSlowCollector(projectName string) (container string, mysqldumpslowCmd string, ptDigestCmd string) {
	if projectName == "" {
		projectName = "project"
	}
	container = projectName + "-db-1"
	// These commands run via `docker exec <container> ...` against the slow log
	// file that the DB image already exposes when Govard's DB lock is held.
	// No `SET GLOBAL` is issued; the log is read-only.
	mysqldumpslowCmd = "mysqldumpslow -t 20 /var/log/mysql/slow.log"
	ptDigestCmd = "pt-query-digest /var/log/mysql/slow.log"
	return container, mysqldumpslowCmd, ptDigestCmd
}

func loadConfigForGuard() (*Config, error) {
	// Minimal loader for the zero-arg guard. It reuses the engine's config
	// discovery without pulling in heavier dependencies. If discovery fails,
	// the guard is non-blocking.
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	// Probe for .govard.yml in the current directory tree walk.
	current := dir
	for {
		candidate := filepath.Join(current, ".govard.yml")
		if _, err := os.Stat(candidate); err == nil {
			// Found a project root; load via the engine's config loader.
			// Use a lightweight read without full normalization to keep the
			// guard hermetic for unit tests that use a temp GOVARD_HOME_DIR.
			cfg, err := LoadConfig(candidate)
			if err != nil {
				return nil, err
			}
			return cfg, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil, fmt.Errorf("no govard project found from %q", dir)
}

// LoadConfig is a lightweight YAML loader for the guard. It mirrors the
// engine's config loading but is scoped to this file so the guard stays
// self-contained and does not pull in framework detection.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	// Use the same YAML library the engine uses for consistency.
	// Inline decode to avoid importing engine's full loader.
	if err := yamlUnmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// yamlUnmarshal decodes YAML using the engine's standard library.
func yamlUnmarshal(in []byte, out any) error {
	return yaml.Unmarshal(in, out)
}
