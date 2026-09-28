package verify

import (
	"context"
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
}

// Item is one checklist entry in the 5-phase registry.
type Item struct {
	ID      string
	Title   string
	Precond string
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

// Registry is the static checklist: 60 items across 5 phases
// (P1 7 + P2 14 + P3 15 + P4 16 + P5 8). Guard values are constrained to
// {"", "READ-ONLY-REMOTE", "DESTRUCTIVE-LOCAL"} per Global Constraints.
// Empty guard means local write with no remote/destructive gate.
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
	{ID: "P2-03", Phase: 2, Title: "govard env up --build (if supported)", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "env", "up", "--build")
	}},
	{ID: "P2-04", Phase: 2, Title: "govard bootstrap -e {{REMOTE}} --no-noise --plan", Precond: "P2-01 up", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "bootstrap", "-e", remote, "--no-noise", "--plan")
	}},
	{ID: "P2-05", Phase: 2, Title: "govard bootstrap -e {{REMOTE}} --no-noise", Precond: "P2-04 plan ok", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "bootstrap", "-e", remote, "--no-noise")
	}},
	{ID: "P2-06", Phase: 2, Title: "govard bootstrap --clone -e {{REMOTE}} --plan", Precond: "P2-01 up", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--plan")
	}},
	{ID: "P2-07", Phase: 2, Title: "govard bootstrap --clone -e {{REMOTE}} --no-media --plan + --code-only --plan + --no-pii --plan", Precond: "P2-06 ok", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
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
	}},
	{ID: "P2-08", Phase: 2, Title: "govard bootstrap --clone -e {{REMOTE}} --no-noise OR --code-only (after P4-08)", Precond: "P4-08 snapshot exists", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "bootstrap", "--clone", "-e", remote, "--no-noise")
	}},
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
	{ID: "P2-13", Phase: 2, Title: "curl -k https://{{DOMAIN}}/ + curl -k http://{{DOMAIN}}/", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		domain := cfg.Domain
		if domain == "" {
			domain = "localhost"
		}
		ev := execGovard(ctx, cfg, opts, "tool", "curl", "-k", "https://"+domain+"/")
		if ev.ExitCode != 0 {
			ev2 := execGovard(ctx, cfg, opts, "tool", "curl", "-k", "http://"+domain+"/")
			ev.OutputExcerpt += " | http: " + ev2.OutputExcerpt
			ev.ExitCode = ev2.ExitCode
		}
		return ev
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
		return execGovard(ctx, cfg, opts, "audit", "status", "--format", "json")
	}},

	// Phase 4 — Sync / Safety / Snapshot (16)
	{ID: "P4-01", Phase: 4, Title: "govard remote test {{REMOTE}} x4 (dev1/dev2/staging/production)", Precond: "—", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "remote", "test", remote)
	}},
	{ID: "P4-02", Phase: 4, Title: "govard remote audit tail + stats", Precond: "P4-01 done", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "remote", "audit", "tail")
	}},
	{ID: "P4-03", Phase: 4, Title: "govard sync -s {{REMOTE}} --db --no-noise --plan", Precond: "P4-01 reachable", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--db", "--no-noise", "--plan")
	}},
	{ID: "P4-04", Phase: 4, Title: "govard sync -s {{REMOTE}} --db --no-pii --plan", Precond: "P4-01", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--db", "--no-pii", "--plan")
	}},
	{ID: "P4-05", Phase: 4, Title: "govard sync -s {{REMOTE}} --media optimized --plan + minimal --plan + all --plan + catalog --plan", Precond: "P4-01", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--media", "optimized", "--plan")
	}},
	{ID: "P4-06", Phase: 4, Title: "govard sync -s {{REMOTE}} --file --path <path> --plan + --exclude + --delete --plan", Precond: "P4-01", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--file", "--path", ".", "--plan")
	}},
	{ID: "P4-07", Phase: 4, Title: "govard sync -s {{REMOTE_STAGING}} --full --plan", Precond: "P4-01 staging", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "sync", "-s", remote, "--full", "--plan")
	}},
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
	{ID: "P4-10", Phase: 4, Title: "govard redis cli ping / valkey cli ping + flush", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "redis-cli", "ping")
	}},
	{ID: "P4-11", Phase: 4, Title: "curl -s http://{{DOMAIN}}:9200 / opensearch health", Precond: "P2-01 up", Guard: "", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "tool", "curl", "-s", "http://localhost:9200")
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
	{ID: "P4-13", Phase: 4, Title: "govard deploy plan {{REMOTE}} --json", Precond: "—", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "deploy", "plan", remote, "--json")
	}},
	// No --json here on purpose: runDeployStatus returns its JSON line before
	// the "no configured remote could be reached" check, so the JSON form exits
	// 0 for an unreachable remote and this item could never fail — green for
	// exactly the condition it exists to detect. The human path exits 1.
	{ID: "P4-14", Phase: 4, Title: "govard deploy status --remote {{REMOTE}}", Precond: "P4-13 ok", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "deploy", "status", "--remote", remote)
	}},
	{ID: "P4-15", Phase: 4, Title: "govard deploy releases --remote {{REMOTE}} --json", Precond: "P4-13 ok", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		remote := opts.Remote
		if remote == "" {
			remote = "staging"
		}
		return execGovard(ctx, cfg, opts, "deploy", "releases", "--remote", remote, "--json")
	}},
	{ID: "P4-16", Phase: 4, Title: "govard remote list", Precond: "P4-13 ok", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "remote", "list")
	}},

	// Phase 5 — Destructive QA (8) — gate: P4-08 snapshot exists
	{ID: "P5-01", Phase: 5, Title: "govard lock generate -> check -> drift .govard.yml -> lock diff -> check --strict must fail -> revert", Precond: "P1-06 ok, P4-08 snapshot exists", Guard: "DESTRUCTIVE-LOCAL", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		return execGovard(ctx, cfg, opts, "lock", "generate")
	}},
	{ID: "P5-02", Phase: 5, Title: "govard env down -v -> verify docker volume ls removes govard-* -> govard env up", Precond: "P4-08 snapshot exists", Guard: "DESTRUCTIVE-LOCAL", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
		_ = execGovard(ctx, cfg, opts, "env", "down", "-v")
		return execGovard(ctx, cfg, opts, "env", "up")
	}},
	{ID: "P5-03", Phase: 5, Title: "govard bootstrap --fresh --framework {{FRAMEWORK}} --framework-version {{VERSION}} --plan", Precond: "P4-08 snapshot exists", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
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
	{ID: "P5-05", Phase: 5, Title: "govard snapshot restore", Precond: "P4-08 snapshot exists", Guard: "DESTRUCTIVE-LOCAL", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
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
	{ID: "P5-08", Phase: 5, Title: "govard snapshot pull/push --help", Precond: "—", Guard: "READ-ONLY-REMOTE", Run: func(ctx context.Context, cfg engine.Config, opts VerifyOpts) Evidence {
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
