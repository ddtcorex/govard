package bootstrap

import (
	"fmt"
	"govard/internal/conventions"
	"os"
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

	createInStage := func(stageDir string) error {
		return runComposerProjectCommand(projectDir, nil, "create-project", "--prefer-dist", "cakephp/app", stageDir, "--no-interaction")
	}
	runnerCommand := "composer create-project --prefer-dist cakephp/app \"$GOVARD_STAGE_DIR\" --no-interaction"
	if err := runStagedCreateProject(projectDir, c.Options.Runner, createInStage, runnerCommand, conventions.DefaultWorkDir); err != nil {
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
		_ = os.MkdirAll(filepath.Dir(appConfigPath), conventions.DefaultDirPerm)
		if err := os.WriteFile(appConfigPath, []byte(content), conventions.DefaultFilePerm); err != nil {
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
				_ = os.WriteFile(appConfigPath, []byte(updated), conventions.DefaultFilePerm)
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
			_ = os.WriteFile(appConfigPath, []byte(content), conventions.DefaultFilePerm)
		}
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (c *CakePHPBootstrap) runComposerCommand(projectDir string, args ...string) error {
	return runComposerProjectCommand(projectDir, c.Options.Runner, args...)
}
