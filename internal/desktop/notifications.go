package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
)

const (
	operationNotificationsPollInterval = 2 * time.Second
	operationNotificationsReadLimit    = 256
)

// operationNotifyCursorPath is the persisted notification cursor: the last
// delivered event signature, so a restart resumes without double-notifying
// and without silently losing the downtime gap. It lives in the desktop
// state dir, honoring GOVARD_HOME_DIR through conventions like history.
func operationNotifyCursorPath() string {
	return filepath.Join(operationHistoryDir(), "operation-notify-cursor")
}

// OperationNotification is emitted to the frontend when a tracked operation finishes.
type OperationNotification struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Level     string `json:"level"`
	Operation string `json:"operation"`
	Status    string `json:"status"`
	Project   string `json:"project,omitempty"`
	Timestamp string `json:"timestamp"`
	// Actions carries the toast buttons for a failed operation, resolved
	// through the humanOpMessage catalog (Retry/Open-logs). Success
	// notifications carry none.
	Actions []OpMessageAction `json:"actions,omitempty"`
	// RetryID is the event's exact source terminal record id, which the Retry
	// action replays through RetryOp. Empty when the event names no failed
	// record, in which case the toast offers no Retry button.
	RetryID string `json:"retryId,omitempty"`
}

func watchOperationNotifications(ctx context.Context, p Platform, onEvent func(), pollInterval time.Duration) {
	// Restart recovery: a persisted cursor resumes where the last run left
	// off, so the first poll delivers the downtime gap instead of
	// suppressing it. No persisted cursor means a fresh start, which keeps
	// suppressing pre-existing log history (see selectOperationEventsSince).
	cursor, havePersisted := readNotifyCursor()
	if !havePersisted {
		if events, err := engine.ReadOperationEvents(operationNotificationsReadLimit); err == nil {
			_, cursor = selectOperationEventsSince(events, cursor)
		}
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cursor = pollOperationNotifications(p, onEvent, cursor)
		}
	}
}

// pollOperationNotifications runs one watcher iteration and reports the next
// cursor: it collects new events since cursor, emits their notifications
// first, and only then persists the advanced cursor. That order is
// at-least-once delivery: a crash between emit and persist leaves the old
// cursor behind, so the restart redelivers (a visible duplicate) instead of
// silently losing a notification that was never shown.
func pollOperationNotifications(p Platform, onEvent func(), cursor string) string {
	events, err := engine.ReadOperationEvents(operationNotificationsReadLimit)
	if err != nil {
		return cursor
	}
	newEvents, nextCursor := selectOperationEventsSince(events, cursor)
	emitted := false
	for _, event := range newEvents {
		notification, ok := buildOperationNotification(event)
		if !ok {
			continue
		}
		p.Emit(EventOperationsNotification, notification)
		emitted = true
	}
	if nextCursor != cursor {
		writeNotifyCursor(nextCursor)
	}
	// The tray refresh rebuilds the whole menu and reads every project's
	// container state, so it runs once per poll rather than once per
	// event: a burst would otherwise serialise that work inside this
	// goroutine and delay the notifications behind it.
	if emitted && onEvent != nil {
		onEvent()
	}
	return nextCursor
}

// readNotifyCursor loads the persisted notification cursor: the last
// delivered event signature. It reports false when no cursor was persisted
// (fresh start) or the file holds no usable signature, in which case the
// caller falls back to suppressing pre-existing history.
func readNotifyCursor() (string, bool) {
	data, err := os.ReadFile(operationNotifyCursorPath())
	if err != nil {
		return "", false
	}
	if cursor := strings.TrimSpace(string(data)); cursor != "" {
		return cursor, true
	}
	return "", false
}

// writeNotifyCursor persists the cursor atomically (temp + rename) so a
// crash mid-write never leaves a half-written signature behind. Errors are
// swallowed: notification state is best-effort durable like history, so a
// full disk must never fail the watcher loop. A blank cursor is never
// written: it would read back as "no cursor" anyway.
func writeNotifyCursor(cursor string) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return
	}
	if err := os.MkdirAll(operationHistoryDir(), conventions.SecretDirPerm); err != nil {
		return
	}
	tmp, err := os.CreateTemp(operationHistoryDir(), "operation-notify-cursor-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(cursor + "\n"); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Chmod(tmpName, conventions.SecretFilePerm); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, operationNotifyCursorPath()); err != nil {
		_ = os.Remove(tmpName)
	}
}

