package desktop

import (
	"context"
	"os"
	"sort"
	"sync"
	"time"
)

// OpInfo describes one cancellable desktop operation. WS-A/B/C/D/E register
// their long ops here so the UI can list and cancel them. Project is the
// canonical registry ID (see canonicalProjectIDForRegistry), never raw
// caller input, so guard, list, and history key one project under every
// identifier spelling. Args are scrubbed at register (see
// scrubToolHistoryArgv): the running snapshot never carries raw secrets.
type OpInfo struct {
	ID        string
	Project   string
	Kind      string
	Args      []string
	StartedAt time.Time
}

type operationEntry struct {
	info   OpInfo
	cancel context.CancelFunc
}

// operationRegistry tracks running desktop operations by id. It holds
// only running ops: FinishOp and CancelOp delete the entry and append one
// terminal record to history, so repeated register/finish cycles leave no
// residue in the live map while history keeps the durable trail (capped;
// see operation_history.go).
var operationRegistry = struct {
	sync.Mutex
	running map[string]operationEntry
}{running: map[string]operationEntry{}}

// RegisterOp records a cancellable op, stamping StartedAt. Registering an
// id that is already running is rejected (false) and the original entry is
// kept, so a duplicate can never displace a cancel func and orphan its
// context. Callers must FinishOp or CancelOp the id first. The project is
// stored canonicalized; use RegisterOpWithArgs when replay args matter.
func RegisterOp(id string, project string, kind string, cancel context.CancelFunc) bool {
	return RegisterOpWithArgs(id, project, kind, nil, cancel)
}

// CancelOp removes a running op, records one cancelled terminal record,
// and fires its cancel func exactly once (after releasing the mutex, so a
// cancel func that re-enters the registry cannot deadlock). It reports
// true when an op was cancelled and false when no running op exists for
// the id (unknown or already finished); callers treat that as success and
// show no error toast. Only the side that claims the running entry
// records, so a cancel racing a finish yields exactly one terminal state.

// RegisterDBTransferOp registers one database dump/import op under the
// backend single-flight rule: at most one running db transfer per project.
// The scan and the insert hold the same mutex, so two concurrent starts
// cannot both succeed (the frontend guard is UX only; a double-click race
// would otherwise orphan the first op). A refusal names the EXISTING running
// op id, never the new one, so the error tells the user what to cancel.
// Different projects never block each other; non-transfer ops are ignored.
// After FinishOp/CancelOp the project accepts a new transfer again.
func RegisterDBTransferOp(id string, project string, kind string, cancel context.CancelFunc) (bool, string) {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	want := normalizeDBProject(project)
	for _, entry := range operationRegistry.running {
		if entry.info.Kind != dbTransferOpKindDump && entry.info.Kind != dbTransferOpKindImport {
			continue
		}
		if normalizeDBProject(entry.info.Project) != want {
			continue
		}
		return false, entry.info.ID
	}
	if _, ok := operationRegistry.running[id]; ok {
		return false, id
	}
	operationRegistry.running[id] = operationEntry{
		info:   OpInfo{ID: id, Project: project, Kind: kind, StartedAt: time.Now()},
		cancel: cancel,
	}
	return true, ""
}
func CancelOp(id string) bool {
	operationRegistry.Lock()
	entry, ok := operationRegistry.running[id]
	if ok {
		delete(operationRegistry.running, id)
		now := time.Now()
		// Terminal-append BEFORE sidecar-delete (see FinishOpWithStatus):
		// a crash between the two leaves a state recovery dedupes instead
		// of mislabels.
		appendHistoryRecord(OpRecord{
			ID:         entry.info.ID,
			Project:    entry.info.Project,
			Kind:       entry.info.Kind,
			Args:       scrubToolHistoryArgv(entry.info.Args),
			Status:     OpStatusCancelled,
			StartedAt:  entry.info.StartedAt,
			EndedAt:    now,
			DurationMS: now.Sub(entry.info.StartedAt).Milliseconds(),
		})
		persistInflightLocked()
	}
	operationRegistry.Unlock()

	if !ok {
		return false
	}
	if entry.cancel != nil {
		entry.cancel()
	}
	return true
}

// FinishOp unregisters a normally completed op without firing its cancel
// func and records one completed terminal record. A later CancelOp for the
// id reports false (nothing running), which callers treat as a harmless
// no-op success.
func FinishOp(id string) bool {
	return FinishOpWithStatus(id, OpStatusCompleted, "")
}

// ListOps returns a snapshot of running ops ordered by start time.
func ListOps() []OpInfo {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	ops := make([]OpInfo, 0, len(operationRegistry.running))
	for _, entry := range operationRegistry.running {
		info := entry.info
		info.Args = append([]string{}, info.Args...)
		ops = append(ops, info)
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].StartedAt.Equal(ops[j].StartedAt) {
			return ops[i].ID < ops[j].ID
		}
		return ops[i].StartedAt.Before(ops[j].StartedAt)
	})
	return ops
}

// isOpRegistered reports whether id is currently running. The tool runner's
// publish stage uses it to drop stream lines from cancelled or superseded
// ops so a fast rerun never mixes tails.
func isOpRegistered(id string) bool {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	_, ok := operationRegistry.running[id]
	return ok
}

// resetOperationRegistry clears running ops and drops persisted history,
// so registry tests stay hermetic. Tests only.
func resetOperationRegistry() {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	operationRegistry.running = map[string]operationEntry{}
	_ = os.Remove(operationHistoryPath())
	_ = os.Remove(operationInflightPath())
}
