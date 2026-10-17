package tests

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"govard/internal/desktop"
)

// Final whole-branch review fixes: C1 (query password via env-file), I1
// (backend single-flight naming the running op), I2 (import re-validation
// after open + dump dest re-stat at op start), I3 (denied-function gate).

func TestFinalFix_QueryPasswordNeverInProcessArgs(t *testing.T) {
	const secret = "s3cret-final-fix-pw"
	for _, tc := range []struct {
		name   string
		scheme string
		user   string
		sql    string
	}{
		{"mysql", "mysql", "app-user", "SELECT 1"},
		{"postgres", "postgresql", "app-user", "SELECT 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, envFile, cleanup, err := desktop.BuildDBQueryDockerArgsForTest(
				"sample-project-db-1", tc.scheme, tc.user, secret, tc.sql,
			)
			if err != nil {
				t.Fatalf("BuildDBQueryDockerArgs error = %v, want nil", err)
			}
			defer cleanup()
			for _, arg := range args {
				if strings.Contains(arg, secret) {
					t.Errorf("docker arg %q leaks the database password", arg)
				}
			}
			if envFile == "" {
				t.Fatalf("env file path is empty, want a 0600 env file carrying the secret")
			}
			raw, err := os.ReadFile(envFile)
			if err != nil {
				t.Fatalf("read env file: %v", err)
			}
			if !strings.Contains(string(raw), secret) {
				t.Errorf("env file must carry the password, got %q", string(raw))
			}
			info, err := os.Stat(envFile)
			if err != nil {
				t.Fatalf("stat env file: %v", err)
			}
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Errorf("env file perm = %o, want 600", perm)
			}
			found := false
			for _, arg := range args {
				if arg == "--env-file" {
					found = true
				}
			}
			if !found {
				t.Errorf("docker args %q must pass the secret via --env-file", args)
			}
		})
	}
}

func TestFinalFix_ConcurrentSecondTransferRefusedNamingRunningOp(t *testing.T) {
	prepareDBTransferTest(t)
	stubDBTransferSpace(t, 1<<20, 10<<30)
	sawCancel := &atomic.Bool{}
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(ctx context.Context, _ desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			return io.NopCloser(&ctxBlockReader{ctx: ctx, sawCancel: sawCancel}), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)

	svc, svcFake := newDBTransferService()
	dest1 := filepath.Join(t.TempDir(), "first.sql")
	op1, err := svc.DumpDB("sample-project", dest1)
	if err != nil {
		t.Fatalf("first DumpDB error = %v, want nil", err)
	}
	if op1 == "" {
		t.Fatalf("first DumpDB op id is empty")
	}
	waitForDBTransferCondition(t, "first op to register", func() bool {
		for _, op := range desktop.ListOps() {
			if op.ID == op1 {
				return true
			}
		}
		return false
	})

	dest2 := filepath.Join(t.TempDir(), "second.sql")
	_, err = svc.DumpDB("sample-project", dest2)
	if err == nil {
		t.Fatalf("second DumpDB error = nil, want refusal naming the running op")
	}
	if !strings.Contains(err.Error(), op1) {
		t.Errorf("second-start refusal %q must name the existing running op %q", err.Error(), op1)
	}

	if !svc.CancelDBOp(op1) {
		t.Fatalf("CancelDBOp(%q) = false, want true", op1)
	}
	waitForDBTransferCondition(t, "first op to finish", func() bool {
		return len(desktop.ListOps()) == 0
	})

	restoreReader()
	restoreReader2 := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(context.Context, desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			return io.NopCloser(strings.NewReader(dbTransferMySQLDump)), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader2)
	dest3 := filepath.Join(t.TempDir(), "third.sql")
	if _, err := svc.DumpDB("sample-project", dest3); err != nil {
		t.Fatalf("DumpDB after finish error = %v, want nil", err)
	}
	completedBefore := len(svcFake.EventsNamed("db:completed"))
	waitForDBTransferCondition(t, "third dump completion", func() bool {
		return len(svcFake.EventsNamed("db:completed")) > completedBefore
	})
}

