package types

// AuditTargetMode describes the filesystem relationship between an audit
// target and its framework project.
type AuditTargetMode string

const (
	AuditTargetAuto       AuditTargetMode = "auto"
	AuditTargetProject    AuditTargetMode = "project"
	AuditTargetModule     AuditTargetMode = "module_in_project"
	AuditTargetStandalone AuditTargetMode = "standalone"
)

// AuditTargetResolveRequest supplies the user-selected starting point and an
// optional mode override to a framework-owned target resolver.
type AuditTargetResolveRequest struct {
	StartPath    string
	ModeOverride AuditTargetMode
}

// AuditTarget is the resolved framework project and the path to lint.
type AuditTarget struct {
	Framework   string
	ProjectRoot string
	TargetPath  string
	Mode        AuditTargetMode
}

// AuditTargetResolver resolves framework-specific audit targets without
// imposing framework-name branches on the generic audit command.
type AuditTargetResolver func(AuditTargetResolveRequest) (AuditTarget, bool, error)

// AuditLintProfile is framework-owned lint policy consumed by the generic
// audit runner. It declares policy only; execution remains in internal/audit.
type AuditLintProfile struct {
	ProjectPHPVersions    []string
	StandalonePHPVersions []string
	Linters               []string
	CodingStandard        string
	PHPStanLevel          int
	PHPStanExtension      string
}

// AuditProfilerProfile declares the stock runtime profiler contract owned by a
// framework. The command layer consumes these values through the generic
// FastCGI adapter without branching on the framework name.
type AuditProfilerProfile struct {
	EnvironmentVariable string
	EnvironmentValue    string
	OutputPath          string
}

func cloneAuditProfilerProfile(profile *AuditProfilerProfile) *AuditProfilerProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	return &cloned
}

func cloneAuditLintProfile(profile *AuditLintProfile) *AuditLintProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.ProjectPHPVersions = cloneStrings(profile.ProjectPHPVersions)
	cloned.StandalonePHPVersions = cloneStrings(profile.StandalonePHPVersions)
	cloned.Linters = cloneStrings(profile.Linters)
	return &cloned
}
