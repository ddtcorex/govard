package desktop

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// SyncProgressPayload is the structured sync:progress event payload: a short
// human message for toasts plus the phase and percent for spinners. Raw log
// lines keep flowing through sync:output for the progress panel.
type SyncProgressPayload struct {
	Phase   string `json:"phase"`
	Percent int    `json:"percent"`
	Message string `json:"message"`
}

// Sync progress phases in the order the mapper tries them. Prepare, db, and
// media come before the generic transfer fallthrough so a line mentioning two
// stages (e.g. "checking config file") lands on the specific one, and the
// transfer markers stay narrow (rsync's own vocabulary) instead of claiming
// generic words like "file".
type syncPhaseSeed struct {
	phase   string
	percent int
	message string
	markers []string
}

var syncPhaseSeeds = []syncPhaseSeed{
	{"finalize", 95, "Finalizing sync...", []string{"finaliz", "complet", "success", "verif", "cleanup", "clean", "done"}},
	{"prepare", 5, "Preparing sync...", []string{"prepar", "plan", "connect", "check", "resolv", "detect", "config", "start", "load"}},
	{"db", 60, "Importing database...", []string{"database", "mysqldump", "mariadb", "mariadb-dump", "dump", "import", "--db", " db"}},
	{"media", 75, "Syncing media...", []string{"media"}},
	{"transfer", 40, "Transferring files...", []string{"rsync", "transfer", "download", "upload", "sending", "receiv", "syncing", "pulling", "pull ", "copy", "copied", "copying"}},
}

// workingPhase is the neutral progress for lines matching no stage: the toast
// stays honest instead of claiming a stage it cannot see.
const workingPhase = "working"

var syncPercentPattern = regexp.MustCompile(`(\d{1,3})\s*%`)

// mapSyncProgressLine folds one sync log line into structured progress. The
// match runs on text already passed through sanitizeUIText, and the message
// is a static human copy (never the raw tail), so ANSI escapes, control
// bytes, and over-long lines cannot reach the toast. Lines that match no
// stage report the neutral working phase while the panel still shows every
// raw line.
func mapSyncProgressLine(raw string) (SyncProgressPayload, bool) {
	lowered := strings.ToLower(strings.TrimSpace(sanitizeUIText(raw, 0)))
	if lowered == "" {
		return SyncProgressPayload{}, false
	}
	if seed, ok := syncSeedByPhase("finalize"); ok && containsSyncMarker(lowered, seed.markers) {
		return SyncProgressPayload{Phase: seed.phase, Percent: seed.percent, Message: seed.message}, true
	}
	// "download"/"upload" unambiguously denote transfer, but "download"
	// contains the prepare marker "load": carve them out before the stage
	// table so "45% downloaded" never reads as preparation.
	if containsSyncMarker(lowered, []string{"download", "upload"}) {
		return transferProgress(lowered), true
	}
	for _, phase := range []string{"prepare", "db", "media", "transfer"} {
		seed, ok := syncSeedByPhase(phase)
		if !ok || !containsSyncMarker(lowered, seed.markers) {
			continue
		}
		if phase == "transfer" {
			return transferProgress(lowered), true
		}
		return SyncProgressPayload{Phase: seed.phase, Percent: seed.percent, Message: seed.message}, true
	}
	// A bare "45%" with no stage words is still transfer progress.
	if parsed, ok := parseSyncPercent(lowered); ok {
		return SyncProgressPayload{
			Phase:   "transfer",
			Percent: parsed,
			Message: fmt.Sprintf("Transferring files... %d%%", parsed),
		}, true
	}
	return SyncProgressPayload{Phase: workingPhase, Percent: 0, Message: "Working..."}, true
}

// transferProgress folds a transfer line, honoring an rsync-style "45%"
// figure when present and the stage default otherwise.
func transferProgress(lowered string) SyncProgressPayload {
	if parsed, ok := parseSyncPercent(lowered); ok {
		return SyncProgressPayload{
			Phase:   "transfer",
			Percent: parsed,
			Message: fmt.Sprintf("Transferring files... %d%%", parsed),
		}
	}
	return SyncProgressPayload{Phase: "transfer", Percent: 40, Message: "Transferring files..."}
}

func syncSeedByPhase(phase string) (syncPhaseSeed, bool) {
	for _, seed := range syncPhaseSeeds {
		if seed.phase == phase {
			return seed, true
		}
	}
	return syncPhaseSeed{}, false
}

// syncFailedMessage formats the sync:failed payload through the shared
// UI-text sanitizer: failure text can carry ANSI escapes and control bytes
// from the CLI's own output, and the payload reaches toasts as well as the
// panel. No truncation: the frontend toast caps display length itself, and
// the panel keeps the full text.
func syncFailedMessage(err error) string {
	if err == nil {
		return "Sync failed"
	}
	return sanitizeUIText(fmt.Sprintf("Sync failed: %v", err), 0)
}

func containsSyncMarker(lowered string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

// parseSyncPercent reads an rsync-style "45%" figure off a transfer line and
// clamps it to a real percent so a bogus "140%" never becomes bar state.
func parseSyncPercent(lowered string) (int, bool) {
	match := syncPercentPattern.FindStringSubmatch(lowered)
	if match == nil {
		return 0, false
	}
	parsed, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	if parsed < 0 {
		parsed = 0
	}
	if parsed > 100 {
		parsed = 100
	}
	return parsed, true
}

// scanSyncPipe streams one sync pipe into the UI on top of the shared log
// batching, and adds the structured progress the toast renders: every flush
// emits the batched raw lines on sync:output for the panel, plus one
// sync:progress for the last line in the batch. The context is kept for the
// same reason as scanLogPipe: every pipe carries the cancellation parent of
// the exec.CommandContext that produced it.
func scanSyncPipe(ctx context.Context, p Platform, pipe io.Reader, done chan<- struct{}) {
	_ = ctx

	streamSanitizedBatches(p, pipe, func(batch []string) {
		p.Emit(EventSyncOutput, strings.Join(batch, "\n"))
		if payload, ok := mapSyncProgressLine(batch[len(batch)-1]); ok {
			p.Emit(EventSyncProgress, payload)
		}
	})
	done <- struct{}{}
}
