package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pterm/pterm"
)

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

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read project directory: %w", err)
	}

	if len(entries) > 0 {
		pterm.Warning.Println("Project directory is not empty. Cleaning up...")
		for _, entry := range entries {
			if entry.Name() == ".govard" || entry.Name() == "govard.yml" {
				continue
			}
			path := filepath.Join(projectDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("failed to remove %s: %w", entry.Name(), err)
			}
		}
	}

	pterm.Info.Println("Running npx create-next-app...")

	cmd := exec.Command("npx", "create-next-app@latest", ".",
		"--typescript",
		"--tailwind",
		"--eslint",
		"--app",
		"--no-src-dir",
		"--import-alias", "@/*",
		"--use-npm",
		"--yes",
	)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Next.js project: %w", err)
	}

	pterm.Success.Println("Next.js project created successfully")
	return nil
}

func (n *NextJSBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Installing Next.js dependencies...")

	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); os.IsNotExist(err) {
		return fmt.Errorf("package.json not found, cannot install dependencies")
	}

	cmd := exec.Command("npm", "install")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm install failed: %w", err)
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
		if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to create .env.local: %w", err)
		}
		pterm.Success.Println("Created .env.local")
	}

	envExamplePath := filepath.Join(projectDir, ".env.example")
	if _, err := os.Stat(envExamplePath); os.IsNotExist(err) {
		content := `NODE_ENV=development
NEXT_PUBLIC_APP_URL=http://localhost:3000
`
		if err := os.WriteFile(envExamplePath, []byte(content), 0644); err != nil {
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
