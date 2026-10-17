package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"govard/internal/engine"
	"govard/internal/frameworks"
)

// ToolArgSpec describes one named argument a tool preset accepts. Pattern is
// a full-match regex for the value; empty means the default safe pattern.
// An optional arg (Required false) may also be omitted or empty.
// Flag renders the arg as a --name option instead of a positional value:
// present-empty/true/1 emits --name, false/0 omits it, any other validated
// value emits --name=value. Non-flag args render positionally (bare value)
// in ArgSpec order, or are omitted when empty.
type ToolArgSpec struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Pattern  string `json:"pattern"`
	Flag     bool   `json:"flag"`
}

// ToolPreset is one allowlisted tool invocation the desktop may run.
// Display is the human-readable form of the base command, never executed.
// Argv is the exec-ready base argv (no shell metacharacters by
// construction); Task 2 appends validated args and runs it with exec.Command
// and no shell.
type ToolPreset struct {
	ID      string        `json:"id"`
	Label   string        `json:"label"`
	Display string        `json:"display"`
	Argv    []string      `json:"argv"`
	ArgSpec []ToolArgSpec `json:"argSpec"`
}

// defaultSafeToolArgPattern allows plain tokens only: letters, digits, and
// a small set of inert punctuation. It rejects whitespace and every shell
// metacharacter (; & | $ ( ) < > \ ` " ' * ? ~ # ! ^ { } [ ]) with no
// quoting exemption: Task 2 executes via shell-free argv, so a quote char
// is data, never protection, and is rejected like any other metachar.
const defaultSafeToolArgPattern = `^[A-Za-z0-9_@+.,:/=-]+$`

// maxToolArgLength bounds validated arg values.
const maxToolArgLength = 256

func magentoToolPresets() []ToolPreset {
	cacheType := ToolArgSpec{
		Name:     "type",
		Label:    "Cache type",
		Required: false,
		Pattern:  `^[a-z_]+$`,
	}
	return []ToolPreset{
		{ID: "magento-cache-flush", Label: "Magento: flush cache", Display: "bin/magento cache:flush", Argv: []string{"bin/magento", "cache:flush"}, ArgSpec: []ToolArgSpec{cacheType}},
		{ID: "magento-cache-clean", Label: "Magento: clean cache", Display: "bin/magento cache:clean", Argv: []string{"bin/magento", "cache:clean"}, ArgSpec: []ToolArgSpec{cacheType}},
		{ID: "magento-setup-di-compile", Label: "Magento: compile DI", Display: "bin/magento setup:di:compile", Argv: []string{"bin/magento", "setup:di:compile"}},
		{ID: "magento-indexer-reindex", Label: "Magento: reindex", Display: "bin/magento indexer:reindex", Argv: []string{"bin/magento", "indexer:reindex"}, ArgSpec: []ToolArgSpec{
			{Name: "indexer", Label: "Indexer", Required: false, Pattern: `^[a-z_]+$`},
		}},
	}
}

func laravelToolPresets() []ToolPreset {
	return []ToolPreset{
		{ID: "laravel-migrate", Label: "Laravel: run migrations", Display: "php artisan migrate", Argv: []string{"php", "artisan", "migrate"}, ArgSpec: []ToolArgSpec{
			// No Pattern: exercised through the default safe pattern.
			{Name: "database", Label: "Connection", Required: false, Flag: true},
			{Name: "force", Label: "Force in production", Required: false, Pattern: `^(true|false|1|0)?$`, Flag: true},
		}},
		{ID: "laravel-cache-clear", Label: "Laravel: clear cache", Display: "php artisan cache:clear", Argv: []string{"php", "artisan", "cache:clear"}},
		{ID: "laravel-config-clear", Label: "Laravel: clear config", Display: "php artisan config:clear", Argv: []string{"php", "artisan", "config:clear"}},
	}
}

