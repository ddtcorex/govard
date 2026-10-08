package symfony

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

type SymfonyBootstrap struct {
	Options bootstrap.Options
}

func NewSymfonyBootstrap(opts bootstrap.Options) *SymfonyBootstrap {
	return &SymfonyBootstrap{Options: opts}
}

// BootstrapSymfony runs Symfony's fresh-install command generation.
func BootstrapSymfony(opts bootstrap.Options) error {
	symfonyBootstrap := NewSymfonyBootstrap(opts)
	_ = symfonyBootstrap.FreshCommands()
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
		return bootstrap.RunComposerProjectCommand(projectDir, nil, "create-project", skeleton, stageDir, "--no-interaction")
	}
	runnerCommand := "composer create-project " + skeleton + " \"$GOVARD_STAGE_DIR\" --no-interaction"
	if err := bootstrap.RunStagedCreateProject(projectDir, s.Options.Runner, createInStage, runnerCommand, conventions.DefaultWorkDir); err != nil {
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
DATABASE_URL="%s"
MAILER_DSN=smtp://%s:%d
`, s.databaseURL(dbUser, dbPass, dbHost, dbName), conventions.DefaultMailHost, conventions.SMTPPort)
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

			desired := fmt.Sprintf("DATABASE_URL=\"%s\"", s.databaseURL(dbUser, dbPass, dbHost, dbName))
			if updated := setEnvAssignment(string(content), "DATABASE_URL", desired); updated != string(content) {
				_ = os.WriteFile(envLocalPath, []byte(updated), conventions.DefaultFilePerm)
			}
		}
	}

	_ = s.runSymfonyConsole(projectDir, "cache:clear")

	pterm.Success.Println("Symfony configured successfully")
	return nil
}

// setEnvAssignment makes the active `key=...` line of a dotenv file equal to
// assignment (a full `KEY=value` line). Only the line's own value is replaced,
// so a quoted or unquoted old value cannot leak into the new one; commented
// lines are left alone; the line is appended when the key has no active line.
// An already-equal file is returned unchanged.
func setEnvAssignment(content, key, assignment string) string {
	lines := strings.Split(content, "\n")
	needle := key + "="
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			lines[i] = assignment
			replaced = true
		}
	}
	if !replaced {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, assignment, "")
	}
	return strings.Join(lines, "\n")
}

// databaseURL builds the Doctrine DATABASE_URL for the local database service.
// serverVersion comes from the stack (engine and db_version) because Doctrine
// uses it to pick the SQL platform without connecting; a literal that does not
// match the running server makes it generate the wrong platform's SQL.
func (s *SymfonyBootstrap) databaseURL(user, pass, host, name string) string {
	return fmt.Sprintf("mysql://%s:%s@%s:%d/%s?serverVersion=%s&charset=utf8mb4",
		user, pass, host, conventions.MySQLPort, name, doctrineServerVersion(s.Options.DBEngine, s.Options.DBVersion))
}

// doctrineServerVersion renders the serverVersion query value: MariaDB needs
// the "-MariaDB" suffix so Doctrine selects the MariaDB platform; MySQL takes
// the bare version. An unknown stack falls back to the default database
// (MariaDB 10.11).
func doctrineServerVersion(dbEngine, dbVersion string) string {
	dbVersion = strings.TrimSpace(dbVersion)
	if dbVersion == "" {
		dbEngine, dbVersion = "mariadb", "10.11"
	}
	if strings.EqualFold(strings.TrimSpace(dbEngine), "mysql") {
		return dbVersion
	}
	return dbVersion + "-MariaDB"
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
	return bootstrap.RunComposerProjectCommand(projectDir, s.Options.Runner, args...)
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

	return bootstrap.RunPHPProjectScript(projectDir, s.Options.Runner, consolePath, args...)
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
