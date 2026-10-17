package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"govard/internal/engine"
)

// SnapshotService exposes project snapshot management to the desktop data
// tab. It wraps the existing snapshot engine paths (create/list/restore/
// export) and never reimplements the snapshot format. Listing is synchronous;
// create, restore, and export run as cancellable ops registered with the op
// registry. Restore is two-step only: PreviewSnapshotRestore describes what
// will be replaced and returns a single-use preview token, and
// ApplySnapshotRestore requires that token plus an explicit confirm flag.
// There is deliberately no one-shot restore entry point.
type SnapshotService struct {
	serviceBase
}

// NewSnapshotService builds a SnapshotService with the default platform.
// (Wiring into App/boundServices plus bridge routes lands with the data tab
// UI task.)
func NewSnapshotService() *SnapshotService {
	return &SnapshotService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// NewSnapshotServiceWithPlatform builds a SnapshotService that emits op
// events on p. Tests pass a FakePlatform.
func NewSnapshotServiceWithPlatform(p Platform) *SnapshotService {
	if p == nil {
		p = newDefaultPlatform()
	}
	return &SnapshotService{serviceBase: serviceBase{platform: p}}
}

// SnapshotInfo is one listed snapshot: identity plus the size and creation
// facts the data tab shows. It carries no paths and no secrets.
type SnapshotInfo struct {
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	SizeBytes int64  `json:"sizeBytes"`
	SizeHuman string `json:"sizeHuman"`
	HasDB     bool   `json:"hasDb"`
	HasMedia  bool   `json:"hasMedia"`
}

// SnapshotRestorePreview describes exactly what an apply would replace. The
// PreviewToken is single-use and bound to the project, snapshot, and scope:
// ApplySnapshotRestore replays nothing else.
type SnapshotRestorePreview struct {
	PreviewToken     string `json:"previewToken"`
	Project          string `json:"project"`
	Snapshot         string `json:"snapshot"`
	DBOnly           bool   `json:"dbOnly"`
	MediaOnly        bool   `json:"mediaOnly"`
	WillReplaceDB    bool   `json:"willReplaceDb"`
	WillReplaceMedia bool   `json:"willReplaceMedia"`
	Summary          string `json:"summary"`
}

// SnapshotRestoreTarget is the scrubbed restore endpoint handed to the
// restore seam. It carries no secrets by construction.
type SnapshotRestoreTarget struct {
	Root      string
	Name      string
	DBOnly    bool
	MediaOnly bool
}

// SnapshotOpCompleted is the snapshot:completed payload for a finished op.
type SnapshotOpCompleted struct {
	OpID    string `json:"opId"`
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// SnapshotOpFailed is the snapshot:failed payload for a failed or cancelled op.
type SnapshotOpFailed struct {
	OpID    string `json:"opId"`
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Snapshot op kinds registered with the op registry.
const (
	snapshotOpKindCreate  = "snapshot-create"
	snapshotOpKindRestore = "snapshot-restore"
	snapshotOpKindExport  = "snapshot-export"
)

// snapshotRestoreTokenPrefix prefixes every issued preview token. Apply
// validates the shape before the store lookup, so a malformed token reads as
// "preview first", never as a store probe result.
const snapshotRestoreTokenPrefix = "snap-restore-"

// snapshotRestorePreviewTTL bounds how long a preview token stays valid: a
// restore preview describes the world as it was, and after fifteen minutes
// the snapshot set may have changed, so the dialog previews again.
const snapshotRestorePreviewTTL = 15 * time.Minute

// snapshotRestorePreviewMax bounds the live preview-token map: issuing past
// the cap evicts the oldest token first, so a client that previews in a loop
// cannot grow the map without bound.
const snapshotRestorePreviewMax = 32

// snapshotRestoreTokenHexBytes sizes the token nonce: 16 random bytes read as
// 32 hex chars, unguessable without the store.
const snapshotRestoreTokenHexBytes = 16

var snapshotOpCounter int64
var snapshotOpGenCounter uint64

// ListSnapshots returns the sanitized snapshot list for a project, newest
// first (the engine already sorts by creation time).
func (s *SnapshotService) ListSnapshots(project string) (res []SnapshotInfo, err error) {
	defer RecoverPanic(&err, "ListSnapshots")
	root, _, err := resolveSnapshotContextForDesktop(project)
	if err != nil {
		return nil, err
	}
	metas, err := listSnapshotsForDesktop(root)
	if err != nil {
		return nil, err
	}
	out := make([]SnapshotInfo, 0, len(metas))
	for _, meta := range metas {
		created := "-"
		if !meta.CreatedAt.IsZero() {
			created = meta.CreatedAt.Format("2006-01-02 15:04:05")
		}
		size := meta.SizeBytes
		if size < 0 {
			size = 0
		}
		out = append(out, SnapshotInfo{
			Name:      sanitizeUIText(meta.Name, 0),
			CreatedAt: created,
			SizeBytes: size,
			SizeHuman: formatDBBytes(size),
			HasDB:     meta.DB,
			HasMedia:  meta.Media,
		})
	}
	return out, nil
}

// CreateSnapshot starts an async snapshot create and returns its op id. An
// empty name lets the engine timestamp the snapshot, mirroring
// `govard snapshot create`.
func (s *SnapshotService) CreateSnapshot(project string, name string) (res string, err error) {
	defer RecoverPanic(&err, "CreateSnapshot")
	cleanName := strings.TrimSpace(name)
	if cleanName != "" {
		if cleanName, err = validateSnapshotName(cleanName); err != nil {
			return "", err
		}
	}
	root, cfg, err := resolveSnapshotContextForDesktop(project)
	if err != nil {
		return "", err
	}
	connProject := snapshotDisplayProject(project, root, cfg)
	timeout := govardCommandTimeout([]string{"snapshot", "create"})
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), timeout)
	opID := fmt.Sprintf("snapshot-create-%d", atomic.AddInt64(&snapshotOpCounter, 1))
	generation := atomic.AddUint64(&snapshotOpGenCounter, 1)
	if !registerSnapshotOpForDesktop(opID, connProject, snapshotOpKindCreate, cancel) {
		cancel()
		return "", fmt.Errorf("snapshot create already running: %s", sanitizeUIText(opID, 0))
	}
	noteSnapshotOp(opID, generation, connProject, snapshotOpKindCreate)
	go s.runSnapshotCreate(ctx, opID, generation, connProject, root, cfg, cleanName)
	return opID, nil
}

// PreviewSnapshotRestore describes exactly what restoring name would replace
// and returns a single-use preview token for ApplySnapshotRestore. It writes
// nothing: the snapshot only reads the snapshot directory.
func (s *SnapshotService) PreviewSnapshotRestore(project string, name string, dbOnly bool, mediaOnly bool) (res SnapshotRestorePreview, err error) {
	defer RecoverPanic(&err, "PreviewSnapshotRestore")
	cleanName, err := validateSnapshotName(name)
	if err != nil {
		return SnapshotRestorePreview{}, err
	}
	if dbOnly && mediaOnly {
		return SnapshotRestorePreview{}, fmt.Errorf("cannot use db-only and media-only together")
	}
	root, cfg, err := resolveSnapshotContextForDesktop(project)
	if err != nil {
		return SnapshotRestorePreview{}, err
	}
	displayProject := snapshotDisplayProject(project, root, cfg)
	metas, err := listSnapshotsForDesktop(root)
	if err != nil {
		return SnapshotRestorePreview{}, err
	}
	var found *engine.SnapshotMetadata
	for i := range metas {
		if metas[i].Name == cleanName {
			found = &metas[i]
			break
		}
	}
	if found == nil {
		return SnapshotRestorePreview{}, fmt.Errorf(
			"snapshot %q was not found for project %s; preview again after creating it",
			sanitizeUIText(cleanName, 0),
			sanitizeUIText(displayProject, 0),
		)
	}
	willDB := !mediaOnly && found.DB
	willMedia := !dbOnly && found.Media
	token, err := newSnapshotRestoreToken()
	if err != nil {
		return SnapshotRestorePreview{}, err
	}
	issueSnapshotRestorePreview(token, snapshotRestorePreviewRecord{
		project:   normalizeSnapshotProject(displayProject),
		name:      cleanName,
		dbOnly:    dbOnly,
		mediaOnly: mediaOnly,
		root:      root,
		issuedAt:  snapshotRestoreNow(),
	})
	return SnapshotRestorePreview{
		PreviewToken:     token,
		Project:          sanitizeUIText(displayProject, 0),
		Snapshot:         sanitizeUIText(cleanName, 0),
		DBOnly:           dbOnly,
		MediaOnly:        mediaOnly,
		WillReplaceDB:    willDB,
		WillReplaceMedia: willMedia,
		Summary:          buildSnapshotRestoreSummary(displayProject, cleanName, dbOnly, mediaOnly, willDB, willMedia, found),
	}, nil
}

// ApplySnapshotRestore runs the previewed restore and returns its op id. It
// requires the single-use preview token from PreviewSnapshotRestore plus
// confirm=true (the destructive-action convention, mirroring the onboarding
// ConfirmFolderOverride gate): either missing, the restore never starts. A
// refused apply consumes nothing, so confirming afterwards still works; a
// validated apply consumes the token, so a replay never reaches the engine.
func (s *SnapshotService) ApplySnapshotRestore(project string, name string, previewToken string, confirm bool, dbOnly bool, mediaOnly bool) (res string, err error) {
	defer RecoverPanic(&err, "ApplySnapshotRestore")
	cleanName, err := validateSnapshotName(name)
	if err != nil {
		return "", err
	}
	if !confirm {
		return "", fmt.Errorf(
			"restore refused: restoring snapshot %q overwrites the local database and media; pass the preview token with explicit confirmation",
			sanitizeUIText(cleanName, 0),
		)
	}
	token := strings.TrimSpace(previewToken)
	if !validSnapshotRestoreTokenShape(token) {
		return "", fmt.Errorf("restore refused: preview the snapshot first, then apply with its preview token")
	}
	// Atomic claim: lookup, binding validation, and consume happen in one
	// critical section, so two parallel applies cannot both pass the check.
	// A claimed token is spent: every return below this point that starts no
	// engine op restores it with its original issue time, while an op that
	// starts keeps it consumed (single-use, anti-replay).
	record, err := claimSnapshotRestorePreview(token, project, cleanName, dbOnly, mediaOnly)
	if err != nil {
		return "", err
	}
	root, cfg, err := resolveSnapshotContextForDesktop(project)
	if err != nil {
		restoreSnapshotRestorePreview(token, record)
		return "", err
	}
	if filepath.Clean(root) != filepath.Clean(record.root) {
		restoreSnapshotRestorePreview(token, record)
		return "", fmt.Errorf("restore refused: the project location changed since the preview; preview again")
	}
	target := SnapshotRestoreTarget{Root: root, Name: cleanName, DBOnly: dbOnly, MediaOnly: mediaOnly}
	timeout := govardCommandTimeout([]string{"snapshot", "restore"})
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), timeout)
	opID := fmt.Sprintf("snapshot-restore-%d", atomic.AddInt64(&snapshotOpCounter, 1))
	generation := atomic.AddUint64(&snapshotOpGenCounter, 1)
	displayProject := snapshotDisplayProject(project, root, cfg)
	if !registerSnapshotOpForDesktop(opID, displayProject, snapshotOpKindRestore, cancel) {
		cancel()
		restoreSnapshotRestorePreview(token, record)
		return "", fmt.Errorf("snapshot restore already running: %s", sanitizeUIText(opID, 0))
	}
	noteSnapshotOp(opID, generation, displayProject, snapshotOpKindRestore)
	go s.runSnapshotRestore(ctx, opID, generation, displayProject, target)
	return opID, nil
}