func genericToolPresets() []ToolPreset {
	return []ToolPreset{
		{ID: "composer-install", Label: "Composer: install dependencies", Display: "composer install", Argv: []string{"composer", "install"}, ArgSpec: []ToolArgSpec{
			{Name: "no-dev", Label: "Skip dev dependencies", Required: false, Pattern: `^(true|false|1|0)?$`, Flag: true},
		}},
		{ID: "npm-install", Label: "npm: install dependencies", Display: "npm install", Argv: []string{"npm", "install"}},
		{ID: "npm-run-build", Label: "npm: build assets", Display: "npm run build", Argv: []string{"npm", "run", "build"}},
	}
}

// allToolPresets is the complete preset allowlist across frameworks.
func allToolPresets() []ToolPreset {
	var all []ToolPreset
	all = append(all, magentoToolPresets()...)
	all = append(all, laravelToolPresets()...)
	all = append(all, genericToolPresets()...)
	return all
}

// toolFrameworkPresets returns the framework-scoped presets for a normalized
// framework name, without the generic set. Unknown frameworks yield none.
func toolFrameworkPresets(framework string) []ToolPreset {
	normalized := frameworks.Normalize(framework)
	switch {
	case normalized == "laravel":
		return laravelToolPresets()
	case normalized == "magento1" || normalized == "magento2" || normalized == "mageos" || normalized == "openmage" ||
		frameworks.IsA(normalized, "magento2") || frameworks.IsA(normalized, "magento1"):
		return magentoToolPresets()
	default:
		return nil
	}
}

// ListToolPresetsForFramework returns the preset allowlist for a framework
// name: framework-scoped presets plus the generic set. Unknown or empty
// framework names yield the generic set only.
func ListToolPresetsForFramework(framework string) []ToolPreset {
	return append(toolFrameworkPresets(framework), genericToolPresets()...)
}

// FindToolPreset looks a preset up by id in the allowlist.
func FindToolPreset(id string) (ToolPreset, bool) {
	for _, preset := range allToolPresets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return ToolPreset{}, false
}

// ListToolPresets returns the preset allowlist for a project: it resolves
// the project the same way remotes do and reads its framework from the
// stored config. Resolution or config-load failures are returned (never a
// silent fallback) so the palette can show guidance instead of a misleading
// list; a resolved project with an unknown framework yields the generic set.
// RunTool (Task 2) re-resolves the project at execution time.
func ListToolPresets(project string) ([]ToolPreset, error) {
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return nil, fmt.Errorf("resolve project: %w", err)
	}
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		return nil, fmt.Errorf("load project config: %w", err)
	}
	return ListToolPresetsForFramework(strings.ToLower(strings.TrimSpace(cfg.Framework))), nil
}

