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

type LaravelBootstrap struct {
	Options Options
}

func NewLaravelBootstrap(opts Options) *LaravelBootstrap {
	return &LaravelBootstrap{Options: opts}
}

func (l *LaravelBootstrap) Name() string {
	return "laravel"
}

func (l *LaravelBootstrap) SupportsFreshInstall() bool {
	return true
}

func (l *LaravelBootstrap) SupportsClone() bool {
	return true
}

func (l *LaravelBootstrap) FreshCommands() []string {
	version := l.Options.Version
	if version == "" {
		version = "11"
	}

	var laravelVersion string
	majorVersion := strings.Split(version, ".")[0]
	switch majorVersion {
	case "12":
		laravelVersion = "laravel/laravel"
	case "11":
		laravelVersion = "laravel/laravel"
	case "10":
		laravelVersion = "laravel/laravel:^10.0"
	case "9":
		laravelVersion = "laravel/laravel:^9.0"
	default:
		laravelVersion = "laravel/laravel"
	}

	return []string{
		"composer create-project " + laravelVersion + " .",
	}
}

func (l *LaravelBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh Laravel project...")

	laravelVersion := l.getLaravelVersion(l.Options.Version)

	createInStage := func(stageDir string) error {
		return runComposerProjectCommand(projectDir, nil, "create-project", laravelVersion, stageDir, "--no-interaction")
	}
	runnerCommand := "composer create-project " + laravelVersion + " \"$GOVARD_STAGE_DIR\" --no-interaction"
	if err := runStagedCreateProject(projectDir, l.Options.Runner, createInStage, runnerCommand, conventions.DefaultWorkDir); err != nil {
		return fmt.Errorf("failed to create Laravel project: %w", err)
	}

	pterm.Success.Println("Laravel project created successfully")
	return nil
}

func (l *LaravelBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running Laravel installation steps...")

	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		examplePath := filepath.Join(projectDir, ".env.example")
		if _, err := os.Stat(examplePath); err == nil {
			data, _ := os.ReadFile(examplePath)
			_ = os.WriteFile(envPath, data, conventions.DefaultFilePerm)
		}
	}

	if data, err := os.ReadFile(envPath); err == nil {
		dbHost := l.Options.DBHost
		if dbHost == "" {
			dbHost = conventions.DefaultDBHost
		}
		dbUser := l.Options.DBUser
		if dbUser == "" {
			dbUser = "laravel"
		}
		dbPass := l.Options.DBPass
		if dbPass == "" {
			dbPass = "laravel"
		}
		dbName := l.Options.DBName
		if dbName == "" {
			dbName = "laravel"
		}

		content := string(data)
		content = strings.ReplaceAll(content, "APP_ENV=production", "APP_ENV=local")
		content = strings.ReplaceAll(content, "APP_DEBUG=false", "APP_DEBUG=true")
		content = strings.ReplaceAll(content, "DB_HOST=127.0.0.1", "DB_HOST="+dbHost)
		content = strings.ReplaceAll(content, "DB_DATABASE=laravel", "DB_DATABASE="+dbName)
		content = strings.ReplaceAll(content, "DB_USERNAME=root", "DB_USERNAME="+dbUser)
		content = strings.ReplaceAll(content, "DB_PASSWORD=", "DB_PASSWORD="+dbPass)
		_ = os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm)
	}

	if err := l.runArtisanCommand(projectDir, "key:generate"); err != nil {
		pterm.Warning.Printf("Key generation warning: %v\n", err)
	}

	if err := l.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Composer install warning: %v\n", err)
	}

	pterm.Info.Println("Running migrations...")
	if err := l.runArtisanCommand(projectDir, "migrate", "--force"); err != nil {
		pterm.Warning.Printf("Migrations warning: %v\n", err)
	}

	l.ensureLaravelDirsWritable(projectDir)

	pterm.Success.Println("Laravel installation completed")
	return nil
}

func (l *LaravelBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring Laravel environment...")

	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		content, err := os.ReadFile(envPath)
		if err == nil {
			dbHost := l.Options.DBHost
			if dbHost == "" {
				dbHost = conventions.DefaultDBHost
			}
			updated := string(content)
			if !strings.Contains(updated, "DB_HOST="+dbHost) {
				updated = strings.ReplaceAll(updated, "DB_HOST=127.0.0.1", "DB_HOST="+dbHost)
				updated = strings.ReplaceAll(updated, "DB_HOST=localhost", "DB_HOST="+dbHost)
				_ = os.WriteFile(envPath, []byte(updated), conventions.DefaultFilePerm)
			}
		}
	}

	_ = l.runArtisanCommand(projectDir, "config:clear")
	_ = l.runArtisanCommand(projectDir, "cache:clear")

	pterm.Success.Println("Laravel configured successfully")
	return nil
}

func (l *LaravelBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned Laravel project...")

	if err := l.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		examplePath := filepath.Join(projectDir, ".env.example")
		if _, err := os.Stat(examplePath); err == nil {
			data, _ := os.ReadFile(examplePath)
			content := string(data)
			content = strings.ReplaceAll(content, "APP_ENV=production", "APP_ENV=local")
			content = strings.ReplaceAll(content, "APP_DEBUG=false", "APP_DEBUG=true")
			_ = os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm)
		}
	}

	if _, err := os.Stat(envPath); err == nil {
		if data, err := os.ReadFile(envPath); err == nil {
			dbHost := l.Options.DBHost
			if dbHost == "" {
				dbHost = conventions.DefaultDBHost
			}
			content := string(data)
			content = strings.ReplaceAll(content, "DB_HOST=127.0.0.1", "DB_HOST="+dbHost)
			content = strings.ReplaceAll(content, "DB_HOST=localhost", "DB_HOST="+dbHost)
			_ = os.WriteFile(envPath, []byte(content), conventions.DefaultFilePerm)
		}
	}

	if err := l.runArtisanCommand(projectDir, "key:generate"); err != nil {
		pterm.Warning.Printf("Key generation warning: %v\n", err)
	}

	_ = l.runArtisanCommand(projectDir, "migrate", "--force")

	l.ensureLaravelDirsWritable(projectDir)

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (l *LaravelBootstrap) getLaravelVersion(version string) string {
	if version == "" {
		return "laravel/laravel"
	}

	parts := strings.Split(version, ".")
	major := parts[0]

	switch major {
	case "12":
		return "laravel/laravel"
	case "11":
		return "laravel/laravel"
	case "10":
		return "laravel/laravel:^10.0"
	case "9":
		return "laravel/laravel:^9.0"
	default:
		return "laravel/laravel"
	}
}

func (l *LaravelBootstrap) runComposerCommand(projectDir string, args ...string) error {
	return runComposerProjectCommand(projectDir, l.Options.Runner, args...)
}

func (l *LaravelBootstrap) runArtisanCommand(projectDir string, args ...string) error {
	artisanPath := filepath.Join(projectDir, "artisan")
	if _, err := os.Stat(artisanPath); os.IsNotExist(err) {
		pterm.Warning.Println("Artisan not found, skipping artisan commands")
		return nil
	}

	relArtisanPath, err := filepath.Rel(projectDir, artisanPath)
	if err != nil {
		relArtisanPath = artisanPath
	}

	command := "php " + relArtisanPath + " " + strings.Join(args, " ")
	if l.Options.Runner != nil {
		return l.Options.Runner(command)
	}

	cmd := exec.Command("php", append([]string{artisanPath}, args...)...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (l *LaravelBootstrap) ensureLaravelDirsWritable(projectDir string) {
	for _, dir := range []string{"storage", "bootstrap/cache", "database"} {
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
