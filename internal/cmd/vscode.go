package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// vscodeCmd groups PHP tooling entry points meant to be wired into editor
// settings (VSCode's php.validate.executablePath, phpstan.binCommand,
// php-cs-fixer.executablePath, a PHPUnit test-explorer's binary path, etc).
//
// Editors invoke these with whatever working directory they happen to use
// for the setting (often the active file's directory, not the workspace
// root), so unlike `govard tool`, these subcommands resolve the project by
// walking up from the current directory to find the nearest .govard.yml
// instead of requiring an exact match. That walk-up is intentionally kept
// local to this file rather than folded into the shared config loader, so it
// can't change directory-resolution behavior for any other command.
var vscodeCmd = &cobra.Command{
	Use:   "vscode [command]",
	Short: "Run PHP tooling inside the project container for editor integrations",
	Long: `Run PHP, Composer, and common PHP tool binaries inside the project's container,
resolving the project by walking up from the current directory to find the
nearest .govard.yml. Meant to be wired into editor/IDE settings (VSCode's
php.validate.executablePath, phpstan.binCommand, php-cs-fixer.executablePath,
a PHPUnit test-explorer's binary path, etc.) so those tools run against the
container instead of requiring PHP, Composer, or vendor binaries on the host.`,
	Example: `  # Settings that accept a command array (no wrapper script needed):
  "phpstan.binCommand": ["govard", "vscode", "phpstan"]

  # Settings that require a single executable path (VSCode spawns them
  # without a shell, so a multi-word string won't parse) still need a
  # one-line wrapper script that execs "govard vscode php", e.g. for:
  "php.validate.executablePath": "/path/to/govard-php-wrapper"
  "php-cs-fixer.executablePath": "/path/to/govard-php-cs-fixer-wrapper"`,
}

type vscodeToolCommand struct {
	Name        string
	Short       string
	Binary      string
	PrependArgs []string
}

var vscodeToolCommands = []vscodeToolCommand{
	{Name: "php", Short: "Run the PHP CLI", Binary: "php"},
	{Name: "composer", Short: "Run composer", Binary: "composer"},
	{Name: "phpstan", Short: "Run vendor/bin/phpstan", Binary: "php", PrependArgs: []string{"vendor/bin/phpstan"}},
	{Name: "phpcs", Short: "Run vendor/bin/phpcs", Binary: "php", PrependArgs: []string{"vendor/bin/phpcs"}},
	{Name: "php-cs-fixer", Short: "Run vendor/bin/php-cs-fixer", Binary: "php", PrependArgs: []string{"vendor/bin/php-cs-fixer"}},
	{Name: "phpunit", Short: "Run vendor/bin/phpunit", Binary: "php", PrependArgs: []string{"-d", "memory_limit=-1", "vendor/bin/phpunit"}},
}

func initVSCodeCommands() {
	for _, vc := range vscodeToolCommands {
		vc := vc
		cmd := &cobra.Command{
			Use:                fmt.Sprintf("%s [args]", vc.Name),
			Short:              vc.Short,
			DisableFlagParsing: true,
			RunE: func(c *cobra.Command, args []string) error {
				root, err := findProjectRootUpward()
				if err != nil {
					return err
				}
				if err := os.Chdir(root); err != nil {
					return fmt.Errorf("switch to project root %q: %w", root, err)
				}

				config := loadConfig()
				target := resolveToolExecution(config, vc.Binary, "")
				runErr := RunInContainerAt(target.ContainerName, target.User, target.Workdir, vc.Binary, append(vc.PrependArgs, args...))
				if runErr == nil {
					return nil
				}
				if code, ok := toolExitCode(runErr); ok {
					// The tool ran and exited non-zero on its own terms (e.g.
					// phpcs/phpstan exit 1 when they find issues, not because
					// anything failed to run). Exit with that code directly
					// instead of returning it to Cobra: Execute() would print
					// it via pterm, which writes to stdout and would corrupt
					// machine-readable output (e.g. --report=json) that the
					// tool already flushed there.
					os.Exit(code)
				}
				return runErr
			},
		}
		vscodeCmd.AddCommand(cmd)
	}
	vscodeCmd.AddCommand(vscodeSetupCmd)
	rootCmd.AddCommand(vscodeCmd)
}

// toolExitCode extracts the process exit code from err if it's an
// *exec.ExitError — i.e. the tool actually ran and exited non-zero on its own
// terms (phpcs/phpstan exit 1 when they find issues; that's normal output,
// not a failure to run). Returns ok=false for any other error (docker not
// found, container not running, etc), which should still be reported as a
// real failure.
func toolExitCode(err error) (int, bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), true
	}
	return 0, false
}

// ToolExitCodeForTest exposes toolExitCode to the tests package.
func ToolExitCodeForTest(err error) (int, bool) {
	return toolExitCode(err)
}

// findProjectRootUpward walks up from the current working directory looking
// for .govard.yml, returning the first directory that contains one.
func findProjectRootUpward() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("determine working directory: %w", err)
	}
	return findProjectRootFrom(start)
}

// findProjectRootFrom walks up from start looking for .govard.yml, returning
// the first directory that contains one.
func findProjectRootFrom(start string) (string, error) {
	dir := start
	for {
		if _, statErr := os.Stat(filepath.Join(dir, ".govard.yml")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .govard.yml found in %q or any parent directory", start)
		}
		dir = parent
	}
}

// FindProjectRootFromForTest exposes findProjectRootFrom to the tests package.
func FindProjectRootFromForTest(start string) (string, error) {
	return findProjectRootFrom(start)
}
