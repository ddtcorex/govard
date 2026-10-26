package cmd

import (
	"fmt"
	"io"
	"os"

	"govard/internal/engine"
)

// warnRootOwnershipRisk prints one warning when Govard runs as root on a
// project owned by a regular user. It never fails the command: the desktop
// service and install scripts legitimately run as root.
func warnRootOwnershipRisk(w io.Writer, projectRoot string) {
	warnRootOwnershipRiskWith(w, projectRoot, os.Geteuid(), engine.FileOwnerUID)
}

func warnRootOwnershipRiskWith(w io.Writer, projectRoot string, euid int, ownerUID func(string) (int, error)) {
	risk, err := engine.RootOwnershipRisk(projectRoot, euid, ownerUID)
	if err != nil || !risk {
		return
	}
	_, _ = fmt.Fprintln(w, "WARNING: running as root on a project owned by another user creates root-owned files under .govard/; run Govard as the project owner, and repair existing files with `govard doctor --fix`.")
}

// WarnRootOwnershipRiskForTest exposes the warning with injected identity.
func WarnRootOwnershipRiskForTest(w io.Writer, projectRoot string, euid int, ownerUID func(string) (int, error)) {
	warnRootOwnershipRiskWith(w, projectRoot, euid, ownerUID)
}
