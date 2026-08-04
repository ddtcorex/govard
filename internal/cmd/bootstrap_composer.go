package cmd

import (
	"fmt"
	"govard/internal/conventions"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/engine"
	"govard/internal/frameworks"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func bootstrapComposerDumpAutoload(cmd *cobra.Command, cwd string) error {
	if !fileExists(filepath.Join(cwd, "composer.json")) {
		return nil
	}
	if err := runGovardSubcommand(cmd, govardComposerSubcommandArgs("dump-autoload", "-n")...); err != nil {
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

func FixComposerCompatibility(config engine.Config) error {
	return engine.FixComposerCompatibility(config)
}

func ensureBootstrapAuthJSON(config engine.Config, opts BootstrapRuntimeOptions) error {
	definition, _ := frameworks.Get(config.Framework)
	auth := definition.ComposerAuth
	cwd, _ := os.Getwd()
	authPath := filepath.Join(cwd, "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		return nil
	}

	globalAuthPath := filepath.Join(os.Getenv("HOME"), ".composer", "auth.json")
	if _, err := os.Stat(globalAuthPath); err == nil {
		useGlobal := opts.AssumeYes || shouldUseGlobalAuthByDefault()
		if !useGlobal {
			useGlobal, _ = pterm.DefaultInteractiveConfirm.
				WithDefaultValue(true).
				Show("Use your global Composer credentials (auth.json) for this project?")
		}

		if useGlobal {
			// Instead of copying, we just acknowledge it's there.
			// If we need to write project-specific logic later, we can,
			// but for now we rely on the mount handled in Render.
			pterm.Success.Printf("Using global auth.json from %s\n", globalAuthPath)
			return nil
		}
	}

	if opts.MageUsername != "" && opts.MagePassword != "" {
		return createAuthJSONFromCredentials(globalAuthPath, auth.Repository, opts.MageUsername, opts.MagePassword, cwd)
	}

	if auth.Repository != "" && !shouldUseGlobalAuthByDefault() && !opts.AssumeYes {
		pterm.Info.Printf("%s requires authentication for %s.\n", auth.DisplayName, auth.Repository)
		if auth.CredentialURL != "" {
			pterm.Info.Printf("You can find your keys at: %s\n", auth.CredentialURL)
		}

		username, _ := pterm.DefaultInteractiveTextInput.Show("Magento Public Key")
		password, _ := pterm.DefaultInteractiveTextInput.WithMask("*").Show("Magento Private Key")

		if username != "" && password != "" {
			return createAuthJSONFromCredentials(globalAuthPath, auth.Repository, username, password, cwd)
		}
	}

	pterm.Warning.Println("auth.json not found. Provide --mage-username/--mage-password or create auth.json before composer-related steps.")
	return nil
}

func createAuthJSONFromCredentials(path, repository, username, password, cwd string) error {
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return fmt.Errorf("framework does not declare a Composer authentication repository")
	}
	payload := fmt.Sprintf("{\n    \"http-basic\": {\n        %q: {\n            \"username\": %q,\n            \"password\": %q\n        }\n    }\n}\n", repository, username, password)
	if err := os.MkdirAll(filepath.Dir(path), conventions.SecretDirPerm); err != nil {
		return fmt.Errorf("failed to ensure directory for auth.json: %w", err)
	}
	if err := os.WriteFile(path, []byte(payload), conventions.SecretFilePerm); err != nil {
		return fmt.Errorf("failed writing auth.json to %s: %w", path, err)
	}
	pterm.Success.Printf("✅ Created auth.json in %s with provided credentials.\n", path)
	return nil
}

func shouldUseGlobalAuthByDefault() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return (stdinInfo.Mode() & os.ModeCharDevice) == 0
}

// BootstrapComposerDumpAutoloadForTest exposes bootstrapComposerDumpAutoload for tests in /tests.
func BootstrapComposerDumpAutoloadForTest(cmd *cobra.Command, cwd string) error {
	return bootstrapComposerDumpAutoload(cmd, cwd)
}

// RunBootstrapComposerPrepareForTest exposes runBootstrapComposerPrepare for tests in /tests.
func RunBootstrapComposerPrepareForTest(config engine.Config) error {
	return runBootstrapComposerPrepare(config)
}

// EnsureBootstrapAuthJSONForTest exposes ensureBootstrapAuthJSON for tests in /tests.
func EnsureBootstrapAuthJSONForTest(config engine.Config, mageUsername, magePassword string, assumeYes bool) error {
	return ensureBootstrapAuthJSON(config, BootstrapRuntimeOptions{
		MageUsername: strings.TrimSpace(mageUsername),
		MagePassword: strings.TrimSpace(magePassword),
		AssumeYes:    assumeYes,
	})
}
