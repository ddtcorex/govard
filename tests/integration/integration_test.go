//go:build integration
// +build integration

// Package integration provides integration tests for Govard.
// These tests verify end-to-end workflows and interactions between components.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"govard/internal/engine"
)

// TestEnvironment holds the configuration for integration tests
type TestEnvironment struct {
	ProjectRoot    string
	TestProjects   map[string]string
	BinaryPath     string
	BlueprintsPath string
}

// RuntimeShims describes the command shim environment used by integration tests.
type RuntimeShims struct {
	Dir      string
	LogPath  string
	ExtraEnv []string
}

var (
	buildGovardBinaryOnce sync.Once
	buildGovardBinaryErr  error
)

// NewTestEnvironment creates a new test environment
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	t.Helper()

	// Find project root
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	// Build binary if not exists
	binaryPath := filepath.Join(projectRoot, "bin", "govard-test")
	buildGovardBinaryOnce.Do(func() {
		buildGovardBinaryErr = buildTestBinary(projectRoot, binaryPath)
	})
	if buildGovardBinaryErr != nil {
		t.Fatalf("Failed to build integration test binary: %v", buildGovardBinaryErr)
	}

	return &TestEnvironment{
		ProjectRoot:    projectRoot,
		TestProjects:   make(map[string]string),
		BinaryPath:     binaryPath,
		BlueprintsPath: filepath.Join(projectRoot, "internal", "blueprints", "files"),
	}
}

// buildTestBinary builds the govard binary for testing
func buildTestBinary(projectRoot, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create binary output dir: %w", err)
	}

	cmd := exec.Command("go", "build", "-o", outputPath, "-tags", "integration", filepath.Join(projectRoot, "cmd", "govard", "main.go"))
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build govard test binary: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("verify built binary at %s: %w", outputPath, err)
	}
	return nil
}

// CreateTestProject creates a temporary test project with given files
func (env *TestEnvironment) CreateTestProject(t *testing.T, name string, files map[string]string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create test project dir: %v", err)
	}

	for relPath, content := range files {
		fullPath := filepath.Join(dir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create dir for %s: %v", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", relPath, err)
		}
	}

	env.TestProjects[name] = dir
	return dir
}

// CreateProjectFromFixture copies a fixture project from tests/integration/projects into a temp project directory.
func (env *TestEnvironment) CreateProjectFromFixture(t *testing.T, fixturePath string, name string) string {
	t.Helper()

	src := filepath.Join(env.ProjectRoot, "tests", "integration", "projects", filepath.FromSlash(fixturePath))
	if info, err := os.Stat(src); err != nil || !info.IsDir() {
		if err != nil {
			t.Fatalf("fixture %s is not available: %v", src, err)
		}
		t.Fatalf("fixture %s is not a directory", src)
	}

	dst := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("failed to create fixture project dir: %v", err)
	}

	copyDir(t, src, dst)
	env.TestProjects[name] = dst
	return dst
}

// CreateMagento2Project creates a test Magento 2 project
func (env *TestEnvironment) CreateMagento2Project(t *testing.T, name string) string {
	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"name": "test/magento2-project",
			"require": map[string]string{
				"magento/product-community-edition": "2.4.7",
			},
		}),
		"app/etc/env.php": "<?php return [];",
	}
	return env.CreateTestProject(t, name, files)
}

// CreateLaravelProject creates a test Laravel project
func (env *TestEnvironment) CreateLaravelProject(t *testing.T, name string) string {
	files := map[string]string{
		"composer.json": MustMarshalJSON(t, map[string]interface{}{
			"name": "test/laravel-project",
			"require": map[string]string{
				"laravel/framework": "^11.0",
			},
		}),
		".env.example": "APP_NAME=Laravel\nAPP_ENV=local",
	}
	return env.CreateTestProject(t, name, files)
}

// CreateNextJSProject creates a test Next.js project
func (env *TestEnvironment) CreateNextJSProject(t *testing.T, name string) string {
	files := map[string]string{
		"package.json": MustMarshalJSON(t, map[string]interface{}{
			"name": "nextjs-project",
			"dependencies": map[string]string{
				"next":  "^14.0.0",
				"react": "^18.0.0",
			},
		}),
	}
	return env.CreateTestProject(t, name, files)
}

// RunGovard executes the govard binary with given arguments
func (env *TestEnvironment) RunGovard(t *testing.T, projectDir string, args ...string) *CommandResult {
	t.Helper()

	return env.RunGovardWithEnv(t, projectDir, nil, args...)
}

