package bootstrap

import (
	"fmt"
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

	laravelVersion := l.getLaravelVersion(l.Options.Version)

	cmd := exec.Command("composer", "create-project", laravelVersion, ".", "--no-interaction")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
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
			os.WriteFile(envPath, data, 0600)
		}
	}

	if data, err := os.ReadFile(envPath); err == nil {
		content := string(data)
		content = strings.ReplaceAll(content, "APP_ENV=production", "APP_ENV=local")
		content = strings.ReplaceAll(content, "APP_DEBUG=false", "APP_DEBUG=true")
		content = strings.ReplaceAll(content, "DB_HOST=127.0.0.1", "DB_HOST=db")
		content = strings.ReplaceAll(content, "DB_DATABASE=laravel", "DB_DATABASE=laravel")
		content = strings.ReplaceAll(content, "DB_USERNAME=root", "DB_USERNAME=laravel")
		content = strings.ReplaceAll(content, "DB_PASSWORD=", "DB_PASSWORD=laravel")
		os.WriteFile(envPath, []byte(content), 0600)
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

	pterm.Success.Println("Laravel installation completed")
	return nil
}

func (l *LaravelBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring Laravel environment...")

	envPath := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		content, err := os.ReadFile(envPath)
		if err == nil {
			updated := string(content)
			if !strings.Contains(updated, "DB_HOST=db") {
				updated = strings.ReplaceAll(updated, "DB_HOST=127.0.0.1", "DB_HOST=db")
				updated = strings.ReplaceAll(updated, "DB_HOST=localhost", "DB_HOST=db")
				os.WriteFile(envPath, []byte(updated), 0600)
			}
		}
	}

	l.runArtisanCommand(projectDir, "config:clear")
	l.runArtisanCommand(projectDir, "cache:clear")

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
			os.WriteFile(envPath, []byte(content), 0600)
		}
	}

	if _, err := os.Stat(envPath); err == nil {
		if data, err := os.ReadFile(envPath); err == nil {
			content := string(data)
			content = strings.ReplaceAll(content, "DB_HOST=127.0.0.1", "DB_HOST=db")
			content = strings.ReplaceAll(content, "DB_HOST=localhost", "DB_HOST=db")
			os.WriteFile(envPath, []byte(content), 0600)
		}
	}

	if err := l.runArtisanCommand(projectDir, "key:generate"); err != nil {
		pterm.Warning.Printf("Key generation warning: %v\n", err)
	}

	l.runArtisanCommand(projectDir, "migrate", "--force")

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
	cmd := exec.Command("composer", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (l *LaravelBootstrap) runArtisanCommand(projectDir string, args ...string) error {
	artisanPath := filepath.Join(projectDir, "artisan")
	if _, err := os.Stat(artisanPath); os.IsNotExist(err) {
		pterm.Warning.Println("Artisan not found, skipping artisan commands")
		return nil
	}

	args = append([]string{artisanPath}, args...)
	cmd := exec.Command("php", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
