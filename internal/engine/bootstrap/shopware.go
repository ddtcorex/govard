package bootstrap

import (
	"fmt"
	"govard/internal/conventions"
	"os"
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

	createInStage := func(stageDir string) error {
		return runComposerProjectCommand(projectDir, nil, "create-project", "shopware/production", stageDir, "--no-interaction")
	}
	runnerCommand := "composer create-project shopware/production \"$GOVARD_STAGE_DIR\" --no-interaction"
	if err := runStagedCreateProject(projectDir, s.Options.Runner, createInStage, runnerCommand, conventions.DefaultWorkDir); err != nil {
		return fmt.Errorf("failed to create Shopware project: %w", err)
	}

	pterm.Success.Println("Shopware project created successfully")
	return nil
}

func (s *ShopwareBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running Shopware installation steps...")

	dbHost, dbUser, dbPass, dbName := s.resolveDBConfig()
	databaseURL := fmt.Sprintf("mysql://%s:%s@%s:%d/%s", dbUser, dbPass, dbHost, conventions.MySQLPort, dbName)
	siteURL := s.resolveSiteURL()
	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		content := `APP_ENV=dev
APP_URL=` + siteURL + `
DATABASE_URL=` + databaseURL + `
MAILER_DSN=smtp://mailpit:1025
PROXY_URL=` + siteURL + `
`
		if err := os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm); err != nil {
			return fmt.Errorf("failed to create .env: %w", err)
		}
		pterm.Success.Println("Created .env")
	} else if data, err := os.ReadFile(envPath); err == nil {
		content := replaceOrAppendEnvAssignment(string(data), "DATABASE_URL", databaseURL)
		content = replaceOrAppendEnvAssignment(content, "APP_URL", siteURL)
		content = replaceOrAppendEnvAssignment(content, "PROXY_URL", siteURL)
		_ = os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm)
	}

	if err := s.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Composer install warning: %v\n", err)
	}

	pterm.Info.Println("Running Shopware system install...")
	if err := s.runBinConsole(projectDir, "system:install", "--basic-setup", "--force", "--drop-database"); err != nil {
		pterm.Warning.Printf("System install warning: %v\n", err)
	}

	// Re-apply APP_URL and PROXY_URL to .env as system:install resets them
	if data, err := os.ReadFile(envPath); err == nil {
		content := replaceOrAppendEnvAssignment(string(data), "APP_URL", siteURL)
		content = replaceOrAppendEnvAssignment(content, "PROXY_URL", siteURL)
		_ = os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm)
	}

	if err := s.syncSalesChannelDomain(projectDir, siteURL); err != nil {
		pterm.Warning.Printf("Sales channel URL warning: %v\n", err)
	}

	s.ensureShopwareDirsWritable(projectDir)

	pterm.Success.Println("Shopware installation completed")
	return nil
}

func (s *ShopwareBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring Shopware environment...")

	dbHost, dbUser, dbPass, dbName := s.resolveDBConfig()
	databaseURL := fmt.Sprintf("mysql://%s:%s@%s:%d/%s", dbUser, dbPass, dbHost, conventions.MySQLPort, dbName)
	siteURL := s.resolveSiteURL()
	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		content, err := os.ReadFile(envPath)
		if err == nil {
			updated := replaceOrAppendEnvAssignment(string(content), "DATABASE_URL", databaseURL)
			updated = replaceOrAppendEnvAssignment(updated, "APP_URL", siteURL)
			updated = replaceOrAppendEnvAssignment(updated, "PROXY_URL", siteURL)
			_ = os.WriteFile(envPath, []byte(updated), conventions.DefaultFilePerm)
		}
	}

	_ = s.runBinConsole(projectDir, "cache:clear")
	_ = s.syncSalesChannelDomain(projectDir, siteURL)

	s.ensureShopwareDirsWritable(projectDir)

	pterm.Success.Println("Shopware configured successfully")
	return nil
}

