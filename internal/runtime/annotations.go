package runtime

import (
	"strings"

	"github.com/spf13/cobra"
)

// AnnotationRequires is the cobra annotation key that declares a command's
// runtime requirements, e.g. Annotations{AnnotationRequires: "docker"}.
const AnnotationRequires = "govard.io/requires"

var knownCapabilities = map[Capability]struct{}{
	CapNone: {}, CapDocker: {}, CapSSH: {}, CapRsync: {}, CapCloudflared: {}, CapNet: {},
}

// Requires resolves the requirement for a command: the nearest annotation wins,
// groups without a run function have none, and a runnable command that declares
// nothing defaults to docker (default-deny).
func Requires(cmd *cobra.Command) []Capability {
	if cmd == nil {
		return nil
	}
	for current := cmd; current != nil; current = current.Parent() {
		raw, ok := current.Annotations[AnnotationRequires]
		if !ok {
			continue
		}
		capabilities := parseRequirements(raw)
		if len(capabilities) > 0 {
			return capabilities
		}
	}
	if cmd.RunE == nil && cmd.Run == nil {
		return nil
	}
	return []Capability{CapDocker}
}

func parseRequirements(raw string) []Capability {
	seen := map[Capability]struct{}{}
	capabilities := make([]Capability, 0, 2)
	for _, part := range strings.Split(raw, ",") {
		capability := Capability(strings.ToLower(strings.TrimSpace(part)))
		if capability == "" {
			continue
		}
		if _, known := knownCapabilities[capability]; !known {
			// Unknown values must never silently relax the gate.
			capability = CapDocker
		}
		if _, duplicate := seen[capability]; duplicate {
			continue
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

// alwaysRunnableNames are commands that must work on any host: they are how an
// operator diagnoses a broken environment or discovers what this host can do.
var alwaysRunnableNames = map[string]bool{
	"help": true, "completion": true, "doctor": true, "capabilities": true, "version": true,
}

// AlwaysRunnable reports whether a command must run regardless of capability
// state. It is shared by the command gate and the manifest completeness guard.
func AlwaysRunnable(cmd *cobra.Command) bool {
	return cmd != nil && alwaysRunnableNames[cmd.Name()]
}

// HasDeclaredRequirement reports whether the command or one of its ancestors
// declares requirements. Unlike Requires it never applies the default-deny
// fallback, so callers can tell "declared none" from "declared nothing".
func HasDeclaredRequirement(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if _, ok := current.Annotations[AnnotationRequires]; ok {
			return true
		}
	}
	return false
}
