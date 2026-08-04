package cmd

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
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
	if command, ok := frameworkTestCommand(config.Framework, "default"); ok {
		return runInPHPContainer(config, command.Binary, command.Args)
	}
	return runPHPUnit(config, nil)
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
	} else if def, ok := frameworks.Get(config.Framework); ok && len(def.PHPStanPaths) > 0 {
		cmdArgs = append(cmdArgs, def.PHPStanPaths...)
	} else {
		cmdArgs = append(cmdArgs, "app", "src")
	}

	return RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
}

func runMFTF(config engine.Config, args []string) error {
	return runFrameworkTestSuite(config, "mftf", args)
}

func runIntegrationTests(config engine.Config, args []string) error {
	return runFrameworkTestSuite(config, "integration", args)
}

func runFrameworkTestSuite(config engine.Config, suite string, args []string) error {
	command, ok := frameworkTestCommand(config.Framework, suite)
	if !ok {
		return fmt.Errorf("%s tests not configured for framework: %s", suite, config.Framework)
	}
	if command.Label != "" {
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Running " + command.Label + " ")
		fmt.Println()
	}
	commandArgs := append(append([]string(nil), command.Args...), args...)
	return RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), command.Binary, commandArgs)
}

func frameworkTestCommand(framework string, suite string) (types.TestCommand, bool) {
	definition, ok := frameworks.Get(framework)
	if !ok {
		return types.TestCommand{}, false
	}
	if suite == "default" {
		command := definition.DefaultTestCommand
		return command, command.Binary != ""
	}
	command, ok := definition.TestSuiteCommands[suite]
	return command, ok && command.Binary != ""
}

// FrameworkTestCommandForTest resolves one command without starting a
// container, so tests cover registry ownership of suite availability.
func FrameworkTestCommandForTest(framework string, suite string) (types.TestCommand, bool) {
	return frameworkTestCommand(framework, suite)
}

func runInPHPContainer(config engine.Config, binary string, args []string) error {
	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
	if err := ensureContainerReadyForExec(containerName, "PHP"); err != nil {
		return err
	}

	user := ResolveProjectExecUser(config, conventions.UserWWWData)

	return RunInContainer(containerName, user, binary, args)
}
