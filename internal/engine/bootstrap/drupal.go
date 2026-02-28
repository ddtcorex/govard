package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

type DrupalBootstrap struct {
	Options Options
}

func NewDrupalBootstrap(opts Options) *DrupalBootstrap {
	return &DrupalBootstrap{Options: opts}
}

func (d *DrupalBootstrap) Name() string {
	return "drupal"
}

func (d *DrupalBootstrap) SupportsFreshInstall() bool {
	return true
}

func (d *DrupalBootstrap) SupportsClone() bool {
	return true
}

func (d *DrupalBootstrap) FreshCommands() []string {
	version := d.Options.Version
	if version == "" {
		version = "10"
	}

	var template string
	majorVersion := strings.Split(version, ".")[0]
	switch majorVersion {
	case "11":
		template = "drupal/recommended-project"
	case "10":
		template = "drupal/recommended-project:^10"
	case "9":
		template = "drupal/recommended-project:^9"
	default:
		template = "drupal/recommended-project"
	}

	return []string{
		"composer create-project " + template + " .",
	}
}

func (d *DrupalBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh Drupal project...")

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

	template := d.getDrupalTemplate(d.Options.Version)

	cmd := exec.Command("composer", "create-project", template, ".", "--no-interaction")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Drupal project: %w", err)
	}

	pterm.Success.Println("Drupal project created successfully")
	return nil
}

func (d *DrupalBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running Drupal installation steps...")

	if err := d.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Composer install warning: %v\n", err)
	}

	sitePath := filepath.Join(projectDir, "web", "sites", "default")
	_ = os.MkdirAll(sitePath, 0755)

	filesPath := filepath.Join(sitePath, "files")
	_ = os.MkdirAll(filesPath, 0777)

	settingsPath := filepath.Join(sitePath, "settings.php")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		defaultSettings := filepath.Join(sitePath, "default.settings.php")
		if _, err := os.Stat(defaultSettings); err == nil {
			data, _ := os.ReadFile(defaultSettings)
			_ = os.WriteFile(settingsPath, data, 0644)
		}
	}

	pterm.Info.Println("Installing Drupal site...")
	drupalCmd := []string{
		"site:install",
		"standard",
		"--db-type=mysql",
		"--db-host=db",
		"--db-name=drupal",
		"--db-user=drupal",
		"--db-pass=drupal",
		"--site-name=Drupal Site",
		"--account-name=admin",
		"--account-pass=admin",
		"--account-mail=admin@local.test",
		"--no-interaction",
	}

	if err := d.runDrushCommand(projectDir, drupalCmd...); err != nil {
		pterm.Warning.Printf("Site install warning: %v\n", err)
	}

	pterm.Success.Println("Drupal installation completed")
	return nil
}

func (d *DrupalBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring Drupal environment...")

	sitePath := filepath.Join(projectDir, "web", "sites", "default")
	settingsPath := filepath.Join(sitePath, "settings.php")

	if _, err := os.Stat(settingsPath); err == nil {
		content, err := os.ReadFile(settingsPath)
		if err == nil {
			updated := string(content)
			if !strings.Contains(updated, "'host' => 'db'") {
				updated = strings.ReplaceAll(updated, "'host' => 'localhost'", "'host' => 'db'")
				updated = strings.ReplaceAll(updated, "'host' => '127.0.0.1'", "'host' => 'db'")
				_ = os.WriteFile(settingsPath, []byte(updated), 0644)
			}
		}
	}

	_ = d.runDrushCommand(projectDir, "cache:rebuild")

	pterm.Success.Println("Drupal configured successfully")
	return nil
}

func (d *DrupalBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned Drupal project...")

	if err := d.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	sitePath := filepath.Join(projectDir, "web", "sites", "default")
	filesPath := filepath.Join(sitePath, "files")
	_ = os.MkdirAll(filesPath, 0777)

	settingsPath := filepath.Join(sitePath, "settings.php")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		defaultSettings := filepath.Join(sitePath, "default.settings.php")
		if _, err := os.Stat(defaultSettings); err == nil {
			data, _ := os.ReadFile(defaultSettings)
			_ = os.WriteFile(settingsPath, data, 0644)
		}
	}

	if _, err := os.Stat(settingsPath); err == nil {
		if data, err := os.ReadFile(settingsPath); err == nil {
			content := string(data)
			content = strings.ReplaceAll(content, "'host' => 'localhost'", "'host' => 'db'")
			content = strings.ReplaceAll(content, "'host' => '127.0.0.1'", "'host' => 'db'")
			_ = os.WriteFile(settingsPath, []byte(content), 0644)
		}
	}

	_ = d.runDrushCommand(projectDir, "cache:rebuild")

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (d *DrupalBootstrap) getDrupalTemplate(version string) string {
	if version == "" {
		return "drupal/recommended-project"
	}

	parts := strings.Split(version, ".")
	major := parts[0]

	switch major {
	case "11":
		return "drupal/recommended-project"
	case "10":
		return "drupal/recommended-project:^10"
	case "9":
		return "drupal/recommended-project:^9"
	default:
		return "drupal/recommended-project"
	}
}

func (d *DrupalBootstrap) runComposerCommand(projectDir string, args ...string) error {
	cmd := exec.Command("composer", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (d *DrupalBootstrap) runDrushCommand(projectDir string, args ...string) error {
	drushPath := filepath.Join(projectDir, "vendor", "bin", "drush")
	if _, err := os.Stat(drushPath); os.IsNotExist(err) {
		drushPath = filepath.Join(projectDir, "web", "vendor", "bin", "drush")
		if _, err := os.Stat(drushPath); os.IsNotExist(err) {
			pterm.Warning.Println("Drush not found, skipping drush commands")
			return nil
		}
	}

	args = append([]string{drushPath}, args...)
	cmd := exec.Command("php", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
