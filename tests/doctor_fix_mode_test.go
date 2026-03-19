package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestApplyDoctorSafeFixesForTestCreatesGovardHomeDirectories(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "govard-home")
	t.Setenv("GOVARD_HOME_DIR", homeDir)
	if err := os.RemoveAll(homeDir); err != nil {
		t.Fatalf("remove govard home: %v", err)
	}

	report := engine.DoctorReport{
		Checks: []engine.DoctorCheck{
			{
				ID:      "host.govard.home",
				Title:   "Govard home directory",
				Status:  engine.DoctorStatusWarn,
				Message: "missing",
			},
		},
	}

	results := cmd.ApplyDoctorSafeFixesForTest(report)
	if len(results) != 1 {
		t.Fatalf("expected 1 fix result, got %d", len(results))
	}
	result := results[0]
	if result.Status != cmd.DoctorFixStatusApplied {
		t.Fatalf("expected applied result, got %s: %s", result.Status, result.Message)
	}
	if len(result.Actions) == 0 {
		t.Fatal("expected fix actions to be recorded")
	}

	required := []string{
		homeDir,
		filepath.Join(homeDir, "compose"),
		filepath.Join(homeDir, "diagnostics"),
	}
	for _, path := range required {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected path %s: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected directory %s", path)
		}
	}
}

func TestDoctorCommandFixModeRepairsGovardHomeWarning(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "govard-home")
	t.Setenv("GOVARD_HOME_DIR", homeDir)
	if err := os.RemoveAll(homeDir); err != nil {
		t.Fatalf("remove govard home: %v", err)
	}

	restoreDeps := cmd.SetDoctorDependenciesForTest(engine.DoctorDependencies{
		CheckDockerStatus:        func() error { return nil },
		CheckDockerComposePlugin: func() error { return nil },
		CheckPortAvailable:       func(port string) bool { return true },
		CheckDiskScratch:         func() error { return nil },
		CheckGovardHomeWritable:  engine.CheckGovardHomeWritable,
		CheckNetworkConnectivity: func() error { return nil },
	})
	defer restoreDeps()

	root := cmd.RootCommandForTest()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"doctor", "--json", "--fix"})

	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --json --fix failed: %v\nstderr=%s", err, stderr.String())
	}

	var report engine.DoctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor report: %v\nstdout=%s", err, stdout.String())
	}
	if report.Failures != 0 {
		t.Fatalf("expected 0 failures after fix, got %d", report.Failures)
	}
	if report.Warnings != 0 {
		t.Fatalf("expected 0 warnings after fix, got %d", report.Warnings)
	}

	var homeCheck *engine.DoctorCheck
	for index := range report.Checks {
		if report.Checks[index].ID == "host.govard.home" {
			homeCheck = &report.Checks[index]
			break
		}
	}
	if homeCheck == nil {
		t.Fatal("expected host.govard.home check in report")
	} else if homeCheck.Status != engine.DoctorStatusPass {
		t.Fatalf("expected host.govard.home to pass after fix, got %s", homeCheck.Status)
	}

	if !strings.Contains(stderr.String(), "Doctor --fix action: mkdir -p "+homeDir) {
		t.Fatalf("expected fix action output in stderr, got: %s", stderr.String())
	}

	checkRoot := cmd.RootCommandForTest()
	checkRoot.SetOut(io.Discard)
	checkRoot.SetErr(io.Discard)
	checkRoot.SetArgs([]string{"doctor", "--json"})
	if err := checkRoot.Execute(); err != nil {
		t.Fatalf("doctor --json after fix failed: %v", err)
	}
}