// ExportSnapshot starts an async snapshot export to destPath (the save-dialog
// path) and returns its op id.
func (s *SnapshotService) ExportSnapshot(project string, name string, destPath string) (res string, err error) {
	defer RecoverPanic(&err, "ExportSnapshot")
	cleanName, err := validateSnapshotName(name)
	if err != nil {
		return "", err
	}
	dest := filepath.Clean(strings.TrimSpace(destPath))
	if strings.TrimSpace(destPath) == "" {
		return "", fmt.Errorf("snapshot export destination path is empty")
	}
	// Containment before anything is registered or started: the destination
	// must be a file path inside an existing directory (save-dialog paths
	// always are). A directory destination or a missing parent is refused
	// here, so the engine never decides what that means. Overwrite
	// confirmation for an existing file stays the UI's job.
	if info, err := os.Stat(dest); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf(
				"export refused: %s is a directory, not a file; pick a file path",
				sanitizeUIText(dest, 0),
			)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf(
			"export refused: cannot inspect destination %s: %v",
			sanitizeUIText(dest, 0), err,
		)
	}
	parent := filepath.Dir(dest)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return "", fmt.Errorf(
			"export refused: parent directory %s does not exist; pick a path inside an existing folder",
			sanitizeUIText(parent, 0),
		)
	}
	root, cfg, err := resolveSnapshotContextForDesktop(project)
	if err != nil {
		return "", err
	}
	displayProject := snapshotDisplayProject(project, root, cfg)
	timeout := govardCommandTimeout([]string{"snapshot", "export"})
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), timeout)
	opID := fmt.Sprintf("snapshot-export-%d", atomic.AddInt64(&snapshotOpCounter, 1))
	generation := atomic.AddUint64(&snapshotOpGenCounter, 1)
	if !registerSnapshotOpForDesktop(opID, displayProject, snapshotOpKindExport, cancel) {
		cancel()
		return "", fmt.Errorf("snapshot export already running: %s", sanitizeUIText(opID, 0))
	}
	noteSnapshotOp(opID, generation, displayProject, snapshotOpKindExport)
	go s.runSnapshotExport(ctx, opID, generation, displayProject, root, cleanName, dest)
	return opID, nil
}

