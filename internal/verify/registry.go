package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	Retries       int
	// Artifacts names what this item produced (a snapshot directory, for
	// example). The phase-5 gate uses it to prove a restore target exists.
	Artifacts []string
	// Skipped marks an item that did not run because it does not apply here.
	// SkipReason says why. See Skip.
	Skipped    bool
	SkipReason string
}

// The Guard values an item may carry. DecideGuard is their only reader:
// GuardReadOnlyRemote and the empty value run, the other two are gated.
const (
	// GuardReadOnlyRemote marks an item whose argv names a remote and writes
	// nothing there. It has no runtime gate of its own: plan mode already
	// replaces every Run with a stub, so blocking these would delete the
	// coverage rather than protect anything.
	GuardReadOnlyRemote = "READ-ONLY-REMOTE"
	// GuardDestructiveLocal marks an item that destroys local state
	// irreversibly. It runs in phase 5 only.
	GuardDestructiveLocal = "DESTRUCTIVE-LOCAL"
	// GuardRemoteWrite marks an item whose argv writes through a remote. It is
	// skipped unless the operator opted in with --allow-remote-write.
	GuardRemoteWrite = "REMOTE-WRITE"
)

// Item is one checklist entry in the 5-phase registry.
type Item struct {
	ID      string
	Title   string
	Precond string
	// Guard is the item's taxonomy label, not documentation: the runner acts on
	// it through DecideGuard.
	Guard   string
	Phase   int
	Timeout time.Duration
	When    func(engine.Config) bool
	Run     func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence
}

