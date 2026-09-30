package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"govard/internal/engine"
)

// VerifyOpts controls execution of verify items.
type VerifyOpts struct {
	Plan             bool
	JSON             bool
	Remote           string
	BaseRef          string
	Timeout          string
	Checks           []string
	LintJobs         int
	AllowDestructive bool
	AllowRemoteWrite bool
	AllowXdebug      bool
	ProjectRoot      string
}

// Evidence is the per-item execution result.
type Evidence struct {
	DurationMs    int
	ExitCode      int
	OutputExcerpt string
	JSONValid     bool
	// Artifacts names what this item produced (a snapshot directory, for
	// example). The phase-5 gate uses it to prove a restore target exists.
	Artifacts []string
	// Skipped marks an item that did not run because it does not apply here.
	// SkipReason says why. See Skip.
	Skipped    bool
	SkipReason string
	// Fake marks evidence that no real process produced (the hermetic test
	// hook). It flows to the run artifact, which then carries a fake marker
	// (Status stays passed).
	Fake bool
}

// The Guard values an item may carry. DecideGuard is their only reader:
// GuardRemoteProbe and the empty value run, the other two are gated.
const (
	// GuardRemoteProbe marks an item whose argv names a remote and writes
	// nothing there. It is a label, not a protection: plan mode is its only
	// gate, because --plan replaces every Run with a stub. On a live run the
	// item reaches the remote unconditionally, and a runtime gate would delete
	// the coverage instead of protecting anything. The name says "probe" so
	// nobody reads it as a guarantee.
	GuardRemoteProbe = "REMOTE-PROBE"
	// GuardDestructiveLocal marks an item that destroys local state
	// irreversibly. It runs in phase 5 only.
	GuardDestructiveLocal = "DESTRUCTIVE-LOCAL"
	// GuardRemoteWrite marks an item whose argv writes through a remote. It is
	// skipped unless the operator opted in with --allow-remote-write.
	GuardRemoteWrite = "REMOTE-WRITE"
)

// Item is one checklist entry in the 5-phase registry.
type Item struct {
	ID    string
	Title string
	// Requires documents, for a human reading the registry, what this item
	// assumes about the run. It is documentation, not a verdict: the runner
	// never enforces it, and a skip reason must not quote it, because these
	// strings name prior steps ("P2-01 up") while the gate that fires is a
	// framework predicate. frameworkGateReason is what a gated row reports.
	Requires string
	// Guard is the item's taxonomy label, not documentation: the runner acts on
	// it through DecideGuard.
	Guard string
	// Checks names the checks (`lint`, `profiler`, `integrity`) this item
	// exercises, for a run that selected some with --checks. Empty means the
	// item is not check-specific: nothing about it belongs to one check, so a
	// run keeps it whatever it selected. RunPhase decides what the selection
	// leaves out.
	Checks []string
	Phase  int
	When   func(engine.Config) bool
	// WhenReason, when set, explains an unmet When in the item's own terms. It
	// returns "" to fall back to the framework reason.
	WhenReason func(engine.Config) string
	Run        func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence
}

// isMagento2 reports whether cfg targets Magento 2 (or Mage-OS which normalizes to magento2).
func isMagento2(c engine.Config) bool {
	return c.Framework == "magento2"
}

// frontendSyncEnabled gates P3-09: the frontend command only has something to
// start on a framework that supports it, with stack.features.frontend_sync on.
func frontendSyncEnabled(c engine.Config) bool {
	return isMagento2(c) && c.Stack.Features.FrontendSync
}

// frontendSyncGateReason names the frontend_sync switch when the framework is
// right but the feature is off; any other miss is a framework gate.
func frontendSyncGateReason(c engine.Config) string {
	if isMagento2(c) && !c.Stack.Features.FrontendSync {
		return "stack.features.frontend_sync is off: enable it in .govard.yml to exercise `govard frontend start`"
	}
	return ""
}

// withRemote wraps a remote-touching item. A checklist run that was not told
// which remote to use does not guess one: guessing meant probing whatever the
// project happens to call staging.
func withRemote(inner func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence) func(context.Context, engine.Config, VerifyOpts) Evidence {
	return func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		if opts.Remote == "" {
			return Skip("no --remote named: this item contacts a remote")
		}
		return inner(ctx, cfg, opts, opts.Remote)
	}
}

// hyvaTailwindGlob is the marker that makes a theme a Hyva theme: the Tailwind
// manifest its install needs. One vendor and one theme deep, so a theme at an
// unusual path is simply not a match — the item then skips instead of
// installing into a directory nobody asked for.
const hyvaTailwindGlob = "app/design/frontend/*/*/web/tailwind/package.json"

// escapeGlobPath escapes the glob metacharacters in a literal directory path so
// it can be used as the fixed head of a pattern.
//
// filepath.Match's syntax (path/filepath/match.go, "pattern: { term }") makes
// exactly four characters magic: `*`, `?`, `[` and the escape character `\`.
// `]` outside a character class is an ordinary character, so it is left alone.
// On Windows, Match's own documentation says escaping is disabled and `\` is a
// path separator instead, so the separator must be left alone there — a doubled
// separator is not an escape, it is a corrupted path — and only the other three
// are escaped. hyvaTailwindDirs's containment check is what makes a
// metacharacter in the root harmless on a platform where escaping does not
// apply.
func escapeGlobPath(path string) string {
	var escaped strings.Builder
	escaped.Grow(len(path))
	for _, r := range path {
		switch r {
		case '*', '?', '[':
			escaped.WriteRune('\\')
		case '\\':
			if filepath.Separator != '\\' {
				escaped.WriteRune('\\')
			}
		}
		escaped.WriteRune(r)
	}
	return escaped.String()
}