// defaultStartToolProcess spawns argv[0] with argv[1:] directly via
// exec.CommandContext: no shell, no sh -c, no string joining, ever. The run
// context comes from the op registry, so CancelToolRun kills the process.
var defaultStartToolProcess = func(ctx context.Context, dir string, argv []string) (io.Reader, io.Reader, func() error, error) {
	if len(argv) == 0 {
		return nil, nil, nil, fmt.Errorf("tool argv is empty")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = filepath.Clean(dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("pipe tool stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("pipe tool stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("start tool command: %w", err)
	}
	return stdout, stderr, cmd.Wait, nil
}

// startToolProcess spawns the validated tool argv and returns its pipes plus
// a wait func. It is a var so tests can serve canned output without spawning
// real project binaries; the default above runs shell-free exec.
var startToolProcess = defaultStartToolProcess

// toolResultMessageMax caps completion/failure payloads: they render in
// toasts and status lines, where the shared truncated marker (not raw CLI
// output) belongs. Streamed lines keep full length (max <= 0).
const toolResultMessageMax = 500

// toolOpIDForRoot keys the op registry entry for a tool run. The id doubles
// as the per-project lock: one tool op per project at a time, freed on
// completion or cancel. The id is stable per project across reruns, so the
// publish stage additionally tracks the admitted-run generation below: a
// rerun re-registers the same id, and only the latest generation may emit.
func toolOpIDForRoot(root string) string {
	return "tool:" + root
}

// toolRunGenerations counts admitted runs per op id. One uint64 per project
// that ever ran a tool; bounded by the project count, never cleaned, so a
// stale tail can always be told apart from the current run.
var toolRunMu sync.Mutex
var toolRunGenerations = map[string]uint64{}

// nextToolRunGeneration records a newly admitted run for opID. Call only
// after RegisterOp succeeds: refused runs must not advance the sequence.
func nextToolRunGeneration(opID string) uint64 {
	toolRunMu.Lock()
	defer toolRunMu.Unlock()
	toolRunGenerations[opID]++
	return toolRunGenerations[opID]
}

// isCurrentToolRun reports whether gen is still the latest admitted run for
// opID and the op is still registered. A cancelled op is unregistered;
// a superseded op is registered but stale. Either way its tail is dropped.
func isCurrentToolRun(opID string, gen uint64) bool {
	if !isOpRegistered(opID) {
		return false
	}
	toolRunMu.Lock()
	defer toolRunMu.Unlock()
	return toolRunGenerations[opID] == gen
}

// finishToolRun frees the op only when gen is still the latest admitted run.
// The lock id is stable per project across reruns, so a superseded watcher's
// deferred FinishOp must not remove its successor's registration: without
// the generation guard, A's stale watcher would unregister B and B's own
// lines would then fail the publish gate.
func finishToolRun(opID string, gen uint64) {
	toolRunMu.Lock()
	current := toolRunGenerations[opID] == gen
	toolRunMu.Unlock()
	if current {
		FinishOp(opID)
	}
}

// IsCurrentToolGeneration is the consumer-side staleness check for terminal
// tool events: apply a completed/failed payload only when its Gen equals the
// latest admitted generation for its OpID. It deliberately ignores
// registration: a finished run unregisters itself, but its generation stays
// current until the next run is admitted, so late-but-current receipts still
// apply while stale dead-run receipts do not.
func IsCurrentToolGeneration(opID string, gen uint64) bool {
	toolRunMu.Lock()
	defer toolRunMu.Unlock()
	current, ok := toolRunGenerations[opID]
	return ok && current == gen
}

// RunTool resolves the preset once and uses that single instance for both
// validation and execution: unknown ids are refused before any process
// exists, and validated args render into argv in ArgSpec order with no
// shell. It returns the op id immediately; output streams on tool:output
// and the outcome arrives on tool:completed or tool:failed.
func RunTool(ctx context.Context, p Platform, project string, presetID string, args map[string]string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p == nil {
		return "", fmt.Errorf("platform is required")
	}
	trimmedPreset := strings.TrimSpace(presetID)
	preset, ok := FindToolPreset(trimmedPreset)
	if !ok {
		return "", fmt.Errorf("unknown tool preset %q", presetID)
	}
	if err := validateToolArgsResolved(preset, args); err != nil {
		return "", err
	}
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}
	argv := buildToolArgv(preset, args)

	opID := toolOpIDForRoot(root)
	runCtx, cancel := context.WithTimeout(ctx, toolRunTimeout(trimmedPreset))
	if !RegisterOp(opID, strings.TrimSpace(project), "tool", cancel) {
		cancel()
		return "", fmt.Errorf("tool run for project %q is already running", strings.TrimSpace(project))
	}
	gen := nextToolRunGeneration(opID)
	stdout, stderr, wait, err := startToolProcess(runCtx, root, argv)
	if err != nil {
		FinishOp(opID)
		cancel()
		return "", err
	}
	// Record the admission before the watcher starts: the entry carries the
	// unknown outcome until the terminal event promotes it, so a run whose
	// terminal never arrives (crash) reads unknown, never success. Args are
	// scrubbed on write; the raw values never reach the file. Best-effort:
	// a history failure must never fail the run itself.
	_ = appendToolHistoryEntryForRoot(root, ToolHistoryEntry{
		OpID:      opID,
		Gen:       gen,
		Preset:    trimmedPreset,
		Args:      scrubToolHistoryArgs(args),
		Timestamp: time.Now().UTC(),
		Outcome:   ToolHistoryOutcomeUnknown,
	})
	go watchToolRun(runCtx, p, opID, gen, root, strings.TrimSpace(project), trimmedPreset, stdout, stderr, wait, cancel)
	return opID, nil
}

// CancelToolRun kills the running tool op's process via the op registry and
// frees its per-project lock. Cancelling an unknown or finished id is a
// harmless no-op reporting false.
func CancelToolRun(opID string) bool {
	return CancelOp(opID)
}

// watchToolRun drains both pipes through the shared 50-line/150ms batching
// pump (the same streamSanitizedBatches behind log and sync streaming), then
// reports the outcome and frees the op. A done run context means the process
// was killed by cancel or deadline, never a clean failure. The history entry
// for (opID, gen) is promoted to the terminal outcome before its event
// emits, so consumers observing the event see the final history.
func watchToolRun(ctx context.Context, p Platform, opID string, gen uint64, root string, project string, presetID string, stdout io.Reader, stderr io.Reader, wait func() error, cancel context.CancelFunc) {
	defer cancel()
	defer finishToolRun(opID, gen)

	done := make(chan struct{}, 2)
	go scanToolPipe(p, opID, gen, stdout, done)
	go scanToolPipe(p, opID, gen, stderr, done)
	<-done
	<-done

	if err := wait(); err != nil {
		outcome := ToolHistoryOutcomeFailed
		message := "Tool run cancelled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			message = fmt.Sprintf("Tool run timed out after %v", toolRunTimeout(presetID))
		} else if ctx.Err() == nil {
			message = "Tool run failed: " + strings.TrimSpace(err.Error())
		} else {
			outcome = ToolHistoryOutcomeCancelled
		}
		_ = updateToolHistoryOutcomeForRoot(root, opID, gen, outcome)
		p.Emit(EventToolFailed, ToolResultPayload{
			OpID:    opID,
			Project: project,
			Preset:  presetID,
			Message: sanitizeUIText(message, toolResultMessageMax),
			Gen:     gen,
		})
		return
	}
	_ = updateToolHistoryOutcomeForRoot(root, opID, gen, ToolHistoryOutcomeSuccess)
	p.Emit(EventToolCompleted, ToolResultPayload{
		OpID:    opID,
		Project: project,
		Preset:  presetID,
		Message: "Tool run completed",
		Gen:     gen,
	})
}

