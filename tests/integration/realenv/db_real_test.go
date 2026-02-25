//go:build realenv
// +build realenv

package realenv

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDBDumpFromDev(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	dumpFile := filepath.Join(t.TempDir(), "dev_dump.sql")

	result := env.RunGovard(t, localDir, "db", "dump",
		"--environment", "dev",
		"--file", dumpFile,
	)
	result.AssertSuccess(t)

	// Verify dump file was created and has content
	stat, err := os.Stat(dumpFile)
	if err != nil {
		t.Fatalf("Dump file should be created: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatal("Dump file should not be empty")
	}
}

func TestDBImportFromFile(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Create a test dump file
	dumpContent := `-- Test dump
CREATE TABLE IF NOT EXISTS test_import_table (
    id INT AUTO_INCREMENT PRIMARY KEY,
    data VARCHAR(255)
);
INSERT INTO test_import_table (data) VALUES ('test_data_001');
`
	dumpFile := filepath.Join(t.TempDir(), "test_dump.sql")
	if err := os.WriteFile(dumpFile, []byte(dumpContent), 0644); err != nil {
		t.Fatalf("Failed to create dump file: %v", err)
	}

	result := env.RunGovard(t, localDir, "db", "import",
		"--environment", "local",
		"--file", dumpFile,
	)
	result.AssertSuccess(t)
}

func TestDBStreamFromRemote(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	result := env.RunGovard(t, localDir, "db", "import",
		"--environment", "dev",
		"--stream-db",
	)
	result.AssertSuccess(t)

	// Verify stream was mentioned
	result.AssertOutputContains(t, "stream")
}

func TestDBDumpWithFullOption(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	dumpFile := filepath.Join(t.TempDir(), "full_dump.sql")

	result := env.RunGovard(t, localDir, "db", "dump",
		"--environment", "dev",
		"--file", dumpFile,
		"--full",
	)
	result.AssertSuccess(t)

	// Verify dump was created
	stat, err := os.Stat(dumpFile)
	if err != nil {
		t.Fatalf("Dump file should be created: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatal("Dump file should not be empty")
	}
}

func TestDBImportInvalidFile(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	result := env.RunGovard(t, localDir, "db", "import",
		"--environment", "local",
		"--file", "/nonexistent/file.sql",
	)
	result.AssertFailure(t)
}

func TestDBDumpToNonexistentDir(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	result := env.RunGovard(t, localDir, "db", "dump",
		"--environment", "dev",
		"--file", "/nonexistent/dir/dump.sql",
	)
	result.AssertFailure(t)
}

func TestDBOperationsWithTimestamp(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	timestamp := time.Now().Format("20060102_150405")
	dumpFile := filepath.Join(t.TempDir(), "dump_"+timestamp+".sql")

	// Dump from DEV
	result := env.RunGovard(t, localDir, "db", "dump",
		"--environment", "dev",
		"--file", dumpFile,
	)
	result.AssertSuccess(t)

	// Verify
	_, err := os.Stat(dumpFile)
	if err != nil {
		t.Fatalf("Dump file with timestamp should exist: %v", err)
	}
}