// hyvaTailwindDirs lists the Tailwind roots of the project's Hyva themes as
// project-root-relative paths with forward slashes, sorted lexicographically so
// that a project with several themes always resolves to the same one.
//
// It is a filesystem rule, not a framework call: internal/verify names no
// framework package, and the marker is the manifest `npm install` needs rather
// than a theme's name or its vendor. A root that cannot be read, or a path that
// no longer exists when it is stat'ed, contributes no theme — skipping is the
// safe direction for a rule whose failure mode is a stray package-lock.json
// written into the checkout (issue #494).
func hyvaTailwindDirs(projectRoot string) []string {
	pattern := filepath.Join(escapeGlobPath(projectRoot), filepath.FromSlash(hyvaTailwindGlob))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	dirs := make([]string, 0, len(matches))
	for _, match := range matches {
		// Glob matches the name alone, so a *directory* called package.json
		// would be collected too. A theme's marker is a regular file.
		info, err := os.Stat(match)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		rel, err := filepath.Rel(projectRoot, filepath.Dir(match))
		if err != nil {
			continue
		}
		// A candidate that escapes the project root would make --prefix point
		// outside the checkout, and npm install would write into another
		// project's theme — the harm class this item exists to prevent. A
		// containment test on the path string cannot see it: the match comes
		// from the pattern, so a sibling reached through a glob metacharacter in
		// the root satisfies "starts with <root>/" lexically, and `..` is not an
		// absolute path. IsLocal is lexical on purpose, so a theme reached
		// through a symlinked directory still resolves.
		if !filepath.IsLocal(rel) {
			continue
		}
		dirs = append(dirs, filepath.ToSlash(rel))
	}
	sort.Strings(dirs)
	return dirs
}

// hyvaTailwindDir returns the directory P2-09 installs into — the first Hyva
// theme in sorted order — or false when the project has no Hyva theme at all.
func hyvaTailwindDir(projectRoot string) (string, bool) {
	dirs := hyvaTailwindDirs(projectRoot)
	if len(dirs) == 0 {
		return "", false
	}
	return dirs[0], true
}

// hyvaThemeEvidenceLine names the theme the item installed into, and discloses
// the choice when the project has more than one: an artifact that silently
// picked one of several themes would be as hard to audit as the row that named
// no theme at all. It re-runs the discovery instead of widening
// hyvaTailwindDir's return, which the decision itself does not need.
func hyvaThemeEvidenceLine(projectRoot, theme string) string {
	if themes := hyvaTailwindDirs(projectRoot); len(themes) > 1 {
		return fmt.Sprintf("Hyva theme: %s (first of %d Hyva themes, sorted)\n", theme, len(themes))
	}
	return "Hyva theme: " + theme + "\n"
}

// verifyLintJobsDefault and verifyTimeoutDefault are the `verify` command's own
// flag defaults (internal/cmd/verify.go). The audit items used to hardcode a
// worker count and a timeout; they read the flag instead, so these two values
// are how an item tells "the operator asked for this" from "the flag was not
// touched" — an item sees the default, never whether it was passed.
const (
	verifyLintJobsDefault = 4
	verifyTimeoutDefault  = "auto"
)

// FlagDefaultsForTest returns the two defaults the audit items read, so a test
// can pin them to the values the `verify` command actually registers
// (internal/cmd/verify.go) instead of repeating the literals. A drift between the
// two would silently stop an untouched flag from meaning "no preference": the
// item would forward a value the operator never asked for.
func FlagDefaultsForTest() (lintJobs int, timeout string) {
	return verifyLintJobsDefault, verifyTimeoutDefault
}

// p5RelintTimeout is P5-04's own value: the phase-5 re-lint drops the lint
// result cache and re-analyses everything, so it runs without a deadline. A
// default is not a request for one, so the item keeps 0.
const p5RelintTimeout = "0"

// auditLintJobsArgs returns the `--lint-jobs` pair for an audit item, or nothing
// when the operator chose no count.
//
// Verify's flag default is a literal 4, while `audit run --lint-jobs` defaults
// to engine.AuditRunJobs() — min(nproc,4) clamped 2-4, the host's auto-tuned
// worker count. Forwarding verify's own 4 would override that tuning on every
// run (measured: it is the value in the argv today, as a literal), so an
// untouched flag stays out of the argv and the child chooses. A non-positive
// value is not a request either: `audit run` rejects fewer than one worker.
func auditLintJobsArgs(opts VerifyOpts) []string {
	if opts.LintJobs < 1 || opts.LintJobs == verifyLintJobsDefault {
		return nil
	}
	return []string{"--lint-jobs", strconv.Itoa(opts.LintJobs)}
}

// auditTimeoutArgs returns the `--timeout` pair for an audit item whose own
// value is itemDefault. Verify's flag default ("auto") means the operator asked
// for nothing, and what that leaves is per item: `auto` for the phase-3 lint
// items — which is also `audit run`'s own default — and 0 for the phase-5
// re-lint, whose no-deadline choice verify's default must not turn into one.
func auditTimeoutArgs(opts VerifyOpts, itemDefault string) []string {
	timeout := strings.TrimSpace(opts.Timeout)
	if timeout == "" || timeout == verifyTimeoutDefault {
		timeout = itemDefault
	}
	return []string{"--timeout", timeout}
}

// argvEvidence prefixes an item's evidence with the argv that ran. RunItem's
// Command is the item's Title (runner.go), so a title cannot carry a value the
// run resolves — the title would describe an argv that did not run. The
// resolved argv goes here instead.
func argvEvidence(args []string, ev Evidence) Evidence {
	ev.OutputExcerpt = "argv: " + strings.Join(args, " ") + "\n" + ev.OutputExcerpt
	return ev
}