// CancelSnapshotOp stops tracking a running snapshot op: the registry fires
// the op context and the op identity is dropped so late events from the dying
// goroutine never reach the UI. Unknown ids report false, which callers treat
// as a harmless no-op success.
func (s *SnapshotService) CancelSnapshotOp(opID string) bool {
	id := strings.TrimSpace(opID)
	if id == "" {
		return false
	}
	snapshotOpGens.Lock()
	ident, ok := snapshotOpGens.byOp[id]
	if ok {
		delete(snapshotOpGens.byOp, id)
	}
	snapshotOpGens.Unlock()
	cancelled := CancelOp(id)
	if cancelled {
		s.platform.Emit(EventSnapshotFailed, SnapshotOpFailed{
			OpID:    sanitizeUIText(id, 0),
			Project: sanitizeUIText(ident.project, 0),
			Kind:    ident.kind,
			Message: sanitizeUIText(snapshotOpCancelMessage(ident.kind), 0),
		})
	}
	return cancelled
}

// validateSnapshotName rejects empty names and path escapes. The engine joins
// the name into the snapshot directory, so separators and parent references
// must never reach it; the error echoes only the sanitized name.
func validateSnapshotName(name string) (string, error) {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" {
		return "", fmt.Errorf("snapshot name is empty")
	}
	if strings.Contains(cleaned, "/") || strings.Contains(cleaned, "\\") ||
		cleaned == "." || cleaned == ".." || strings.Contains(cleaned, "..") {
		return "", fmt.Errorf(
			"snapshot name %q is invalid: names must not contain paths",
			sanitizeUIText(cleaned, 0),
		)
	}
	return cleaned, nil
}

