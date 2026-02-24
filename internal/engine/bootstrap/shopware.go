package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

type ShopwareBootstrap struct {
	Options Options
}

func NewShopwareBootstrap(opts Options) *ShopwareBootstrap {
	return &ShopwareBootstrap{Options: opts}
}

func (s *ShopwareBootstrap) Name() string {
	return "shopware"
}

func (s *ShopwareBootstrap) SupportsFreshInstall() bool {
	return true
}

func (s *ShopwareBootstrap) SupportsClone() bool {
	return true
}

func (s *ShopwareBootstrap) FreshCommands() []string {
	return []string{
		"composer create-project shopware/production .",
	}
}

func (s *ShopwareBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh Shopware project...")

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read project directory: %w", err)
	}

	if len(entries) > 0 {
		pterm.Warning.Println("Project directory is not empty. Cleaning up...")
		for _, entry := range entries {
			if entry.Name() == ".govard" || entry.Name() == ".govard.yml" {
				continue
			}
			path := filepath.Join(projectDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("failed to remove %s: %w", entry.Name(), err)
			}
		}
	}

	cmd := exec.Command("composer", "create-project", "shopware/production", ".", "--no-interaction")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Shopware project: %w", err)
	}

	pterm.Success.Println("Shopware project created successfully")
	return nil
}

func (s *ShopwareBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running Shopware installation steps...")

	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		content := `APP_ENV=dev
APP_URL=http://localhost
DATABASE_URL=mysql://shopware:shopware@db:3306/shopware
MAILER_URL=smtp://mailpit:1025
`
		if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to create .env: %w", err)
		}
		pterm.Success.Println("Created .env")
	} else if data, err := os.ReadFile(envPath); err == nil {
		content := string(data)
		content = strings.ReplaceAll(content, "DATABASE_URL=mysql://root@127.0.0.1", "DATABASE_URL=mysql://shopware:shopware@db")
		content = strings.ReplaceAll(content, "DATABASE_URL=mysql://shopware:shopware@127.0.0.1", "DATABASE_URL=mysql://shopware:shopware@db")
		os.WriteFile(envPath, []byte(content), 0600)
	}

	if err := s.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Composer install warning: %v\n", err)
	}

	pterm.Info.Println("Running Shopware system install...")
	if err := s.runBinConsole(projectDir, "system:install", "--basic-setup", "--force", "--drop-database"); err != nil {
		pterm.Warning.Printf("System install warning: %v\n", err)
	}

	pterm.Success.Println("Shopware installation completed")
	return nil
}

func (s *ShopwareBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring Shopware environment...")

	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		content, err := os.ReadFile(envPath)
		if err == nil {
			updated := string(content)
			if !strings.Contains(updated, "@db") {
				updated = strings.ReplaceAll(updated, "DATABASE_URL=mysql://", "DATABASE_URL=mysql://shopware:shopware@db:3306/shopware")
				os.WriteFile(envPath, []byte(updated), 0600)
			}
		}
	}

	s.runBinConsole(projectDir, "cache:clear")

	pterm.Success.Println("Shopware configured successfully")
	return nil
}

func (s *ShopwareBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned Shopware project...")

	if err := s.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envLocalPath := filepath.Join(projectDir, ".env.local")
		if _, err := os.Stat(envLocalPath); err == nil {
			data, _ := os.ReadFile(envLocalPath)
			content := string(data)
			content = strings.ReplaceAll(content, "DATABASE_URL=mysql://", "DATABASE_URL=mysql://shopware:shopware@db:3306/shopware")
			os.WriteFile(envPath, []byte(content), 0600)
		}
	}

	s.runBinConsole(projectDir, "cache:clear")

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (s *ShopwareBootstrap) runComposerCommand(projectDir string, args ...string) error {
	cmd := exec.Command("composer", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (s *ShopwareBootstrap) runBinConsole(projectDir string, args ...string) error {
	consolePath := filepath.Join(projectDir, "bin", "console")
	if _, err := os.Stat(consolePath); os.IsNotExist(err) {
		pterm.Warning.Println("Console not found, skipping console commands")
		return nil
	}

	args = append([]string{consolePath}, args...)
	cmd := exec.Command("php", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