// auditRunIdentity is the part of `audit run --format json` the lifecycle needs:
// the session it created and the run it recorded. It is a local struct rather
// than internal/audit's RunResult — verify drives the CLI, so it must not gain a
// dependency on the audit store.
type auditRunIdentity struct {
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
}

// auditRunIdentityFrom reads session_id and run_id out of a run's JSON excerpt.
//
// It walks tokens instead of calling json.Unmarshal for two structural reasons.
// execGovard keeps only the first 500 characters of a child's output
// (exec.go:66-69) while a real run is far longer — every job's evidence map is
// inlined, measured at 1996 bytes on a skeleton project — so the document is
// normally cut short and Unmarshal would reject it, turning the item into a
// permanent skip. And the excerpt is the child's merged stdout and stderr
// (exec.go:52-53), so a line logged before the JSON sits in front of it — a
// queued or stale audit lock logs through slog on exactly that path
// (internal/cmd/audit_target.go:290, :328, :345). Both ids are encoded before
// anything else (internal/audit/model.go:92-93), so the walk starts at the first
// brace and stops as soon as it has them; it never has to read the truncated
// tail.
func auditRunIdentityFrom(excerpt string) auditRunIdentity {
	var identity auditRunIdentity
	start := strings.Index(excerpt, "{")
	if start < 0 {
		return identity
	}
	decoder := json.NewDecoder(strings.NewReader(excerpt[start:]))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return identity
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			break
		}
		name, ok := key.(string)
		if !ok {
			break
		}
		switch name {
		case "session_id":
			_ = decoder.Decode(&identity.SessionID)
		case "run_id":
			_ = decoder.Decode(&identity.RunID)
		default:
			// Skipping a value means consuming it whole, nested objects and
			// arrays included; a truncated one ends the walk with what was read.
			var discard json.RawMessage
			if err := decoder.Decode(&discard); err != nil {
				return identity
			}
		}
		if identity.SessionID != "" && identity.RunID != "" {
			return identity
		}
	}
	return identity
}

// auditLifecycleEvidence is P3-15. Every command in its title needs a session id
// the item has to supply itself: `audit status` errors with "audit status
// requires --session" before it reads any project state (internal/cmd/audit.go:
// 425) and `audit result` also requires --run (:451), so the row was red on
// every project and could never go green (issue #493).
//
// The session comes from the item's own container-free `audit run --checks
// integrity` — the `audit` group declares runtime requirement `none`
// (internal/cmd/audit.go:194) and a run only needs a container runtime for the
// lint/profiler checks it did not select (:250-256) — never from a
// package-level variable: Run receives VerifyOpts by value and the registry is
// shared, so remembering the last session there would be mutable library state
// serving one item.
//
// The creating run is this item's session factory, not its subject: its exit
// code is the integrity verdict (a manifest finding makes it exit non-zero while
// still printing the JSON that carries the ids), and the item does not read it —
// rerun re-executes the same checks, so a findings verdict still reaches the row
// through the lifecycle. A run that yielded no complete identity — no project, no
// framework, an xdebug guard this run did not waive, or a decode that stopped
// between the two ids — skips, and its reason names the missing half and the
// child's exit code so the artifact says why.
//
// Because that `--checks integrity` create run is machinery rather than the
// item's subject, the item declares no Checks: `Checks` names the checks an
// item's subject verifies, so declaring integrity here would let a `--checks
// profiler` (or any other) selection drop the audit lifecycle that the lint
// audits' session is the precondition for. It stays check-specific to nothing,
// and the next reader should not "fix" it into declaring one.
func auditLifecycleEvidence(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
	create := []string{"audit", "run", "--checks", "integrity", "--format", "json"}
	if opts.AllowXdebug {
		create = append(create, "--allow-xdebug")
	}
	created := execGovard(ctx, cfg, opts, create...)
	identity := auditRunIdentityFrom(created.OutputExcerpt)
	// Both ids are required, not just the session: `audit result` rejects an
	// empty --run (internal/cmd/audit.go:451), so a half identity would turn the
	// skip the brief mandates into a red. The decode can stop between the two
	// keys — the excerpt is cut at 500 characters (exec.go:66-69) and a walk that
	// hits the cut after session_id returns exactly that half identity.
	if identity.SessionID == "" || identity.RunID == "" {
		// Only a clean exit with no ids is "nothing to drive". A creating run
		// that failed is a failure the row must show: skipping it would let a
		// broken audit pass as not applicable.
		if created.ExitCode != 0 {
			return created
		}
		return Skip(auditIdentitySkipReason(identity, created.ExitCode))
	}
	// Exit 3 (capability missing) and 4 (configuration) are never a findings
	// verdict, even when a JSON body carried ids: the run did not do its job.
	if created.ExitCode == 3 || created.ExitCode == 4 {
		return created
	}

	status := []string{"audit", "status", "--session", identity.SessionID, "--format", "json"}
	result := []string{"audit", "result", "--session", identity.SessionID, "--run", identity.RunID, "--format", "json"}
	rerun := []string{"audit", "rerun", "--session", identity.SessionID, "--format", "json"}
	// `--allow-xdebug` belongs on the two calls that enforce the guard — `run`
	// and `rerun` (internal/cmd/audit.go:264, :384) — not on the read-only pair,
	// which never consult it. The flag is persistent on the audit group (:211),
	// so all four subcommands accept it; waiving it on the creating run alone
	// would just move the red to the rerun that follows.
	if opts.AllowXdebug {
		rerun = append(rerun, "--allow-xdebug")
	}

	// The verdict is the lifecycle's: a failing command is returned as it is
	// instead of being overwritten by a later green one, and every non-zero exit
	// is red. Measured on this branch: `status` and `result` never exit non-zero
	// for a findings verdict (they exit 0 and print the session or the run),
	// while `rerun` exits 1 both for "the re-run adjudicated findings" (exit 1
	// *with* the RunResult on stdout) and for a genuine failure (exit 1 with
	// nothing on stdout), so the code alone cannot separate the two and the item
	// does not try. Red set: 1 (execution), 2 (usage), 3 (capability), 4
	// (config).
	var evidence Evidence
	for _, argv := range [][]string{status, result, rerun} {
		evidence = execGovard(ctx, cfg, opts, argv...)
		if evidence.ExitCode != 0 {
			return evidence
		}
	}
	return evidence
}