// normalizeSnapshotProject canonicalizes a project name for preview-token
// binding, so a token the dialog shows with different casing still verifies
// against the same project.
func normalizeSnapshotProject(project string) string {
	return strings.ToLower(strings.TrimSpace(project))
}

// snapshotDisplayProject prefers the caller's project label and falls back to
// the resolved config name and then the root base, so op registry entries and
// events always carry something human.
func snapshotDisplayProject(project string, root string, cfg engine.Config) string {
	if trimmed := strings.TrimSpace(project); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(cfg.ProjectName); trimmed != "" {
		return trimmed
	}
	return filepath.Base(filepath.Clean(root))
}

// buildSnapshotRestoreSummary renders the exactly-what-will-be-replaced human
// line for the confirm dialog. Every part is sanitized, and parts the
// snapshot does not hold (or the scope excludes) read as staying untouched,
// so the dialog never implies more will be replaced than the apply touches.
func buildSnapshotRestoreSummary(project string, name string, dbOnly bool, mediaOnly bool, willDB bool, willMedia bool, meta *engine.SnapshotMetadata) string {
	safeProject := sanitizeUIText(project, 0)
	safeName := sanitizeUIText(name, 0)
	var scope string
	switch {
	case dbOnly:
		scope = "the local database"
	case mediaOnly:
		scope = "the local media"
	default:
		scope = "the local database and media"
	}
	var detail string
	switch {
	case willDB && willMedia:
		detail = "both will be replaced"
	case willDB:
		detail = "the database will be replaced; media stays untouched"
	case willMedia:
		detail = "media will be replaced; the database stays untouched"
	default:
		detail = "nothing will be replaced: the snapshot holds neither database nor media in this scope"
	}
	context := ""
	if meta != nil && !meta.CreatedAt.IsZero() {
		context = " (snapshot from " + meta.CreatedAt.Format("2006-01-02 15:04:05") +
			", ~" + formatDBBytes(meta.SizeBytes) + ")"
	}
	return sanitizeUIText(
		"Restoring snapshot "+safeName+" will replace "+scope+
			" for project "+safeProject+": "+detail+context+".",
		0,
	)
}

