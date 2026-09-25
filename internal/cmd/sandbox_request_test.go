package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// sandboxRequestCommand is a fresh command carrying the flags
// sandboxCommandRequest reads, so a test never mutates the shared sandboxUpCmd.
func sandboxRequestCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "up"}
	cmd.Flags().String("profile", "php", "")
	cmd.Flags().String("php", "", "")
	cmd.Flags().String("docroot", "", "")
	cmd.Flags().String("layout", "", "")
	cmd.Flags().Bool("recreate", false, "")
	cmd.Flags().Bool("purge", false, "")
	cmd.Flags().Bool("no-seed", false, "")
	cmd.Flags().Bool("volumes", false, "")
	if err := cmd.Flags().Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	cmd.SetContext(context.Background())
	return cmd
}

// The stack's PHP series is a default for a new image, never a request: sent
// as one, it refused a bare `sandbox up` on an existing 8.4 sandbox and
// described a base-image sandbox as the stack's series. Only a named `--php`
// may disagree with the container that exists.
func TestSandboxRequestTreatsTheStackSeriesAsADefault(t *testing.T) {
	dir := t.TempDir()
	config := "project_name: sample-project\nframework: laravel\ndomain: sample-project.test\nstack:\n  php_version: \"8.5\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Chdir(dir)

	bare, err := sandboxCommandRequest(sandboxRequestCommand(t))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if bare.PHP != "" || bare.PHPDefault != "8.5" {
		t.Fatalf("bare up: PHP=%q PHPDefault=%q, want no request and the stack series as the default", bare.PHP, bare.PHPDefault)
	}

	named, err := sandboxCommandRequest(sandboxRequestCommand(t, "--php", "8.4"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if named.PHP != "8.4" {
		t.Fatalf("--php 8.4: PHP=%q, want the named series as the request", named.PHP)
	}
}