// scanToolPipe streams one tool pipe into the UI on top of the shared log
// batching: every flush emits the batched sanitized lines on tool:output,
// tagged with the emitting op. Batches from an op that is no longer current
// for the project (cancelled, timed out, or superseded by a rerun) are
// dropped, so a fast rerun never mixes tails. Terminal completed/failed
// events are intentionally ungated: the cancellation receipt itself must
// arrive, and Gen lets the palette apply it only when current
// (IsCurrentToolGeneration); the synchronous CancelOp return is the
// cancelling caller's confirmation, so no event is needed for that path.
func scanToolPipe(p Platform, opID string, gen uint64, pipe io.Reader, done chan<- struct{}) {
	streamSanitizedBatches(p, pipe, func(batch []string) {
		if !isCurrentToolRun(opID, gen) {
			return
		}
		p.Emit(EventToolOutput, ToolOutputPayload{OpID: opID, Text: strings.Join(batch, "\n")})
	})
	done <- struct{}{}
}

// toolRunTimeout maps a preset to its execution ceiling from the shared
// govardCommandTimeout policy: slow presets (compile, reindex, migrate,
// installs, builds) get the long ceiling so a 15-minute run is never killed
// by the short default; fast cache clears keep it.
func toolRunTimeout(presetID string) time.Duration {
	switch strings.TrimSpace(presetID) {
	case "magento-setup-di-compile", "magento-indexer-reindex",
		"laravel-migrate", "composer-install", "npm-install", "npm-run-build":
		return govardCommandLongTimeout
	default:
		return govardCommandDefaultTimeout
	}
}

