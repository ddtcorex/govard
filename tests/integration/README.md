# Integration Tests for Govard

This directory contains integration tests for the Govard project. These tests verify end-to-end workflows and component interactions.

## Structure

```
tests/integration/
├── integration_test.go   # Core test utilities and helpers
├── framework_test.go     # Framework detection and configuration tests
├── cli_test.go           # CLI command integration tests
├── docker_test.go        # Docker operations tests
├── blueprint_test.go     # Blueprint rendering tests
└── workflow_test.go      # End-to-end workflow tests
```

## Running Tests

Integration tests are opt-in and use the `integration` build tag.

### Run all tests

```bash
make test
```

### Run only unit tests

```bash
make test-unit
```

### Run only integration tests

```bash
make test-integration
```

### Direct `go test` commands

```bash
# Unit-only (default path used by Makefile)
go test $(go list ./... | grep -v '^govard/tests/integration$') -v -short

# Integration-only (explicit tag required)
go test -tags integration ./tests/integration/... -v -timeout 30m
```

### Run specific test categories

```bash
# Framework detection tests
go test -tags integration ./tests/integration/... -v -run TestFramework

# CLI tests
go test -tags integration ./tests/integration/... -v -run TestCLI

# Blueprint tests
go test -tags integration ./tests/integration/... -v -run TestRender

# Docker tests
go test -tags integration ./tests/integration/... -v -run TestDocker
```

## Test Environment

The integration tests create a test environment that includes:

1. **Test Binary**: A govard binary is built specifically for testing
2. **Fixture Projects**: reusable projects live under `tests/integration/projects/`
3. **Temporary Copies**: fixtures are copied into temporary directories per test
4. **Blueprint Copies**: blueprints can be copied to test projects for rendering

## Fixture Projects

Integration fixtures are stored in-repo and copied per test for isolation:

```text
tests/integration/projects/
└── magento2/
    ├── clone-basic/
    ├── fresh-basic/
    └── policy-protected/
```

Use `CreateProjectFromFixture(t, "<group>/<fixture>", "<name>")` in tests.

## Runtime Shim Harness

For runtime-path validation without external Docker/SSH/rsync dependencies, tests can use command shims:

- `SetupRuntimeShims(t, map[string]int{...})` installs temporary `docker`, `ssh`, and `rsync` stubs.
- `RunGovardWithEnv(..., shim.Env(), ...)` runs Govard with shimmed `PATH`.
- `shim.ReadLog(t)` returns ordered command invocations for assertions.

## Utilities

### TestEnvironment

The main helper for integration tests:

```go
env := NewTestEnvironment(t)

// Create test projects
projectDir := env.CreateMagento2Project(t, "test-name")
projectDir := env.CreateLaravelProject(t, "test-name")
projectDir := env.CreateNextJSProject(t, "test-name")
projectDir := env.CreateTestProject(t, "test-name", files)
projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "fixture-copy")

// Run govard commands
result := env.RunGovard(t, projectDir, "init", "--recipe", "magento2")
result.AssertSuccess(t)
result.AssertOutputContains(t, "expected text")

// Runtime shim validation
shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
result = env.RunGovardWithEnv(t, projectDir, shim.Env(), "sync", "--source", "dev", "--file")
result.AssertSuccess(t)
_ = shim.ReadLog(t)

// Cleanup
env.CleanupProject(t, "test-name")
```

### Helpers

- `CopyBlueprints(t, src, dst)` - Copy blueprints to test project
- `CreateGovardConfig(t, dir, config)` - Create .govard.yml file
- `MustMarshalJSON(t, data)` - Marshal data to JSON
- `MustMarshalYAML(t, data)` - Marshal data to YAML
- `SkipIfNoDocker(t)` - Skip test if Docker unavailable
- `ContainerExists(name)` - Check if container exists
- `ContainerRunning(name)` - Check if container is running
- `NetworkExists(name)` - Check if network exists

## Writing New Tests

### Basic test structure

```go
func TestMyFeature(t *testing.T) {
    env := NewTestEnvironment(t)
    
    // Create test project
    projectDir := env.CreateTestProject(t, "my-test", map[string]string{
        "composer.json": `{"name": "test/project"}`,
    })
    
    // Run command
    result := env.RunGovard(t, projectDir, "my-command")
    result.AssertSuccess(t)
    
    // Verify results
    if _, err := os.Stat(filepath.Join(projectDir, "expected-file")); err != nil {
        t.Error("Expected file not created")
    }
}
```

### Framework-specific tests

```go
func TestMagento2SpecificFeature(t *testing.T) {
    env := NewTestEnvironment(t)
    
    projectDir := env.CreateMagento2Project(t, "m2-test")
    CopyBlueprints(t, env.BlueprintsPath, filepath.Join(projectDir, "blueprints"))
    
    // Test Magento 2 specific functionality
}
```

### Tests requiring Docker

```go
func TestDockerFeature(t *testing.T) {
    SkipIfNoDocker(t)
    
    env := NewTestEnvironment(t)
    // Test Docker-dependent features
}
```

## CI/CD

Integration tests run automatically on:

- Push to `main`, `master`, or `develop` branches
- Pull requests to these branches
- Manual workflow dispatch

Test categories run in parallel for faster feedback.

## Environment Variables

- `SKIP_DOCKER_TESTS` - Set to skip Docker-dependent tests
- `GOVARD_ENV` - Environment for config loading tests
