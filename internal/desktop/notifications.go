package desktop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"govard/internal/engine"
)

const (
	operationNotificationsPollInterval = 2 * time.Second
	operationNotificationsReadLimit    = 256
)

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
}

func watchOperationNotifications(ctx context.Context, p Platform, onEvent func()) {
	cursor := ""
	if events, err := engine.ReadOperationEvents(operationNotificationsReadLimit); err == nil {
		_, cursor = selectOperationEventsSince(events, cursor)
	}

	ticker := time.NewTicker(operationNotificationsPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events, err := engine.ReadOperationEvents(operationNotificationsReadLimit)
			if err != nil {
				continue
			}
			newEvents, nextCursor := selectOperationEventsSince(events, cursor)
			cursor = nextCursor
			emitted := false
			for _, event := range newEvents {
				notification, ok := buildOperationNotification(event)
				if !ok {
					continue
				}
				p.Emit(EventOperationsNotification, notification)
				emitted = true
			}
			// The tray refresh rebuilds the whole menu and reads every project's
			// container state, so it runs once per poll rather than once per
			// event: a burst would otherwise serialise that work inside this
			// goroutine and delay the notifications behind it.
			if emitted && onEvent != nil {
				onEvent()
			}
		}
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
		// Cursor might be truncated due to log rotation; resume from current end.
		return nil, lastSignature
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

	body := strings.TrimSpace(event.Message)
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
	return notification, true
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
	}, "|")
}
