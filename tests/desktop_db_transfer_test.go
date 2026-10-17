package tests

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"govard/internal/desktop"
)

// Desktop database dump/import must preflight disk space before writing a
// byte, stream with progress, kill the transfer and remove the partial file
// on cancel, and validate the dump header before writing anything to the
// database. Secrets travel via env file, never in process args.

func waitForDBTransferCondition(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func newDBTransferService() (*desktop.DBService, *desktop.FakePlatform) {
	fake := &desktop.FakePlatform{}
	return desktop.NewDBServiceWithPlatform(fake), fake
}

func stubDBTransferSpace(t *testing.T, estimate int64, free uint64) {
	t.Helper()
	restoreSize := desktop.SetEstimateDBSizeForDesktopForTest(
		func(desktop.DBTransferTarget) (int64, error) { return estimate, nil },
	)
	restoreFree := desktop.SetFreeDiskSpaceForDesktopForTest(
		func(string) (uint64, error) { return free, nil },
	)
	t.Cleanup(func() {
		restoreFree()
		restoreSize()
	})
}

func prepareDBTransferTest(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	t.Cleanup(desktop.ResetOperationRegistryForTest)
}

func dbTransferProgressEvents(fake *desktop.FakePlatform) []desktop.DBTransferProgress {
	var out []desktop.DBTransferProgress
	for _, ev := range fake.EventsNamed("db:progress") {
		payload, ok := ev.(desktop.DBTransferProgress)
		if !ok {
			continue
		}
		out = append(out, payload)
	}
	return out
}

// ctxBlockReader blocks until ctx ends, proving the copy loop observes
// cancellation (the production kill mechanism is exec.CommandContext).
type ctxBlockReader struct {
	ctx       context.Context
	sawCancel *atomic.Bool
}

func (r *ctxBlockReader) Read([]byte) (int, error) {
	<-r.ctx.Done()
	r.sawCancel.Store(true)
	return 0, r.ctx.Err()
}

// gateDBReader serves one chunk, then blocks until ctx ends: after cancel no
// further read can succeed, so any later progress event is a stale leak.
type gateDBReader struct {
	ctx         context.Context
	first       []byte
	servedFirst atomic.Bool
}

func (r *gateDBReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}
	if r.servedFirst.Swap(true) {
		<-r.ctx.Done()
		return 0, r.ctx.Err()
	}
	return copy(p, r.first), nil
}

// countingDBWriter records import bytes; in blocking mode Write waits for
// ctx, proving cancel kills a stuck import.
type countingDBWriter struct {
	ctx    context.Context
	mu     sync.Mutex
	buf    bytes.Buffer
	writes atomic.Int64
	once   sync.Once
	opened chan struct{}
	block  bool
	sawEnd *atomic.Bool
}

func (w *countingDBWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.opened) })
	if w.block {
		<-w.ctx.Done()
		if w.sawEnd != nil {
			w.sawEnd.Store(true)
		}
		return 0, w.ctx.Err()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes.Add(int64(len(p)))
	return w.buf.Write(p)
}

func (w *countingDBWriter) Close() error { return nil }