// buildToolArgv renders validated args onto the preset's exec-ready Argv in
// ArgSpec order. Flag args become --name (set) or are omitted (false/0);
// other args append positionally. It never joins, quotes, or shells: the
// result goes straight to exec.Command.
func buildToolArgv(preset ToolPreset, args map[string]string) []string {
	argv := make([]string, 0, len(preset.Argv)+len(args))
	argv = append(argv, preset.Argv...)
	for _, spec := range preset.ArgSpec {
		value, present := args[spec.Name]
		if !present {
			continue
		}
		if spec.Flag {
			switch value {
			case "", "true", "1":
				argv = append(argv, "--"+spec.Name)
			case "false", "0":
				// Flag off: omit.
			default:
				argv = append(argv, "--"+spec.Name+"="+value)
			}
			continue
		}
		if value == "" {
			continue
		}
		argv = append(argv, value)
	}
	return argv
}

// ToolService methods: the Wails-bound surface for the palette. The palette
// sends a preset ID plus an args map only; the exec argv stays backend-owned
// (see buildToolArgv), so a hostile frontend cannot upgrade to a shell.

// ListToolPresets returns the preset allowlist for a project. Resolution
// failures surface as errors (never a misleading list) so the palette can
// show guidance instead of presets.
func (s *ToolService) ListToolPresets(project string) (res []ToolPreset, err error) {
	defer RecoverPanic(&err, "ListToolPresets")
	return ListToolPresets(project)
}

// RunTool validates the preset ID plus args map and starts the run, returning
// the op ID immediately. Output streams on tool:output; the outcome arrives
// on tool:completed or tool:failed tagged with the run generation.
func (s *ToolService) RunTool(project string, presetID string, args map[string]string) (res string, err error) {
	defer RecoverPanic(&err, "RunTool")
	return RunTool(s.lifecycleContext(), s.platform, project, presetID, args)
}

// CancelToolRun kills the running tool op's process and frees its
// per-project lock. Cancelling an unknown or finished id reports false.
func (s *ToolService) CancelToolRun(opID string) (res bool, err error) {
	defer RecoverPanic(&err, "CancelToolRun")
	return CancelToolRun(opID), nil
}

// ValidateToolArgs checks args against the allowlisted preset's ArgSpec:
// the preset is resolved by id inside the allowlist, so a caller-supplied
// preset can never bypass membership. Unknown ids, unknown arg names,
// missing required args, over-long values, and values outside the arg's
// pattern (default: no shell metacharacters) are rejected.
func ValidateToolArgs(presetID string, args map[string]string) error {
	preset, ok := FindToolPreset(strings.TrimSpace(presetID))
	if !ok {
		return fmt.Errorf("unknown tool preset %q", presetID)
	}
	return validateToolArgsResolved(preset, args)
}

// validateToolArgsResolved checks args against one already-resolved preset.
// RunTool resolves via FindToolPreset once and reuses that instance for
// validation and execution, so the allowlist is consulted exactly once per
// run and no second lookup can disagree with the first.
func validateToolArgsResolved(preset ToolPreset, args map[string]string) error {
	specs := make(map[string]ToolArgSpec, len(preset.ArgSpec))
	for _, spec := range preset.ArgSpec {
		specs[spec.Name] = spec
	}
	for name := range args {
		if _, ok := specs[name]; !ok {
			return fmt.Errorf("preset %q has no argument %q", preset.ID, name)
		}
	}
	for _, spec := range preset.ArgSpec {
		value, present := args[spec.Name]
		if !present || value == "" {
			if spec.Required {
				return fmt.Errorf("preset %q requires argument %q", preset.ID, spec.Name)
			}
			continue
		}
		if len(value) > maxToolArgLength {
			return fmt.Errorf("preset %q argument %q exceeds %d characters", preset.ID, spec.Name, maxToolArgLength)
		}
		pattern := spec.Pattern
		if pattern == "" {
			pattern = defaultSafeToolArgPattern
		}
		matched, err := regexp.MatchString(pattern, value)
		if err != nil || !matched {
			return fmt.Errorf("preset %q argument %q has an invalid value", preset.ID, spec.Name)
		}
	}
	return nil
}
