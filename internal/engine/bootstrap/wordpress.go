package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pterm/pterm"
)

type WordPressBootstrap struct {
	Options Options
}

func NewWordPressBootstrap(opts Options) *WordPressBootstrap {
	return &WordPressBootstrap{Options: opts}
}

func (w *WordPressBootstrap) Name() string {
	return "wordpress"
}

func (w *WordPressBootstrap) SupportsFreshInstall() bool {
	return true
}

func (w *WordPressBootstrap) SupportsClone() bool {
	return true
}

func (w *WordPressBootstrap) FreshCommands() []string {
	return []string{
		"wp core download",
	}
}

func (w *WordPressBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh WordPress project...")

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

	cmd := exec.Command("wp", "core", "download", "--path=.", "--allow-root")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download WordPress: %w", err)
	}

	pterm.Success.Println("WordPress downloaded successfully")
	return nil
}

func (w *WordPressBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running WordPress installation steps...")

	configPath := filepath.Join(projectDir, "wp-config.php")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cmd := exec.Command("wp", "config", "create",
			"--dbname=wordpress",
			"--dbuser=wordpress",
			"--dbpass=wordpress",
			"--dbhost=db",
			"--dbprefix=wp_",
			"--allow-root",
		)
		cmd.Dir = projectDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create wp-config.php: %w", err)
		}
		pterm.Success.Println("Created wp-config.php")
	}

	cmd := exec.Command("wp", "core", "install",
		"--url=http://localhost",
		"--title=WordPress Site",
		"--admin_user=admin",
		"--admin_password=admin",
		"--admin_email=admin@local.test",
		"--allow-root",
	)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		pterm.Warning.Printf("WordPress install warning: %v\n", err)
	}

	pterm.Success.Println("WordPress installation completed")
	return nil
}

func (w *WordPressBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring WordPress environment...")

	configPath := filepath.Join(projectDir, "wp-config.php")
	if _, err := os.Stat(configPath); err == nil {
		if data, err := os.ReadFile(configPath); err == nil {
			content := string(data)
			content = os.Expand(content, func(key string) string {
				switch key {
				case "DB_HOST":
					return "db"
				case "DB_NAME":
					return "wordpress"
				case "DB_USER":
					return "wordpress"
				case "DB_PASSWORD":
					return "wordpress"
				default:
					return os.Getenv(key)
				}
			})
			os.WriteFile(configPath, []byte(content), 0644)
		}
	}

	pterm.Success.Println("WordPress configured successfully")
	return nil
}

func (w *WordPressBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned WordPress project...")

	configPath := filepath.Join(projectDir, "wp-config.php")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		examplePath := filepath.Join(projectDir, "wp-config-sample.php")
		if _, err := os.Stat(examplePath); err == nil {
			data, _ := os.ReadFile(examplePath)
			content := string(data)
			content = os.Expand(content, func(key string) string {
				switch key {
				case "DB_HOST":
					return "db"
				case "DB_NAME":
					return "wordpress"
				case "DB_USER":
					return "wordpress"
				case "DB_PASSWORD":
					return "wordpress"
				default:
					return os.Getenv(key)
				}
			})
			os.WriteFile(configPath, []byte(content), 0644)
		}
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}
