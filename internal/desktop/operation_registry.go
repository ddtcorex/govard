package desktop

import (
	"context"
	"sort"
	"sync"
	"time"
)

// OpInfo describes one cancellable desktop operation. WS-A/B/C/D/E register
// their long ops here so the UI can list and cancel them.
type OpInfo struct {
	ID        string
	Project   string
	Kind      string
	StartedAt time.Time
}

type operationEntry struct {
	info   OpInfo
	cancel context.CancelFunc
}

// operationRegistry tracks running desktop operations by id. It holds
// only running ops: FinishOp and CancelOp delete the entry, and finished
// ids are never retained, so repeated register/finish cycles leave no
// residue (history persistence belongs to a later task).
var operationRegistry = struct {
	sync.Mutex
	running map[string]operationEntry
}{running: map[string]operationEntry{}}

// RegisterOp records a cancellable op, stamping StartedAt. Registering an
// id that is already running is rejected (false) and the original entry is
// kept, so a duplicate can never displace a cancel func and orphan its
// context. Callers must FinishOp or CancelOp the id first.
func RegisterOp(id string, project string, kind string, cancel context.CancelFunc) bool {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	if _, ok := operationRegistry.running[id]; ok {
		return false
	}
	operationRegistry.running[id] = operationEntry{
		info:   OpInfo{ID: id, Project: project, Kind: kind, StartedAt: time.Now()},
		cancel: cancel,
	}
	return true
}

// CancelOp removes a running op and fires its cancel func exactly once
// (after releasing the mutex, so a cancel func that re-enters the registry
// cannot deadlock). It reports true when an op was cancelled and false when
// no running op exists for the id (unknown or already finished); callers
// treat that as success and show no error toast.
func CancelOp(id string) bool {
	operationRegistry.Lock()
	entry, ok := operationRegistry.running[id]
	if ok {
		delete(operationRegistry.running, id)
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
// func. A later CancelOp for the id reports false (nothing running), which
// callers treat as a harmless no-op success.
func FinishOp(id string) bool {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	if _, ok := operationRegistry.running[id]; !ok {
		return false
	}
	delete(operationRegistry.running, id)
	return true
}

// ListOps returns a snapshot of running ops ordered by start time.
func ListOps() []OpInfo {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	ops := make([]OpInfo, 0, len(operationRegistry.running))
	for _, entry := range operationRegistry.running {
		ops = append(ops, entry.info)
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].StartedAt.Equal(ops[j].StartedAt) {
			return ops[i].ID < ops[j].ID
		}
		return ops[i].StartedAt.Before(ops[j].StartedAt)
	})
	return ops
}

// resetOperationRegistry clears running ops. Tests only.
func resetOperationRegistry() {
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	operationRegistry.running = map[string]operationEntry{}
}