// newSnapshotRestoreToken mints an unguessable single-use preview token.
func newSnapshotRestoreToken() (string, error) {
	var nonce [snapshotRestoreTokenHexBytes]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("issue restore preview token: %w", err)
	}
	return snapshotRestoreTokenPrefix + hex.EncodeToString(nonce[:]), nil
}

// validSnapshotRestoreTokenShape rejects malformed tokens before the store
// lookup so every refusal reads as "preview first".
func validSnapshotRestoreTokenShape(token string) bool {
	trimmed := strings.TrimSpace(token)
	if !strings.HasPrefix(trimmed, snapshotRestoreTokenPrefix) {
		return false
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(trimmed, snapshotRestoreTokenPrefix))
	if err != nil || len(raw) != snapshotRestoreTokenHexBytes {
		return false
	}
	return true
}

// snapshotRestorePreviewRecord is one issued, not yet applied preview.
type snapshotRestorePreviewRecord struct {
	project   string
	name      string
	dbOnly    bool
	mediaOnly bool
	root      string
	issuedAt  time.Time
}

var snapshotRestorePreviews = struct {
	sync.Mutex
	byToken map[string]snapshotRestorePreviewRecord
}{byToken: map[string]snapshotRestorePreviewRecord{}}

func issueSnapshotRestorePreview(token string, record snapshotRestorePreviewRecord) {
	snapshotRestorePreviews.Lock()
	defer snapshotRestorePreviews.Unlock()
	for len(snapshotRestorePreviews.byToken) >= snapshotRestorePreviewMax {
		oldest := ""
		var oldestAt time.Time
		first := true
		for candidate, issued := range snapshotRestorePreviews.byToken {
			if first || issued.issuedAt.Before(oldestAt) {
				oldest, oldestAt, first = candidate, issued.issuedAt, false
			}
		}
		if oldest == "" {
			break
		}
		delete(snapshotRestorePreviews.byToken, oldest)
	}
	snapshotRestorePreviews.byToken[token] = record
}

// registerSnapshotOpForDesktop is a seam over RegisterOp so tests can force
// the contention path. Production always uses the real registry.
var registerSnapshotOpForDesktop = RegisterOp

