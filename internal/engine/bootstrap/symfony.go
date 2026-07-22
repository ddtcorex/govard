package bootstrap

import (
	"fmt"
	"govard/internal/conventions"
	"os"
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

// BootstrapSymfony runs Symfony's fresh-install command generation.
func BootstrapSymfony(opts Options) error {
	bootstrap := NewSymfonyBootstrap(opts)
	_ = bootstrap.FreshCommands()
	return nil
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

	skeleton := s.getSkeletonForVersion(s.Options.Version)

	createInStage := func(stageDir string) error {
		return runComposerProjectCommand(projectDir, nil, "create-project", skeleton, stageDir, "--no-interaction")
	}
	runnerCommand := "composer create-project " + skeleton + " \"$GOVARD_STAGE_DIR\" --no-interaction"
	if err := runStagedCreateProject(projectDir, s.Options.Runner, createInStage, runnerCommand, conventions.DefaultWorkDir); err != nil {
		return fmt.Errorf("failed to create Symfony project: %w", err)
	}

	pterm.Success.Println("Symfony project created successfully")
	return nil
}

func (s *SymfonyBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running Symfony installation steps...")

	envLocalPath := filepath.Join(projectDir, ".env.local")
	if _, err := os.Stat(envLocalPath); os.IsNotExist(err) {
		dbHost := s.Options.DBHost
		if dbHost == "" {
			dbHost = conventions.DefaultDBHost
		}
		dbUser := s.Options.DBUser
		if dbUser == "" {
			dbUser = "symfony"
		}
		dbPass := s.Options.DBPass
		if dbPass == "" {
			dbPass = "symfony"
		}
		dbName := s.Options.DBName
		if dbName == "" {
			dbName = "symfony"
		}

		content := fmt.Sprintf(`APP_ENV=dev
APP_SECRET=your-secret-key-here
DATABASE_URL="mysql://%s:%s@%s:%d/%s?serverVersion=11.4.0-MariaDB&charset=utf8mb4"
MAILER_DSN=smtp://mailpit:1025
`, dbUser, dbPass, dbHost, conventions.MySQLPort, dbName)
		if err := os.WriteFile(envLocalPath, []byte(content), conventions.DefaultFilePerm); err != nil {
			return fmt.Errorf("failed to create .env.local: %w", err)
		}
		pterm.Success.Println("Created .env.local")
	}

	if err := s.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Composer install warning: %v\n", err)
	}

	if s.hasDoctrine(projectDir) {
		pterm.Info.Println("Creating database...")
		if err := s.runSymfonyConsole(projectDir, "doctrine:database:create", "--if-not-exists"); err != nil {
			pterm.Warning.Printf("Database creation warning: %v\n", err)
		}
	} else {
		pterm.Info.Println("Doctrine ORM is not installed, skipping database creation")
	}

	if s.hasMigrations(projectDir) {
		pterm.Info.Println("Running database migrations...")
		if err := s.runSymfonyConsole(projectDir, "doctrine:migrations:migrate", "--no-interaction"); err != nil {
			pterm.Warning.Printf("Migrations warning: %v\n", err)
		}
	} else {
		pterm.Info.Println("Doctrine Migrations are not installed, skipping database migrations")
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
			dbHost := s.Options.DBHost
			if dbHost == "" {
				dbHost = conventions.DefaultDBHost
			}
			dbUser := s.Options.DBUser
			if dbUser == "" {
				dbUser = "symfony"
			}
			dbPass := s.Options.DBPass
			if dbPass == "" {
				dbPass = "symfony"
			}
			dbName := s.Options.DBName
			if dbName == "" {
				dbName = "symfony"
			}

			updated := string(content)
			if !strings.Contains(updated, "@"+dbHost+":") {
				updated = strings.ReplaceAll(updated,
					"DATABASE_URL=",
					fmt.Sprintf("DATABASE_URL=\"mysql://%s:%s@%s:%d/%s?serverVersion=11.4.0-MariaDB&charset=utf8mb4\"",
						dbUser, dbPass, dbHost, conventions.MySQLPort, dbName))
				_ = os.WriteFile(envLocalPath, []byte(updated), conventions.DefaultFilePerm)
			}
		}
	}

	_ = s.runSymfonyConsole(projectDir, "cache:clear")

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
			_ = os.WriteFile(envLocalPath, []byte(localContent), conventions.DefaultFilePerm)
		}
	}

	if s.hasDoctrine(projectDir) {
		_ = s.runSymfonyConsole(projectDir, "doctrine:database:create", "--if-not-exists")
	}

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
	return runComposerProjectCommand(projectDir, s.Options.Runner, args...)
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

	return runPHPProjectScript(projectDir, s.Options.Runner, consolePath, args...)
}

func (s *SymfonyBootstrap) hasDoctrine(projectDir string) bool {
	composerPath := filepath.Join(projectDir, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, `"doctrine/`) || strings.Contains(content, `"symfony/orm-pack"`)
}

func (s *SymfonyBootstrap) hasMigrations(projectDir string) bool {
	composerPath := filepath.Join(projectDir, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, `"doctrine/doctrine-migrations-bundle"`) || strings.Contains(content, `"doctrine/migrations"`)
}

func (s *SymfonyBootstrap) HasDoctrineForTest(projectDir string) bool {
	return s.hasDoctrine(projectDir)
}

func (s *SymfonyBootstrap) HasMigrationsForTest(projectDir string) bool {
	return s.hasMigrations(projectDir)
}
