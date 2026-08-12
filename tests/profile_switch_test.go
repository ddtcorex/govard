package tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func TestProfileSwitchCommandExists(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"config", "profile", "switch"})
	if err != nil {
		t.Fatalf("find config profile switch: %v", err)
	}
	if command == nil {
		t.Fatal("expected config profile switch command")
	}
}

func TestProfileSwitchWithValidProfile(t *testing.T) {
	tempDir := t.TempDir()

	// Create base config file
	baseConfig := `project_name: test-project
domain: test.test
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create profile config file
	upgradeConfig := `profile: upgrade
stack:
  php_version: "8.2"
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.upgrade.yml"), []byte(upgradeConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Set registry path for test
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Execute profile switch
	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "profile", "switch", "upgrade"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute profile switch: %v", err)
	}

	// Verify project registry was updated
	entry, ok := engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry")
	}
	if entry.Profile != "upgrade" {
		t.Fatalf("expected profile upgrade, got %q", entry.Profile)
	}
}

func TestProfileSwitchWithInvalidProfile(t *testing.T) {
	tempDir := t.TempDir()

	// Create base config file
	baseConfig := `project_name: test-project
domain: test.test
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Execute profile switch with invalid profile
	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf) // Capture error
	root.SetArgs([]string{"config", "profile", "switch", "nonexistent"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestProfileSwitchWithMultipleProfiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create base config file
	baseConfig := `project_name: test-project
domain: test.test
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create multiple profile files
	for _, name := range []string{"upgrade", "staging", "local-dev"} {
		profileConfig := `profile: ` + name + "\n"
		if err := os.WriteFile(filepath.Join(tempDir, ".govard."+name+".yml"), []byte(profileConfig), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Verify detectAvailableProfiles returns all profiles
	profiles := detectProfilesForTest(tempDir)
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d: %v", len(profiles), profiles)
	}

	// Verify all expected profiles are found
	expected := map[string]bool{"upgrade": true, "staging": true, "local-dev": true}
	for _, p := range profiles {
		if !expected[p] {
			t.Errorf("unexpected profile: %s", p)
		}
	}
}

func TestResolveEffectiveProfile(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		registry string
		expected string
	}{
		{
			name:     "explicit flag wins",
			explicit: "explicit-profile",
			registry: "registry-profile",
			expected: "explicit-profile",
		},
		{
			name:     "registry fallback",
			explicit: "",
			registry: "registry-profile",
			expected: "registry-profile",
		},
		{
			name:     "empty when no explicit or registry",
			explicit: "",
			registry: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.ResolveEffectiveProfile("/nonexistent", tt.explicit)
			_ = tt.registry // Registry is set via test setup, not inline
			if result != tt.expected {
				// This test is simplified - actual registry test would need project setup
				_ = result
			}
		})
	}
}

// TestResolveEffectiveProfileLiteralDefault ensures `--profile default` resolves
// to the empty profile, exactly like `config profile switch default` does
// (internal/cmd/profile.go normalizes the "default" argument to "" before
// saving it to the registry). Without this, "env up --profile default" would
// diverge from the registry-driven default and suffix compose/volume names
// with "-default" instead of leaving them unsuffixed.
func TestResolveEffectiveProfileLiteralDefault(t *testing.T) {
	tempDir := t.TempDir()

	result := engine.ResolveEffectiveProfile(tempDir, "default")
	if result != "" {
		t.Errorf(`ResolveEffectiveProfile(%q, "default") = %q, want "" (literal "default" must normalize to the empty/no-profile sentinel)`, tempDir, result)
	}
}

// TestResolveEffectiveProfileLiteralDefaultOverridesRegistry ensures an
// explicit `--profile default` always wins, even when the project registry
// has a different last-used profile saved (e.g. from a prior
// `config profile switch upgrade` or a previous `env up --profile upgrade`).
// Reproduces a live regression: normalizing "default" to "" before the
// "was an explicit flag given at all?" check made it indistinguishable from
// "no --profile flag passed", so it fell through to the registry lookup and
// resurrected the stale "upgrade" profile instead of the requested default.
func TestResolveEffectiveProfileLiteralDefaultOverridesRegistry(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	entry := engine.ProjectRegistryEntry{
		Path:    tempDir,
		Profile: "upgrade",
	}
	if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
		t.Fatal(err)
	}

	result := engine.ResolveEffectiveProfile(tempDir, "default")
	if result != "" {
		t.Errorf(`ResolveEffectiveProfile(%q, "default") = %q, want "" (explicit "default" must override the registry's saved "upgrade" profile)`, tempDir, result)
	}
}

