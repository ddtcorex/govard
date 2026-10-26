package tests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

func quietDoctorDependencies() engine.DoctorDependencies {
	return engine.DoctorDependencies{
		CheckDockerStatus:        func() error { return nil },
		CheckDockerComposePlugin: func() error { return nil },
		CheckPortAvailable:       func(string) bool { return true },
		CheckDiskScratch:         func() error { return nil },
		CheckGovardHomeWritable:  func() error { return nil },
		CheckNetworkConnectivity: func() error { return nil },
		CheckSearchIndexBlock:    func() error { return nil },
		CheckSSHAgentStatus:      func() (string, error) { return "ok", nil },
		CheckComposeSpam:         func() error { return nil },
		CheckGovardRegistry:      func() error { return nil },
		CheckSystemDependencies:  func() []string { return nil },
		CheckRuntimeImages:       func() ([]string, error) { return nil, nil },
		CheckProfileSync:         func() error { return nil },
		CheckLegacyConfig:        func() error { return nil },
		CheckConfigDrift:         func() error { return nil },
	}
}

func findDoctorCheck(report engine.DoctorReport, id string) (engine.DoctorCheck, bool) {
	for _, check := range report.Checks {
		if check.ID == id {
			return check, true
		}
	}
	return engine.DoctorCheck{}, false
}

// Every path under .govard is owned by root (uid 0) in this fake stat.
func rootOwned(string) (int, int, error) { return 0, 0, nil }

func TestForeignOwnedGovardPathsUsesLstat(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".govard", "apache", "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".govard", "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	paths, err := engine.ForeignOwnedGovardPathsForTest(root, 1000, rootOwned)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range paths {
		got = append(got, p.Path)
		if strings.HasPrefix(p.Path, outside) {
			t.Fatalf("walked through the symlink into %s", p.Path)
		}
	}
	want := []string{
		filepath.Join(root, ".govard"),
		filepath.Join(root, ".govard", "apache"),
		filepath.Join(root, ".govard", "apache", "custom"),
		link,
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("paths = %v, want %v", got, want)
	}
}

func TestForeignOwnedGovardPathsMissingDir(t *testing.T) {
	paths, err := engine.ForeignOwnedGovardPathsForTest(t.TempDir(), 1000, rootOwned)
	if err != nil || len(paths) != 0 {
		t.Fatalf("missing .govard: paths %v err %v, want none", paths, err)
	}
}

func TestDoctorReportsGovardOwnershipWarn(t *testing.T) {
	deps := quietDoctorDependencies()
	deps.CheckGovardOwnership = func() error {
		return errors.New("2 paths under .govard are not owned by uid 1000: .govard, .govard/apache")
	}
	check, ok := findDoctorCheck(engine.RunDoctorDiagnosticsInRoot(deps, t.TempDir()), "project.govard.ownership")
	if !ok || check.Status != engine.DoctorStatusWarn || check.SuggestedCommand != "govard doctor --fix" {
		t.Fatalf("check = %#v, want warn suggesting govard doctor --fix", check)
	}
}

func TestDoctorGovardOwnershipPassWhenClean(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".govard"), 0o755); err != nil {
		t.Fatal(err)
	}
	deps := quietDoctorDependencies()
	deps.CheckGovardOwnership = func() error { return nil }
	check, ok := findDoctorCheck(engine.RunDoctorDiagnosticsInRoot(deps, root), "project.govard.ownership")
	if !ok || check.Status != engine.DoctorStatusPass {
		t.Fatalf("check = %#v, want pass", check)
	}
}