// auditIdentitySkipReason says which half of the identity the creating run did
// not yield, and what that run exited with.
//
// The exit code is the whole diagnostic this skip can carry: Skip has no
// evidence field, and the child's excerpt is truncated log noise that must not
// be pasted into a report. A zero here means the run claims success yet printed
// nothing usable — a decode that stopped inside the document — while a non-zero
// one says the child itself failed and the reason is that failure, not the parse.
//
// The phrase "audit run produced no session id" is the one the brief pins, and it
// survives verbatim whenever the session id is what is missing.
func auditIdentitySkipReason(identity auditRunIdentity, exitCode int) string {
	missing := make([]string, 0, 2)
	if identity.SessionID == "" {
		missing = append(missing, "session id")
	}
	if identity.RunID == "" {
		missing = append(missing, "run id")
	}
	return fmt.Sprintf("audit run produced no %s (exit %d)", strings.Join(missing, " or "), exitCode)
}

// noAuditModuleReason is what both module-scoped audit items report when there
// is nothing to audit: the project has no module, or its framework registers no
// module discovery at all. Both rows stay in the report as skips — a `When`
// would have dropped them — so the reason is the whole account of why the phase
// audited no module.
const noAuditModuleReason = "no Magento module under app/code"

// standaloneAuditFixtureDirName is the parent P3-14's title names for its
// scratch copy of the module, below the OS temp directory.
const standaloneAuditFixtureDirName = "govard-audit-standalone"

// auditModuleDir asks the project's framework for the module directory the
// audit's module_in_project and standalone modes require. internal/verify names
// no framework package: the discovery is registered by the framework (Magento 2
// globs it out of app/code) and read back through the engine, so a framework
// without a module concept simply reports nothing and both rows skip.
func auditModuleDir(cfg engine.Config, opts VerifyOpts) (string, bool) {
	support, ok := engine.VerifySupportFor(cfg.Framework)
	if !ok || support.AuditModuleDir == nil {
		return "", false
	}
	return support.AuditModuleDir(opts.ProjectRoot)
}

// errStandaloneFixtureOccupied reports that the titled fixture path was already
// taken, so this run did not create it and must not touch it.
var errStandaloneFixtureOccupied = errors.New("standalone audit fixture already exists")

// standaloneAuditFixtureDir is the path P3-14's title names for its scratch copy
// of moduleDir: <tmp>/govard-audit-standalone/<Module>, below the OS temp
// directory.
func standaloneAuditFixtureDir(moduleDir string) string {
	return filepath.Join(os.TempDir(), standaloneAuditFixtureDirName, filepath.Base(filepath.Clean(moduleDir)))
}

// standaloneFixtureOccupiedReason is the skip a run reports when the titled
// fixture path is already taken. The item must not clear it — a directory the
// operator already had there is not the item's to delete — and only a human can
// tell a leftover of an interrupted run from a tree that was never ours.
func standaloneFixtureOccupiedReason(fixtureDir string) string {
	return fmt.Sprintf("%s already exists and was not created by this run; remove it if it is a leftover from an interrupted run", fixtureDir)
}

// prepareStandaloneAuditFixture creates the directory P3-14's title names —
// <tmp>/govard-audit-standalone/<Module> — and copies the project's module into
// it. Standalone mode requires a module with no Magento project above it, so the
// copy has to leave the project tree: auditing the module in place would resolve
// its enclosing project and the mode would refuse it. Nothing in the copy is
// written by the audit — the child scans it read-only — so the module in the
// project is only ever read.
//
// Creation is exclusive, and what this call did not create it never removes. The
// path is fixed by the item's title, which makes it shared machine state: a
// directory already there belongs to someone else — a tree the operator keeps at
// that path, or a concurrent run that won the same module name — so it is
// reported as occupied (errStandaloneFixtureOccupied) instead of being replaced.
// That is what keeps the second of two same-named runs a skip rather than a red,
// and what makes the caller's deferred removal safe: it only ever runs on the
// directory this call created.
//
// On errStandaloneFixtureOccupied the returned path is still filled in, because
// the skip reason has to name the directory the operator must inspect.
func prepareStandaloneAuditFixture(moduleDir string) (string, error) {
	module := filepath.Base(filepath.Clean(moduleDir))
	if module == "" || module == "." || module == string(filepath.Separator) {
		// The fixture path is built from this name, so a nameless module must fail
		// here rather than resolve to the shared parent directory.
		return "", fmt.Errorf("module directory %q has no name to build a standalone fixture from", moduleDir)
	}
	info, err := os.Stat(moduleDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", moduleDir)
	}

	fixtureDir := standaloneAuditFixtureDir(moduleDir)
	perm := info.Mode().Perm()
	if err := os.MkdirAll(filepath.Dir(fixtureDir), perm); err != nil {
		return "", fmt.Errorf("create standalone audit fixture parent for %s: %w", fixtureDir, err)
	}
	if err := os.Mkdir(fixtureDir, perm); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fixtureDir, fmt.Errorf("%w: %s", errStandaloneFixtureOccupied, fixtureDir)
		}
		return "", fmt.Errorf("create standalone audit fixture %s: %w", fixtureDir, err)
	}
	if err := copyTree(moduleDir, fixtureDir); err != nil {
		// Only the directory this call just created is removed: it is incomplete
		// scratch state, and leaving it behind would make the next run skip.
		_ = os.RemoveAll(fixtureDir)
		return "", fmt.Errorf("prepare standalone audit fixture %s: %w", fixtureDir, err)
	}
	return fixtureDir, nil
}

