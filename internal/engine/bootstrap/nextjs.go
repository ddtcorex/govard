package bootstrap

import (
	"fmt"
	"govard/internal/conventions"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

var nextJSStageProjectCreator = createNextJSProjectInStage

type NextJSBootstrap struct {
	Options Options
}

func NewNextJSBootstrap(opts Options) *NextJSBootstrap {
	return &NextJSBootstrap{Options: opts}
}

func (n *NextJSBootstrap) Name() string {
	return "nextjs"
}

func (n *NextJSBootstrap) SupportsFreshInstall() bool {
	return true
}

func (n *NextJSBootstrap) SupportsClone() bool {
	return true
}

func (n *NextJSBootstrap) FreshCommands() []string {
	return []string{
		"npx create-next-app@latest .",
	}
}

func (n *NextJSBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh Next.js project...")

	runnerCommand := strings.Join([]string{
		"npx", "create-next-app@latest", `"$GOVARD_STAGE_DIR"`,
		"--typescript", "--tailwind", "--eslint", "--app", "--no-src-dir",
		"--import-alias", "'@/*'", "--use-npm", "--yes",
	}, " ")
	if err := runStagedCreateProject(projectDir, n.Options.Runner, nextJSStageProjectCreator, runnerCommand, conventions.NodeWorkDir); err != nil {
		return fmt.Errorf("failed to create Next.js project: %w", err)
	}

	pterm.Success.Println("Next.js project created successfully")
	return nil
}

func createNextJSProjectInStage(stageDir string) error {
	var cmd *exec.Cmd

	if _, err := exec.LookPath("npx"); err == nil {
		pterm.Info.Println("Running npx create-next-app...")
		cmd = exec.Command("npx", "create-next-app@latest", stageDir,
			"--typescript",
			"--tailwind",
			"--eslint",
			"--app",
			"--no-src-dir",
			"--import-alias", "@/*",
			"--use-npm",
			"--yes",
		)
	} else if _, err := exec.LookPath("yarn"); err == nil {
		pterm.Info.Println("npx not found. Running yarn create next-app...")
		cmd = exec.Command("yarn", "create", "next-app", stageDir,
			"--typescript",
			"--tailwind",
			"--eslint",
			"--app",
			"--no-src-dir",
			"--import-alias", "@/*",
			"--use-yarn",
			"--yes",
		)
	} else {
		return fmt.Errorf("neither npx nor yarn found in PATH, cannot create Next.js project")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func (n *NextJSBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Installing Next.js dependencies...")

	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); os.IsNotExist(err) {
		return fmt.Errorf("package.json not found, cannot install dependencies")
	}

	var cmd *exec.Cmd
	if _, err := exec.LookPath("npm"); err == nil {
		cmd = exec.Command("npm", "install")
	} else if _, err := exec.LookPath("yarn"); err == nil {
		pterm.Info.Println("npm not found. Running yarn install...")
		cmd = exec.Command("yarn", "install")
	} else {
		return fmt.Errorf("neither npm nor yarn found in PATH, cannot install dependencies")
	}

	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dependency installation failed: %w", err)
	}

	pterm.Success.Println("Next.js dependencies installed")
	return nil
}

func (n *NextJSBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring Next.js environment...")

	envPath := filepath.Join(projectDir, ".env.local")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		content := `NODE_ENV=development
NEXT_PUBLIC_APP_URL=http://localhost:3000
`
		if err := os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm); err != nil {
			return fmt.Errorf("failed to create .env.local: %w", err)
		}
		pterm.Success.Println("Created .env.local")
	}

	envExamplePath := filepath.Join(projectDir, ".env.example")
	if _, err := os.Stat(envExamplePath); os.IsNotExist(err) {
		content := `NODE_ENV=development
NEXT_PUBLIC_APP_URL=http://localhost:3000
`
		if err := os.WriteFile(envExamplePath, []byte(content), conventions.DefaultFilePerm); err != nil {
			pterm.Warning.Printf("Failed to create .env.example: %v\n", err)
		}
	}

	pterm.Success.Println("Next.js configured successfully")
	return nil
}

func (n *NextJSBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned Next.js project...")

	if err := n.Install(projectDir); err != nil {
		return err
	}

	if err := n.Configure(projectDir); err != nil {
		return err
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func SetNextJSStageProjectCreatorForTest(fn func(stageDir string) error) func() {
	previous := nextJSStageProjectCreator
	if fn != nil {
		nextJSStageProjectCreator = fn
	}
	return func() {
		nextJSStageProjectCreator = previous
	}
}