// isMagento2 reports whether cfg targets Magento 2 (or Mage-OS which normalizes to magento2).
func isMagento2(c engine.Config) bool {
	return c.Framework == "magento2"
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
		return Skip(auditIdentitySkipReason(identity, created.ExitCode))
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

// Registry is the static checklist: 60 items across 5 phases
// (P1 7 + P2 14 + P3 15 + P4 16 + P5 8). Every item carries a Guard label and
// the runner acts on it through DecideGuard: an empty Guard is local work with
// no remote and nothing irreversible, GuardReadOnlyRemote documents that the
// argv names a remote and writes nothing there (no runtime gate of its own),
// GuardRemoteWrite is skipped unless --allow-remote-write was passed, and
// GuardDestructiveLocal runs in phase 5 only. An item that puts a remote in its
// argv is wrapped in withRemote instead of defaulting one: it takes the name
// from --remote and skips when the run named none.
var Registry = []Item{
	// Phase 1 — Preflight (7)
	{ID: "P1-01", Phase: 1, Title: "govard doctor", Precond: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "doctor")
	}},
	{ID: "P1-02", Phase: 1, Title: "govard doctor --json", Precond: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "doctor", "--json")
	}},
	{ID: "P1-03", Phase: 1, Title: "govard doctor trust", Precond: "P1-02 green", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "doctor", "trust")
	}},
	{ID: "P1-04", Phase: 1, Title: "govard config get project_name / framework / domain / stack.php_version / stack.*", Precond: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "config", "get", "project_name")
	}},
	{ID: "P1-05", Phase: 1, Title: "govard env config", Precond: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "env", "config")
	}},
	{ID: "P1-06", Phase: 1, Title: "govard lock check + govard lock diff (or generate -> check if missing)", Precond: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		ev := execGovard(ctx, cfg, opts, "lock", "check")
		if ev.ExitCode != 0 {
			// Try diff for evidence, keep original exit
			ev2 := execGovard(ctx, cfg, opts, "lock", "diff")
			ev.OutputExcerpt = ev.OutputExcerpt + " | diff: " + ev2.OutputExcerpt
		}
		return ev
	}},
	{ID: "P1-07", Phase: 1, Title: "govard status / govard project list", Precond: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "status")
	}},

	// Phase 2 — Bootstrap & Env (14)
	{ID: "P2-01", Phase: 2, Title: "govard env down -> govard env up -> govard env ps", Precond: "P1 green", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		_ = execGovard(ctx, cfg, opts, "env", "down")
		ev := execGovard(ctx, cfg, opts, "env", "up")
		if ev.ExitCode != 0 {
			return ev
		}
		return execGovard(ctx, cfg, opts, "env", "ps")
	}},
	{ID: "P2-02", Phase: 2, Title: "govard env logs --tail 20", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "env", "logs", "--tail", "20")
	}},
	{ID: "P2-03", Phase: 2, Title: "govard env up --force-recreate", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "env", "up", "--force-recreate")
	}},
	{ID: "P2-04", Phase: 2, Title: "govard bootstrap -e <remote> --no-noise --plan", Precond: "P2-01 up", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "bootstrap", "-e", remote, "--no-noise", "--plan")
	})},
	{ID: "P2-05", Phase: 2, Title: "govard bootstrap -e <remote> --no-noise", Precond: "P2-04 plan ok", Guard: GuardRemoteWrite, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "bootstrap", "-e", remote, "--no-noise")
	})},
	{ID: "P2-06", Phase: 2, Title: "govard bootstrap --clone -e <remote> --plan", Precond: "P2-01 up", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--plan")
	})},
	{ID: "P2-07", Phase: 2, Title: "govard bootstrap --clone -e <remote> --no-media --plan + --code-only --plan + --no-pii --plan", Precond: "P2-06 ok", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
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
	{ID: "P2-08", Phase: 2, Title: "govard bootstrap --clone -e <remote> --no-noise OR --code-only (after P4-08)", Precond: "P4-08 snapshot exists", Guard: GuardRemoteWrite, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--no-noise")
	})},
	{ID: "P2-09", Phase: 2, Title: "govard tool npm --prefix <hyva-theme>/web/tailwind install + run build (Hyva only)", Precond: "P2-05 or P2-08", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "npm", "--prefix", "web/tailwind", "install")
	}},
	{ID: "P2-10", Phase: 2, Title: "govard config auto", Precond: "P2-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "config", "auto")
	}},
	{ID: "P2-11", Phase: 2, Title: "govard tool magento --version + module:status (magento)", Precond: "P2-05 done", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "--version")
	}},
	{ID: "P2-12", Phase: 2, Title: "govard tool composer validate", Precond: "P2-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "composer", "validate")
	}},
	{ID: "P2-13", Phase: 2, Title: "<domain> answers over https (http only when the TLS handshake fails)", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return probeSite(ctx, cfg.Domain)
	}},
	{ID: "P2-14", Phase: 2, Title: "govard open --help", Precond: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "open", "--help")
	}},

	// Phase 3 — Dev Loop (15)
	{ID: "P3-01", Phase: 3, Title: "govard tool magento cache:flush + cache:status (or framework equiv)", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "cache:flush")
	}},
	{ID: "P3-02", Phase: 3, Title: "govard tool magento setup:upgrade", Precond: "P3-01 ok", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "setup:upgrade")
	}},
	{ID: "P3-03", Phase: 3, Title: "govard tool magento setup:di:compile", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "setup:di:compile")
	}},
	{ID: "P3-04", Phase: 3, Title: "govard tool magento setup:static-content:deploy {{LOCALES}} -f", Precond: "P2-09 Hyva built", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "setup:static-content:deploy", "-f")
	}},
	{ID: "P3-05", Phase: 3, Title: "govard tool magento indexer:reindex", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "indexer:reindex")
	}},
	{ID: "P3-06", Phase: 3, Title: "govard tool magento cron:run", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "cron:run")
	}},
	{ID: "P3-07", Phase: 3, Title: "govard tool php vendor/bin/phpstan analyse --help / phpcs --standard", Precond: "P2-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "php", "vendor/bin/phpstan", "analyse", "--help")
	}},
	{ID: "P3-08", Phase: 3, Title: "govard debug status -> on -> verify php-debug routes with XDEBUG_SESSION -> off", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "debug", "status")
	}},
	{ID: "P3-09", Phase: 3, Title: "govard frontend start -> logs -> stop", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "frontend", "start")
	}},
	{ID: "P3-10", Phase: 3, Title: "govard audit run --checks lint --scope project --mode auto --format json --lint-jobs 4 --timeout auto", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		args := []string{"audit", "run", "--checks", "lint", "--scope", "project", "--mode", "auto", "--format", "json", "--lint-jobs", "4", "--timeout", "auto"}
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return execGovard(ctx, cfg, opts, args...)
	}},
	{ID: "P3-11", Phase: 3, Title: "govard audit run --checks lint --scope diff --base {{BASE_BRANCH}} --format json --lint-jobs 4", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		base := opts.BaseRef
		if base == "" {
			base = "origin/master"
		}
		args := []string{"audit", "run", "--checks", "lint", "--scope", "diff", "--base", base, "--format", "json", "--lint-jobs", "4"}
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return execGovard(ctx, cfg, opts, args...)
	}},
	{ID: "P3-12", Phase: 3, Title: "govard audit run --checks profiler --url https://{{DOMAIN}}/ --format json --allow-xdebug", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		domain := cfg.Domain
		if domain == "" {
			domain = "localhost"
		}
		args := []string{"audit", "run", "--checks", "profiler", "--url", "https://" + domain + "/", "--format", "json"}
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return execGovard(ctx, cfg, opts, args...)
	}},
	{ID: "P3-13", Phase: 3, Title: "govard audit run --checks lint --mode module_in_project --format json --allow-xdebug (from app/code/<Vendor>/<Module>)", Precond: "P2-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		args := []string{"audit", "run", "--checks", "lint", "--mode", "module_in_project", "--format", "json"}
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return execGovard(ctx, cfg, opts, args...)
	}},
	{ID: "P3-14", Phase: 3, Title: "govard audit run --checks lint --mode standalone --format json (from /tmp/govard-audit-standalone/<Module>)", Precond: "P3-13 ok", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "audit", "run", "--checks", "lint", "--mode", "standalone", "--format", "json")
	}},
	{ID: "P3-15", Phase: 3, Title: "govard audit status --session <id> --format json + result --session <id> --run <run> --format json + rerun --session <id> --format json", Precond: "P3-10 or P3-11 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return auditLifecycleEvidence(ctx, cfg, opts)
	}},

	// Phase 4 — Sync / Safety / Snapshot (16)
	{ID: "P4-01", Phase: 4, Title: "govard remote test <remote> x4 (dev1/dev2/staging/production)", Precond: "—", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "remote", "test", remote)
	})},
	{ID: "P4-02", Phase: 4, Title: "govard remote audit tail + stats", Precond: "P4-01 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "remote", "audit", "tail")
	}},
	{ID: "P4-03", Phase: 4, Title: "govard sync -s <remote> --db --no-noise --plan", Precond: "P4-01 reachable", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--db", "--no-noise", "--plan")
	})},
	{ID: "P4-04", Phase: 4, Title: "govard sync -s <remote> --db --no-pii --plan", Precond: "P4-01", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--db", "--no-pii", "--plan")
	})},
	{ID: "P4-05", Phase: 4, Title: "govard sync -s <remote> --media optimized --plan + minimal --plan + all --plan + catalog --plan", Precond: "P4-01", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--media", "optimized", "--plan")
	})},
	{ID: "P4-06", Phase: 4, Title: "govard sync -s <remote> --file --path <path> --plan + --exclude + --delete --plan", Precond: "P4-01", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--file", "--path", ".", "--plan")
	})},
	{ID: "P4-07", Phase: 4, Title: "govard sync -s <remote> --full --plan", Precond: "P4-01 staging", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--full", "--plan")
	})},
	{ID: "P4-08", Phase: 4, Title: "govard snapshot create + govard snapshot list", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		ev := execGovard(ctx, cfg, opts, "snapshot", "create")
		if ev.ExitCode != 0 {
			return ev
		}
		ev2 := execGovard(ctx, cfg, opts, "snapshot", "list")
		ev.OutputExcerpt += " | list: " + ev2.OutputExcerpt
		ev.ExitCode = ev2.ExitCode
		if ev.ExitCode == 0 {
			if name, ok := LatestSnapshotName(opts.ProjectRoot); ok {
				ev.Artifacts = []string{name}
			}
		}
		return ev
	}},
	{ID: "P4-09", Phase: 4, Title: "govard snapshot export + delete --help", Precond: "P4-08 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "snapshot", "export", "--help")
	}},
	{ID: "P4-10", Phase: 4, Title: "govard redis cli ping / valkey cli ping", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		// Not `tool redis-cli`: toolCmd has no RunE, so cobra printed help and
		// returned nil — the item went green without running redis at all.
		return execGovard(ctx, cfg, opts, "redis", "cli", "ping")
	}},
	{ID: "P4-11", Phase: 4, Title: "<domain>:9200/_cluster/health answers a search health payload", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return probeSearchHealth(ctx, cfg.Domain)
	}},
	{ID: "P4-12", Phase: 4, Title: "govard logs --tail 20 + govard ps cross-project", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "logs", "--tail", "20")
	}},
	// P4-13..P4-16 close the read-only half of the remote surface. They are
	// safe against a production remote by construction: `deploy plan` does not
	// connect at all, and `deploy status`, `deploy releases` and `remote list`
	// only read. The writing halves (`deploy check` creates the deploy path,
	// `deploy unlock`/`rollback` mutate the target, `db`/`snapshot`/`open -e`
	// can bypass write protection or copy a key, `tunnel stop` kills every
	// cloudflared on the host) stay manual recipes until govard#466-#469 land.
	{ID: "P4-13", Phase: 4, Title: "govard deploy plan <remote> --json", Precond: "—", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "deploy", "plan", remote, "--json")
	})},
	// No --json here on purpose: runDeployStatus returns its JSON line before
	// the "no configured remote could be reached" check, so the JSON form exits
	// 0 for an unreachable remote and this item could never fail — green for
	// exactly the condition it exists to detect. The human path exits 1.
	{ID: "P4-14", Phase: 4, Title: "govard deploy status --remote <remote>", Precond: "P4-13 ok", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "deploy", "status", "--remote", remote)
	})},
	{ID: "P4-15", Phase: 4, Title: "govard deploy releases --remote <remote> --json", Precond: "P4-13 ok", Guard: GuardReadOnlyRemote, Run: withRemote(func(ctx context.Context, cfg engine.Config, opts VerifyOpts, remote string) Evidence {
		return execGovard(ctx, cfg, opts, "deploy", "releases", "--remote", remote, "--json")
	})},
	{ID: "P4-16", Phase: 4, Title: "govard remote list", Precond: "P4-13 ok", Guard: GuardReadOnlyRemote, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "remote", "list")
	}},

	// Phase 5 — Destructive QA (8) — gate: P4-08 snapshot exists
	{ID: "P5-01", Phase: 5, Title: "govard lock generate -> check -> drift .govard.yml -> lock diff -> check --strict must fail -> revert", Precond: "P1-06 ok, P4-08 snapshot exists", Guard: GuardDestructiveLocal, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "lock", "generate")
	}},
	{ID: "P5-02", Phase: 5, Title: "govard env down -v -> verify docker volume ls removes govard-* -> govard env up", Precond: "P4-08 snapshot exists", Guard: GuardDestructiveLocal, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		_ = execGovard(ctx, cfg, opts, "env", "down", "-v")
		return execGovard(ctx, cfg, opts, "env", "up")
	}},
	{ID: "P5-03", Phase: 5, Title: "govard bootstrap --fresh --framework {{FRAMEWORK}} --framework-version {{VERSION}} --plan", Precond: "P4-08 snapshot exists", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		fw := cfg.Framework
		if fw == "" {
			fw = "magento2"
		}
		return execGovard(ctx, cfg, opts, "bootstrap", "--fresh", "--framework", fw, "--plan")
	}},
	{ID: "P5-04", Phase: 5, Title: "govard audit run --checks lint --no-lint-result-cache --lint-jobs 4 --timeout 0 --format json --allow-xdebug", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		args := []string{"audit", "run", "--checks", "lint", "--no-lint-result-cache", "--lint-jobs", "4", "--timeout", "0", "--format", "json"}
		if opts.AllowXdebug {
			args = append(args, "--allow-xdebug")
		}
		return execGovard(ctx, cfg, opts, args...)
	}},
	{ID: "P5-05", Phase: 5, Title: "govard snapshot restore", Precond: "P4-08 snapshot exists", Guard: GuardDestructiveLocal, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		// Restore the exact snapshot the gate verified. The command takes the
		// name as a positional argument, so omitting it made this item fail
		// argument validation and restore nothing (issue #461).
		name, ok := GateSatisfyingSnapshot(opts)
		if !ok {
			return Evidence{ExitCode: 1, OutputExcerpt: "no snapshot recorded by a phase-4 run for this project"}
		}
		return execGovard(ctx, cfg, opts, "snapshot", "restore", name)
	}},
	{ID: "P5-06", Phase: 5, Title: "govard env down && govard env up (no -v)", Precond: "P5-05 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		_ = execGovard(ctx, cfg, opts, "env", "down")
		return execGovard(ctx, cfg, opts, "env", "up")
	}},
	{ID: "P5-07", Phase: 5, Title: "govard tool magento deploy:mode:show + cache:flush after restore", Precond: "P5-05 done", Guard: "", When: isMagento2, Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "magento", "deploy:mode:show")
	}},
	{ID: "P5-08", Phase: 5, Title: "govard snapshot pull/push --help", Precond: "—", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
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
			ID:      decl.ID,
			Phase:   decl.Phase,
			Title:   decl.Title,
			Precond: "P2-01 up",
			Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
				return execGovard(ctx, cfg, opts, args...)
			},
		})
	}

	return items
}