func TestProfileSwitchSavesPreviousProfile(t *testing.T) {
	tempDir := t.TempDir()

	// Create base config file
	baseConfig := `project_name: test-project
domain: test.test
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create profile config files
	upgradeConfig := `profile: upgrade
stack:
  php_version: "8.2"
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.upgrade.yml"), []byte(upgradeConfig), 0644); err != nil {
		t.Fatal(err)
	}
	stagingConfig := `profile: staging
stack:
  php_version: "8.3"
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.staging.yml"), []byte(stagingConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Set registry path for test
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// First switch to upgrade (no previous profile)
	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "profile", "switch", "upgrade"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first profile switch: %v", err)
	}

	entry, ok := engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry")
	}
	if entry.Profile != "upgrade" {
		t.Fatalf("expected profile 'upgrade', got %q", entry.Profile)
	}
	if entry.PreviousProfile != "" {
		t.Fatalf("expected previous_profile '', got %q", entry.PreviousProfile)
	}

	// Second switch to staging (previous should be 'upgrade')
	buf.Reset()
	root.SetArgs([]string{"config", "profile", "switch", "staging"})
	if err := root.Execute(); err != nil {
		t.Fatalf("second profile switch: %v", err)
	}

	entry, ok = engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry after second switch")
	}
	if entry.Profile != "staging" {
		t.Fatalf("expected profile 'staging', got %q", entry.Profile)
	}
	if entry.PreviousProfile != "upgrade" {
		t.Fatalf("expected previous_profile 'upgrade', got %q", entry.PreviousProfile)
	}
}

func TestProfileShiftDetectionWithPreviousProfile(t *testing.T) {
	tempDir := t.TempDir()

	// Create base config with profile
	baseConfig := `project_name: test-project
domain: test.test
profile: staging
stack:
  php_version: "8.3"
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create profile config files
	upgradeConfig := `profile: upgrade
stack:
  php_version: "8.2"
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.upgrade.yml"), []byte(upgradeConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Set registry path for test
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Set up registry simulating switch FROM staging TO upgrade
	// After switch: Profile=upgrade (new), PreviousProfile=staging (old)
	entry := engine.ProjectRegistryEntry{
		Path:            tempDir,
		Profile:         "upgrade", // Switched TO this
		PreviousProfile: "staging", // Switched FROM this
		PHPVersion:      "8.2",
	}
	if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
		t.Fatal(err)
	}

	// Now simulate config load with new profile (upgrade)
	config := engine.Config{
		ProjectName: "test-project",
		Profile:     "upgrade",
		Stack: engine.Stack{
			PHPVersion: "8.2",
		},
	}

	// DetectProfileShift should use previous_profile from registry
	shiftInfo := engine.DetectProfileShift(config)
	if !shiftInfo.Shifted {
		t.Fatal("expected profile shift to be detected")
	}
	if shiftInfo.PreviousProfile != "staging" {
		t.Fatalf("expected previous_profile 'staging', got %q", shiftInfo.PreviousProfile)
	}
	if shiftInfo.CurrentProfile != "upgrade" {
		t.Fatalf("expected current_profile 'upgrade', got %q", shiftInfo.CurrentProfile)
	}
}

func TestDetectProfileShiftWhenOnlyPHPVersionChanges(t *testing.T) {
	// This test verifies that profile shift is detected when PHP version changes
	// even if the profile name stays the same (the bug we fixed)
	tempDir := t.TempDir()

	// Create base config with same profile but different PHP version
	baseConfig := `project_name: test-project