// copyTree copies src into dst recursively. Directories keep the source's own
// permissions, regular files their bytes, and symlinks are recreated as
// symlinks rather than followed — a link inside a module usually points at a
// sibling, and following it would either duplicate the tree or loop.
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		source := filepath.Join(src, entry.Name())
		target := filepath.Join(dst, entry.Name())
		entryInfo, err := os.Lstat(source)
		if err != nil {
			return err
		}
		switch {
		case entryInfo.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(source)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		case entryInfo.IsDir():
			if err := copyTree(source, target); err != nil {
				return err
			}
		case entryInfo.Mode().IsRegular():
			contents, err := os.ReadFile(source)
			if err != nil {
				return err
			}
			if err := os.WriteFile(target, contents, entryInfo.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

// Registry is the static checklist: 60 items across 5 phases
// (P1 7 + P2 14 + P3 15 + P4 16 + P5 8). Every item carries a Guard label and
// the runner acts on it through DecideGuard: an empty Guard is local work with
// no remote and nothing irreversible, GuardRemoteProbe documents that the
// argv names a remote and writes nothing there (no runtime gate of its own),
// GuardRemoteWrite is skipped unless --allow-remote-write was passed, and
// GuardDestructiveLocal runs in phase 5 only. An item that puts a remote in its
// argv is wrapped in withRemote instead of defaulting one: it takes the name
// from --remote and skips when the run named none.
var Registry = []Item{
	// Phase 1 — Preflight (7)
	{ID: "P1-01", Phase: 1, Title: "govard doctor", Requires: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "doctor")
	}},
	{ID: "P1-02", Phase: 1, Title: "govard doctor --json", Requires: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "doctor", "--json")
	}},
	{ID: "P1-03", Phase: 1, Title: "govard doctor trust", Requires: "P1-02 green", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "doctor", "trust")
	}},
	{ID: "P1-04", Phase: 1, Title: "govard config get project_name", Requires: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "config", "get", "project_name")
	}},
	{ID: "P1-05", Phase: 1, Title: "govard env config", Requires: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "env", "config")
	}},
	{ID: "P1-06", Phase: 1, Title: "govard lock check + govard lock diff", Requires: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		// A project with no lock file has nothing to check. Generating one here
		// would write a tracked file from a verify phase, so the row skips and
		// names the command that creates it. A lock that exists and disagrees
		// with the project is a real finding and stays red.
		if _, err := os.Stat(engine.LockFilePath(opts.ProjectRoot)); err != nil && errors.Is(err, os.ErrNotExist) {
			return Skip("no lock file: run `govard lock generate` to create one, then rerun this item")
		}
		ev := execGovard(ctx, cfg, opts, "lock", "check")
		if ev.ExitCode != 0 {
			// Run diff for evidence only; the verdict stays the check's exit.
			ev2 := execGovard(ctx, cfg, opts, "lock", "diff")
			ev.OutputExcerpt = ev.OutputExcerpt + " | diff: " + ev2.OutputExcerpt
		}
		return ev
	}},
	{ID: "P1-07", Phase: 1, Title: "govard status", Requires: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "status")
	}},

	// Phase 2 — Bootstrap & Env (14)
	{ID: "P2-01", Phase: 2, Title: "govard env down -> govard env up -> govard env ps", Requires: "P1 green", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		_ = execGovard(ctx, cfg, opts, "env", "down")
		ev := execGovard(ctx, cfg, opts, "env", "up")
		if ev.ExitCode != 0 {
			return ev
		}
		return execGovard(ctx, cfg, opts, "env", "ps")
	}},
	{ID: "P2-02", Phase: 2, Title: "govard env logs --tail 20", Requires: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "env", "logs", "--tail", "20")
	}},
	{ID: "P2-03", Phase: 2, Title: "govard env up --force-recreate", Requires: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "env", "up", "--force-recreate")
	}},
	{ID: "P2-04", Phase: 2, Title: "govard bootstrap -e <remote> --no-noise --plan", Requires: "P2-01 up", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "bootstrap", "-e", remote, "--no-noise", "--plan")
	})},
	{ID: "P2-05", Phase: 2, Title: "govard bootstrap -e <remote> --no-noise", Requires: "P2-04 plan ok", Guard: GuardRemoteWrite, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "bootstrap", "-e", remote, "--no-noise")
	})},
	{ID: "P2-06", Phase: 2, Title: "govard bootstrap --clone -e <remote> --plan", Requires: "P2-01 up", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--plan")
	})},
	{ID: "P2-07", Phase: 2, Title: "govard bootstrap --clone -e <remote> --no-media --plan + --code-only --plan + --no-pii --plan", Requires: "P2-06 ok", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		ev := execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--no-media", "--plan")
		if ev.ExitCode != 0 {
			return ev
		}
		ev2 := execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--code-only", "--plan")
		if ev2.ExitCode != 0 {
			ev.OutputExcerpt += " | code-only: " + ev2.OutputExcerpt
			return ev2
		}
		ev3 := execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--no-pii", "--plan")
		ev.OutputExcerpt += " | no-pii: " + ev3.OutputExcerpt
		ev.ExitCode = ev3.ExitCode
		ev.JSONValid = ev3.JSONValid
		return ev
	})},
	{ID: "P2-08", Phase: 2, Title: "govard bootstrap --clone -e <remote> --no-noise", Requires: "P4-08 snapshot exists", Guard: GuardRemoteWrite, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--no-noise")
	})},
	{ID: "P2-09", Phase: 2, Title: "govard tool npm install in the Hyva theme's web/tailwind (Hyva only)", Requires: "P2-05 or P2-08", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		// When sees only engine.Config, which carries no project root, so the
		// Hyva rule is decided here. isMagento2 alone let this row install into
		// a Luma project, leaving a stray web/tailwind/package-lock.json in the
		// checkout (issue #494).
		theme, ok := hyvaTailwindDir(opts.ProjectRoot)
		if !ok {
			return Skip("no Hyva theme: " + hyvaTailwindGlob + " not found")
		}
		ev := execGovard(ctx, cfg, opts, "tool", "npm", "--prefix", theme, "install")
		ev.OutputExcerpt = hyvaThemeEvidenceLine(opts.ProjectRoot, theme) + ev.OutputExcerpt
		return ev
	}},
	{ID: "P2-10", Phase: 2, Title: "govard config auto", Requires: "P2-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "config", "auto")
	}},
	{ID: "P2-11", Phase: 2, Title: "govard tool magento --version", Requires: "P2-05 done", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "--version")
	}},
	{ID: "P2-12", Phase: 2, Title: "govard tool composer validate", Requires: "P2-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "composer", "validate")
	}},
	{ID: "P2-13", Phase: 2, Title: "<domain> answers over https (http only when the TLS handshake fails)", Requires: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return probeSite(ctx, cfg.Domain)
	}},
	{ID: "P2-14", Phase: 2, Title: "govard open --help", Requires: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "open", "--help")
	}},

	// Phase 3 — Dev Loop (15)
	{ID: "P3-01", Phase: 3, Title: "govard tool magento cache:flush", Requires: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "cache:flush")
	}},
	{ID: "P3-02", Phase: 3, Title: "govard tool magento setup:upgrade", Requires: "P3-01 ok", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "setup:upgrade")
	}},
	{ID: "P3-03", Phase: 3, Title: "govard tool magento setup:di:compile", Requires: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "setup:di:compile")
	}},
	{ID: "P3-04", Phase: 3, Title: "govard tool magento setup:static-content:deploy -f", Requires: "P2-09 Hyva theme present", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "setup:static-content:deploy", "-f")
	}},
	{ID: "P3-05", Phase: 3, Title: "govard tool magento indexer:reindex", Requires: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "indexer:reindex")
	}},
	{ID: "P3-06", Phase: 3, Title: "govard tool magento cron:run", Requires: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "cron:run")
	}},
	{ID: "P3-07", Phase: 3, Title: "govard tool php vendor/bin/phpstan analyse --help", Requires: "P2-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "php", "vendor/bin/phpstan", "analyse", "--help")
	}},
	{ID: "P3-08", Phase: 3, Title: "govard debug status", Requires: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "debug", "status")
	}},
	{ID: "P3-09", Phase: 3, Title: "govard frontend start", Requires: "P2-01 up", Guard: "", When: frontendSyncEnabled, WhenReason: frontendSyncGateReason, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "frontend", "start")
	}},
	{ID: "P3-10", Phase: 3, Title: "govard audit run --checks lint --scope project --mode auto --format json", Requires: "P2-01 up", Guard: "", Checks: []string{"lint"}, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		args := []string{"audit", "run", "--checks", "lint", "--scope", "project", "--mode", "auto", "--format", "json"}
		args = append(args, auditLintJobsArgs(opts)...)
		args = append(args, auditTimeoutArgs(opts, verifyTimeoutDefault)...)
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return argvEvidence(args, execGovard(ctx, cfg, opts, args...))
	}},
	{ID: "P3-11", Phase: 3, Title: "govard audit run --checks lint --scope diff --base {{BASE_BRANCH}} --format json", Requires: "P2-01 up", Guard: "", Checks: []string{"lint"}, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		base := opts.BaseRef
		if base == "" {
			base = "origin/master"
		}
		args := []string{"audit", "run", "--checks", "lint", "--scope", "diff", "--base", base, "--format", "json"}
		args = append(args, auditLintJobsArgs(opts)...)
		args = append(args, auditTimeoutArgs(opts, verifyTimeoutDefault)...)
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return argvEvidence(args, execGovard(ctx, cfg, opts, args...))
	}},
	{ID: "P3-12", Phase: 3, Title: "govard audit run --checks profiler --url https://{{DOMAIN}}/ --format json", Requires: "P2-01 up", Guard: "", Checks: []string{"profiler"}, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		if cfg.Domain == "" {
			return Skip("no domain configured: set `domain` in .govard.yml so the profiler has a URL to audit")
		}
		args := []string{"audit", "run", "--checks", "profiler", "--url", "https://" + cfg.Domain + "/", "--format", "json"}
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return argvEvidence(args, execGovard(ctx, cfg, opts, args...))
	}},
	{ID: "P3-13", Phase: 3, Title: "govard audit run --checks lint --mode module_in_project --format json (from app/code/<Vendor>/<Module>)", Requires: "P2-05 done", Guard: "", Checks: []string{"lint"}, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		moduleDir, ok := auditModuleDir(cfg, opts)
		if !ok {
			return Skip(noAuditModuleReason)
		}
		args := []string{"audit", "run", "--checks", "lint", "--mode", "module_in_project", "--format", "json", "--path", moduleDir}
		args = append(args, auditLintJobsArgs(opts)...)
		args = append(args, auditTimeoutArgs(opts, verifyTimeoutDefault)...)
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return argvEvidence(args, execGovard(ctx, cfg, opts, args...))
	}},
	{ID: "P3-14", Phase: 3, Title: "govard audit run --checks lint --mode standalone --format json (from os.TempDir()/govard-audit-standalone/<Module>)", Requires: "P3-13 ok", Guard: "", Checks: []string{"lint"}, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		moduleDir, ok := auditModuleDir(cfg, opts)
		if !ok {
			return Skip(noAuditModuleReason)
		}
		fixtureDir, err := prepareStandaloneAuditFixture(moduleDir)
		if errors.Is(err, errStandaloneFixtureOccupied) {
			return Skip(standaloneFixtureOccupiedReason(fixtureDir))
		}
		if err != nil {
			return Evidence{ExitCode: 1, OutputExcerpt: err.Error()}
		}
		// Reaching here means this run created the directory, and only a
		// directory it created is removed: the titled path is shared machine
		// state, so anything that was already there belongs to someone else.
		defer func() { _ = os.RemoveAll(fixtureDir) }()
		args := []string{"audit", "run", "--checks", "lint", "--mode", "standalone", "--format", "json", "--path", fixtureDir}
		args = append(args, auditLintJobsArgs(opts)...)
		args = append(args, auditTimeoutArgs(opts, verifyTimeoutDefault)...)
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return argvEvidence(args, execGovard(ctx, cfg, opts, args...))
	}},
	{ID: "P3-15", Phase: 3, Title: "govard audit status --session <id> --format json + result --session <id> --run <run> --format json + rerun --session <id> --format json", Requires: "P3-10 or P3-11 done", Guard: "", Checks: []string{"integrity"}, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return auditLifecycleEvidence(ctx, cfg, opts)
	}},

	// Phase 4 — Sync / Safety / Snapshot (16)
	// P4-01 — `remote test` is REMOTE-PROBE because a verify child can
	// never satisfy the key-copy offer: `offerSSHKeyCopyOnAuthFailure` returns
	// early when `!stdinIsTerminal()` (internal/cmd/ssh_copy_id.go:154-156), and
	// `execGovard` never sets `cmd.Stdin` (internal/verify/exec.go), so the child
	// has no terminal to offer into. That coupling is the whole reason this label
	// is honest, and the guard fence cannot see it — its classifier is a pure
	// function of the argv. Keep this comment in step with
	// offerSSHKeyCopyOnAuthFailure: if that tty gate ever goes, this label goes
	// with it.
	{ID: "P4-01", Phase: 4, Title: "govard remote test <remote>", Requires: "—", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "remote", "test", remote)
	})},
	{ID: "P4-02", Phase: 4, Title: "govard remote audit tail", Requires: "P4-01 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "remote", "audit", "tail")
	}},
	{ID: "P4-03", Phase: 4, Title: "govard sync -s <remote> --db --no-noise --plan", Requires: "P4-01 reachable", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--db", "--no-noise", "--plan")
	})},
	{ID: "P4-04", Phase: 4, Title: "govard sync -s <remote> --db --no-pii --plan", Requires: "P4-01", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--db", "--no-pii", "--plan")
	})},
	{ID: "P4-05", Phase: 4, Title: "govard sync -s <remote> --media optimized --plan", Requires: "P4-01", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--media", "optimized", "--plan")
	})},
	{ID: "P4-06", Phase: 4, Title: "govard sync -s <remote> --file --path . --plan", Requires: "P4-01", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--file", "--path", ".", "--plan")
	})},
	{ID: "P4-07", Phase: 4, Title: "govard sync -s <remote> --full --plan", Requires: "P4-01 remote reachable", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--full", "--plan")
	})},
	{ID: "P4-08", Phase: 4, Title: "govard snapshot create + govard snapshot list", Requires: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		// The artifact is the snapshot this run created, found by diffing the
		// store before and after `snapshot create`. "The newest usable one"
		// could be an older snapshot standing in for a create that produced
		// nothing, and phase 5 would then restore data this run never made.
		before := SnapshotNames(opts.ProjectRoot)
		ev := execGovard(ctx, cfg, opts, "snapshot", "create")
		if ev.ExitCode != 0 {
			return ev
		}
		created := newSnapshotNames(before, SnapshotNames(opts.ProjectRoot))
		if len(created) != 1 {
			ev.ExitCode = 1
			ev.OutputExcerpt += fmt.Sprintf(" | expected exactly one new snapshot, found %d %v", len(created), created)
			return ev
		}
		if !usableSnapshot(opts.ProjectRoot, created[0]) {
			ev.ExitCode = 1
			ev.OutputExcerpt += fmt.Sprintf(" | snapshot %s has no restorable database dump", created[0])
			return ev
		}
		ev2 := execGovard(ctx, cfg, opts, "snapshot", "list")
		ev.OutputExcerpt += " | list: " + ev2.OutputExcerpt
		ev.ExitCode = ev2.ExitCode
		if ev.ExitCode == 0 {
			ev.Artifacts = []string{created[0]}
		}
		return ev
	}},
	{ID: "P4-09", Phase: 4, Title: "govard snapshot export --help", Requires: "P4-08 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "snapshot", "export", "--help")
	}},
	{ID: "P4-10", Phase: 4, Title: "govard redis cli ping", Requires: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		// Not `tool redis-cli`: toolCmd has no RunE, so cobra printed help and
		// returned nil — the item went green without running redis at all.
		return execGovard(ctx, cfg, opts, "redis", "cli", "ping")
	}},
	{ID: "P4-11", Phase: 4, Title: "<domain>:9200/_cluster/health answers a search health payload", Requires: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return probeSearchHealth(ctx, cfg.Domain)
	}},
	{ID: "P4-12", Phase: 4, Title: "govard logs --tail 20", Requires: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "logs", "--tail", "20")
	}},
	// P4-13..P4-16 close the read-only half of the remote surface. They are
	// safe against a production remote by construction: `deploy plan` does not
	// connect at all, and `deploy status`, `deploy releases` and `remote list`
	// only read. The rest stay manual recipes, and each reason it gives is one a
	// reader can re-derive from the code rather than assume: `deploy check` is a
	// preflight of its own that leaves nothing behind on the target,
	// `deploy unlock`/`rollback` mutate the target, `db query`/`db connect` and
	// `db import` (without `--stream-db`, which only reads the remote's dump)
	// are refused against a write-protected remote, as are `snapshot push` and
	// `snapshot restore`, `db dump` only adds a new archive file there, and
	// `open -e <remote>` hands over an interactive shell. `tunnel stop` signals
	// one recorded pid. Keep this in step with those gates: a new writing half
	// needs a reason here, not a row.
	{ID: "P4-13", Phase: 4, Title: "govard deploy plan <remote> --json", Requires: "—", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "deploy", "plan", remote, "--json")
	})},
	// No --json here on purpose: runDeployStatus returns its JSON line before
	// the "no configured remote could be reached" check, so the JSON form exits
	// 0 for an unreachable remote and this item could never fail — green for
	// exactly the condition it exists to detect. The human path exits 1.
	{ID: "P4-14", Phase: 4, Title: "govard deploy status --remote <remote>", Requires: "P4-13 ok", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "deploy", "status", "--remote", remote)
	})},
	{ID: "P4-15", Phase: 4, Title: "govard deploy releases --remote <remote> --json", Requires: "P4-13 ok", Guard: GuardRemoteProbe, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "deploy", "releases", "--remote", remote, "--json")
	})},
	{ID: "P4-16", Phase: 4, Title: "govard remote list", Requires: "P4-13 ok", Guard: GuardRemoteProbe, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "remote", "list")
	}},

	// Phase 5 — Destructive QA (8) — gate: P4-08 snapshot exists
	{ID: "P5-01", Phase: 5, Title: "govard lock generate (destructive overwrite, phase 5)", Requires: "P1-06 ok, P4-08 snapshot exists", Guard: GuardDestructiveLocal, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "lock", "generate")
	}},
	{ID: "P5-02", Phase: 5, Title: "govard env down -v -> govard env up", Requires: "P4-08 snapshot exists", Guard: GuardDestructiveLocal, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		// A failed `down -v` left the volume in place, so bringing the
		// environment up afterwards would report a wipe that never happened.
		if down := execGovard(ctx, cfg, opts, "env", "down", "-v"); down.ExitCode != 0 {
			return down
		}
		return execGovard(ctx, cfg, opts, "env", "up")
	}},
	{ID: "P5-03", Phase: 5, Title: "govard bootstrap --fresh --framework {{FRAMEWORK}} --plan", Requires: "P4-08 snapshot exists", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		fw := cfg.Framework
		if fw == "" {
			fw = "magento2"
		}
		return execGovard(ctx, cfg, opts, "bootstrap", "--fresh", "--framework", fw, "--plan")
	}},
	{ID: "P5-04", Phase: 5, Title: "govard audit run --checks lint --no-lint-result-cache --format json", Requires: "P2-01 up", Guard: "", Checks: []string{"lint"}, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		args := []string{"audit", "run", "--checks", "lint", "--no-lint-result-cache"}
		args = append(args, auditLintJobsArgs(opts)...)
		args = append(args, auditTimeoutArgs(opts, p5RelintTimeout)...)
		args = append(args, "--format", "json")
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return argvEvidence(args, execGovard(ctx, cfg, opts, args...))
	}},
	{ID: "P5-05", Phase: 5, Title: "govard snapshot restore", Requires: "P4-08 snapshot exists", Guard: GuardDestructiveLocal, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		// Restore the exact snapshot the gate verified. The command takes the
		// name as a positional argument, so omitting it made this item fail
		// argument validation and restore nothing (issue #461).
		name, ok := GateSatisfyingSnapshot(opts)
		if !ok {
			return Evidence{ExitCode: 1, OutputExcerpt: "no snapshot recorded by a phase-4 run for this project"}
		}
		return execGovard(ctx, cfg, opts, "snapshot", "restore", name)
	}},
	{ID: "P5-06", Phase: 5, Title: "govard env down && govard env up (no -v)", Requires: "P5-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		_ = execGovard(ctx, cfg, opts, "env", "down")
		return execGovard(ctx, cfg, opts, "env", "up")
	}},
	{ID: "P5-07", Phase: 5, Title: "govard tool magento deploy:mode:show", Requires: "P5-05 done", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "deploy:mode:show")
	}},
	{ID: "P5-08", Phase: 5, Title: "govard snapshot pull/push --help", Requires: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "snapshot", "pull", "--help")
	}},
}

// RegistryFor returns the checklist items to execute for cfg: the static
// registry plus the items the project's framework declared for itself. It
// returns a copy, so a caller that swaps an item's Run (a test seam) cannot
// mutate the shared registry.
//
// Framework items are appended, so their ids must not collide with a static
// one — the composed list is not de-duplicated. Each runs one
// `govard tool <Tool> <Args...>` invocation through the same seam every static
// item uses, which is what lets a framework own its dev-loop checks without
// internal/verify naming any framework.
func RegistryFor(cfg engine.Config) []Item {
	items := make([]Item, 0, len(Registry))
	items = append(items, Registry...)

	for _, decl := range engine.VerifyToolItems(cfg.Framework) {
		args := append([]string{"tool", decl.Tool}, decl.Args...)
		items = append(items, Item{
			ID:       decl.ID,
			Phase:    decl.Phase,
			Title:    decl.Title,
			Requires: "P2-01 up",
			Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
				return execGovard(ctx, cfg, opts, args...)
			},
		})
	}

	return items
}
