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

func TestSyncFilesDevToLocal(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Create test file in DEV via SSH
	testContent := "SYNC_TEST_" + time.Now().Format("20060102_150405")
	sshCmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-i", env.SSHKeyPath,
		"-p", "9023", "linuxserver.io@localhost",
		"mkdir -p /var/www/html/app/code \u0026\u0026 echo '"+testContent+"' \u003e /var/www/html/app/code/sync_test.txt")
	if err := sshCmd.Run(); err != nil {
		t.Fatalf("Failed to create test file on DEV: %v", err)
	}

	// Sync files from DEV
	result := env.RunGovard(t, localDir, "sync",
		"--source", "dev",
		"--destination", "local",
		"--file",
		"--path", "app/code",
	)
	result.AssertSuccess(t)

	// Verify rsync was used
	result.AssertOutputContains(t, "rsync")
}

func TestSyncPlanMode(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	result := env.RunGovard(t, localDir, "sync",
		"--source", "staging",
		"--destination", "local",
		"--file",
		"--full",
		"--plan",
	)
	result.AssertSuccess(t)

	// Should show plan without executing
	result.AssertOutputContains(t, "Sync Plan Summary")
	result.AssertOutputContains(t, "source:")
	result.AssertOutputContains(t, "destination:")
}

func TestSyncSameSourceAndDestination(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	result := env.RunGovard(t, localDir, "sync",
		"--source", "dev",
		"--destination", "dev",
		"--file",
	)
	result.AssertFailure(t)
	result.AssertOutputContains(t, "source and destination cannot be the same")
}

func TestSyncWithPatterns(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Create test files on DEV
	sshCmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-i", env.SSHKeyPath,
		"-p", "9023", "linuxserver.io@localhost",
		"mkdir -p /var/www/html/app \u0026\u0026 touch /var/www/html/app/included.txt /var/www/html/app/excluded.log")
	if err := sshCmd.Run(); err != nil {
		t.Fatalf("Failed to create test files on DEV: %v", err)
	}

	result := env.RunGovard(t, localDir, "sync",
		"--source", "dev",
		"--destination", "local",
		"--file",
		"--include", "*.txt",
		"--exclude", "*.log",
	)
	result.AssertSuccess(t)

	// Verify patterns in command
	result.AssertOutputContains(t, "--include")
	result.AssertOutputContains(t, "--exclude")
}

func TestSyncLocalToStaging(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Create a file in LOCAL
	testFile := filepath.Join(localDir, "local_change.txt")
	if err := os.WriteFile(testFile, []byte("local-content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := env.RunGovard(t, localDir, "sync",
		"--source", "local",
		"--destination", "staging",
		"--file",
	)
	result.AssertSuccess(t)
}

func TestSyncWithDelete(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	result := env.RunGovard(t, localDir, "sync",
		"--source", "dev",
		"--destination", "local",
		"--file",
		"--delete",
	)
	result.AssertSuccess(t)

	// Should warn about delete mode
	result.AssertOutputContains(t, "delete")
}

func TestSyncFull(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Ensure source directories exist on DEV for media sync
	sshCmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-i", env.SSHKeyPath,
		"-p", "9023", "linuxserver.io@localhost",
		"mkdir -p /var/www/html/pub/media")
	if err := sshCmd.Run(); err != nil {
		t.Fatalf("Failed to create media dir on DEV: %v", err)
	}

	result := env.RunGovard(t, localDir, "sync",
		"--source", "dev",
		"--destination", "local",
		"--full",
	)
	result.AssertSuccess(t)

	// Should include all scopes
	result.AssertOutputContains(t, "files")
	result.AssertOutputContains(t, "media")
	result.AssertOutputContains(t, "db")
}
