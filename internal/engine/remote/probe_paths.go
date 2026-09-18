package remote

import (
	"errors"
	"fmt"
	"strings"
)

// ErrProbeFilesNotFound marks a probe that reached the target but found no
// usable database configuration there. TryProbeCandidatePaths advances to the
// next candidate on it; any other error aborts the probe run.
var ErrProbeFilesNotFound = errors.New("probe files not found")

// servedDirNames are the deploy-layout directories a remote application may
// live under when the configured remote path is the layout root instead of
// the app root (sandbox `public_html`, deploy `current`).
var servedDirNames = []string{"public_html", "current"}

// ProbeCandidatePaths lists where a remote app root may live, configured path
// first, then each deploy-layout served directory under it. Empty segments
// and duplicates are dropped, so a path that already ends in a served
// directory is not doubled.
func ProbeCandidatePaths(path string) []string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	candidates := []string{trimmed}
	seen := map[string]bool{trimmed: true}
	for _, dir := range servedDirNames {
		if trimmed == dir || strings.HasSuffix(trimmed, "/"+dir) {
			continue
		}
		candidate := strings.TrimSuffix(trimmed, "/") + "/" + dir
		if !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

// TryProbeCandidatePaths runs probe against each candidate app root in order
// and returns the first success. A probe that finds no usable configuration
// reports ErrProbeFilesNotFound (via errors.Is) to advance; a transport or
// authentication failure aborts immediately, since retrying the same broken
// connection against another directory cannot help. When every candidate
// misses, the error names each tried path.
func TryProbeCandidatePaths[T any](basePath string, probe func(path string) (T, error)) (T, error) {
	var zero T
	candidates := ProbeCandidatePaths(basePath)
	if len(candidates) == 0 {
		return zero, fmt.Errorf("%w: empty remote path", ErrProbeFilesNotFound)
	}
	var lastErr error
	for _, candidate := range candidates {
		result, err := probe(candidate)
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, ErrProbeFilesNotFound) {
			return zero, err
		}
		lastErr = err
	}
	return zero, fmt.Errorf("no database configuration at %s: %w", strings.Join(candidates, ", "), lastErr)
}
