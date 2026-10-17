package desktop

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// OpRetryFunc replays one op kind through its original service path: the
// handler receives the stored terminal record's project and exact args and
// runs the same validation as the first run, not a shortcut. It reports the
// replayed run's op id. Service paths register their entry point with
// RegisterOpRetryHandler; RetryOp refuses kinds with no handler instead of
// inventing a replay.
type OpRetryFunc func(ctx context.Context, project string, args []string) (string, error)

// opRetryHandlers maps lower-cased op kind to its replay entry point. Kinds
// are matched case-insensitively because stored kinds come from many
// callers; the stored kind itself is never rewritten.
var opRetryHandlers = struct {
	sync.Mutex
	handlers map[string]OpRetryFunc
}{handlers: map[string]OpRetryFunc{}}

// OperationService is the Operation Center backend: the visible home for
// long ops. It lists running ops, reads history, cancels the tracked run,
// and retries failed ops by replaying their stored args through the
// original service path.
type OperationService struct {
	serviceBase
}

// NewOperationService builds the bound OperationService.
func NewOperationService() *OperationService {
	return &OperationService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// RegisterOpRetryHandler records the replay entry point for one op kind. A
// nil handler or a blank kind is ignored. Registering a kind twice replaces
// the entry, so tests and service init can both claim kinds without error.
func RegisterOpRetryHandler(kind string, handler OpRetryFunc) {
	key := strings.ToLower(strings.TrimSpace(kind))
	if key == "" || handler == nil {
		return
	}
	opRetryHandlers.Lock()
	defer opRetryHandlers.Unlock()
	opRetryHandlers.handlers[key] = handler
}

// ResetOpRetryHandlersForTest drops registered replay handlers and clears
// in-progress retry marks so retry tests stay hermetic. Tests only.
func ResetOpRetryHandlersForTest() {
	opRetryHandlers.Lock()
	defer opRetryHandlers.Unlock()
	opRetryHandlers.handlers = map[string]OpRetryFunc{}
	opRetryInflight.Lock()
	defer opRetryInflight.Unlock()
	opRetryInflight.ids = map[string]bool{}
}

// opRetryInflight marks record IDs with a dispatched retry still running, so
// a concurrent RetryOp for the same record is refused instead of dispatching
// a second run. It has its own mutex: the mark is taken while holding the
// registry lock and cleared after dispatch without it, and no path ever
// takes the two locks in the opposite order.
var opRetryInflight = struct {
	sync.Mutex
	ids map[string]bool
}{ids: map[string]bool{}}

// RetryOp replays a failed op's stored args through its original service
// path and reports the replayed run's op id. Only failed terminal records
// are retryable: succeeded, cancelled, and interrupted records are refused,
// and an id with no terminal record is an error, never an invented run.
//
// Fail-closed scrub enforcement (deferred from Task 2): when the stored
// args carry a "***" scrub placeholder the replay is refused with an
// actionable error asking for a fresh run with fresh credentials. The
// placeholder no longer authenticates, so replaying it could only fail
// opaquely deep in the service path. The refusal never prints the args.
//
// Terminal attribution: RetryOp never rewrites the old terminal record.
// The handler registers its replayed run through RegisterOpWithArgs under a
// fresh id (a new generation), so finish/cancel attribution keeps the
// existing exactly-once claim semantics and a cancel always targets the
// tracked (currently running) generation by its own id.
func RetryOp(recordID string) (string, error) {
	id := strings.TrimSpace(recordID)
	if id == "" {
		return "", fmt.Errorf("retry: record unavailable; pick a failed entry from history")
	}
	operationRegistry.Lock()
	record, ok := newestTerminalRecordLocked(id)
	if !ok {
		operationRegistry.Unlock()
		return "", fmt.Errorf("retry: unknown operation %q: no terminal record to replay", id)
	}
	if record.Status != OpStatusFailed {
		operationRegistry.Unlock()
		return "", fmt.Errorf("retry: operation %q ended as %s; only failed operations can be retried", id, record.Status)
	}
	if opArgsContainScrubMarker(record.Args) {
		operationRegistry.Unlock()
		return "", fmt.Errorf("retry: operation %q stored redacted secrets; re-run it with fresh credentials instead of replaying", id)
	}
	opRetryHandlers.Lock()
	handler, ok := opRetryHandlers.handlers[strings.ToLower(strings.TrimSpace(record.Kind))]
	opRetryHandlers.Unlock()
	if !ok {
		operationRegistry.Unlock()
		return "", fmt.Errorf("retry: operation kind %q has no replay path; re-run it from its original view", record.Kind)
	}
	opRetryInflight.Lock()
	if opRetryInflight.ids[id] {
		opRetryInflight.Unlock()
		operationRegistry.Unlock()
		return "", fmt.Errorf("retry: operation %q already has a retry in progress; wait for the current run to finish before retrying", id)
	}
	opRetryInflight.ids[id] = true
	opRetryInflight.Unlock()
	// The unmark is deferred before unlock+dispatch so it runs on every
	// path, including a handler panic: otherwise a panic would wedge the
	// record into refusing retries forever. A panic itself is NOT recovered
	// here; it propagates to the service-level recovery boundary, which owns
	// recovery.
	defer func() {
		opRetryInflight.Lock()
		delete(opRetryInflight.ids, id)
		opRetryInflight.Unlock()
	}()
	operationRegistry.Unlock()

	// INVARIANT: handlers run OUTSIDE the registry lock. A handler replays
	// through the original service path, which lists and registers ops like
	// any first run; dispatching under the lock would deadlock such a
	// handler and stall ListOps for every other caller. The in-progress mark
	// above is what keeps a concurrent retry from dispatching twice.
	replayed, err := handler(context.Background(), record.Project, append([]string{}, record.Args...))
	if err != nil {
		return "", fmt.Errorf("retry: replay of operation %q failed: %w", id, err)
	}
	return replayed, nil
}

// newestTerminalRecordLocked finds the newest terminal record for id.
// History is oldest-first on disk, and ids recycle across register cycles,
// so the newest match is the latest generation. Callers hold the registry
// mutex.
func newestTerminalRecordLocked(id string) (OpRecord, bool) {
	records := readHistoryRecords()
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].ID == id {
			return records[i], true
		}
	}
	return OpRecord{}, false
}

// ListOps snapshots running ops ordered by start time.
func (s *OperationService) ListOps() []OpInfo {
	return ListOps()
}

// ListHistory returns persisted terminal records, newest first, capped at
// limit (limit <= 0 means all).
func (s *OperationService) ListHistory(limit int) []OpRecord {
	return ListHistory(limit)
}

// CancelOp cancels the tracked running op, firing its cancel func exactly
// once. It reports false when no running op exists for the id (unknown or
// already finished); callers treat that as a harmless no-op success, so a
// cancel arriving as the op completes stays exactly-once with no error.
func (s *OperationService) CancelOp(id string) bool {
	return CancelOp(id)
}

// RetryOp replays a failed op's stored args through its original service
// path. See the package function for the failed-only and fail-closed scrub
// contract.
func (s *OperationService) RetryOp(id string) (string, error) {
	return RetryOp(id)
}
