package audit

import (
	"fmt"
	"sort"
	"strings"
)

// IntegrityCheck is the audit check id for container-free host analysis. It runs
// Go analyzers directly on the host, so it needs no container runtime and no PHP
// toolchain (unlike the lint and profiler checks).
const IntegrityCheck = "integrity"

// IntegrityRequest carries the host-visible inputs an analyzer may read. It
// deliberately has no container or PHP-version fields: analyzers are expected to
// be deterministic over the checked-out source plus the resolved project config.
type IntegrityRequest struct {
	ProjectRoot     string
	TargetPath      string
	Framework       string
	StackPHPVersion string
}

// IntegrityAnalyzer is one framework-declared host analyzer. Implementations
// self-register by ID; a framework opts in by listing IDs in its
// AuditIntegrityProfile, so no framework name is hardcoded here.
type IntegrityAnalyzer interface {
	ID() string
	Analyze(IntegrityRequest) ([]LintFinding, error)
}

var integrityAnalyzers = map[string]IntegrityAnalyzer{}

// RegisterIntegrityAnalyzer adds an analyzer to the process registry. It is
// called from analyzer init functions, mirroring the framework capability
// registry pattern.
func RegisterIntegrityAnalyzer(analyzer IntegrityAnalyzer) {
	if analyzer == nil || strings.TrimSpace(analyzer.ID()) == "" {
		return
	}
	integrityAnalyzers[analyzer.ID()] = analyzer
}

// RegisteredIntegrityAnalyzerIDs lists the available analyzer IDs in stable
// order for error messages.
func RegisteredIntegrityAnalyzerIDs() []string {
	ids := make([]string, 0, len(integrityAnalyzers))
	for id := range integrityAnalyzers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// IntegrityAnalyzers resolves the analyzer IDs declared by a framework profile.
// An unknown ID is an error naming the available analyzers, never a silent skip.
func IntegrityAnalyzers(ids []string) ([]IntegrityAnalyzer, error) {
	analyzers := make([]IntegrityAnalyzer, 0, len(ids))
	for _, id := range ids {
		analyzer, ok := integrityAnalyzers[id]
		if !ok {
			return nil, fmt.Errorf("integrity analyzer %q is not registered (available: %s)",
				id, strings.Join(RegisteredIntegrityAnalyzerIDs(), ", "))
		}
		analyzers = append(analyzers, analyzer)
	}
	return analyzers, nil
}
