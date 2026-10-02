package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"govard/internal/deploy"
)

// sandboxComposerAuth finds the credentials a server-mode build on the sandbox
// needs. A project whose dependencies come from a private repository cannot
// install them on a target that has no credentials, and the sandbox, unlike a
// real remote, is the developer's own throwaway container: handing it the host's
// composer credentials is what lets `--build server` rehearse a build at all.
//
// It answers only for the sandbox, only when the target will run the dependency
// install (a server build), and only when COMPOSER_AUTH is not already set: an
// explicit value is what the executor reads and needs no help. The file lookup
// follows the order `bootstrap` uses, the project's auth.json then the user's.
// A file that is not valid JSON is skipped with a note that never echoes its
// content.
func sandboxComposerAuth(remoteIsSandbox, serverBuild bool, getenv func(string) string, readFile func(string) ([]byte, error), home, cwd string) (auth, note string) {
	if !remoteIsSandbox || !serverBuild || strings.TrimSpace(getenv(deploy.ComposerAuthEnv)) != "" {
		return "", ""
	}
	candidates := []string{filepath.Join(cwd, "auth.json")}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".composer", "auth.json"))
	}
	for _, candidate := range candidates {
		raw, err := readFile(candidate)
		if err != nil {
			continue
		}
		var parsed map[string]any
		if json.Unmarshal(raw, &parsed) != nil {
			return "", fmt.Sprintf("note: %s is not valid JSON; the sandbox build runs without composer credentials", candidate)
		}
		return strings.TrimSpace(string(raw)), ""
	}
	return "", ""
}

// applySandboxComposerAuth puts those credentials where the executor reads them
// and returns the function that restores the environment. The executor then
// carries them on the command's standard input, never in argv or on disk.
func applySandboxComposerAuth(cmd *cobra.Command, remoteIsSandbox, serverBuild bool) func() {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	auth, note := sandboxComposerAuth(remoteIsSandbox, serverBuild, os.Getenv, os.ReadFile, home, cwd)
	if note != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), note)
	}
	if auth == "" {
		return func() {}
	}
	previous, had := os.LookupEnv(deploy.ComposerAuthEnv)
	_ = os.Setenv(deploy.ComposerAuthEnv, auth)
	return func() {
		if had {
			_ = os.Setenv(deploy.ComposerAuthEnv, previous)
		} else {
			_ = os.Unsetenv(deploy.ComposerAuthEnv)
		}
	}
}

// SandboxComposerAuthForTest exposes sandboxComposerAuth to the tests/ package.
func SandboxComposerAuthForTest(remoteIsSandbox, serverBuild bool, getenv func(string) string, readFile func(string) ([]byte, error), home, cwd string) (string, string) {
	return sandboxComposerAuth(remoteIsSandbox, serverBuild, getenv, readFile, home, cwd)
}
