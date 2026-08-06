package dagster

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"

	"github.com/pterm/pterm"
)

// dagsterStorageConfig is dagster.yaml's Postgres storage block, using
// Dagster's `env:` indirection so the file reads connection info from the
// container's already-injected POSTGRES_* env vars - no per-project Go
// templating needed, unlike Django's settings.py patch.
const dagsterStorageConfig = `storage:
  postgres:
    postgres_db:
      username:
        env: POSTGRES_USER
      password:
        env: POSTGRES_PASSWORD
      hostname:
        env: POSTGRES_HOST
      db_name:
        env: POSTGRES_DB
      port:
        env: POSTGRES_PORT
`

// moduleNameFromPyprojectToml extracts `module_name` from the
// `[tool.dagster]` section of a scaffolded pyproject.toml's contents.
// `dagster project scaffold` puts the actual Definitions object in
// "<pkg>/definitions.py", not the bare top-level package, and records the
// real autodiscovery target here - reading it back avoids hardcoding an
// on-disk layout that dagster itself controls and has changed across
// releases. The scaffolded [tool.dagster] block is a small, stable, flat
// key="value" section, so a line-scan is used instead of a pulling in a
// full TOML parser dependency for one field.
func moduleNameFromPyprojectToml(content string) (string, error) {
	inSection := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inSection = trimmed == "[tool.dagster]"
			continue
		}
		if !inSection {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != "module_name" {
			continue
		}
		moduleName := strings.Trim(strings.TrimSpace(value), `"`)
		if moduleName == "" {
			return "", fmt.Errorf("module_name value is empty in [tool.dagster] section")
		}
		return moduleName, nil
	}
	return "", fmt.Errorf("module_name not found in [tool.dagster] section of pyproject.toml")
}

// dagsterAutodiscoveryModule reads the scaffolded project's pyproject.toml
// (already synced into projectDir by RunStagedCreateProject) and returns
// the module_name Dagster itself recorded for autodiscovery.
func dagsterAutodiscoveryModule(projectDir string) (string, error) {
	content, err := os.ReadFile(filepath.Join(projectDir, "pyproject.toml"))
	if err != nil {
		return "", fmt.Errorf("read pyproject.toml: %w", err)
	}
	return moduleNameFromPyprojectToml(string(content))
}

// writeDagsterConfigFiles writes dagster.yaml (Postgres storage config) and
// workspace.yaml (pointing `python_module` at the scaffolded project's real
// Definitions module, read back from pyproject.toml) into projectDir.
func writeDagsterConfigFiles(projectDir string) error {
	moduleName, err := dagsterAutodiscoveryModule(projectDir)
	if err != nil {
		return fmt.Errorf("determine Dagster autodiscovery module: %w", err)
	}

	if err := os.WriteFile(filepath.Join(projectDir, "dagster.yaml"), []byte(dagsterStorageConfig), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write dagster.yaml: %w", err)
	}

	workspaceYAML := "load_from:\n  - python_module: " + moduleName + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, "workspace.yaml"), []byte(workspaceYAML), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write workspace.yaml: %w", err)
	}

	return nil
}

// WriteDagsterConfigFilesForTest exposes writeDagsterConfigFiles for tests in /tests.
func WriteDagsterConfigFilesForTest(projectDir string) error {
	return writeDagsterConfigFiles(projectDir)
}

// ModuleNameFromPyprojectTomlForTest exposes moduleNameFromPyprojectToml for tests in /tests.
func ModuleNameFromPyprojectTomlForTest(content string) (string, error) {
	return moduleNameFromPyprojectToml(content)
}

// freshInstall runs Dagster's fresh-install sequence: CreateProject ->
// (if not skipping up) EnvUp -> Install. Overrides opts.Runner to the
// Python container runner - the registry dispatcher wires opts.Runner to
// the PHP container runner by default, which is wrong for a Python
// framework. Dagster manages its own env-up timing
// (FreshInstallManagesOwnEnvUp) because its compose "web" container runs
// `dagster dev -w workspace.yaml` directly and can't come up against an
// empty project directory - CreateProject must scaffold the project
// first. Install is currently a no-op for Dagster, but is still called
// (and its error checked) for interface parity with other frameworks.
func freshInstall(opts bootstrap.Options, projectDir string, helpers bootstrap.CmdHelpers) error {
	opts.Runner = helpers.PythonRunner
	d := NewDagsterBootstrap(opts)

	if err := d.CreateProject(projectDir); err != nil {
		return err
	}

	if opts.SkipUp {
		pterm.Info.Println("Skipping env up (--no-up); run `govard env up` manually.")
		return bootstrap.ErrFreshInstallSkipUp
	}

	if err := helpers.EnvUp(); err != nil {
		return fmt.Errorf("failed to start local environment: %w", err)
	}

	return d.Install(projectDir)
}