func selectOperationEventsSince(events []engine.OperationEvent, cursor string) ([]engine.OperationEvent, string) {
	if len(events) == 0 {
		return nil, cursor
	}

	lastSignature := operationEventSignature(events[len(events)-1])
	if strings.TrimSpace(cursor) == "" {
		return nil, lastSignature
	}

	cursorIndex := -1
	for index := len(events) - 1; index >= 0; index-- {
		if operationEventSignature(events[index]) == cursor {
			cursorIndex = index
			break
		}
	}

	if cursorIndex == -1 {
		// The cursor is gone: the log rotated, the read window slid past it
		// after a burst, or the app was down while events landed. Either way
		// those events were never delivered, so the whole window is the gap
		// and it is delivered rather than silently dropped. (An empty cursor
		// still suppresses: that is a fresh start, not a gap.)
		return events, lastSignature
	}
	if cursorIndex == len(events)-1 {
		return nil, lastSignature
	}

	return events[cursorIndex+1:], lastSignature
}

func buildOperationNotification(event engine.OperationEvent) (OperationNotification, bool) {
	operation := strings.TrimSpace(event.Operation)
	if operation == "" {
		return OperationNotification{}, false
	}

	status := event.Status
	level := ""
	titlePrefix := ""
	switch status {
	case engine.OperationStatusSuccess:
		level = "success"
		titlePrefix = "Completed"
	case engine.OperationStatusFailure:
		level = "error"
		titlePrefix = "Failed"
	default:
		return OperationNotification{}, false
	}

	project := strings.TrimSpace(event.Project)
	title := fmt.Sprintf("%s: %s", titlePrefix, operation)
	if project != "" {
		title = fmt.Sprintf("%s: %s (%s)", titlePrefix, operation, project)
	}

	body := scrubToolHistoryText(strings.TrimSpace(event.Message))
	if body == "" {
		body = operation
	}

	notification := OperationNotification{
		ID:        operationEventSignature(event),
		Title:     title,
		Body:      body,
		Level:     level,
		Operation: operation,
		Status:    string(status),
		Project:   project,
		Timestamp: strings.TrimSpace(event.Timestamp),
	}
	if level == "error" {
		// Failed operations carry the catalog actions (Retry/Open-logs)
		// with the exact source terminal record the Retry button replays.
		// The Retry button is offered only when that record resolved: a
		// missing id fails closed with no Retry rather than scanning for a
		// substitute that could replay the wrong run.
		notification.RetryID = retryRecordIDForEvent(event)
		notification.Actions = filterNotificationActions(
			humanOpMessage(operation, errors.New(strings.TrimSpace(event.Message))).Actions,
			notification.RetryID,
		)
	}
	return notification, true
}

// filterNotificationActions keeps the catalog actions except Retry when no
// failed record matched the event: offering Retry then would only fail
// closed at click time. Open-logs and Copy stay available regardless.
func filterNotificationActions(actions []OpMessageAction, retryID string) []OpMessageAction {
	if strings.TrimSpace(retryID) != "" {
		return actions
	}
	kept := make([]OpMessageAction, 0, len(actions))
	for _, action := range actions {
		if action.ID == OpActionRetry {
			continue
		}
		kept = append(kept, action)
	}
	return kept
}

// retryRecordIDForEvent binds a failure notification's Retry button to its
// exact source terminal record: the event's OpID must name a history record
// that exists and ended failed, and that id is what RetryOp replays. Anything
// else (no id, unknown id, non-failed record) binds nothing, so the toast
// offers no Retry and a retry without a bound id fails closed as record
// unavailable. It never scans for a substitute: with two failures on one
// project the newest scan could replay the wrong run. Callers hold no lock;
// this takes the registry mutex like every other history reader.
func retryRecordIDForEvent(event engine.OperationEvent) string {
	id := strings.TrimSpace(event.OpID)
	if id == "" {
		return ""
	}
	operationRegistry.Lock()
	defer operationRegistry.Unlock()
	records := readHistoryRecords()
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].ID != id {
			continue
		}
		if records[i].Status != OpStatusFailed {
			return ""
		}
		return records[i].ID
	}
	return ""
}

func operationEventSignature(event engine.OperationEvent) string {
	return strings.Join([]string{
		strings.TrimSpace(event.Timestamp),
		strings.TrimSpace(event.Operation),
		string(event.Status),
		strings.TrimSpace(event.Project),
		strings.TrimSpace(event.Message),
		fmt.Sprintf("%d", event.DurationMS),
		strings.TrimSpace(event.Source),
		strings.TrimSpace(event.Destination),
		strings.TrimSpace(event.OpID),
	}, "|")
}
