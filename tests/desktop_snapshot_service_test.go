package tests

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

// Desktop snapshot management wraps the existing snapshot engine: listing
// shows size and creation time, create/export run as cancellable ops, and
// restore is two-step only. There is no one-shot restore: ApplySnapshotRestore
// requires the preview token from PreviewSnapshotRestore plus an explicit
// confirm flag, mirroring the onboarding ConfirmFolderOverride convention.

const snapshotServiceTestRoot = "/tmp/sample-project"

func prepareSnapshotServiceTest(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	t.Cleanup(desktop.ResetOperationRegistryForTest)
}

func stubSnapshotContext(t *testing.T) {
	t.Helper()
	restore := desktop.SetResolveSnapshotContextForDesktopForTest(
		func(project string) (string, engine.Config, error) {
			if strings.TrimSpace(project) != "sample-project" {
				t.Errorf("resolve project = %q, want sample-project", project)
			}
			return snapshotServiceTestRoot, engine.Config{ProjectName: "sample-project"}, nil
		},
	)
	t.Cleanup(restore)
}

func stubSnapshotList(t *testing.T, metas []engine.SnapshotMetadata) {
	t.Helper()
	restore := desktop.SetListSnapshotsForDesktopForTest(
		func(root string) ([]engine.SnapshotMetadata, error) {
			if root != snapshotServiceTestRoot {
				t.Errorf("list root = %q, want %q", root, snapshotServiceTestRoot)
			}
			return metas, nil
		},
	)
	t.Cleanup(restore)
}

func waitForSnapshotCondition(t *testing.T, what string, cond func() bool) {
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

func snapshotCompletedEvents(fake *desktop.FakePlatform) []desktop.SnapshotOpCompleted {
	var out []desktop.SnapshotOpCompleted
	for _, ev := range fake.EventsNamed("snapshot:completed") {
		payload, ok := ev.(desktop.SnapshotOpCompleted)
		if !ok {
			continue
		}
		out = append(out, payload)
	}
	return out
}

func snapshotFailedEvents(fake *desktop.FakePlatform) []desktop.SnapshotOpFailed {
	var out []desktop.SnapshotOpFailed
	for _, ev := range fake.EventsNamed("snapshot:failed") {
		payload, ok := ev.(desktop.SnapshotOpFailed)
		if !ok {
			continue
		}
		out = append(out, payload)
	}
	return out
}

func sampleSnapshotMetas() []engine.SnapshotMetadata {
	return []engine.SnapshotMetadata{
		{
			Name:      "nightly",
			CreatedAt: time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC),
			Framework: "magento2",
			DB:        true,
			Media:     true,
			SizeBytes: 2048,
		},
	}
}

func TestSnapshotService_RestoreWithoutPreviewTokenRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", "", true, false, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(empty token) error = nil, want preview refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "preview") {
		t.Fatalf("ApplySnapshotRestore(empty token) error = %q, want it to demand a preview token", err.Error())
	}
	if got := restores.Load(); got != 0 {
		t.Fatalf("engine restores = %d, want 0: refusal must happen before the engine", got)
	}
}

func TestSnapshotService_RestoreWithUnknownTokenRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	_, err := svc.ApplySnapshotRestore("sample-project", "nightly", "snap-restore-deadbeefdeadbeefdeadbeefdeadbeef", true, false, false)
	if err == nil {
		t.Fatalf("ApplySnapshotRestore(unknown token) error = nil, want refusal")
	}
	if got := restores.Load(); got != 0 {
		t.Fatalf("engine restores = %d, want 0", got)
	}
}

