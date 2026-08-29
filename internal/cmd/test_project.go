package cmd

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
	"os"
	"path/filepath"
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
	if hasConfigFlag(args) {
		cmdArgs := []string{"-d", "memory_limit=-1", binaryPath}
		cmdArgs = append(cmdArgs, args...)
		err := RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
		if err != nil {
			if hinted := hintMissingTestBinary(err, "phpunit", binaryPath, "composer require --dev phpunit/phpunit (Laravel Pest users: composer require --dev pestphp/pest)"); hinted != err {
				return hinted
			}
			return fmt.Errorf("phpunit failed: %w\nHint: If you see \"Trait \\\"Magento\\Framework\\TestFramework\\Unit\\Helper\\MockCreationTrait\\\" not found\", run composer install and bin/magento setup:di:compile (or ensure dev/tests/unit/framework autoload is available)", err)
		}
		return nil
	}

	// No explicit -c: Magento projects keep their unit config only as a
	// .dist file on fresh checkout (dev/tests/unit/phpunit.xml.dist). A
	// plain `vendor/bin/phpunit` therefore shows help / "Available test
	// suite(s):" empty. Detect via host file probe and retry with fallback.
	projectRoot := findProjectRootForTest()
	fallbackChosen := ""
	if pathExists(filepath.Join(projectRoot, "dev/tests/unit/phpunit.xml")) {
		fallbackChosen = "dev/tests/unit/phpunit.xml"
	} else if pathExists(filepath.Join(projectRoot, "dev/tests/unit/phpunit.xml.dist")) {
		fallbackChosen = "dev/tests/unit/phpunit.xml.dist"
	}
	hasRootPHPUnit := pathExists(filepath.Join(projectRoot, "phpunit.xml")) || pathExists(filepath.Join(projectRoot, "phpunit.xml.dist"))
	if fallbackChosen != "" && !hasRootPHPUnit {
		pterm.Info.Printf("No -c provided and no phpunit.xml found at project root; using fallback -c %s\n", fallbackChosen)
		cmdArgs := []string{"-d", "memory_limit=-1", binaryPath, "-c", fallbackChosen}
		cmdArgs = append(cmdArgs, args...)
		err := RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
		if err != nil {
			if hinted := hintMissingTestBinary(err, "phpunit", binaryPath, "composer require --dev phpunit/phpunit (Laravel Pest users: composer require --dev pestphp/pest)"); hinted != err {
				return hinted
			}
			return fmt.Errorf("phpunit failed with fallback -c %s: %w\nHint: Trait \"Magento\\Framework\\TestFramework\\Unit\\Helper\\MockCreationTrait\" not found usually means missing generated code — run composer install and bin/magento setup:di:compile, or ensure dev/tests/unit/framework autoload is available. No suites? Verify %s exists and lists suites via: govard tool php -d memory_limit=-1 vendor/bin/phpunit -c %s --list-suites", fallbackChosen, err, fallbackChosen, fallbackChosen)
		}
		return nil
	}

	cmdArgs := []string{"-d", "memory_limit=-1", binaryPath}
	cmdArgs = append(cmdArgs, args...)
	err := RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
	if err != nil {
		if hinted := hintMissingTestBinary(err, "phpunit", binaryPath, "composer require --dev phpunit/phpunit (Laravel Pest users: composer require --dev pestphp/pest)"); hinted != err {
			return hinted
		}
		if fallbackChosen != "" {
			return fmt.Errorf("phpunit failed (no test suites / help shown): %w\nHint: Try govard test unit -- -c %s or ensure phpunit.xml exists at project root", err, fallbackChosen)
		}
		return fmt.Errorf("phpunit failed (no test suites found — try -c dev/tests/unit/phpunit.xml.dist or ensure phpunit.xml exists): %w", err)
	}
	return nil
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

	err := RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), "php", cmdArgs)
	if err != nil {
		if hinted := hintMissingTestBinary(err, "phpstan", binaryPath, "composer require --dev phpstan/phpstan"); hinted != err {
			return hinted
		}
		return err
	}
	return nil
}

