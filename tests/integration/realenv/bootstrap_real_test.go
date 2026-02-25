//go:build realenv
// +build realenv

package realenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestBootstrapCloneFromDevToLocal(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	// Setup LOCAL project
	localDir := env.CreateTempProject(t, "local")

	// Create marker file in DEV via SSH
	markerContent := "DEV_MARKER_" + time.Now().Format("20060102_150405")
	sshCmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-i", env.SSHKeyPath,
		"-p", "9023", "linuxserver.io@localhost",
		"echo '"+markerContent+"' > /var/www/html/.dev_marker")
	if err := sshCmd.Run(); err != nil {
		t.Fatalf("Failed to create marker on DEV: %v", err)
	}

	// Clone from DEV
	result := env.RunGovard(t, localDir, "bootstrap", "--clone",
		"--environment", "dev",
		"--skip-up",
		"--no-composer",
		"--no-db",
		"--no-media",
		"--no-admin",
	)
	result.AssertSuccess(t)

	// Verify: marker file should be synced
	result.AssertOutputContains(t, "sync")
}

func TestBootstrapCloneCodeOnly(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	result := env.RunGovard(t, localDir, "bootstrap", "--clone",
		"--environment", "dev",
		"--code-only",
		"--skip-up",
		"--no-composer",
		"--no-admin",
	)
	result.AssertSuccess(t)

	// Should NOT contain database or media operations
	result.AssertOutputNotContains(t, "mysqldump")
}

func TestBootstrapValidationFreshAndClone(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Try to use both --fresh and --clone (should fail)
	result := env.RunGovard(t, localDir, "bootstrap", "--fresh", "--clone", "--skip-up")
	result.AssertFailure(t)
	result.AssertOutputContains(t, "--fresh and --clone cannot be used together")
}

func TestBootstrapValidationCodeOnlyRequiresClone(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// --clone defaults to true, so we must explicitly set --clone=false to trigger the validation.
	result := env.RunGovard(t, localDir, "bootstrap", "--code-only", "--clone=false", "--skip-up")
	result.AssertFailure(t)
	result.AssertOutputContains(t, "--code-only requires --clone")
}

func TestBootstrapCloneRequiresConfiguredRemote(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	// Create a minimal .govard.yml with no remotes so ensureBootstrapInit skips
	// running `govard init` (which would block waiting for interactive input).
	localDir := t.TempDir()
	minimalConfig := "project_name: test-no-remotes\ndomain: test-no-remotes.test\nrecipe: magento2\n"
	if err := os.WriteFile(filepath.Join(localDir, ".govard.yml"), []byte(minimalConfig), 0644); err != nil {
		t.Fatalf("Failed to write .govard.yml: %v", err)
	}

	result := env.RunGovard(t, localDir, "bootstrap", "--clone", "--environment", "dev", "--skip-up")
	result.AssertFailure(t)
	result.AssertOutputContains(t, "remote 'dev' is not configured")
}

func TestBootstrapCloneFull(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Create test data in DEV
	// 1. File
	sshCmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-i", env.SSHKeyPath,
		"-p", "9023", "linuxserver.io@localhost",
		"mkdir -p /var/www/html/app/code && echo 'FULL_CLONE_CODE' > /var/www/html/app/code/full_test.txt")
	if err := sshCmd.Run(); err != nil {
		t.Fatalf("Failed to create test file on DEV: %v", err)
	}

	// 2. Media
	sshCmd = exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-i", env.SSHKeyPath,
		"-p", "9023", "linuxserver.io@localhost",
		"mkdir -p /var/www/html/pub/media && echo 'FULL_CLONE_MEDIA' > /var/www/html/pub/media/media_test.txt")
	if err := sshCmd.Run(); err != nil {
		t.Fatalf("Failed to create media file on DEV: %v", err)
	}

	// 3. DB
	sshCmd = exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-i", env.SSHKeyPath,
		"-p", "9023", "linuxserver.io@localhost",
		"mysql -umagento -pmagento magento -e 'CREATE TABLE IF NOT EXISTS clone_test (val VARCHAR(255)); INSERT INTO clone_test VALUES (\"FULL_CLONE_DB\");'")
	if err := sshCmd.Run(); err != nil {
		t.Fatalf("Failed to create test DB table on DEV: %v", err)
	}

	result := env.RunGovard(t, localDir, "bootstrap", "--clone",
		"--environment", "dev",
		"--skip-up",
		"--no-composer",
		"--no-admin",
		"--yes",
	)
	result.AssertSuccess(t)

	// Verify output mentions all sync types
	result.AssertOutputContains(t, "syncing files")
	result.AssertOutputContains(t, "syncing media")
	result.AssertOutputContains(t, "database")
}

func TestBootstrapCloneCombinations(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	t.Run("no-db", func(t *testing.T) {
		localDir := env.CreateTempProject(t, "local")
		result := env.RunGovard(t, localDir, "bootstrap", "--clone",
			"--environment", "dev", "--no-db", "--skip-up", "--no-composer", "--no-admin", "--yes")
		result.AssertSuccess(t)
		result.AssertOutputContains(t, "syncing files")
		result.AssertOutputNotContains(t, "database")
	})

	t.Run("no-media", func(t *testing.T) {
		localDir := env.CreateTempProject(t, "local")
		result := env.RunGovard(t, localDir, "bootstrap", "--clone",
			"--environment", "dev", "--no-media", "--skip-up", "--no-composer", "--no-admin", "--yes")
		result.AssertSuccess(t)
		result.AssertOutputContains(t, "syncing files")
		result.AssertOutputNotContains(t, "syncing media")
	})
}

func TestBootstrapFreshMagento(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := t.TempDir()

	// Run bootstrap --fresh (will also run init because no config exists)
	result := env.RunGovard(t, localDir, "bootstrap", "--fresh",
		"--recipe", "magento2",
		"--version", "2.4.6",
		"--skip-up",
		"--no-composer",
		"--no-admin",
		"--yes",
	)
	result.AssertSuccess(t)

	// Verify .govard.yml was created
	if _, err := os.Stat(filepath.Join(localDir, ".govard.yml")); err != nil {
		t.Errorf("Expected .govard.yml to be created, got %v", err)
	}

	result.AssertOutputContains(t, "Fresh install")
}