func TestSnapshotService_RestoreWithoutConfirmRefusedAndTokenSurvives(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, false, false, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(confirm=false) error = nil, want confirm refusal")
	}
	if got := restores.Load(); got != 0 {
		t.Fatalf("engine restores = %d, want 0", got)
	}
	// The refused apply must not consume the token: confirming now still works.
	opID, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false)
	if err != nil {
		t.Fatalf("ApplySnapshotRestore(confirmed retry) error = %v, want nil", err)
	}
	if strings.TrimSpace(opID) == "" {
		t.Fatalf("ApplySnapshotRestore(confirmed retry) opID is empty, want an op id")
	}
	waitForSnapshotCondition(t, "restore completion", func() bool {
		return len(snapshotCompletedEvents(fake)) == 1
	})
	if got := restores.Load(); got != 1 {
		t.Fatalf("engine restores = %d, want 1", got)
	}
}

func TestSnapshotService_PreviewApplyRoundTripRestores(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	var sawTarget desktop.SnapshotRestoreTarget
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(_ context.Context, target desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			sawTarget = target
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	if strings.TrimSpace(preview.PreviewToken) == "" {
		t.Fatalf("preview token is empty, want a token")
	}
	if !preview.WillReplaceDB || !preview.WillReplaceMedia {
		t.Fatalf("preview = %+v, want both DB and media marked for replacement", preview)
	}
	if !strings.Contains(preview.Summary, "nightly") {
		t.Fatalf("preview summary = %q, want it to name the snapshot", preview.Summary)
	}
	opID, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false)
	if err != nil {
		t.Fatalf("ApplySnapshotRestore error = %v, want nil", err)
	}
	if strings.TrimSpace(opID) == "" {
		t.Fatalf("opID is empty, want a registered op id")
	}
	waitForSnapshotCondition(t, "restore completion", func() bool {
		return len(snapshotCompletedEvents(fake)) == 1
	})
	if got := restores.Load(); got != 1 {
		t.Fatalf("engine restores = %d, want 1", got)
	}
	if sawTarget.Name != "nightly" || sawTarget.Root != snapshotServiceTestRoot {
		t.Fatalf("restore target = %+v, want nightly at %q", sawTarget, snapshotServiceTestRoot)
	}
	done := snapshotCompletedEvents(fake)[0]
	if done.OpID != opID {
		t.Errorf("completed OpID = %q, want %q", done.OpID, opID)
	}
}

func TestSnapshotService_PreviewTokenSingleUse(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false); err != nil {
		t.Fatalf("first ApplySnapshotRestore error = %v, want nil", err)
	}
	waitForSnapshotCondition(t, "restore completion", func() bool {
		return len(snapshotCompletedEvents(fake)) == 1
	})
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false); err == nil {
		t.Fatalf("second ApplySnapshotRestore error = nil, want single-use refusal")
	}
	if got := restores.Load(); got != 1 {
		t.Fatalf("engine restores = %d, want 1: replay must never reach the engine", got)
	}
}

func TestSnapshotService_PreviewTokenBoundToSnapshotAndOptions(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, append(sampleSnapshotMetas(), engine.SnapshotMetadata{
		Name:      "weekly",
		CreatedAt: time.Date(2026, 10, 10, 8, 30, 0, 0, time.UTC),
		DB:        true,
		Media:     false,
		SizeBytes: 1024,
	}))
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	if _, err := svc.ApplySnapshotRestore("sample-project", "weekly", preview.PreviewToken, true, false, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(other snapshot) error = nil, want binding refusal")
	}
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, true, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(other options) error = nil, want binding refusal")
	}
	if got := restores.Load(); got != 0 {
		t.Fatalf("engine restores = %d, want 0", got)
	}
}

func TestSnapshotService_PreviewMissingSnapshotRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, nil)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	if _, err := svc.PreviewSnapshotRestore("sample-project", "ghost", false, false); err == nil {
		t.Fatalf("PreviewSnapshotRestore(ghost) error = nil, want not-found")
	}
}

func TestSnapshotService_PreviewConflictingScopeRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	if _, err := svc.PreviewSnapshotRestore("sample-project", "nightly", true, true); err == nil {
		t.Fatalf("PreviewSnapshotRestore(db-only+media-only) error = nil, want refusal")
	}
}

