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

func TestDBImportGzipped(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Create a test dump file
	dumpContent := "CREATE TABLE IF NOT EXISTS test_gzip (id INT);"
	sqlFile := filepath.Join(t.TempDir(), "test.sql")
	if err := os.WriteFile(sqlFile, []byte(dumpContent), 0644); err != nil {
		t.Fatalf("Failed to create sql file: %v", err)
	}

	// Gzip it
	gzipFile := sqlFile + ".gz"
	cmd := exec.Command("gzip", "-c", sqlFile)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to gzip file: %v", err)
	}
	if err := os.WriteFile(gzipFile, output, 0644); err != nil {
		t.Fatalf("Failed to write gzip file: %v", err)
	}

	result := env.RunGovard(t, localDir, "db", "import",
		"--environment", "local",
		"--file", gzipFile,
	)
	result.AssertSuccess(t)
}

func TestDBQuery(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	// Test local query
	result := env.RunGovard(t, localDir, "db", "query", "SELECT 1", "--environment", "local")
	result.AssertSuccess(t)

	// Test remote query
	result = env.RunGovard(t, localDir, "db", "query", "SELECT 1", "--environment", "dev")
	result.AssertSuccess(t)
}

func TestDBInfo(t *testing.T) {
	env := NewRealEnvTest(t)
	env.Setup(t)

	localDir := env.CreateTempProject(t, "local")

	result := env.RunGovard(t, localDir, "db", "info")
	result.AssertSuccess(t)
	result.AssertOutputContains(t, "Database Connection Info")
}
