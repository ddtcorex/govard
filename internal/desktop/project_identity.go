package desktop

import (
	"path/filepath"
	"strings"

	"govard/internal/engine"
)

// syncingProjects maps a canonical project ID to the remote an in-flight
// sync is pulling from. Keys are canonical (see canonicalProjectID), never
// raw caller input: lifecycle entry points receive names, domains and paths
// interchangeably while the sync runner registers whatever identifier it
// was given, so raw-string keys let a Toggle by domain slip past a sync
// started by name.

// canonicalProjectID resolves any project identifier (registry name,
// domain, root path, fuzzy query) to one stable key: the registry project
// name when the registry knows the project, otherwise the resolved root
// path. Fuzzy matches count only when strong (below
// engine.ScoreAmbiguousThreshold): a weak subsequence hit is a typo, and
// claiming a project name for it would guard the wrong project, so it
// falls through to the path fallback, which never yields a name.
// Unknown identifiers fall back to the cleaned input so the guard still
// keys consistently for that caller.
func canonicalProjectID(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if id, ok := canonicalRegistryID(trimmed); ok {
		return id
	}

	// Root path fallback: direct paths with config, orphans, Docker
	// labels (see resolveProjectRootForRemotes). Weak-score resolutions
	// are refused the same way DeleteProject refuses weak matches.
	if root, score, err := resolveProjectRootForRemotes(trimmed); err == nil && score < engine.ScoreAmbiguousThreshold {
		return canonicalPathKey(root)
	}

	if filepath.IsAbs(trimmed) || strings.Contains(trimmed, string(filepath.Separator)) {
		return canonicalPathKey(trimmed)
	}
	return strings.ToLower(trimmed)
}

// canonicalProjectIDForRegistry resolves identifiers for op registry and
// history keys WITHOUT Docker-backed probes: exact registry match, strong
// fuzzy match, then the cleaned-input fallback (fail-consistent: an unknown
// identifier keys by cleaned input whether Docker is up or not). The full
// Docker-backed resolution (orphan listing via `docker compose ls`,
// container-label lookup) lives at lifecycle edges (UI/start), never in
// the registry path, so RegisterOp never shells out: no per-op latency,
// safe Docker-down, hermetic under -race. Every step here is a registry
// file read or a path syscall.
func canonicalProjectIDForRegistry(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if id, ok := canonicalRegistryID(trimmed); ok {
		return id
	}

	if filepath.IsAbs(trimmed) || strings.Contains(trimmed, string(filepath.Separator)) {
		return canonicalPathKey(trimmed)
	}
	return strings.ToLower(trimmed)
}

// canonicalRegistryID resolves the registry-backed steps both canonicalizers
// share: exact name or domain first, then a strong-only fuzzy match. The
// fuzzy gate stays here so a weak subsequence hit never claims a project
// name — it falls through to the path fallback, which never yields a name.
func canonicalRegistryID(trimmed string) (string, bool) {
	if entry, err := findExactProjectMatch(trimmed); err == nil {
		if name := strings.TrimSpace(entry.ProjectName); name != "" {
			return name, true
		}
	}

	if entry, score, err := engine.FindProjectByQuery(trimmed); err == nil && score < engine.ScoreAmbiguousThreshold {
		if name := strings.TrimSpace(entry.ProjectName); name != "" {
			return name, true
		}
		if path := strings.TrimSpace(entry.Path); path != "" {
			return canonicalPathKey(path), true
		}
	}
	return "", false
}

// canonicalPathKey normalizes a path fallback to one stable key:
// symlinks resolved (falling back to Clean when resolution fails, e.g.
// the path is gone), then mapped back to the registry project name when
// the path belongs to a registered project, so a symlinked access and a
// direct access to the same project share one key.
func canonicalPathKey(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = filepath.Clean(resolved)
	}
	if name := registryNameForPath(cleaned); name != "" {
		return name
	}
	return cleaned
}

// registryNameForPath reports the registry project name for a resolved
// root path, or "" when no registered project lives there. Registry
// paths are symlink-normalized at upsert, so they compare directly
// against resolved keys. Legacy entries written before that normalization
// compare Clean-only and miss coalescing (fail open, no misdirection):
// documented follow-up, see the SDD ledger accepted nit (no behavior
// change here).
func registryNameForPath(path string) string {
	entries, err := engine.ReadProjectRegistryEntries()
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.ProjectName)
		if name == "" || strings.TrimSpace(entry.Path) == "" {
			continue
		}
		if filepath.Clean(entry.Path) == path {
			return name
		}
	}
	return ""
}

// CanonicalProjectIDForTest exposes canonicalProjectID for tests in /tests.
func CanonicalProjectIDForTest(input string) string {
	return canonicalProjectID(input)
}

// TryRegisterSyncingProject atomically checks the sync guard and registers
// the in-flight sync under one mutex hold: it reports false (leaving any
// existing entry untouched) when the canonical project already has a sync
// running, true when the claim succeeded. A separate check-then-register
// lets two concurrent starters both pass the guard and overwrite each
// other, so sync starters must claim through here, never through the
// check and the register as two steps.
func TryRegisterSyncingProject(project, remote string) bool {
	return tryRegisterSyncingProjectByID(canonicalProjectID(project), remote)
}

// tryRegisterSyncingProjectByID claims an already-canonical project ID:
// same atomic check-and-register as TryRegisterSyncingProject without
// resolving again. Sync starters resolve once and reuse the ID for the
// claim, the event and the release, so a registry miss (Docker-backed
// probes) is paid once per sync, not three times.
func tryRegisterSyncingProjectByID(id, remote string) bool {
	if id == "" {
		return false
	}
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	if _, ok := syncingProjects[id]; ok {
		return false
	}
	syncingProjects[id] = remote
	return true
}

// unregisterSyncingProjectByID releases a claim by canonical ID, the
// release half of tryRegisterSyncingProjectByID.
func unregisterSyncingProjectByID(id string) {
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	delete(syncingProjects, id)
}

// ResetSyncingProjectsForTest clears the in-flight sync map so guard tests
// stay hermetic.
func ResetSyncingProjectsForTest() {
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	syncingProjects = make(map[string]string)
}