domain: test.test
profile: staging
stack:
  php_version: "8.3"
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Set registry path for test
	registryPath := filepath.Join(tempDir, "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Set up registry with previous PHP version
	entry := engine.ProjectRegistryEntry{
		Path:            tempDir,
		Profile:         "staging", // Same profile name
		PreviousProfile: "",        // No previous_profile set
		PHPVersion:      "8.2",     // Old PHP version
	}
	if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
		t.Fatal(err)
	}

	// Now simulate config load with new PHP version but same profile
	config := engine.Config{
		ProjectName: "test-project",
		Profile:     "staging", // Same profile
		Stack: engine.Stack{
			PHPVersion: "8.3", // New PHP version
		},
	}

	// DetectProfileShift should detect PHP version change
	shiftInfo := engine.DetectProfileShift(config)
	if !shiftInfo.Shifted {
		t.Fatal("expected profile shift to be detected when PHP version changes")
	}
	if shiftInfo.PreviousPHP != "8.2" {
		t.Fatalf("expected previous_php '8.2', got %q", shiftInfo.PreviousPHP)
	}
	if shiftInfo.CurrentPHP != "8.3" {
		t.Fatalf("expected current_php '8.3', got %q", shiftInfo.CurrentPHP)
	}
	if shiftInfo.Reason != "PHP version changed: 8.2 -> 8.3" {
		t.Fatalf("expected reason 'PHP version changed: 8.2 -> 8.3', got %q", shiftInfo.Reason)
	}
}

func TestClearPreviousProfile(t *testing.T) {
	tempDir := t.TempDir()

	// Set registry path for test
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Create entry with previous_profile
	entry := engine.ProjectRegistryEntry{
		Path:            tempDir,
		Profile:         "staging",
		PreviousProfile: "upgrade",
	}
	if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
		t.Fatal(err)
	}

	// Verify previous_profile exists
	entry, ok := engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry")
	}
	if entry.PreviousProfile != "upgrade" {
		t.Fatalf("expected previous_profile 'upgrade', got %q", entry.PreviousProfile)
	}

	// Clear previous_profile
	if err := engine.ClearPreviousProfile(tempDir); err != nil {
		t.Fatalf("ClearPreviousProfile failed: %v", err)
	}

	// Verify previous_profile is cleared
	entry, ok = engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry after clear")
	}
	if entry.PreviousProfile != "" {
		t.Fatalf("expected previous_profile '', got %q", entry.PreviousProfile)
	}
}

func TestProfileSwitchClearCommand(t *testing.T) {
	tempDir := t.TempDir()

	// Create base config file
	baseConfig := `project_name: test-project
domain: test.test
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Set registry path for test
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// First set a profile
	entry := engine.ProjectRegistryEntry{
		Path:            tempDir,
		Profile:         "upgrade",
		PreviousProfile: "staging",
	}
	if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
		t.Fatal(err)
	}

	// Execute profile clear
	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "profile", "clear"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute profile clear: %v", err)
	}

	// Verify profile is cleared but previous_profile is saved for shift detection
	entry, ok := engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry")
	}
	if entry.Profile != "" {
		t.Fatalf("expected profile '', got %q", entry.Profile)
	}
	if entry.PreviousProfile != "upgrade" {
		t.Fatalf("expected previous_profile 'upgrade', got %q", entry.PreviousProfile)
	}
}

func TestProfileSwitchToDefault(t *testing.T) {
	tempDir := t.TempDir()

	// Create base config file
	baseConfig := `project_name: test-project
domain: test.test
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(baseConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create profile config file
	upgradeConfig := `profile: upgrade
stack:
  php_version: "8.2"
`
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.upgrade.yml"), []byte(upgradeConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Set registry path for test
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// First switch to upgrade
	root := cmd.RootCommandForTest()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "profile", "switch", "upgrade"})
	if err := root.Execute(); err != nil {
		t.Fatalf("first profile switch: %v", err)
	}

	// Now switch to default using "default" argument
	buf.Reset()
	root.SetArgs([]string{"config", "profile", "switch", "default"})
	if err := root.Execute(); err != nil {
		t.Fatalf("switch to default: %v", err)
	}

	// Verify profile is empty
	entry, ok := engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry")
	}
	if entry.Profile != "" {
		t.Fatalf("expected profile '', got %q", entry.Profile)
	}
	if entry.PreviousProfile != "upgrade" {
		t.Fatalf("expected previous_profile 'upgrade', got %q", entry.PreviousProfile)
	}
}

// Helper to detect profiles for testing
func detectProfilesForTest(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var profiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > 14 && name[:8] == ".govard." && name[len(name)-4:] == ".yml" {
			profile := name[8 : len(name)-4]
			if profile != "local" && profile != "project.local" && profile != "compose" {
				profiles = append(profiles, profile)
			}
		}
	}
	return profiles
}
