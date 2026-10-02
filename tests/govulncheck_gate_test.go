package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// govulncheckGoodReport has the shape of a real `govulncheck -format text`
// run: one reachable finding block plus the closing summary.
const govulncheckGoodReport = `=== Symbol Results ===

Vulnerability #1: GO-2026-4887
    Moby has AuthZ plugin bypass when provided oversized request bodies in
    github.com/docker/docker
  More info: https://pkg.go.dev/vuln/GO-2026-4887
  Module: github.com/docker/docker
    Found in: github.com/docker/docker@v28.5.2+incompatible
    Fixed in: N/A

Your code is affected by 1 vulnerability from 1 module.
This scan also found 0 vulnerabilities in packages you import and 3
vulnerabilities in modules you require, but your code doesn't appear to call
these vulnerabilities.
Use '-show verbose' for more details.
`

const govulncheckCleanReport = "=== Symbol Results ===\n\nNo vulnerabilities found.\n"

func runGovulncheckGate(t *testing.T, report, allowlist string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.txt")
	allowPath := filepath.Join(dir, "allow.txt")
	if err := os.WriteFile(reportPath, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(allowPath, []byte(allowlist), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", "../scripts/govulncheck-gate.sh", reportPath, allowPath).CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run gate: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return string(out), code
}

func TestGovulncheckGatePassesARealShapedReport(t *testing.T) {
	out, code := runGovulncheckGate(t, govulncheckGoodReport, "GO-2026-4887 triaged\n")
	if code != 0 || !strings.Contains(out, "gate passed") {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
}

func TestGovulncheckGatePassesACleanReport(t *testing.T) {
	_, code := runGovulncheckGate(t, govulncheckCleanReport, "")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
}

func TestGovulncheckGateStillFailsOnAnUntriagedFinding(t *testing.T) {
	_, code := runGovulncheckGate(t, govulncheckGoodReport, "")
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestGovulncheckGateFailsDistinctlyOnAnErrorOnlyReport(t *testing.T) {
	report := "govulncheck: loading packages:\nThere are errors with the provided package patterns:\n\n-: build failed\n"
	out, code := runGovulncheckGate(t, report, "GO-2026-4887 triaged\n")
	if code != 3 {
		t.Fatalf("exit %d, want 3; output:\n%s", code, out)
	}
	if strings.Contains(out, "gate passed") || !strings.Contains(out, "incomplete") {
		t.Fatalf("message must say the report is incomplete, got:\n%s", out)
	}
}

func TestGovulncheckGateFailsWhenAnErrorMarkerFollowsASummary(t *testing.T) {
	_, code := runGovulncheckGate(t, govulncheckCleanReport+"govulncheck: fatal: network unreachable\n", "")
	if code != 3 {
		t.Fatalf("exit %d, want 3", code)
	}
}

func TestGovulncheckGateFailsOnAnEmptyReport(t *testing.T) {
	_, code := runGovulncheckGate(t, "", "")
	if code != 3 {
		t.Fatalf("exit %d, want 3", code)
	}
}
