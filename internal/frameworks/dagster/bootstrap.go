package dagster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"

	"github.com/pterm/pterm"
)

type DagsterBootstrap struct {
	Options bootstrap.Options
}

func NewDagsterBootstrap(opts bootstrap.Options) *DagsterBootstrap {
	return &DagsterBootstrap{Options: opts}
}

func (d *DagsterBootstrap) Name() string {
	return "dagster"
}

func (d *DagsterBootstrap) SupportsFreshInstall() bool {
	return true
}

func (d *DagsterBootstrap) SupportsClone() bool {
	return true
}

func (d *DagsterBootstrap) FreshCommands() []string {
	return []string{}
}

func (d *DagsterBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh Dagster project...")

	createInStage := func(stageDir string) error {
		return createDagsterProjectInStage(stageDir, d.Options.ProjectName)
	}
	quotedName := conventions.ShellQuote(d.Options.ProjectName)
	// The `shopt` tolerance is deliberately scoped inside its own "(... ; ...)"
	// group, joined to the scaffold step with "&&" - so a failure in pip
	// install/cd/scaffold still short-circuits the whole command (exit
	// non-zero), while only shopt's own absence (non-bash sh has no
	// `shopt` builtin) is swallowed via a local "|| true" before the
	// mv/rm-rf chain runs. A bare ";" before mv here would otherwise mask
	// any upstream failure, since sh's exit status after ";" reflects only
	// the last statement.
	runnerCommand := "pip install --no-cache-dir dagster dagster-webserver dagster-postgres && " +
		`cd "$GOVARD_STAGE_DIR" && dagster project scaffold --name ` + quotedName + ` && ` +
		`(shopt -s dotglob 2>/dev/null || true; mv ` + quotedName + `/* "$GOVARD_STAGE_DIR"/ && rm -rf ` + quotedName + `)`
	if err := bootstrap.RunStagedCreateProject(projectDir, d.Options.Runner, createInStage, runnerCommand, conventions.PythonWorkDir); err != nil {
		return fmt.Errorf("failed to create Dagster project: %w", err)
	}

	if err := writeDagsterRequirements(projectDir); err != nil {
		return fmt.Errorf("failed to write requirements.txt: %w", err)
	}

	if err := writeDagsterConfigFiles(projectDir); err != nil {
		return fmt.Errorf("failed to write Dagster config files: %w", err)
	}

	pterm.Success.Println("Dagster project created successfully")
	return nil
}

// createDagsterProjectInStage is the host-side fallback used only when
// Options.Runner is nil (no Docker runner configured, e.g. tests). It
// requires `dagster` to already be on the host's PATH. Like the container
// path's runnerCommand, it flattens the scaffolded <projectName>/
// subdirectory up to stageDir's root afterward, so pyproject.toml ends up
// at stageDir's root either way and later autodiscovery
// (dagsterAutodiscoveryModule) can find it.
func createDagsterProjectInStage(stageDir string, projectName string) error {
	if _, err := exec.LookPath("dagster"); err != nil {
		return fmt.Errorf("dagster CLI not found in PATH, cannot create Dagster project %q", projectName)
	}

	cmd := exec.Command("dagster", "project", "scaffold", "--name", projectName)
	cmd.Dir = stageDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return err
	}

	return flattenScaffoldedProject(stageDir, projectName)
}

// flattenScaffoldedProject moves the contents of stageDir/projectName up
// to stageDir's root and removes the now-empty scaffolded subdirectory,
// mirroring the mv/rm-rf flattening the container path performs in shell
// (see runnerCommand in CreateProject above).
func flattenScaffoldedProject(stageDir string, projectName string) error {
	scaffoldedDir := filepath.Join(stageDir, projectName)
	entries, err := os.ReadDir(scaffoldedDir)
	if err != nil {
		return fmt.Errorf("read scaffolded project directory: %w", err)
	}

	for _, entry := range entries {
		src := filepath.Join(scaffoldedDir, entry.Name())
		dst := filepath.Join(stageDir, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("move scaffolded %q to stage root: %w", entry.Name(), err)
		}
	}

	return os.RemoveAll(scaffoldedDir)
}

// writeDagsterRequirements writes requirements.txt with the Dagster
// packages every fresh project needs: the core library, the local-dev
// webserver, and the Postgres storage backend the compose blueprint
// provisions.
func writeDagsterRequirements(projectDir string) error {
	content := "dagster\ndagster-webserver\ndagster-postgres\n"
	return os.WriteFile(filepath.Join(projectDir, "requirements.txt"), []byte(content), conventions.DefaultFilePerm)
}

func (d *DagsterBootstrap) Install(projectDir string) error {
	return nil
}

func (d *DagsterBootstrap) Configure(projectDir string) error {
	return nil
}

func (d *DagsterBootstrap) PostClone(projectDir string) error {
	containerName := d.Options.ProjectName + conventions.WebSuffix
	script := "pip install --no-cache-dir -r requirements.txt; rc=$?; chown -R \"$(stat -c %u:%g .)\" . 2>/dev/null; exit $rc"

	if err := dagsterContainerExecRunner(containerName, script); err != nil {
		return fmt.Errorf("dagster pip install failed: %w", err)
	}
	return nil
}

// dagsterContainerExecRunner execs a shell script inside the Dagster "web"
// container - overridable in tests via SetDagsterContainerExecRunnerForTest
// so PostClone() doesn't require a real Docker daemon.
var dagsterContainerExecRunner = func(containerName string, script string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "sh", "-lc", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SetDagsterContainerExecRunnerForTest overrides the docker-exec runner
// used by PostClone(), returning a restore function.
func SetDagsterContainerExecRunnerForTest(fn func(containerName string, script string) error) func() {
	previous := dagsterContainerExecRunner
	if fn != nil {
		dagsterContainerExecRunner = fn
	}
	return func() {
		dagsterContainerExecRunner = previous
	}
}