func (s *ShopwareBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned Shopware project...")

	if err := s.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	envPath := filepath.Join(projectDir, ".env")
	siteURL := s.resolveSiteURL()
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envLocalPath := filepath.Join(projectDir, ".env.local")
		if _, err := os.Stat(envLocalPath); err == nil {
			data, _ := os.ReadFile(envLocalPath)
			dbHost, dbUser, dbPass, dbName := s.resolveDBConfig()
			content := replaceOrAppendEnvAssignment(string(data), "DATABASE_URL", fmt.Sprintf("mysql://%s:%s@%s:%d/%s", dbUser, dbPass, dbHost, conventions.MySQLPort, dbName))
			content = replaceOrAppendEnvAssignment(content, "APP_URL", siteURL)
			content = replaceOrAppendEnvAssignment(content, "PROXY_URL", siteURL)
			_ = os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm)
		}
	} else if data, err := os.ReadFile(envPath); err == nil {
		content := replaceOrAppendEnvAssignment(string(data), "APP_URL", siteURL)
		content = replaceOrAppendEnvAssignment(content, "PROXY_URL", siteURL)
		_ = os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm)
	}

	_ = s.runBinConsole(projectDir, "cache:clear")
	_ = s.syncSalesChannelDomain(projectDir, siteURL)

	s.ensureShopwareDirsWritable(projectDir)

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (s *ShopwareBootstrap) runComposerCommand(projectDir string, args ...string) error {
	return runComposerProjectCommand(projectDir, s.Options.Runner, args...)
}

func (s *ShopwareBootstrap) runBinConsole(projectDir string, args ...string) error {
	consolePath := filepath.Join(projectDir, "bin", "console")
	if _, err := os.Stat(consolePath); os.IsNotExist(err) {
		pterm.Warning.Println("Console not found, skipping console commands")
		return nil
	}

	return runPHPProjectScript(projectDir, s.Options.Runner, consolePath, args...)
}

func (s *ShopwareBootstrap) resolveDBConfig() (host, user, pass, name string) {
	host = strings.TrimSpace(s.Options.DBHost)
	if host == "" {
		host = conventions.DefaultDBHost
	}
	user = strings.TrimSpace(s.Options.DBUser)
	if user == "" {
		user = "shopware"
	}
	pass = s.Options.DBPass
	if pass == "" {
		pass = "shopware"
	}
	name = strings.TrimSpace(s.Options.DBName)
	if name == "" {
		name = "shopware"
	}
	return host, user, pass, name
}

func (s *ShopwareBootstrap) resolveSiteURL() string {
	domain := strings.TrimSpace(s.Options.Domain)
	if domain == "" {
		return "http://localhost"
	}

	return "https://" + domain
}

func (s *ShopwareBootstrap) syncSalesChannelDomain(projectDir, siteURL string) error {
	if siteURL == "" {
		return nil
	}

	if s.Options.Runner != nil {
		// Run with redirection to suppress the "No sales channels found" error outputs
		// from appearing in the user's console output during a fresh bootstrap.
		_ = s.Options.Runner("php bin/console sales-channel:replace:url http://127.0.0.1:8000 " + conventions.ShellQuote(siteURL) + " >/dev/null 2>&1")
		_ = s.Options.Runner("php bin/console sales-channel:replace:url http://localhost " + conventions.ShellQuote(siteURL) + " >/dev/null 2>&1")
		return nil
	}

	// Local fallback (without runner)
	_ = s.runBinConsole(projectDir, "sales-channel:replace:url", "http://127.0.0.1:8000", siteURL)
	_ = s.runBinConsole(projectDir, "sales-channel:replace:url", "http://localhost", siteURL)

	return nil
}

func (s *ShopwareBootstrap) ensureShopwareDirsWritable(projectDir string) {
	for _, dir := range []string{"var", "public", "custom", "files"} {
		dirPath := filepath.Join(projectDir, dir)
		if _, err := os.Stat(dirPath); err == nil {
			_ = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if info.IsDir() {
					_ = os.Chmod(path, conventions.PublicDirPerm)
				} else {
					_ = os.Chmod(path, conventions.PublicFilePerm)
				}
				return nil
			})
		}
	}
}

func replaceOrAppendEnvAssignment(content, key, value string) string {
	lines := strings.Split(content, "\n")
	needle := key + "="
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			lines[i] = needle + value
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, needle+value)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}