// hintMissingTestBinary wraps a container-exec error with an actionable hint when the
// project's vendor binary is absent.
func hintMissingTestBinary(origErr error, name, binaryPath, installHint string) error {
	if origErr == nil {
		return nil
	}
	msg := origErr.Error()
	missingHints := []string{"No such file", "not found", "could not open input file", binaryPath, "phpstan", "phpunit", "pest"}
	missing := false
	for _, h := range missingHints {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(h)) {
			if wd, err := os.Getwd(); err == nil {
				if _, statErr := os.Stat(filepath.Join(wd, binaryPath)); os.IsNotExist(statErr) {
					missing = true
					break
				}
			}
			if strings.Contains(msg, binaryPath) {
				missing = true
				break
			}
		}
	}
	if !missing {
		return origErr
	}
	return fmt.Errorf("%w\n\nHint: %s is not installed in this project (missing %s). Install it with:\n  %s", origErr, name, binaryPath, installHint)
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
	// For integration, the framework command embeds "-c dev/tests/integration/phpunit.xml".
	// Fresh Magento only has phpunit.xml.dist and install-config-mysql.php.dist.
	// Probe host files and rewrite the config path to the available fallback, and
	// emit hints for missing install-config-mysql.php.
	if suite == "integration" {
		command.Args = resolveIntegrationArgs(command.Args, args)
		if hint := integrationConfigHint(findProjectRootForTest()); hint != "" {
			pterm.Warning.Println(hint)
		}
		commandArgs := append(append([]string(nil), command.Args...), args...)
		err := RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), command.Binary, commandArgs)
		if err != nil {
			return fmt.Errorf("integration tests failed: %w\nHint: %s", err, integrationFailureHint(findProjectRootForTest()))
		}
		return nil
	}
	commandArgs := append(append([]string(nil), command.Args...), args...)
	err := RunInContainer(config.ProjectName+conventions.PHPSuffix, ResolveProjectExecUser(config, conventions.UserWWWData), command.Binary, commandArgs)
	if err != nil {
		return fmt.Errorf("%s tests failed: %w", suite, err)
	}
	return nil
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

// hasConfigFlag reports whether args already specify a phpunit config.
func hasConfigFlag(args []string) bool {
	for _, a := range args {
		if a == "-c" || a == "--configuration" || strings.HasPrefix(a, "-c=") || strings.HasPrefix(a, "--configuration=") {
			return true
		}
	}
	return false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findProjectRootForTest() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if pathExists(filepath.Join(dir, ".govard.yml")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return wd
		}
	}
}

func resolveIntegrationArgs(baseArgs []string, userArgs []string) []string {
	if hasConfigFlag(userArgs) {
		return baseArgs
	}
	for i := 0; i < len(baseArgs)-1; i++ {
		if baseArgs[i] == "-c" {
			original := baseArgs[i+1]
			projectRoot := findProjectRootForTest()
			if !pathExists(filepath.Join(projectRoot, original)) && pathExists(filepath.Join(projectRoot, original+".dist")) {
				pterm.Info.Printf("Integration config %s not found; using fallback %s.dist\n", original, original)
				baseArgs[i+1] = original + ".dist"
			} else if !pathExists(filepath.Join(projectRoot, original)) {
				if pathExists(filepath.Join(projectRoot, "dev/tests/integration/phpunit.xml.dist")) {
					pterm.Info.Printf("Integration config %s not found; using fallback dev/tests/integration/phpunit.xml.dist\n", original)
					baseArgs[i+1] = "dev/tests/integration/phpunit.xml.dist"
				}
			}
			break
		}
	}
	return baseArgs
}

func integrationConfigHint(projectRoot string) string {
	php := filepath.Join(projectRoot, "dev/tests/integration/etc/install-config-mysql.php")
	dist := php + ".dist"
	if !pathExists(php) && pathExists(dist) {
		return fmt.Sprintf("Missing %s — copy %s to %s and create database magento_integration_tests (see dev/tests/integration/etc/install-config-mysql.php.dist)", php, dist, php)
	}
	if !pathExists(php) && !pathExists(dist) {
		return ""
	}
	return ""
}

func integrationFailureHint(projectRoot string) string {
	hint := "If install-config-mysql.php is missing, copy dev/tests/integration/etc/install-config-mysql.php.dist to install-config-mysql.php and create database magento_integration_tests. For phpunit config, use -c dev/tests/integration/phpunit.xml or .dist"
	if h := integrationConfigHint(projectRoot); h != "" {
		return h + " | " + hint
	}
	return hint
}