func (w *countingDBWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

const dbTransferMySQLDump = `-- MySQL dump 10.13  Distrib 8.0.36, for Linux (x86_64)
--
-- Host: localhost    Database: appdb
CREATE TABLE users (id INT PRIMARY KEY);
INSERT INTO users VALUES (1);
`

const dbTransferPostgresDump = `-- PostgreSQL database dump
-- Dumped by pg_dump version 15.2
CREATE TABLE users (id integer PRIMARY KEY);
`

func writeDBTransferFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeGzipDBTransferFile(t *testing.T, path string, data []byte) {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("gzip %s: %v", path, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close %s: %v", path, err)
	}
	writeDBTransferFile(t, path, buf.Bytes())
}

func TestDBDump_PreflightRefusesBeforeWritingByte(t *testing.T) {
	prepareDBTransferTest(t)
	const estimate = int64(2 << 30)
	const free = uint64(500 << 20)
	stubDBTransferSpace(t, estimate, free)
	var opened atomic.Int64
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(context.Context, desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			opened.Add(1)
			return io.NopCloser(strings.NewReader(dbTransferMySQLDump)), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)

	svc, _ := newDBTransferService()
	dest := filepath.Join(t.TempDir(), "dump.sql")
	_, err := svc.DumpDB("sample-project", dest)
	if err == nil {
		t.Fatalf("DumpDB with 500MB free and 2GB estimate error = nil, want preflight refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "insufficient disk space") {
		t.Errorf("refusal %q must say insufficient disk space", msg)
	}
	if !strings.Contains(msg, desktop.FormatBytesForTest(estimate)) {
		t.Errorf("refusal %q must carry the estimated size %q", msg, desktop.FormatBytesForTest(estimate))
	}
	if !strings.Contains(msg, desktop.FormatBytesForTest(int64(free))) {
		t.Errorf("refusal %q must carry the free space %q", msg, desktop.FormatBytesForTest(int64(free)))
	}
	if strings.Trim(msg, " \t\n.,:;()[]") == strings.Trim("insufficient disk space", " \t\n.,:;()[]") {
		t.Errorf("refusal %q carries no size numbers", msg)
	}
	if opened.Load() != 0 {
		t.Errorf("dump reader opened %d times, want 0 (refused before starting)", opened.Load())
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("dest file exists after refusal, want nothing written")
	}
	if _, statErr := os.Stat(dest + ".partial"); !os.IsNotExist(statErr) {
		t.Errorf("partial file exists after refusal, want nothing written")
	}
}

func TestDBDump_CancelKillsProcessAndRemovesPartial(t *testing.T) {
	prepareDBTransferTest(t)
	stubDBTransferSpace(t, 1<<20, 10<<30)
	var sawCancel atomic.Bool
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(ctx context.Context, _ desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			return io.NopCloser(&ctxBlockReader{ctx: ctx, sawCancel: &sawCancel}), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)

	svc, fake := newDBTransferService()
	dest := filepath.Join(t.TempDir(), "dump.sql")
	opID, err := svc.DumpDB("sample-project", dest)
	if err != nil {
		t.Fatalf("DumpDB error = %v, want nil", err)
	}
	if opID == "" {
		t.Fatalf("DumpDB op id is empty, want a cancellable op")
	}
	waitForDBTransferCondition(t, "partial file to appear", func() bool {
		_, statErr := os.Stat(dest + ".partial")
		return statErr == nil
	})
	if !svc.CancelDBOp(opID) {
		t.Fatalf("CancelDBOp(%q) = false, want true", opID)
	}
	waitForDBTransferCondition(t, "cancel to kill the transfer", func() bool {
		return sawCancel.Load()
	})
	waitForDBTransferCondition(t, "partial file removal", func() bool {
		_, statErr := os.Stat(dest + ".partial")
		return os.IsNotExist(statErr)
	})
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("dest file exists after cancel, want no completed dump")
	}
	failed := fake.EventsNamed("db:failed")
	if len(failed) == 0 {
		t.Fatalf("expected a db:failed event after cancel, got none (events: %+v)", fake.Events())
	}
	payload, ok := failed[0].(desktop.DBTransferFailed)
	if !ok {
		t.Fatalf("db:failed payload is %T, want DBTransferFailed", failed[0])
	}
	if payload.OpID != opID {
		t.Errorf("db:failed op = %q, want %q", payload.OpID, opID)
	}
	if !strings.Contains(strings.ToLower(payload.Message), "cancel") {
		t.Errorf("db:failed message %q must say cancelled", payload.Message)
	}
	if len(fake.EventsNamed("db:completed")) != 0 {
		t.Errorf("expected no db:completed after cancel, got %+v", fake.EventsNamed("db:completed"))
	}
}

func TestDBDump_SuccessStreamsProgressAndCompletes(t *testing.T) {
	prepareDBTransferTest(t)
	payload := dbTransferMySQLDump + strings.Repeat("-- filler row\n", 200)
	stubDBTransferSpace(t, int64(len(payload)), 10<<30)
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(context.Context, desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			return io.NopCloser(strings.NewReader(payload)), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)

	svc, fake := newDBTransferService()
	dest := filepath.Join(t.TempDir(), "dump.sql")
	opID, err := svc.DumpDB("sample-project", dest)
	if err != nil {
		t.Fatalf("DumpDB error = %v, want nil", err)
	}
	waitForDBTransferCondition(t, "dump completion", func() bool {
		return len(fake.EventsNamed("db:completed")) > 0
	})
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != payload {
		t.Errorf("dest is %d bytes, want the %d streamed dump bytes", len(got), len(payload))
	}
	if _, statErr := os.Stat(dest + ".partial"); !os.IsNotExist(statErr) {
		t.Errorf("partial file left behind after success")
	}
	progress := dbTransferProgressEvents(fake)
	if len(progress) == 0 {
		t.Fatalf("expected db:progress events, got none (events: %+v)", fake.Events())
	}
	for _, ev := range progress {
		if ev.OpID != opID {
			t.Errorf("db:progress op = %q, want %q", ev.OpID, opID)
		}
		if ev.Percent < 0 || ev.Percent > 100 {
			t.Errorf("db:progress percent = %d, want 0-100", ev.Percent)
		}
		if ev.Message == "" {
			t.Errorf("db:progress message is empty")
		}
	}
	last := progress[len(progress)-1]
	if last.BytesWritten != int64(len(payload)) {
		t.Errorf("final progress bytes = %d, want %d", last.BytesWritten, len(payload))
	}
	done, ok := fake.EventsNamed("db:completed")[0].(desktop.DBTransferCompleted)
	if !ok {
		t.Fatalf("db:completed payload is %T, want DBTransferCompleted", fake.EventsNamed("db:completed")[0])
	}
	if done.OpID != opID || done.Path != dest {
		t.Errorf("db:completed = %+v, want op %q path %q", done, opID, dest)
	}
}

func TestDBDump_GzipDestinationCompresses(t *testing.T) {
	prepareDBTransferTest(t)
	stubDBTransferSpace(t, int64(len(dbTransferMySQLDump)), 10<<30)
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(context.Context, desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			return io.NopCloser(strings.NewReader(dbTransferMySQLDump)), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)

	svc, fake := newDBTransferService()
	dest := filepath.Join(t.TempDir(), "dump.sql.gz")
	if _, err := svc.DumpDB("sample-project", dest); err != nil {
		t.Fatalf("DumpDB error = %v, want nil", err)
	}
	waitForDBTransferCondition(t, "gzip dump completion", func() bool {
		return len(fake.EventsNamed("db:completed")) > 0
	})
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("dest is not gzip: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("gunzip dest: %v", err)
	}
	if string(decoded) != dbTransferMySQLDump {
		t.Errorf("gunzipped dest differs from the streamed dump")
	}
}

func TestDBImport_InvalidHeaderWritesNothing(t *testing.T) {
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

	svc, fake := newDBTransferService()
	src := filepath.Join(t.TempDir(), "not-a-dump.sql")
	writeDBTransferFile(t, src, []byte("this is definitely not a database dump\njust text\n"))

	if _, err := svc.ImportDB("sample-project", src); err == nil {
		t.Fatalf("ImportDB(garbage) error = nil, want header refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "dump") {
		t.Errorf("refusal %q must name the dump header problem", err.Error())
	}
	if opens.Load() != 0 {
		t.Errorf("import writer opened %d times, want 0 (header checked first)", opens.Load())
	}
	if len(fake.EventsNamed("db:completed")) != 0 {
		t.Errorf("expected no db:completed for a refused import")
	}
}

func TestDBImport_WrongEngineRefusedWithActionableError(t *testing.T) {
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

	svc, _ := newDBTransferService()
	src := filepath.Join(t.TempDir(), "pg.sql")
	writeDBTransferFile(t, src, []byte(dbTransferPostgresDump))
	_, err := svc.ImportDB("sample-project", src)
	if err == nil {
		t.Fatalf("ImportDB(postgres dump into mysql) error = nil, want engine refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "PostgreSQL") || !strings.Contains(strings.ToLower(msg), "mysql") {
		t.Errorf("refusal %q must name both the dump engine and the project engine", msg)
	}
	if opens.Load() != 0 {
		t.Errorf("import writer opened %d times, want 0 (refused before writing)", opens.Load())
	}
}

func TestDBImport_SuccessStreamsToWriter(t *testing.T) {
	prepareDBTransferTest(t)
	writer := &countingDBWriter{opened: make(chan struct{})}
	restoreWriter := desktop.SetOpenDBImportWriterForDesktopForTest(
		func(ctx context.Context, _ desktop.DBTransferTarget) (io.WriteCloser, func() error, error) {
			writer.ctx = ctx
			return writer, func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreWriter)

	svc, fake := newDBTransferService()
	src := filepath.Join(t.TempDir(), "dump.sql")
	writeDBTransferFile(t, src, []byte(dbTransferMySQLDump))
	opID, err := svc.ImportDB("sample-project", src)
	if err != nil {
		t.Fatalf("ImportDB error = %v, want nil", err)
	}
	<-writer.opened
	waitForDBTransferCondition(t, "import completion", func() bool {
		return len(fake.EventsNamed("db:completed")) > 0
	})
	got := writer.String()
	if !strings.Contains(got, "CREATE TABLE users") {
		t.Errorf("imported bytes miss the dump content, got %q", got)
	}
	if !strings.Contains(got, "FOREIGN_KEY_CHECKS") {
		t.Errorf("imported bytes miss the session wrapper, got %q", got)
	}
	if strings.Contains(got, dbServiceTestPassword) {
		t.Errorf("imported bytes leak the database password")
	}
	progress := dbTransferProgressEvents(fake)
	if len(progress) == 0 {
		t.Fatalf("expected db:progress events, got none (events: %+v)", fake.Events())
	}
	for _, ev := range progress {
		if ev.OpID != opID {
			t.Errorf("db:progress op = %q, want %q", ev.OpID, opID)
		}
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat src: %v", err)
	}
	if last := progress[len(progress)-1]; last.BytesWritten != info.Size() {
		t.Errorf("final progress bytes = %d, want source size %d", last.BytesWritten, info.Size())
	}
}

func TestDBImport_GzipSourceDecompresses(t *testing.T) {
	prepareDBTransferTest(t)
	writer := &countingDBWriter{opened: make(chan struct{})}
	restoreWriter := desktop.SetOpenDBImportWriterForDesktopForTest(
		func(ctx context.Context, _ desktop.DBTransferTarget) (io.WriteCloser, func() error, error) {
			writer.ctx = ctx
			return writer, func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreWriter)

	svc, fake := newDBTransferService()
	src := filepath.Join(t.TempDir(), "dump.sql.gz")
	writeGzipDBTransferFile(t, src, []byte(dbTransferMySQLDump))
	if _, err := svc.ImportDB("sample-project", src); err != nil {
		t.Fatalf("ImportDB(.gz) error = %v, want nil", err)
	}
	waitForDBTransferCondition(t, "gzip import completion", func() bool {
		return len(fake.EventsNamed("db:completed")) > 0
	})
	if !strings.Contains(writer.String(), "CREATE TABLE users") {
		t.Errorf("gzipped import missed the dump content, got %q", writer.String())
	}
}

func TestDBImport_CancelKillsProcess(t *testing.T) {
	prepareDBTransferTest(t)
	writer := &countingDBWriter{opened: make(chan struct{}), block: true, sawEnd: &atomic.Bool{}}
	restoreWriter := desktop.SetOpenDBImportWriterForDesktopForTest(
		func(ctx context.Context, _ desktop.DBTransferTarget) (io.WriteCloser, func() error, error) {
			writer.ctx = ctx
			return writer, func() error { return ctx.Err() }, nil
		},
	)
	t.Cleanup(restoreWriter)

	svc, fake := newDBTransferService()
	src := filepath.Join(t.TempDir(), "dump.sql")
	writeDBTransferFile(t, src, []byte(dbTransferMySQLDump))
	opID, err := svc.ImportDB("sample-project", src)
	if err != nil {
		t.Fatalf("ImportDB error = %v, want nil", err)
	}
	<-writer.opened
	if !svc.CancelDBOp(opID) {
		t.Fatalf("CancelDBOp(%q) = false, want true", opID)
	}
	waitForDBTransferCondition(t, "cancel to kill the import", func() bool {
		return writer.sawEnd.Load()
	})
	failed := fake.EventsNamed("db:failed")
	if len(failed) == 0 {
		t.Fatalf("expected a db:failed event after cancel, got none (events: %+v)", fake.Events())
	}
	payload, ok := failed[0].(desktop.DBTransferFailed)
	if !ok {
		t.Fatalf("db:failed payload is %T, want DBTransferFailed", failed[0])
	}
	if payload.OpID != opID {
		t.Errorf("db:failed op = %q, want %q", payload.OpID, opID)
	}
}

func TestDBTransfer_PasswordNeverInProcessArgs(t *testing.T) {
	const secret = "s3cret-test-pw"
	args, envFile, cleanup, err := desktop.BuildDBTransferDockerArgsForTest(
		"sample-project-db-1", secret, "MYSQL_PWD", "echo hi",
	)
	if err != nil {
		t.Fatalf("BuildDBTransferDockerArgs error = %v, want nil", err)
	}
	defer cleanup()
	for _, arg := range args {
		if strings.Contains(arg, secret) {
			t.Errorf("docker arg %q leaks the database password", arg)
		}
	}
	raw, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	if !strings.Contains(string(raw), secret) {
		t.Errorf("env file must carry the password for the transfer, got %q", string(raw))
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
}

func TestDBTransfer_ProgressTaggedAndStaleDropped(t *testing.T) {
	prepareDBTransferTest(t)
	stubDBTransferSpace(t, 1<<20, 10<<30)
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(ctx context.Context, _ desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			return io.NopCloser(&gateDBReader{ctx: ctx, first: []byte("chunk-one\n")}), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)

	svc, fake := newDBTransferService()
	dest := filepath.Join(t.TempDir(), "dump.sql")
	opID, err := svc.DumpDB("sample-project", dest)
	if err != nil {
		t.Fatalf("DumpDB error = %v, want nil", err)
	}
	waitForDBTransferCondition(t, "first progress event", func() bool {
		return len(dbTransferProgressEvents(fake)) >= 1
	})
	progress := dbTransferProgressEvents(fake)
	var generation uint64
	first := true
	for _, ev := range progress {
		if ev.OpID != opID {
			t.Fatalf("db:progress op = %q, want %q", ev.OpID, opID)
		}
		if first {
			generation = ev.Generation
			first = false
		} else if ev.Generation != generation {
			t.Fatalf("db:progress generation changed mid-op: %d then %d", generation, ev.Generation)
		}
	}
	if !svc.CancelDBOp(opID) {
		t.Fatalf("CancelDBOp(%q) = false, want true", opID)
	}
	waitForDBTransferCondition(t, "cancel failure event", func() bool {
		return len(fake.EventsNamed("db:failed")) > 0
	})
	settled := len(dbTransferProgressEvents(fake))
	time.Sleep(200 * time.Millisecond)
	if got := len(dbTransferProgressEvents(fake)); got != settled {
		t.Errorf("stale progress leaked after cancel: %d events then %d", settled, got)
	}
}

func TestDBDumpHeaderSniff(t *testing.T) {
	cases := []struct {
		name   string
		head   []byte
		engine string
		ok     bool
	}{
		{"mysql", []byte(dbTransferMySQLDump), "mysql", true},
		{"mariadb", []byte("-- MariaDB dump 10.19  Distrib 10.6.7-MariaDB\nCREATE TABLE t;"), "mysql", true},
		{"postgres", []byte(dbTransferPostgresDump), "postgresql", true},
		{"garbage", []byte("this is definitely not a database dump\n"), "", false},
		{"empty", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, ok := desktop.SniffDBDumpHeaderForTest(tc.head)
			if ok != tc.ok || engine != tc.engine {
				t.Fatalf("SniffDBDumpHeader = (%q, %v), want (%q, %v)", engine, ok, tc.engine, tc.ok)
			}
		})
	}

	t.Run("gzip", func(t *testing.T) {
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)
		if _, err := writer.Write([]byte(dbTransferMySQLDump)); err != nil {
			t.Fatalf("gzip: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
		engine, ok := desktop.SniffDBDumpHeaderForTest(buf.Bytes())
		if !ok || engine != "mysql" {
			t.Fatalf("SniffDBDumpHeader(gzip) = (%q, %v), want (mysql, true)", engine, ok)
		}
	})
}

// failAfterDBWriter accepts limit bytes, then fails every further write with
// an ENOSPC-style error, pinning mid-stream failure cleanup.
type failAfterDBWriter struct {
	limit   int64
	written int64
}

func (w *failAfterDBWriter) Write(p []byte) (int, error) {
	if w.written >= w.limit {
		return 0, errors.New("no space left on device")
	}
	if room := w.limit - w.written; int64(len(p)) > room {
		w.written = w.limit
		return int(room), errors.New("no space left on device")
	}
	w.written += int64(len(p))
	return len(p), nil
}

func (w *failAfterDBWriter) Close() error { return nil }

// midFailFileWriter wraps a real file at path and fails writes past limit,
// so the test observes genuine partial-file cleanup, not a diskless stub.
type midFailFileWriter struct {
	file    *os.File
	limit   int64
	written int64
}

func (w *midFailFileWriter) Write(p []byte) (int, error) {
	if w.written >= w.limit {
		return 0, errors.New("no space left on device")
	}
	chunk := p
	var short error
	if room := w.limit - w.written; int64(len(p)) > room {
		chunk = p[:room]
		short = errors.New("no space left on device")
	}
	n, err := w.file.Write(chunk)
	w.written += int64(n)
	if err != nil {
		return n, err
	}
	if short != nil {
		return n, short
	}
	return n, nil
}

func (w *midFailFileWriter) Close() error { return w.file.Close() }

func TestDBDump_MidStreamFailureRemovesPartial(t *testing.T) {
	prepareDBTransferTest(t)
	payload := dbTransferMySQLDump + strings.Repeat("-- filler row\n", 200)
	stubDBTransferSpace(t, int64(len(payload)), 10<<30)
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(context.Context, desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			return io.NopCloser(strings.NewReader(payload)), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)
	restoreDest := desktop.SetCreateDBDumpDestForDesktopForTest(
		func(path string) (io.WriteCloser, error) {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return nil, err
			}
			return &midFailFileWriter{file: file, limit: 64}, nil
		},
	)
	t.Cleanup(restoreDest)

	svc, fake := newDBTransferService()
	dest := filepath.Join(t.TempDir(), "dump.sql")
	if _, err := svc.DumpDB("sample-project", dest); err != nil {
		t.Fatalf("DumpDB error = %v, want nil", err)
	}
	waitForDBTransferCondition(t, "mid-stream failure event", func() bool {
		return len(fake.EventsNamed("db:failed")) > 0
	})
	failed, ok := fake.EventsNamed("db:failed")[0].(desktop.DBTransferFailed)
	if !ok {
		t.Fatalf("db:failed payload is %T, want DBTransferFailed", fake.EventsNamed("db:failed")[0])
	}
	if !strings.Contains(failed.Message, "no space left on device") {
		t.Errorf("db:failed message %q must carry the write error", failed.Message)
	}
	if len(fake.EventsNamed("db:completed")) != 0 {
		t.Errorf("expected no db:completed after a mid-stream failure, got %+v", fake.EventsNamed("db:completed"))
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("dest file exists after mid-stream failure, want no completed dump")
	}
	if _, statErr := os.Stat(dest + ".partial"); !os.IsNotExist(statErr) {
		t.Errorf("partial file left behind after mid-stream failure, want cleanup")
	}
}

func TestDBImport_MidStreamFailureIsNotSuccess(t *testing.T) {
	prepareDBTransferTest(t)
	writer := &failAfterDBWriter{limit: 200}
	restoreWriter := desktop.SetOpenDBImportWriterForDesktopForTest(
		func(context.Context, desktop.DBTransferTarget) (io.WriteCloser, func() error, error) {
			return writer, func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreWriter)

	svc, fake := newDBTransferService()
	src := filepath.Join(t.TempDir(), "dump.sql")
	writeDBTransferFile(t, src, []byte(dbTransferMySQLDump))
	if _, err := svc.ImportDB("sample-project", src); err != nil {
		t.Fatalf("ImportDB error = %v, want nil", err)
	}
	waitForDBTransferCondition(t, "mid-stream import failure event", func() bool {
		return len(fake.EventsNamed("db:failed")) > 0
	})
	failed, ok := fake.EventsNamed("db:failed")[0].(desktop.DBTransferFailed)
	if !ok {
		t.Fatalf("db:failed payload is %T, want DBTransferFailed", fake.EventsNamed("db:failed")[0])
	}
	if !strings.Contains(failed.Message, "no space left on device") {
		t.Errorf("db:failed message %q must carry the write error", failed.Message)
	}
	if len(fake.EventsNamed("db:completed")) != 0 {
		t.Errorf("expected no db:completed after a mid-stream import failure, got %+v", fake.EventsNamed("db:completed"))
	}
	if writer.written == 0 {
		t.Errorf("expected some bytes to flow before the failure, got none")
	}
}

func TestDBDump_PreflightRefusalCreatesNothing(t *testing.T) {
	prepareDBTransferTest(t)
	stubDBTransferSpace(t, 2<<30, 500<<20)
	var opened atomic.Int64
	restoreReader := desktop.SetOpenDBDumpReaderForDesktopForTest(
		func(context.Context, desktop.DBTransferTarget) (io.ReadCloser, func() error, error) {
			opened.Add(1)
			return io.NopCloser(strings.NewReader(dbTransferMySQLDump)), func() error { return nil }, nil
		},
	)
	t.Cleanup(restoreReader)

	svc, _ := newDBTransferService()
	dest := filepath.Join(t.TempDir(), "brand-new", "nested", "dump.sql")
	if _, err := svc.DumpDB("sample-project", dest); err == nil {
		t.Fatalf("DumpDB with 500MB free and 2GB estimate error = nil, want preflight refusal")
	}
	if _, statErr := os.Stat(filepath.Dir(filepath.Dir(dest))); !os.IsNotExist(statErr) {
		t.Errorf("refusal created directories, want nothing left behind")
	}
	if _, statErr := os.Stat(dest + ".partial"); !os.IsNotExist(statErr) {
		t.Errorf("refusal created a partial file, want nothing written")
	}
	if opened.Load() != 0 {
		t.Errorf("dump reader opened %d times, want 0", opened.Load())
	}
}