func TestSnapshotService_ListShowsSizeAndCreated(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())

	svc := desktop.NewSnapshotService()
	infos, err := svc.ListSnapshots("sample-project")
	if err != nil {
		t.Fatalf("ListSnapshots error = %v, want nil", err)
	}
	if len(infos) != 1 {
		t.Fatalf("ListSnapshots len = %d, want 1", len(infos))
	}
	info := infos[0]
	if info.Name != "nightly" {
		t.Errorf("Name = %q, want nightly", info.Name)
	}
	if info.SizeBytes != 2048 {
		t.Errorf("SizeBytes = %d, want 2048", info.SizeBytes)
	}
	if strings.TrimSpace(info.SizeHuman) == "" {
		t.Errorf("SizeHuman is empty, want a human size")
	}
	if info.CreatedAt != "2026-10-15 08:30:00" {
		t.Errorf("CreatedAt = %q, want 2026-10-15 08:30:00", info.CreatedAt)
	}
	if !info.HasDB || !info.HasMedia {
		t.Errorf("HasDB/HasMedia = %v/%v, want true/true", info.HasDB, info.HasMedia)
	}
}

func TestSnapshotService_ListSanitizesNames(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, []engine.SnapshotMetadata{
		{Name: "\x1b[31mnightly\x1b[0m\x07", SizeBytes: 10},
	})

	svc := desktop.NewSnapshotService()
	infos, err := svc.ListSnapshots("sample-project")
	if err != nil {
		t.Fatalf("ListSnapshots error = %v, want nil", err)
	}
	if len(infos) != 1 {
		t.Fatalf("ListSnapshots len = %d, want 1", len(infos))
	}
	if strings.Contains(infos[0].Name, "\x1b") || strings.Contains(infos[0].Name, "\x07") {
		t.Fatalf("Name = %q, want ANSI escapes and control bytes stripped", infos[0].Name)
	}
	if !strings.Contains(infos[0].Name, "nightly") {
		t.Fatalf("Name = %q, want it to keep nightly", infos[0].Name)
	}
}

