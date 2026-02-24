package tests

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestCreateDoctorDiagnosticsPack(t *testing.T) {
	projectDir := t.TempDir()
	outputDir := filepath.Join(t.TempDir(), "diagnostics")
	remoteAuditPath := filepath.Join(t.TempDir(), "remote.log")
	t.Setenv("GOVARD_REMOTE_AUDIT_LOG_PATH", remoteAuditPath)
	if err := os.WriteFile(remoteAuditPath, []byte(`{"operation":"remote.test","status":"success"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	baseConfig := []byte("project_name: demo\ndomain: demo.test\n")
	if err := os.WriteFile(filepath.Join(projectDir, "govard.yml"), baseConfig, 0o644); err != nil {
		t.Fatal(err)
	}
	compose := []byte("services:\n  web:\n    image: nginx:alpine\n")
	composePath := engine.ComposeFilePath(projectDir, "demo")
	if err := engine.EnsureComposePathReady(composePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composePath, compose, 0o644); err != nil {
		t.Fatal(err)
	}

	report := engine.DoctorReport{
		Checks: []engine.DoctorCheck{
			{
				ID:      "docker.daemon",
				Title:   "Docker daemon",
				Status:  engine.DoctorStatusPass,
				Message: "Docker is running.",
			},
		},
		Passed: 1,
	}

	zipPath, err := cmd.CreateDoctorDiagnosticsPack(outputDir, projectDir, report)
	if err != nil {
		t.Fatalf("create pack: %v", err)
	}
	if zipPath == "" {
		t.Fatal("expected non-empty zip path")
	}
	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("zip not found: %v", err)
	}

	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
	}
	required := []string{
		"doctor_report.json",
		"environment.json",
		"config_layers.txt",
		"runtime_commands.txt",
		"govard.yml",
		"govard-compose.yml",
		"remote-audit.log",
		"README.txt",
	}
	for _, name := range required {
		if _, ok := files[name]; !ok {
			t.Fatalf("expected %s in diagnostics pack", name)
		}
	}

	reportFile := files["doctor_report.json"]
	reader, err := reportFile.Open()
	if err != nil {
		t.Fatalf("open doctor_report.json: %v", err)
	}
	defer reader.Close()

	var decoded engine.DoctorReport
	if err := json.NewDecoder(reader).Decode(&decoded); err != nil {
		t.Fatalf("decode doctor_report.json: %v", err)
	}
	if decoded.Passed != 1 {
		t.Fatalf("expected passed=1, got %d", decoded.Passed)
	}
}
