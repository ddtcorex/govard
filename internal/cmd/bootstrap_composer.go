package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/engine"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func bootstrapComposerDumpAutoload(cmd *cobra.Command, cwd string) error {
	if !fileExists(filepath.Join(cwd, "composer.json")) {
		return nil
	}
	if err := runGovardSubcommand(cmd, govardComposerSubcommandArgs("dump-autoload", "-o", "-n")...); err != nil {
		autoloadPath := filepath.Join(cwd, "vendor", "autoload.php")
		if !fileExists(autoloadPath) {
			return fmt.Errorf("composer autoload generation failed: %w", err)
		}
		pterm.Warning.Printf("composer dump-autoload skipped (%v).\n", err)
	}
	return nil
}

func runBootstrapComposerPrepare(config engine.Config) error {
	if err := runPHPContainerShellCommand(config, "rm -rf vendor"); err != nil {
		return fmt.Errorf("failed to clean vendor directory: %w", err)
	}
	return nil
}

func ensureBootstrapAuthJSON(config engine.Config, opts bootstrapRuntimeOptions) error {
	cwd, _ := os.Getwd()
	authPath := filepath.Join(cwd, "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		ensureAuthInGitignore(cwd)
		return nil
	}

	globalAuthPath := filepath.Join(os.Getenv("HOME"), ".composer", "auth.json")
	if _, err := os.Stat(globalAuthPath); err == nil {
		useGlobal := opts.AssumeYes || shouldUseGlobalAuthByDefault()
		if !useGlobal {
			useGlobal, _ = pterm.DefaultInteractiveConfirm.
				WithDefaultValue(true).
				Show(fmt.Sprintf("Found global auth.json at %s. Use it for this project?", globalAuthPath))
		}

		if useGlobal {
			data, readErr := os.ReadFile(globalAuthPath)
			if readErr != nil {
				return fmt.Errorf("failed reading global auth.json: %w", readErr)
			}
			if writeErr := os.WriteFile(authPath, data, 0600); writeErr != nil {
				return fmt.Errorf("failed writing project auth.json: %w", writeErr)
			}
			pterm.Success.Printf("Copied global auth.json from %s\n", globalAuthPath)
			ensureAuthInGitignore(cwd)
			return nil
		}
	}

	if opts.MageUsername != "" && opts.MagePassword != "" {
		return createAuthJSONFromCredentials(authPath, opts.MageUsername, opts.MagePassword, cwd)
	}

	if config.Framework == "magento2" && !shouldUseGlobalAuthByDefault() && !opts.AssumeYes {
		pterm.Info.Println("Magento 2 requires authentication for repo.magento.com.")
		pterm.Info.Println("You can find your keys at: https://marketplace.magento.com/customer/accessKeys/")

		username, _ := pterm.DefaultInteractiveTextInput.Show("Magento Public Key")
		password, _ := pterm.DefaultInteractiveTextInput.WithMask("*").Show("Magento Private Key")

		if username != "" && password != "" {
			return createAuthJSONFromCredentials(authPath, username, password, cwd)
		}
	}

	pterm.Warning.Println("auth.json not found. Provide --mage-username/--mage-password or create auth.json before composer-related steps.")
	return nil
}

func createAuthJSONFromCredentials(path, username, password, cwd string) error {
	payload := fmt.Sprintf("{\n    \"http-basic\": {\n        \"repo.magento.com\": {\n            \"username\": %q,\n            \"password\": %q\n        }\n    }\n}\n", username, password)
	if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
		return fmt.Errorf("failed writing auth.json: %w", err)
	}
	ensureAuthInGitignore(cwd)
	pterm.Success.Println("✅ Created auth.json with provided credentials.")
	return nil
}

func ensureAuthInGitignore(cwd string) {
	gitignorePath := filepath.Join(cwd, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		return
	}
	content := string(data)
	if strings.Contains(content, "auth.json") {
		return
	}
	lines := content
	if !strings.HasSuffix(lines, "\n") {
		lines += "\n"
	}
	lines += "/auth.json\n"
	_ = os.WriteFile(gitignorePath, []byte(lines), 0644)
}

func shouldUseGlobalAuthByDefault() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return (stdinInfo.Mode() & os.ModeCharDevice) == 0
}