func TestFinalFix_ImportSwappedBetweenValidateAndRunIsRefused(t *testing.T) {
	prepareDBTransferTest(t)
	var opens atomic.Int64
	restoreWriter := desktop.SetOpenDBImportWriterForDesktopForTest(
		func(ctx context.Context, _ desktop.DBTransferTarget) (io.WriteCloser, func() error, error) {
			opens.Add(1)
			w := &countingDBWriter{ctx: ctx, opened: make(chan struct{})}
			close(w.opened)
			return w, func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreWriter)

	src := filepath.Join(t.TempDir(), "swap.sql")
	writeDBTransferFile(t, src, []byte(dbTransferMySQLDump))

	// Call-time validation passes on the valid dump...
	if _, ok, err := desktop.SniffDBDumpFileForTest(src); err != nil || !ok {
		t.Fatalf("call-time sniff = (%v, %v), want (mysql, true)", ok, err)
	}
	// ...then the file is swapped before the op re-opens it.
	writeDBTransferFile(t, src, []byte("this is definitely not a database dump\njust text\n"))

	if _, _, err := desktop.RevalidateDBImportSourceForTest(src, "mysql"); err == nil {
		t.Fatalf("re-validation of swapped file error = nil, want refusal")
	}
	if opens.Load() != 0 {
		t.Errorf("import writer opened %d times, want 0 (unvalidated bytes never reach the writer)", opens.Load())
	}
}

func TestFinalFix_DumpDestDirRefusedAtOpStart(t *testing.T) {
	prepareDBTransferTest(t)
	stubDBTransferSpace(t, 1<<20, 10<<30)
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(context.Context, desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			return io.NopCloser(strings.NewReader(dbTransferMySQLDump)), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)

	svc, fake := newDBTransferService()
	dir := t.TempDir()
	dest := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest dir: %v", err)
	}
	if _, err := svc.DumpDB("sample-project", dest); err != nil {
		t.Fatalf("DumpDB error = %v, want nil (refusal lands at op start, not call time)", err)
	}
	waitForDBTransferCondition(t, "op-start dest refusal", func() bool {
		return len(fake.EventsNamed("db:failed")) > 0
	})
	if len(fake.EventsNamed("db:completed")) != 0 {
		t.Errorf("expected no db:completed when dest is a directory, got %+v", fake.EventsNamed("db:completed"))
	}
	if _, statErr := os.Stat(dest + ".partial"); !os.IsNotExist(statErr) {
		t.Errorf("partial file left behind after op-start refusal")
	}
}

func TestFinalFix_SelectBorneFunctionWritesNeedOptIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")
	t.Cleanup(desktop.ResetOperationRegistryForTest)

	svc := desktop.NewDBService()
	writes := []string{
		"SELECT pg_terminate_backend(1234)",
		"SELECT pg_cancel_backend(1234)",
		"SELECT lo_unlink(12345)",
		"SELECT lo_import('/tmp/x.dat')",
		"SELECT lo_export(12345, '/tmp/x.dat')",
		"SELECT dblink_exec('conn', 'DROP TABLE users')",
		"select PG_TERMINATE_BACKEND (1234)",
		"SELECT pg_catalog.pg_terminate_backend(1234)",
	}
	for _, sql := range writes {
		if _, err := svc.RunDBQuery("sample-project", sql, false, ""); err == nil {
			t.Errorf("RunDBQuery(%q) error = nil, want opt-in refusal for function-call write", sql)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0 (function writes refused before execution)", calls)
	}

	token := desktop.DBWriteConfirmTokenForTest("sample-project")
	if _, err := svc.RunDBQuery("sample-project", "SELECT pg_terminate_backend(1234)", true, token); err != nil {
		t.Errorf("RunDBQuery(function write with opt-in) error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("executor calls = %d, want 1 (opted-in function call runs)", calls)
	}

	// A function name inside a string literal is not a call.
	res, err := svc.RunDBQuery("sample-project", "SELECT * FROM notes WHERE body = 'pg_terminate_backend did nothing'", false, "")
	if err != nil {
		t.Errorf("RunDBQuery(literal) error = %v, want nil", err)
	} else if !res.ReadOnly {
		t.Errorf("RunDBQuery(literal) ReadOnly = false, want true")
	}
}
