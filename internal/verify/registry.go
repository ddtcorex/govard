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
}

// Evidence is the per-item execution result.
type Evidence struct {
	DurationMs    int
	ExitCode      int
	OutputExcerpt string
	JSONValid     bool
	Retries       int
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

// stubRun is the Task 1 placeholder for Item.Run — runner will flesh it out.
func stubRun(_ context.Context, _ engine.Config, _ VerifyOpts) Evidence {
	return Evidence{ExitCode: 0, OutputExcerpt: "stub"}
}

// isMagento2 reports whether cfg targets Magento 2 (or Mage-OS which normalizes to magento2).
func isMagento2(c engine.Config) bool {
	return c.Framework == "magento2"
}

// Registry is the single source of truth for 56 items across 5 phases
// (P1 7 + P2 14 + P3 15 + P4 12 + P5 8). Guard values are constrained to
// {"", "READ-ONLY-REMOTE", "DESTRUCTIVE-LOCAL"} per Global Constraints.
// Empty guard means local write with no remote/destructive gate.
var Registry = []Item{
	// Phase 1 — Preflight (7)
	{ID: "P1-01", Phase: 1, Title: "govard doctor", Precond: "—", Guard: "", Run: stubRun},
	{ID: "P1-02", Phase: 1, Title: "govard doctor --json", Precond: "—", Guard: "", Run: stubRun},
	{ID: "P1-03", Phase: 1, Title: "govard doctor trust", Precond: "P1-02 green", Guard: "", Run: stubRun},
	{ID: "P1-04", Phase: 1, Title: "govard config get project_name / framework / domain / stack.php_version / stack.*", Precond: "—", Guard: "", Run: stubRun},
	{ID: "P1-05", Phase: 1, Title: "govard env config", Precond: "—", Guard: "", Run: stubRun},
	{ID: "P1-06", Phase: 1, Title: "govard lock check + govard lock diff (or generate -> check if missing)", Precond: "—", Guard: "", Run: stubRun},
	{ID: "P1-07", Phase: 1, Title: "govard status / govard project list", Precond: "—", Guard: "", Run: stubRun},

	// Phase 2 — Bootstrap & Env (14)
	{ID: "P2-01", Phase: 2, Title: "govard env down -> govard env up -> govard env ps", Precond: "P1 green", Guard: "", Run: stubRun},
	{ID: "P2-02", Phase: 2, Title: "govard env logs --tail 20", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P2-03", Phase: 2, Title: "govard env up --build (if supported)", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P2-04", Phase: 2, Title: "govard bootstrap -e {{REMOTE}} --no-noise --plan", Precond: "P2-01 up", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P2-05", Phase: 2, Title: "govard bootstrap -e {{REMOTE}} --no-noise", Precond: "P2-04 plan ok", Guard: "", Run: stubRun},
	{ID: "P2-06", Phase: 2, Title: "govard bootstrap --clone -e {{REMOTE}} --plan", Precond: "P2-01 up", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P2-07", Phase: 2, Title: "govard bootstrap --clone -e {{REMOTE}} --no-media --plan + --code-only --plan + --no-pii --plan", Precond: "P2-06 ok", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P2-08", Phase: 2, Title: "govard bootstrap --clone -e {{REMOTE}} --no-noise OR --code-only (after P4-08)", Precond: "P4-08 snapshot exists", Guard: "", Run: stubRun},
	{ID: "P2-09", Phase: 2, Title: "govard tool npm --prefix <hyva-theme>/web/tailwind install + run build (Hyva only)", Precond: "P2-05 or P2-08", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P2-10", Phase: 2, Title: "govard config auto", Precond: "P2-05 done", Guard: "", Run: stubRun},
	{ID: "P2-11", Phase: 2, Title: "govard tool magento --version + module:status (magento)", Precond: "P2-05 done", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P2-12", Phase: 2, Title: "govard tool composer validate", Precond: "P2-05 done", Guard: "", Run: stubRun},
	{ID: "P2-13", Phase: 2, Title: "curl -k https://{{DOMAIN}}/ + curl -k http://{{DOMAIN}}/", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P2-14", Phase: 2, Title: "govard open --help", Precond: "—", Guard: "", Run: stubRun},

	// Phase 3 — Dev Loop (15)
	{ID: "P3-01", Phase: 3, Title: "govard tool magento cache:flush + cache:status (or framework equiv)", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P3-02", Phase: 3, Title: "govard tool magento setup:upgrade", Precond: "P3-01 ok", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P3-03", Phase: 3, Title: "govard tool magento setup:di:compile", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P3-04", Phase: 3, Title: "govard tool magento setup:static-content:deploy {{LOCALES}} -f", Precond: "P2-09 Hyva built", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P3-05", Phase: 3, Title: "govard tool magento indexer:reindex", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P3-06", Phase: 3, Title: "govard tool magento cron:run", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P3-07", Phase: 3, Title: "govard tool php vendor/bin/phpstan analyse --help / phpcs --standard", Precond: "P2-05 done", Guard: "", Run: stubRun},
	{ID: "P3-08", Phase: 3, Title: "govard debug status -> on -> verify php-debug routes with XDEBUG_SESSION -> off", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P3-09", Phase: 3, Title: "govard frontend start -> logs -> stop", Precond: "P2-01 up", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P3-10", Phase: 3, Title: "govard audit run --checks lint --scope project --mode auto --format json --lint-jobs 4 --timeout auto", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P3-11", Phase: 3, Title: "govard audit run --checks lint --scope diff --base {{BASE_BRANCH}} --format json --lint-jobs 4", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P3-12", Phase: 3, Title: "govard audit run --checks profiler --url https://{{DOMAIN}}/ --format json --allow-xdebug", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P3-13", Phase: 3, Title: "govard audit run --checks lint --mode module_in_project --format json --allow-xdebug (from app/code/<Vendor>/<Module>)", Precond: "P2-05 done", Guard: "", Run: stubRun},
	{ID: "P3-14", Phase: 3, Title: "govard audit run --checks lint --mode standalone --format json (from /tmp/govard-audit-standalone/<Module>)", Precond: "P3-13 ok", Guard: "", Run: stubRun},
	{ID: "P3-15", Phase: 3, Title: "govard audit status --session <id> --format json + result --session <id> --run <run> --format json + rerun --session <id> --format json", Precond: "P3-10 or P3-11 done", Guard: "", Run: stubRun},

	// Phase 4 — Sync / Safety / Snapshot (12)
	{ID: "P4-01", Phase: 4, Title: "govard remote test {{REMOTE}} x4 (dev1/dev2/staging/production)", Precond: "—", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P4-02", Phase: 4, Title: "govard remote audit tail + stats", Precond: "P4-01 done", Guard: "", Run: stubRun},
	{ID: "P4-03", Phase: 4, Title: "govard sync -s {{REMOTE}} --db --no-noise --plan", Precond: "P4-01 reachable", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P4-04", Phase: 4, Title: "govard sync -s {{REMOTE}} --db --no-pii --plan", Precond: "P4-01", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P4-05", Phase: 4, Title: "govard sync -s {{REMOTE}} --media optimized --plan + minimal --plan + all --plan + catalog --plan", Precond: "P4-01", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P4-06", Phase: 4, Title: "govard sync -s {{REMOTE}} --file --path <path> --plan + --exclude + --delete --plan", Precond: "P4-01", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P4-07", Phase: 4, Title: "govard sync -s {{REMOTE_STAGING}} --full --plan", Precond: "P4-01 staging", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P4-08", Phase: 4, Title: "govard snapshot create + govard snapshot list", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P4-09", Phase: 4, Title: "govard snapshot export + delete --help", Precond: "P4-08 done", Guard: "", Run: stubRun},
	{ID: "P4-10", Phase: 4, Title: "govard redis cli ping / valkey cli ping + flush", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P4-11", Phase: 4, Title: "curl -s http://{{DOMAIN}}:9200 / opensearch health", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P4-12", Phase: 4, Title: "govard logs --tail 20 + govard ps cross-project", Precond: "P2-01 up", Guard: "", Run: stubRun},

	// Phase 5 — Destructive QA (8) — gate: P4-08 snapshot exists
	{ID: "P5-01", Phase: 5, Title: "govard lock generate -> check -> drift .govard.yml -> lock diff -> check --strict must fail -> revert", Precond: "P1-06 ok, P4-08 snapshot exists", Guard: "DESTRUCTIVE-LOCAL", Run: stubRun},
	{ID: "P5-02", Phase: 5, Title: "govard env down -v -> verify docker volume ls removes govard-* -> govard env up", Precond: "P4-08 snapshot exists", Guard: "DESTRUCTIVE-LOCAL", Run: stubRun},
	{ID: "P5-03", Phase: 5, Title: "govard bootstrap --fresh --framework {{FRAMEWORK}} --framework-version {{VERSION}} --plan", Precond: "P4-08 snapshot exists", Guard: "READ-ONLY-REMOTE", Run: stubRun},
	{ID: "P5-04", Phase: 5, Title: "govard audit run --checks lint --no-lint-result-cache --lint-jobs 4 --timeout 0 --format json --allow-xdebug", Precond: "P2-01 up", Guard: "", Run: stubRun},
	{ID: "P5-05", Phase: 5, Title: "govard snapshot restore", Precond: "P4-08 snapshot exists", Guard: "DESTRUCTIVE-LOCAL", Run: stubRun},
	{ID: "P5-06", Phase: 5, Title: "govard env down && govard env up (no -v)", Precond: "P5-05 done", Guard: "", Run: stubRun},
	{ID: "P5-07", Phase: 5, Title: "govard tool magento deploy:mode:show + cache:flush after restore", Precond: "P5-05 done", Guard: "", When: isMagento2, Run: stubRun},
	{ID: "P5-08", Phase: 5, Title: "govard snapshot pull/push --help", Precond: "—", Guard: "READ-ONLY-REMOTE", Run: stubRun},
}