// snapshotRestoreNow is the preview-token clock seam: tests age tokens past
// their TTL without sleeping. ResetStateForTest restores it.
var snapshotRestoreNow = time.Now

// claimSnapshotRestorePreview atomically looks up, validates, and consumes a
// preview token in one critical section: the store lookup, the TTL check,
// the binding match against project, snapshot, and scope, and the delete
// happen under a single lock, so two parallel applies cannot both pass.
// A binding mismatch consumes nothing (probing never burns the legit token);
// every other outcome either returns the spent record to the caller or
// deletes an expired entry.
func claimSnapshotRestorePreview(token string, project string, name string, dbOnly bool, mediaOnly bool) (snapshotRestorePreviewRecord, error) {
	snapshotRestorePreviews.Lock()
	defer snapshotRestorePreviews.Unlock()
	key := strings.TrimSpace(token)
	record, ok := snapshotRestorePreviews.byToken[key]
	if !ok {
		return snapshotRestorePreviewRecord{}, fmt.Errorf("restore refused: unknown, expired, or already used preview token; preview the snapshot again")
	}
	if snapshotRestoreNow().Sub(record.issuedAt) > snapshotRestorePreviewTTL {
		delete(snapshotRestorePreviews.byToken, key)
		return snapshotRestorePreviewRecord{}, fmt.Errorf("restore refused: unknown, expired, or already used preview token; preview the snapshot again")
	}
	if record.project != normalizeSnapshotProject(project) || record.name != name ||
		record.dbOnly != dbOnly || record.mediaOnly != mediaOnly {
		return snapshotRestorePreviewRecord{}, fmt.Errorf("restore refused: the preview token does not match this snapshot and options; preview again")
	}
	delete(snapshotRestorePreviews.byToken, key)
	return record, nil
}

// restoreSnapshotRestorePreview re-inserts a claimed record with its original
// issue time (TTL preserved, never extended) when a pre-engine refusal stops
// the apply after the claim: resolve failure, project move, or op contention.
// Only an op that actually starts keeps the token consumed.
func restoreSnapshotRestorePreview(token string, record snapshotRestorePreviewRecord) {
	snapshotRestorePreviews.Lock()
	defer snapshotRestorePreviews.Unlock()
	snapshotRestorePreviews.byToken[strings.TrimSpace(token)] = record
}

func resetSnapshotRestorePreviews() {
	snapshotRestorePreviews.Lock()
	defer snapshotRestorePreviews.Unlock()
	snapshotRestorePreviews.byToken = map[string]snapshotRestorePreviewRecord{}
}

// snapshotOpGens tracks the live identity of every running snapshot op so
// late events from a finished or cancelled op drop stale instead of reaching
// the UI under a recycled op id.
var snapshotOpGens = struct {
	sync.Mutex
	byOp map[string]snapshotOpIdentity
}{byOp: map[string]snapshotOpIdentity{}}

type snapshotOpIdentity struct {
	generation uint64
	project    string
	kind       string
}

func resetSnapshotOpGens() {
	snapshotOpGens.Lock()
	defer snapshotOpGens.Unlock()
	snapshotOpGens.byOp = map[string]snapshotOpIdentity{}
}

func noteSnapshotOp(opID string, generation uint64, project string, kind string) {
	snapshotOpGens.Lock()
	defer snapshotOpGens.Unlock()
	snapshotOpGens.byOp[opID] = snapshotOpIdentity{generation: generation, project: project, kind: kind}
}

func forgetSnapshotOp(opID string, generation uint64) {
	snapshotOpGens.Lock()
	defer snapshotOpGens.Unlock()
	if ident, ok := snapshotOpGens.byOp[opID]; ok && ident.generation == generation {
		delete(snapshotOpGens.byOp, opID)
	}
}

// emitSnapshotOp emits one snapshot event when the op identity is still live
// and drops it stale otherwise: after CancelSnapshotOp (which deletes the
// identity first) or FinishOp, nothing from the dying goroutine may reach
// the UI.
func (s *SnapshotService) emitSnapshotOp(opID string, generation uint64, event string, payload any) {
	snapshotOpGens.Lock()
	ident, ok := snapshotOpGens.byOp[opID]
	match := ok && ident.generation == generation
	snapshotOpGens.Unlock()
	if !match {
		return
	}
	s.platform.Emit(event, payload)
}

