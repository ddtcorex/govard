package cmd

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test [phpunit|phpstan|mftf|unit|integration]",
	Short: "Run project tests (PHPUnit, PHPStan, etc.)",
	Long: `Run various test suites directly inside the project containers.
Supports PHPUnit, PHPStan, MFTF, and more depending on the framework.
If no subcommand is provided, it runs the default unit test suite.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := loadConfig()
		if len(args) == 0 {
			return runDefaultTests(config)
		}

		subcommand := strings.ToLower(args[0])
		remainingArgs := args[1:]

		switch subcommand {
		case "phpunit", "unit":
			return runPHPUnit(config, remainingArgs)
		case "phpstan", "static":
			return runPHPStan(config, remainingArgs)
		case "mftf":
			return runMFTF(config, remainingArgs)
		case "integration":
			return runIntegrationTests(config, remainingArgs)
		default:
			return fmt.Errorf("unknown test suite: %s", subcommand)
		}
	},
}

func init() {
	// Root command registration is in root.go
}

func runDefaultTests(config engine.Config) error {
	if engine.IsMagento2Family(config.Framework) {
		return runPHPUnit(config, nil)
	}
	switch config.Framework {
	case "laravel":
		return runInPHPContainer(config, "php", []string{"artisan", "test"})
	default:
		return runPHPUnit(config, nil)
	}
}

func runPHPUnit(config engine.Config, args []string) error {
	fmt.Println()
	pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Running PHPUnit Tests ")
	fmt.Println()
	binaryPath := "vendor/bin/phpunit"
	// We check for binaryPath but we use it in RunInContainer

	cmdArgs := []string{"-d", "memory_limit=-1", binaryPath}
	cmdArgs = append(cmdArgs, args...)

	return RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
}

func runPHPStan(config engine.Config, args []string) error {
	fmt.Println()
	pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Running PHPStan Static Analysis ")
	fmt.Println()
	binaryPath := "vendor/bin/phpstan"
	cmdArgs := []string{binaryPath, "analyze"}
	if len(args) > 0 {
		cmdArgs = append(cmdArgs, args...)
	} else {
		// Default paths for Magento 2 or others
		if engine.IsMagento2Family(config.Framework) {
			cmdArgs = append(cmdArgs, "app/code", "app/design")
		} else {
			cmdArgs = append(cmdArgs, "app", "src")
		}
	}

	return RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
}

func runMFTF(config engine.Config, args []string) error {
	if !engine.IsMagento2Family(config.Framework) {
		return fmt.Errorf("MFTF is only supported for Magento 2 projects")
	}
	fmt.Println()
	pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Running MFTF Tests ")
	fmt.Println()
	binaryPath := "vendor/bin/mftf"
	cmdArgs := []string{binaryPath, "run:group"}
	cmdArgs = append(cmdArgs, args...)

	return RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
}

func runIntegrationTests(config engine.Config, args []string) error {
	if engine.IsMagento2Family(config.Framework) {
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Running Magento 2 Integration Tests ")
		fmt.Println()
		binaryPath := "vendor/bin/phpunit"
		cmdArgs := []string{"-c", "dev/tests/integration/phpunit.xml", binaryPath}
		cmdArgs = append(cmdArgs, args...)
		return RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
	}
	return fmt.Errorf("integration tests not configured for framework: %s", config.Framework)
}

func runInPHPContainer(config engine.Config, binary string, args []string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	if err := ensureContainerReadyForExec(containerName, "PHP"); err != nil {
		return err
	}

	user := ResolveProjectExecUser(config, conventions.UserWWWData)

	return RunInContainer(containerName, user, binary, args)
}