func TestSnapshotService_ExportWritesToDialogPath(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var exports atomic.Int64
	var sawTarget string
	restore := desktop.SetExportSnapshotForDesktopForTest(
		func(_ context.Context, root string, name string, target string) error {
			exports.Add(1)
			sawTarget = target
			if root != snapshotServiceTestRoot {
				t.Errorf("export root = %q, want %q", root, snapshotServiceTestRoot)
			}
			if name != "nightly" {
				t.Errorf("export name = %q, want nightly", name)
			}
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	dest := filepath.Join(t.TempDir(), "nightly.tar.gz")
	opID, err := svc.ExportSnapshot("sample-project", "nightly", dest)
	if err != nil {
		t.Fatalf("ExportSnapshot error = %v, want nil", err)
	}
	if strings.TrimSpace(opID) == "" {
		t.Fatalf("opID is empty, want a registered op id")
	}
	waitForSnapshotCondition(t, "export completion", func() bool {
		return len(snapshotCompletedEvents(fake)) == 1
	})
	if got := exports.Load(); got != 1 {
		t.Fatalf("engine exports = %d, want 1", got)
	}
	if sawTarget != dest {
		t.Fatalf("export target = %q, want the dialog path %q", sawTarget, dest)
	}
	done := snapshotCompletedEvents(fake)[0]
	if !strings.Contains(done.Path, "nightly.tar.gz") {
		t.Errorf("completed Path = %q, want the dialog path", done.Path)
	}
}

func TestSnapshotService_ExportEmptyPathRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var exports atomic.Int64
	restore := desktop.SetExportSnapshotForDesktopForTest(
		func(context.Context, string, string, string) error {
			exports.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	svc := desktop.NewSnapshotService()
	if _, err := svc.ExportSnapshot("sample-project", "nightly", "   "); err == nil {
		t.Fatalf("ExportSnapshot(empty path) error = nil, want refusal")
	}
	if got := exports.Load(); got != 0 {
		t.Fatalf("engine exports = %d, want 0", got)
	}
}

func TestSnapshotService_CreateRunsEngineAsOp(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	var creates atomic.Int64
	var sawName string
	restore := desktop.SetCreateSnapshotForDesktopForTest(
		func(_ context.Context, root string, _ engine.Config, name string) (string, error) {
			creates.Add(1)
			sawName = name
			if root != snapshotServiceTestRoot {
				t.Errorf("create root = %q, want %q", root, snapshotServiceTestRoot)
			}
			return root + "/nightly", nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	opID, err := svc.CreateSnapshot("sample-project", "nightly")
	if err != nil {
		t.Fatalf("CreateSnapshot error = %v, want nil", err)
	}
	if strings.TrimSpace(opID) == "" {
		t.Fatalf("opID is empty, want a registered op id")
	}
	waitForSnapshotCondition(t, "create completion", func() bool {
		return len(snapshotCompletedEvents(fake)) == 1
	})
	if got := creates.Load(); got != 1 {
		t.Fatalf("engine creates = %d, want 1", got)
	}
	if sawName != "nightly" {
		t.Fatalf("create name = %q, want nightly", sawName)
	}
}

func TestSnapshotService_InvalidSnapshotNameRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())

	svc := desktop.NewSnapshotService()
	if _, err := svc.PreviewSnapshotRestore("sample-project", "../escape", false, false); err == nil {
		t.Fatalf("PreviewSnapshotRestore(../escape) error = nil, want refusal")
	}
	if _, err := svc.CreateSnapshot("sample-project", "a/b"); err == nil {
		t.Fatalf("CreateSnapshot(a/b) error = nil, want refusal")
	}
}

func TestSnapshotService_CancelUnknownOpIsFalse(t *testing.T) {
	prepareSnapshotServiceTest(t)

	svc := desktop.NewSnapshotService()
	if svc.CancelSnapshotOp("snapshot-restore-9999") {
		t.Fatalf("CancelSnapshotOp(unknown) = true, want false")
	}
}

func TestSnapshotService_CancelMidRestoreKillsOp(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	started := make(chan struct{}, 1)
	sawCancel := make(chan struct{}, 1)
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(ctx context.Context, _ desktop.SnapshotRestoreTarget) error {
			started <- struct{}{}
			<-ctx.Done()
			sawCancel <- struct{}{}
			return ctx.Err()
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	opID, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false)
	if err != nil {
		t.Fatalf("ApplySnapshotRestore error = %v, want nil", err)
	}
	waitForSnapshotCondition(t, "restore start", func() bool {
		return len(started) == 1
	})
	if !svc.CancelSnapshotOp(opID) {
		t.Fatalf("CancelSnapshotOp(running) = false, want true")
	}
	waitForSnapshotCondition(t, "restore cancel", func() bool {
		return len(sawCancel) == 1
	})
	waitForSnapshotCondition(t, "cancel failure event", func() bool {
		return len(snapshotFailedEvents(fake)) >= 1
	})
	if len(snapshotCompletedEvents(fake)) != 0 {
		t.Fatalf("completed events = %d, want 0 after cancel", len(snapshotCompletedEvents(fake)))
	}
	failed := snapshotFailedEvents(fake)[0]
	if !strings.Contains(failed.Message, "may run to completion") {
		t.Fatalf("failed message = %q, want it to state the underlying op may run to completion", failed.Message)
	}
	if strings.Contains(failed.Message, "cancelled by user") {
		t.Fatalf("failed message = %q, want no undelivered cancellation claim", failed.Message)
	}
}

func TestSnapshotService_ExportDirectoryAsDestRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var exports atomic.Int64
	restore := desktop.SetExportSnapshotForDesktopForTest(
		func(context.Context, string, string, string) error {
			exports.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	svc := desktop.NewSnapshotService()
	dir := t.TempDir()
	if _, err := svc.ExportSnapshot("sample-project", "nightly", dir); err == nil {
		t.Fatalf("ExportSnapshot(directory) error = nil, want refusal")
	}
	if got := exports.Load(); got != 0 {
		t.Fatalf("engine exports = %d, want 0", got)
	}
}

func TestSnapshotService_ExportMissingParentRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var exports atomic.Int64
	restore := desktop.SetExportSnapshotForDesktopForTest(
		func(context.Context, string, string, string) error {
			exports.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	svc := desktop.NewSnapshotService()
	missing := filepath.Join(t.TempDir(), "no-such-dir", "nightly.tar.gz")
	if _, err := svc.ExportSnapshot("sample-project", "nightly", missing); err == nil {
		t.Fatalf("ExportSnapshot(missing parent) error = nil, want refusal")
	}
	if got := exports.Load(); got != 0 {
		t.Fatalf("engine exports = %d, want 0", got)
	}
}

func TestSnapshotService_ExportValidPickerPathPasses(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var exports atomic.Int64
	restore := desktop.SetExportSnapshotForDesktopForTest(
		func(context.Context, string, string, string) error {
			exports.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	dest := filepath.Join(t.TempDir(), "nightly.tar.gz")
	opID, err := svc.ExportSnapshot("sample-project", "nightly", dest)
	if err != nil {
		t.Fatalf("ExportSnapshot(valid path) error = %v, want nil", err)
	}
	if strings.TrimSpace(opID) == "" {
		t.Fatalf("opID is empty, want a registered op id")
	}
	waitForSnapshotCondition(t, "export completion", func() bool {
		return len(snapshotCompletedEvents(fake)) == 1
	})
	if got := exports.Load(); got != 1 {
		t.Fatalf("engine exports = %d, want 1", got)
	}
}

func TestSnapshotService_CancelMessageStatesTruth(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	started := make(chan struct{}, 1)
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(ctx context.Context, _ desktop.SnapshotRestoreTarget) error {
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	opID, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false)
	if err != nil {
		t.Fatalf("ApplySnapshotRestore error = %v, want nil", err)
	}
	waitForSnapshotCondition(t, "restore start", func() bool {
		return len(started) == 1
	})
	if !svc.CancelSnapshotOp(opID) {
		t.Fatalf("CancelSnapshotOp(running) = false, want true")
	}
	waitForSnapshotCondition(t, "cancel failure events", func() bool {
		return len(snapshotFailedEvents(fake)) >= 1
	})
	for _, ev := range snapshotFailedEvents(fake) {
		if strings.Contains(ev.Message, "cancelled by user") {
			t.Fatalf("cancel message = %q, want no undelivered cancellation claim", ev.Message)
		}
		if !strings.Contains(ev.Message, "may run to completion") {
			t.Fatalf("cancel message = %q, want the may-run-to-completion truth", ev.Message)
		}
	}
}

func TestSnapshotService_PreviewTokenExpiredRefused(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)
	now := time.Now()
	restoreClock := desktop.SetSnapshotRestoreNowForTest(func() time.Time { return now })
	t.Cleanup(restoreClock)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	now = now.Add(16 * time.Minute)
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(expired token) error = nil, want expiry refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "expired") {
		t.Fatalf("ApplySnapshotRestore(expired token) error = %q, want it to name expiry", err.Error())
	}
	if got := restores.Load(); got != 0 {
		t.Fatalf("engine restores = %d, want 0", got)
	}
}

func TestSnapshotService_PreviewTokenEvictionBoundsMap(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())

	svc := desktop.NewSnapshotService()
	const total = 33
	tokens := make([]string, 0, total)
	for i := 0; i < total; i++ {
		preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
		if err != nil {
			t.Fatalf("PreviewSnapshotRestore #%d error = %v, want nil", i, err)
		}
		tokens = append(tokens, preview.PreviewToken)
	}
	if got := desktop.SnapshotRestorePreviewCountForTest(); got != 32 {
		t.Fatalf("preview store size = %d, want bounded at 32", got)
	}
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)
	fake := &desktop.FakePlatform{}
	applySvc := desktop.NewSnapshotServiceWithPlatform(fake)
	if _, err := applySvc.ApplySnapshotRestore("sample-project", "nightly", tokens[0], true, false, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(evicted token) error = nil, want refusal")
	}
	if got := restores.Load(); got != 0 {
		t.Fatalf("engine restores = %d, want 0", got)
	}
}

func TestSnapshotService_ApplyContentionKeepsToken(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)
	restoreReg := desktop.SetRegisterSnapshotOpForDesktopForTest(
		func(string, string, string, context.CancelFunc) bool { return false },
	)
	t.Cleanup(restoreReg)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(contention) error = nil, want registration refusal")
	}
	// Contention must not burn the token: with registration restored, the
	// same token still applies.
	restoreReg()
	opID, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false)
	if err != nil {
		t.Fatalf("ApplySnapshotRestore(retry) error = %v, want nil: contention must not consume the token", err)
	}
	if strings.TrimSpace(opID) == "" {
		t.Fatalf("opID is empty, want a registered op id")
	}
	waitForSnapshotCondition(t, "restore completion", func() bool {
		return len(snapshotCompletedEvents(fake)) == 1
	})
	if got := restores.Load(); got != 1 {
		t.Fatalf("engine restores = %d, want 1", got)
	}
}