// snapshotOpKindLabel renders an op kind for progress and failure messages.
func snapshotOpKindLabel(kind string) string {
	switch kind {
	case snapshotOpKindRestore:
		return "Snapshot restore"
	case snapshotOpKindExport:
		return "Snapshot export"
	default:
		return "Snapshot create"
	}
}

// snapshotOpCancelMessage states the truth about a user stop: desktop
// tracking ends, but the underlying engine call (docker dump/import, tar) is
// not interruptible once started and may run to completion. It never claims
// a cancellation that was not delivered.
func snapshotOpCancelMessage(kind string) string {
	return snapshotOpKindLabel(kind) + " tracking stopped by user; the underlying operation may run to completion"
}

// snapshotOpFailureMessage renders a failure message for kind. A cancelled
// context always reads as a stopped track, never as the context error text
// and never as a delivered cancellation.
func snapshotOpFailureMessage(kind string, err error, ctx context.Context) string {
	if ctx.Err() != nil {
		return snapshotOpCancelMessage(kind)
	}
	if err == nil {
		return snapshotOpKindLabel(kind) + " failed"
	}
	return snapshotOpKindLabel(kind) + " failed: " + err.Error()
}

// runSnapshotCreate executes the engine create for one registered op. A
// failed or cancelled create emits snapshot:failed; only a clean engine
// return emits snapshot:completed.
func (s *SnapshotService) runSnapshotCreate(ctx context.Context, opID string, generation uint64, project string, root string, cfg engine.Config, name string) {
	defer FinishOp(opID)
	defer forgetSnapshotOp(opID, generation)
	fail := func(err error) {
		s.emitSnapshotOp(opID, generation, EventSnapshotFailed, SnapshotOpFailed{
			OpID:    opID,
			Project: sanitizeUIText(project, 0),
			Kind:    snapshotOpKindCreate,
			Message: sanitizeUIText(snapshotOpFailureMessage(snapshotOpKindCreate, err, ctx), 0),
		})
	}
	if ctx.Err() != nil {
		fail(ctx.Err())
		return
	}
	path, err := createSnapshotForDesktop(ctx, root, cfg, name)
	if err != nil || ctx.Err() != nil {
		if err == nil {
			err = ctx.Err()
		}
		fail(err)
		return
	}
	s.emitSnapshotOp(opID, generation, EventSnapshotCompleted, SnapshotOpCompleted{
		OpID:    opID,
		Project: sanitizeUIText(project, 0),
		Kind:    snapshotOpKindCreate,
		Path:    sanitizeUIText(path, 0),
		Message: sanitizeUIText(fmt.Sprintf("Snapshot saved to %s", path), 0),
	})
}

// runSnapshotRestore executes the previewed engine restore for one registered
// op. Reaching here means the preview token validated and confirm was true;
// anything else (cancel, engine error) emits snapshot:failed.
func (s *SnapshotService) runSnapshotRestore(ctx context.Context, opID string, generation uint64, project string, target SnapshotRestoreTarget) {
	defer FinishOp(opID)
	defer forgetSnapshotOp(opID, generation)
	fail := func(err error) {
		s.emitSnapshotOp(opID, generation, EventSnapshotFailed, SnapshotOpFailed{
			OpID:    opID,
			Project: sanitizeUIText(project, 0),
			Kind:    snapshotOpKindRestore,
			Message: sanitizeUIText(snapshotOpFailureMessage(snapshotOpKindRestore, err, ctx), 0),
		})
	}
	if ctx.Err() != nil {
		fail(ctx.Err())
		return
	}
	if err := restoreSnapshotForDesktop(ctx, target); err != nil || ctx.Err() != nil {
		if err == nil {
			err = ctx.Err()
		}
		fail(err)
		return
	}
	s.emitSnapshotOp(opID, generation, EventSnapshotCompleted, SnapshotOpCompleted{
		OpID:    opID,
		Project: sanitizeUIText(project, 0),
		Kind:    snapshotOpKindRestore,
		Path:    sanitizeUIText(target.Name, 0),
		Message: sanitizeUIText(fmt.Sprintf("Snapshot %s restored for %s", target.Name, project), 0),
	})
}

