package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

type CakePHPBootstrap struct {
	Options Options
}

func NewCakePHPBootstrap(opts Options) *CakePHPBootstrap {
	return &CakePHPBootstrap{Options: opts}
}

func (c *CakePHPBootstrap) Name() string {
	return "cakephp"
}

func (c *CakePHPBootstrap) SupportsFreshInstall() bool {
	return true
}

func (c *CakePHPBootstrap) SupportsClone() bool {
	return true
}

func (c *CakePHPBootstrap) FreshCommands() []string {
	return []string{
		"composer create-project --prefer-dist cakephp/app .",
	}
}

func (c *CakePHPBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh CakePHP project...")

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

	cmd := exec.Command("composer", "create-project", "--prefer-dist", "cakephp/app", ".", "--no-interaction")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create CakePHP project: %w", err)
	}

	pterm.Success.Println("CakePHP project created successfully")
	return nil
}

func (c *CakePHPBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running CakePHP installation steps...")

	appConfigPath := filepath.Join(projectDir, "config", "app_local.php")
	if _, err := os.Stat(appConfigPath); os.IsNotExist(err) {
		content := `<?php
return [
    'Datasources' => [
        'default' => [
            'className' => \Cake\Database\Connection::class,
            'driver' => \Cake\Database\Driver\Mysql::class,
            'host' => 'db',
            'username' => 'cakephp',
            'password' => 'cakephp',
            'database' => 'cakephp',
        ],
    ],
];
`
		os.MkdirAll(filepath.Dir(appConfigPath), 0755)
		if err := os.WriteFile(appConfigPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create app_local.php: %w", err)
		}
		pterm.Success.Println("Created app_local.php")
	}

	if err := c.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Composer install warning: %v\n", err)
	}

	pterm.Success.Println("CakePHP installation completed")
	return nil
}

func (c *CakePHPBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring CakePHP environment...")

	appConfigPath := filepath.Join(projectDir, "config", "app_local.php")
	if _, err := os.Stat(appConfigPath); err == nil {
		content, err := os.ReadFile(appConfigPath)
		if err == nil {
			updated := string(content)
			if !strings.Contains(updated, "'host' => 'db'") {
				updated = strings.ReplaceAll(updated, "'host' => 'localhost'", "'host' => 'db'")
				updated = strings.ReplaceAll(updated, "'host' => '127.0.0.1'", "'host' => 'db'")
				os.WriteFile(appConfigPath, []byte(updated), 0644)
			}
		}
	}

	pterm.Success.Println("CakePHP configured successfully")
	return nil
}

func (c *CakePHPBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned CakePHP project...")

	if err := c.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	appConfigPath := filepath.Join(projectDir, "config", "app_local.php")
	if _, err := os.Stat(appConfigPath); os.IsNotExist(err) {
		appDefaultPath := filepath.Join(projectDir, "config", "app.php")
		if _, err := os.Stat(appDefaultPath); err == nil {
			data, _ := os.ReadFile(appDefaultPath)
			content := string(data)
			content = strings.ReplaceAll(content, "'host' => 'localhost'", "'host' => 'db'")
			content = strings.ReplaceAll(content, "'host' => '127.0.0.1'", "'host' => 'db'")
			os.WriteFile(appConfigPath, []byte(content), 0644)
		}
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (c *CakePHPBootstrap) runComposerCommand(projectDir string, args ...string) error {
	cmd := exec.Command("composer", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