func TestSnapshotService_CancelMidCreateKillsOp(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	started := make(chan struct{}, 1)
	sawCancel := make(chan struct{}, 1)
	restore := desktop.SetCreateSnapshotForDesktopForTest(
		func(ctx context.Context, _ string, _ engine.Config, _ string) (string, error) {
			started <- struct{}{}
			<-ctx.Done()
			sawCancel <- struct{}{}
			return "", ctx.Err()
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	opID, err := svc.CreateSnapshot("sample-project", "nightly")
	if err != nil {
		t.Fatalf("CreateSnapshot error = %v, want nil", err)
	}
	waitForSnapshotCondition(t, "create start", func() bool {
		return len(started) == 1
	})
	if !svc.CancelSnapshotOp(opID) {
		t.Fatalf("CancelSnapshotOp(running) = false, want true")
	}
	waitForSnapshotCondition(t, "create cancel", func() bool {
		return len(sawCancel) == 1
	})
	waitForSnapshotCondition(t, "cancel failure event", func() bool {
		return len(snapshotFailedEvents(fake)) >= 1
	})
	if len(snapshotCompletedEvents(fake)) != 0 {
		t.Fatalf("completed events = %d, want 0 after cancel", len(snapshotCompletedEvents(fake)))
	}
}

func TestSnapshotService_CancelMidExportKillsOp(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	started := make(chan struct{}, 1)
	sawCancel := make(chan struct{}, 1)
	restore := desktop.SetExportSnapshotForDesktopForTest(
		func(ctx context.Context, _ string, _ string, _ string) error {
			started <- struct{}{}
			<-ctx.Done()
			sawCancel <- struct{}{}
			return ctx.Err()
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	dest := filepath.Join(t.TempDir(), "nightly.tar.gz")
	opID, err := svc.ExportSnapshot("sample-project", "nightly", dest)
	if err != nil {
		t.Fatalf("ExportSnapshot error = %v, want nil", err)
	}
	waitForSnapshotCondition(t, "export start", func() bool {
		return len(started) == 1
	})
	if !svc.CancelSnapshotOp(opID) {
		t.Fatalf("CancelSnapshotOp(running) = false, want true")
	}
	waitForSnapshotCondition(t, "export cancel", func() bool {
		return len(sawCancel) == 1
	})
	waitForSnapshotCondition(t, "cancel failure event", func() bool {
		return len(snapshotFailedEvents(fake)) >= 1
	})
	if len(snapshotCompletedEvents(fake)) != 0 {
		t.Fatalf("completed events = %d, want 0 after cancel", len(snapshotCompletedEvents(fake)))
	}
}

func TestSnapshotService_ConcurrentDoubleApplyRunsEngineOnce(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	// Rendezvous inside project resolution, installed only now so the
	// preview above never trips the barrier: the two racers must both get
	// past the token check before either runs the engine. The stub holds
	// the first arrival until the second arrives (buggy code: both pass the
	// check) or a timeout fires (fixed code: the loser never arrives).
	var arrivals atomic.Int64
	restoreCtx := desktop.SetResolveSnapshotContextForDesktopForTest(
		func(project string) (string, engine.Config, error) {
			arrivals.Add(1)
			deadline := time.Now().Add(2 * time.Second)
			for arrivals.Load() < 2 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			return snapshotServiceTestRoot, engine.Config{ProjectName: "sample-project"}, nil
		},
	)
	t.Cleanup(restoreCtx)
	const racers = 2
	start := make(chan struct{})
	var wg sync.WaitGroup
	var successes atomic.Int64
	var refusals atomic.Int64
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false); err != nil {
				refusals.Add(1)
			} else {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	// successes/refusals are synchronous (Apply returns after registration);
	// the engine itself runs async, so the counter is asserted only after
	// the completion event proves the winning runner finished. Waiting
	// first also keeps the runner from outliving the test into Cleanup.
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful applies = %d, want exactly 1", got)
	}
	if got := refusals.Load(); got != racers-1 {
		t.Fatalf("refused applies = %d, want %d", got, racers-1)
	}
	waitForSnapshotCondition(t, "restore completion", func() bool {
		return len(snapshotCompletedEvents(fake)) == 1
	})
	if got := restores.Load(); got != 1 {
		t.Fatalf("engine restores = %d, want exactly 1: parallel applies must not double-run", got)
	}
}

func TestSnapshotService_ContentionRestorePreservesTTL(t *testing.T) {
	prepareSnapshotServiceTest(t)
	stubSnapshotContext(t)
	stubSnapshotList(t, sampleSnapshotMetas())
	var restores atomic.Int64
	restore := desktop.SetRestoreSnapshotForDesktopForTest(
		func(context.Context, desktop.SnapshotRestoreTarget) error {
			restores.Add(1)
			return nil
		},
	)
	t.Cleanup(restore)
	now := time.Now()
	restoreClock := desktop.SetSnapshotRestoreNowForTest(func() time.Time { return now })
	t.Cleanup(restoreClock)

	fake := &desktop.FakePlatform{}
	svc := desktop.NewSnapshotServiceWithPlatform(fake)
	preview, err := svc.PreviewSnapshotRestore("sample-project", "nightly", false, false)
	if err != nil {
		t.Fatalf("PreviewSnapshotRestore error = %v, want nil", err)
	}
	// Ten minutes pass, then registration contention refuses the apply. The
	// restored token must keep its original issue time, not the contention
	// time: six minutes later it is expired, not freshly valid.
	now = now.Add(10 * time.Minute)
	restoreReg := desktop.SetRegisterSnapshotOpForDesktopForTest(
		func(string, string, string, context.CancelFunc) bool { return false },
	)
	t.Cleanup(restoreReg)
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(contention) error = nil, want registration refusal")
	}
	restoreReg()
	now = now.Add(6 * time.Minute)
	if _, err := svc.ApplySnapshotRestore("sample-project", "nightly", preview.PreviewToken, true, false, false); err == nil {
		t.Fatalf("ApplySnapshotRestore(after TTL from original issue) error = nil, want expiry refusal")
	} else if !strings.Contains(strings.ToLower(err.Error()), "expired") {
		t.Fatalf("ApplySnapshotRestore error = %q, want it to name expiry", err.Error())
	}
	if got := restores.Load(); got != 0 {
		t.Fatalf("engine restores = %d, want 0", got)
	}
}
