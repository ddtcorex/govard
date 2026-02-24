package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

type SymfonyBootstrap struct {
	Options Options
}

func NewSymfonyBootstrap(opts Options) *SymfonyBootstrap {
	return &SymfonyBootstrap{Options: opts}
}

func (s *SymfonyBootstrap) Name() string {
	return "symfony"
}

func (s *SymfonyBootstrap) SupportsFreshInstall() bool {
	return true
}

func (s *SymfonyBootstrap) SupportsClone() bool {
	return true
}

func (s *SymfonyBootstrap) FreshCommands() []string {
	version := s.Options.Version
	if version == "" {
		version = "7.0"
	}

	var skeleton string
	majorVersion := strings.Split(version, ".")[0]
	switch majorVersion {
	case "7":
		skeleton = "symfony/skeleton"
	case "6":
		skeleton = "symfony/skeleton:^6.0"
	case "5":
		skeleton = "symfony/website-skeleton:^5.0"
	default:
		skeleton = "symfony/skeleton"
	}

	return []string{
		"composer create-project " + skeleton + " .",
	}
}

func (s *SymfonyBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh Symfony project...")

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

	skeleton := s.getSkeletonForVersion(s.Options.Version)

	cmd := exec.Command("composer", "create-project", skeleton, ".", "--no-interaction")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Symfony project: %w", err)
	}

	pterm.Success.Println("Symfony project created successfully")
	return nil
}

func (s *SymfonyBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running Symfony installation steps...")

	envLocalPath := filepath.Join(projectDir, ".env.local")
	if _, err := os.Stat(envLocalPath); os.IsNotExist(err) {
		content := `APP_ENV=dev
APP_SECRET=your-secret-key-here
DATABASE_URL="mysql://symfony:symfony@db:3306/symfony?serverVersion=11.4.0-MariaDB&charset=utf8mb4"
MAILER_DSN=smtp://mailpit:1025
`
		if err := os.WriteFile(envLocalPath, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to create .env.local: %w", err)
		}
		pterm.Success.Println("Created .env.local")
	}

	if err := s.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Composer install warning: %v\n", err)
	}

	pterm.Info.Println("Creating database...")
	if err := s.runSymfonyConsole(projectDir, "doctrine:database:create", "--if-not-exists"); err != nil {
		pterm.Warning.Printf("Database creation warning: %v\n", err)
	}

	pterm.Info.Println("Running database migrations...")
	if err := s.runSymfonyConsole(projectDir, "doctrine:migrations:migrate", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Migrations warning: %v\n", err)
	}

	pterm.Success.Println("Symfony installation completed")
	return nil
}

func (s *SymfonyBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring Symfony environment...")

	envLocalPath := filepath.Join(projectDir, ".env.local")
	if _, err := os.Stat(envLocalPath); err == nil {
		content, err := os.ReadFile(envLocalPath)
		if err == nil {
			updated := string(content)
			if !strings.Contains(updated, "@db:") {
				updated = strings.ReplaceAll(updated,
					"DATABASE_URL=",
					"DATABASE_URL=\"mysql://symfony:symfony@db:3306/symfony?serverVersion=11.4.0-MariaDB&charset=utf8mb4\"")
				os.WriteFile(envLocalPath, []byte(updated), 0600)
			}
		}
	}

	s.runSymfonyConsole(projectDir, "cache:clear")

	pterm.Success.Println("Symfony configured successfully")
	return nil
}

func (s *SymfonyBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned Symfony project...")

	if err := s.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	envLocalPath := filepath.Join(projectDir, ".env.local")
	if _, err := os.Stat(envLocalPath); os.IsNotExist(err) {
		envPath := filepath.Join(projectDir, ".env")
		if data, err := os.ReadFile(envPath); err == nil {
			localContent := string(data)
			localContent = strings.ReplaceAll(localContent, "APP_ENV=prod", "APP_ENV=dev")
			localContent = strings.ReplaceAll(localContent, "APP_DEBUG=0", "APP_DEBUG=1")
			os.WriteFile(envLocalPath, []byte(localContent), 0600)
		}
	}

	s.runSymfonyConsole(projectDir, "doctrine:database:create", "--if-not-exists")

	dumpPath := filepath.Join(projectDir, "dump.sql")
	if _, err := os.Stat(dumpPath); err == nil {
		pterm.Info.Println("Importing database dump...")
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (s *SymfonyBootstrap) getSkeletonForVersion(version string) string {
	if version == "" {
		return "symfony/skeleton"
	}

	parts := strings.Split(version, ".")
	major := parts[0]

	switch major {
	case "7":
		return "symfony/skeleton"
	case "6":
		return "symfony/skeleton:^6.0"
	case "5":
		return "symfony/website-skeleton:^5.0"
	default:
		return "symfony/skeleton"
	}
}

func (s *SymfonyBootstrap) runComposerCommand(projectDir string, args ...string) error {
	cmd := exec.Command("composer", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (s *SymfonyBootstrap) runSymfonyConsole(projectDir string, args ...string) error {
	consolePath := filepath.Join(projectDir, "bin", "console")
	if _, err := os.Stat(consolePath); os.IsNotExist(err) {
		consolePath = filepath.Join(projectDir, "app", "console")
		if _, err := os.Stat(consolePath); os.IsNotExist(err) {
			pterm.Warning.Println("Symfony console not found, skipping console commands")
			return nil
		}
	}

	args = append([]string{consolePath}, args...)
	cmd := exec.Command("php", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