// RunGovardWithEnv executes the govard binary with additional environment variables.
func (env *TestEnvironment) RunGovardWithEnv(t *testing.T, projectDir string, extraEnv []string, args ...string) *CommandResult {
	t.Helper()

	cmd := exec.Command(env.BinaryPath, args...)
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(), extraEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	return &CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: cmd.ProcessState.ExitCode(),
		Duration: duration,
		Error:    err,
	}
}

// SetupRuntimeShims installs command stubs and returns the environment needed to activate them.
func (env *TestEnvironment) SetupRuntimeShims(t *testing.T, exitCodes map[string]int) *RuntimeShims {
	t.Helper()

	shimDir := filepath.Join(t.TempDir(), "runtime-shims")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatalf("failed to create shim dir: %v", err)
	}

	logPath := filepath.Join(shimDir, "commands.log")
	commands := []string{"docker", "ssh", "rsync"}

	for _, name := range commands {
		exitCode := 0
		if code, ok := exitCodes[name]; ok {
			exitCode = code
		}
		script := runtimeShimScript(name, exitCode)
		path := filepath.Join(shimDir, name)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("failed to write %s shim: %v", name, err)
		}
	}

	extraEnv := []string{
		"PATH=" + shimDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GOVARD_TEST_RUNTIME_LOG=" + logPath,
	}

	return &RuntimeShims{
		Dir:      shimDir,
		LogPath:  logPath,
		ExtraEnv: extraEnv,
	}
}

func runtimeShimScript(name string, exitCode int) string {
	exitVar := "GOVARD_TEST_EXIT_" + strings.ToUpper(name)

	sshBehavior := ""
	if name == "ssh" {
		sshBehavior = `
case "$*" in
  *govard-remote-ok*)
    printf '%s\n' "govard-remote-ok"
    ;;
  *govard-rsync-ok*)
    printf '%s\n' "govard-rsync-ok"
    ;;
esac
`
	}

	dockerBehavior := ""
	if name == "docker" {
		dockerBehavior = `
case "$*" in
  *"inspect -f {{.State.Running}}"*)
    printf '%s\n' "true"
    ;;
  *"inspect -f {{range .Config.Env}}{{println .}}{{end}}"*)
    printf '%s\n' "MYSQL_DATABASE=magento"
    printf '%s\n' "MYSQL_USER=magento"
    printf '%s\n' "MYSQL_PASSWORD=magento"
    printf '%s\n' "MYSQL_ROOT_PASSWORD=root"
    ;;
esac
`
	}

	return fmt.Sprintf(`#!/usr/bin/env sh
set -eu
log="${GOVARD_TEST_RUNTIME_LOG:-}"
if [ -n "$log" ]; then
  printf '%%s|%%s\n' %q "$*" >> "$log"
fi
%s
%s
exit "${%s:-%d}"
`, name, sshBehavior, dockerBehavior, exitVar, exitCode)
}

// Env returns additional environment variables required to use the runtime shims.
func (shims *RuntimeShims) Env() []string {
	if shims == nil {
		return nil
	}
	return append([]string{}, shims.ExtraEnv...)
}

// ReadLog returns the collected runtime shim invocation log.
func (shims *RuntimeShims) ReadLog(t *testing.T) string {
	t.Helper()
	if shims == nil {
		t.Fatal("runtime shims are nil")
	}
	data, err := os.ReadFile(shims.LogPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("failed to read shim log: %v", err)
	}
	return string(data)
}

// CommandResult holds the result of a command execution
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Error    error
}

// Success checks if the command succeeded
func (r *CommandResult) Success() bool {
	return r.ExitCode == 0 && r.Error == nil
}

// AssertSuccess fails the test if command didn't succeed
func (r *CommandResult) AssertSuccess(t *testing.T) {
	t.Helper()
	if !r.Success() {
		t.Fatalf("Command failed with exit code %d\nStderr: %s\nError: %v", r.ExitCode, r.Stderr, r.Error)
	}
}

// AssertExitCode fails the test if exit code doesn't match
func (r *CommandResult) AssertExitCode(t *testing.T, expected int) {
	t.Helper()
	if r.ExitCode != expected {
		t.Fatalf("Expected exit code %d, got %d\nStderr: %s", expected, r.ExitCode, r.Stderr)
	}
}

