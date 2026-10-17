package tests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
)

// The data tab never invents file paths: dump, import and export
// destinations all come from native dialogs behind thin service methods, so
// the island only ever sends back ids and dialog-picked paths. A cancelled
// dialog reports an empty pick with no error, like PickProjectDirectory.

func TestDBService_PickDumpDestinationOffersDialogPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	fake := &desktop.FakePlatform{SaveFileResult: filepath.Join(t.TempDir(), "dump.sql")}

	svc := desktop.NewDBServiceWithPlatform(fake)
	pick, err := svc.PickDumpDestination("sample-project")
	if err != nil {
		t.Fatalf("PickDumpDestination error = %v, want nil", err)
	}
	if pick.Path != fake.SaveFileResult {
		t.Errorf("Path = %q, want dialog result %q", pick.Path, fake.SaveFileResult)
	}
	if pick.Exists {
		t.Errorf("Exists = true, want false for a fresh path")
	}
	reqs := fake.SaveFileRequests()
	if len(reqs) != 1 {
		t.Fatalf("save dialog requests = %d, want 1", len(reqs))
	}
	if !strings.Contains(strings.ToLower(reqs[0].Title), "dump") {
		t.Errorf("dialog title = %q, want a dump title", reqs[0].Title)
	}
}

func TestDBService_PickDumpDestinationReportsExistingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	dest := filepath.Join(t.TempDir(), "existing.sql")
	if err := os.WriteFile(dest, []byte("-- dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &desktop.FakePlatform{SaveFileResult: dest}

	svc := desktop.NewDBServiceWithPlatform(fake)
	pick, err := svc.PickDumpDestination("sample-project")
	if err != nil {
		t.Fatalf("PickDumpDestination error = %v, want nil", err)
	}
	if !pick.Exists {
		t.Errorf("Exists = false, want true so the UI shows overwrite-confirm")
	}
}

func TestDBService_PickDumpDestinationCancelIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	fake := &desktop.FakePlatform{SaveFileResult: ""}

	svc := desktop.NewDBServiceWithPlatform(fake)
	pick, err := svc.PickDumpDestination("sample-project")
	if err != nil {
		t.Fatalf("PickDumpDestination error = %v, want nil", err)
	}
	if pick.Path != "" || pick.Exists {
		t.Errorf("cancel pick = %+v, want empty", pick)
	}
}

func TestDBService_PickDumpDestinationDialogError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	fake := &desktop.FakePlatform{SaveFileErr: errors.New("dialog denied")}

	svc := desktop.NewDBServiceWithPlatform(fake)
	if _, err := svc.PickDumpDestination("sample-project"); err == nil {
		t.Errorf("PickDumpDestination error = nil, want dialog error")
	}
}

func TestDBService_PickImportSourceOffersDialogPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	src := filepath.Join(t.TempDir(), "dump.sql")
	fake := &desktop.FakePlatform{OpenFileResult: src}

	svc := desktop.NewDBServiceWithPlatform(fake)
	path, err := svc.PickImportSource()
	if err != nil {
		t.Fatalf("PickImportSource error = %v, want nil", err)
	}
	if path != src {
		t.Errorf("Path = %q, want dialog result %q", path, src)
	}
	if len(fake.OpenFileRequests()) != 1 {
		t.Errorf("open dialog requests = %d, want 1", len(fake.OpenFileRequests()))
	}
}

func TestDBService_PickImportSourceCancelIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	fake := &desktop.FakePlatform{OpenFileResult: "  "}

	svc := desktop.NewDBServiceWithPlatform(fake)
	path, err := svc.PickImportSource()
	if err != nil {
		t.Fatalf("PickImportSource error = %v, want nil", err)
	}
	if path != "" {
		t.Errorf("cancel path = %q, want empty", path)
	}
}

func TestDBService_WriteConfirmTokenMatchesBackendGate(t *testing.T) {
	desktop.ResetStateForTest()
	svc := desktop.NewDBServiceWithPlatform(&desktop.FakePlatform{})
	token, err := svc.GetDBWriteConfirmToken("Sample-Project")
	if err != nil {
		t.Fatalf("GetDBWriteConfirmToken error = %v, want nil", err)
	}
	if token == "" {
		t.Fatalf("GetDBWriteConfirmToken = empty, want the confirm token")
	}
	if token != desktop.DBWriteConfirmTokenForTest("sample-project") {
		t.Errorf("token = %q, want the backend gate token", token)
	}
}

func TestSnapshotService_PickExportDestinationOffersDialogPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	dest := filepath.Join(t.TempDir(), "nightly.tar.gz")
	fake := &desktop.FakePlatform{SaveFileResult: dest}

	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	pick, err := svc.PickExportDestination("nightly")
	if err != nil {
		t.Fatalf("PickExportDestination error = %v, want nil", err)
	}
	if pick.Path != dest {
		t.Errorf("Path = %q, want dialog result %q", pick.Path, dest)
	}
	if pick.Exists {
		t.Errorf("Exists = true, want false for a fresh path")
	}
	reqs := fake.SaveFileRequests()
	if len(reqs) != 1 {
		t.Fatalf("save dialog requests = %d, want 1", len(reqs))
	}
	if !strings.Contains(reqs[0].DefaultFilename, "nightly") {
		t.Errorf("default filename = %q, want it derived from the snapshot name", reqs[0].DefaultFilename)
	}
}

func TestSnapshotService_PickExportDestinationReportsExistingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	dest := filepath.Join(t.TempDir(), "nightly.tar.gz")
	if err := os.WriteFile(dest, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &desktop.FakePlatform{SaveFileResult: dest}

	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	pick, err := svc.PickExportDestination("nightly")
	if err != nil {
		t.Fatalf("PickExportDestination error = %v, want nil", err)
	}
	if !pick.Exists {
		t.Errorf("Exists = false, want true so the UI shows overwrite-confirm")
	}
}

func TestSnapshotService_PickExportDestinationCancelIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	fake := &desktop.FakePlatform{SaveFileResult: ""}

	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	pick, err := svc.PickExportDestination("nightly")
	if err != nil {
		t.Fatalf("PickExportDestination error = %v, want nil", err)
	}
	if pick.Path != "" || pick.Exists {
		t.Errorf("cancel pick = %+v, want empty", pick)
	}
}