// runSnapshotExport executes the engine export for one registered op.
func (s *SnapshotService) runSnapshotExport(ctx context.Context, opID string, generation uint64, project string, root string, name string, dest string) {
	defer FinishOp(opID)
	defer forgetSnapshotOp(opID, generation)
	fail := func(err error) {
		s.emitSnapshotOp(opID, generation, EventSnapshotFailed, SnapshotOpFailed{
			OpID:    opID,
			Project: sanitizeUIText(project, 0),
			Kind:    snapshotOpKindExport,
			Message: sanitizeUIText(snapshotOpFailureMessage(snapshotOpKindExport, err, ctx), 0),
		})
	}
	if ctx.Err() != nil {
		fail(ctx.Err())
		return
	}
	if err := exportSnapshotForDesktop(ctx, root, name, dest); err != nil || ctx.Err() != nil {
		if err == nil {
			err = ctx.Err()
		}
		fail(err)
		return
	}
	s.emitSnapshotOp(opID, generation, EventSnapshotCompleted, SnapshotOpCompleted{
		OpID:    opID,
		Project: sanitizeUIText(project, 0),
		Kind:    snapshotOpKindExport,
		Path:    sanitizeUIText(dest, 0),
		Message: sanitizeUIText(fmt.Sprintf("Snapshot %s exported to %s", name, dest), 0),
	})
}

// Seams below keep the service hermetic in tests; ResetStateForTest restores
// every one of them. The context-carrying seams receive the op context so a
// cancel during a stubbed engine call surfaces as a user cancel.

var resolveSnapshotContextForDesktop = defaultResolveSnapshotContextForDesktop
var listSnapshotsForDesktop = defaultListSnapshotsForDesktop
var createSnapshotForDesktop = defaultCreateSnapshotForDesktop
var restoreSnapshotForDesktop = defaultRestoreSnapshotForDesktop
var exportSnapshotForDesktop = defaultExportSnapshotForDesktop

// defaultResolveSnapshotContextForDesktop maps a project label to its root
// plus loaded config, the same resolution the remote panel uses before it
// touches engine paths.
func defaultResolveSnapshotContextForDesktop(project string) (string, engine.Config, error) {
	trimmed := strings.TrimSpace(project)
	if trimmed == "" {
		return "", engine.Config{}, fmt.Errorf("project is required")
	}
	root, _, err := resolveProjectRootForRemotes(trimmed)
	if err != nil {
		return "", engine.Config{}, err
	}
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		return "", engine.Config{}, fmt.Errorf("load config for snapshots: %w", err)
	}
	return root, cfg, nil
}

func defaultListSnapshotsForDesktop(root string) ([]engine.SnapshotMetadata, error) {
	return engine.ListSnapshots(root)
}

func defaultCreateSnapshotForDesktop(ctx context.Context, root string, cfg engine.Config, name string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return engine.CreateSnapshot(root, cfg, name)
}

// defaultRestoreSnapshotForDesktop wraps engine.RestoreSnapshot. The engine
// call itself is not context-aware (docker import runs to completion once
// started), so a cancel mid-restore stops the desktop op and its events while
// the underlying import finishes: the same semantics as interrupting the CLI.
func defaultRestoreSnapshotForDesktop(ctx context.Context, target SnapshotRestoreTarget) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	cfg, _, err := engine.LoadConfigFromDir(target.Root, true)
	if err != nil {
		return fmt.Errorf("load config for snapshot restore: %w", err)
	}
	return engine.RestoreSnapshot(target.Root, cfg, target.Name, target.DBOnly, target.MediaOnly)
}

func defaultExportSnapshotForDesktop(ctx context.Context, root string, name string, target string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return engine.ExportSnapshot(root, name, target)
}