// AssertOutputContains fails the test if output doesn't contain expected string
func (r *CommandResult) AssertOutputContains(t *testing.T, expected string) {
	t.Helper()
	if !strings.Contains(r.Stdout, expected) {
		t.Fatalf("Expected output to contain %q, got:\n%s", expected, r.Stdout)
	}
}

// WaitForCondition waits for a condition to become true
func WaitForCondition(t *testing.T, timeout time.Duration, interval time.Duration, condition func() bool) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// CopyBlueprints copies blueprints to test project
func CopyBlueprints(t *testing.T, src, dst string) {
	t.Helper()

	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("Failed to create blueprints dir: %v", err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("Failed to read blueprints dir: %v", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
		} else {
			copyFile(t, srcPath, dstPath)
		}
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()

	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("Failed to create dir %s: %v", dst, err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("Failed to read dir %s: %v", src, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
		} else {
			copyFile(t, srcPath, dstPath)
		}
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", src, err)
	}

	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", dst, err)
	}
}

// MustMarshalJSON marshals data to JSON or fails the test
func MustMarshalJSON(t *testing.T, data interface{}) string {
	t.Helper()
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}
	return string(bytes)
}

// MustMarshalYAML marshals data to YAML or fails the test
func MustMarshalYAML(t *testing.T, data interface{}) string {
	t.Helper()
	bytes, err := yaml.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal YAML: %v", err)
	}
	return string(bytes)
}

// CreateGovardConfig creates a .govard.yml config file
func CreateGovardConfig(t *testing.T, projectDir string, config engine.Config) {
	t.Helper()

	data, err := yaml.Marshal(&config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	configPath := filepath.Join(projectDir, ".govard.yml")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write .govard.yml: %v", err)
	}
}

// SkipIfNoDocker skips the test if Docker is not available
func SkipIfNoDocker(t *testing.T) {
	t.Helper()

	if os.Getenv("SKIP_DOCKER_TESTS") != "" {
		t.Skip("Skipping Docker tests (SKIP_DOCKER_TESTS set)")
	}

	cmd := exec.Command("docker", "version")
	if err := cmd.Run(); err != nil {
		t.Skip("Docker not available, skipping test")
	}
}

// SkipIfNoDockerCompose skips the test if Docker Compose is not available
func SkipIfNoDockerCompose(t *testing.T) {
	t.Helper()

	SkipIfNoDocker(t)

	cmd := exec.Command("docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		t.Skip("Docker Compose not available, skipping test")
	}
}

// CleanupProject removes a test project and its resources
func (env *TestEnvironment) CleanupProject(t *testing.T, name string) {
	t.Helper()

	projectDir, exists := env.TestProjects[name]
	if !exists {
		return
	}

	// Try to stop any running containers first
	cmd := exec.Command(env.BinaryPath, "stop")
	cmd.Dir = projectDir
	cmd.Run() // Ignore errors

	// Clean up Docker resources
	projectName := filepath.Base(projectDir)
	if cfg, _, err := engine.LoadConfigFromDir(projectDir, false); err == nil && strings.TrimSpace(cfg.ProjectName) != "" {
		projectName = cfg.ProjectName
	}
	exec.Command("docker", "compose", "-f", engine.ComposeFilePath(projectDir, projectName), "down", "-v").Run()

	delete(env.TestProjects, name)
}

// ContainerExists checks if a Docker container exists
func ContainerExists(containerName string) bool {
	cmd := exec.Command("docker", "ps", "-a", "-q", "-f", fmt.Sprintf("name=%s", containerName))
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// ContainerRunning checks if a Docker container is running
func ContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "-q", "-f", fmt.Sprintf("name=%s", containerName), "-f", "status=running")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// NetworkExists checks if a Docker network exists
func NetworkExists(networkName string) bool {
	cmd := exec.Command("docker", "network", "ls", "-q", "-f", fmt.Sprintf("name=%s", networkName))
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// GenerateTestProjectName generates a unique test project name
func GenerateTestProjectName(t *testing.T) string {
	return fmt.Sprintf("govard-test-%d-%s", time.Now().Unix(), t.Name())
}

// LoadComposeServices loads the "services" section from a rendered compose file.
func LoadComposeServices(t *testing.T, composePath string) map[string]interface{} {
	t.Helper()

	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("Failed to read compose file %s: %v", composePath, err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Failed to parse compose YAML %s: %v", composePath, err)
	}

	services, ok := doc["services"].(map[string]interface{})
	if !ok {
		t.Fatalf("Compose file %s is missing services section", composePath)
	}

	return services
}
