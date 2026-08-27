# Contributing to Govard

Thank you for contributing to **Govard** (`github.com/ddtcorex/govard`) — a Go-based local development orchestrator for PHP and web projects (Magento, Laravel, Symfony, WordPress, and more).

## Getting Started

1. **Fork and clone** `github.com/ddtcorex/govard`.
2. Install prerequisites (Go 1.25+, Node.js 20+, Docker for integration tests, Make):

   ```bash
   go version
   node --version
   docker --version
   make --version
   ```

3. Build the CLI for the current platform:

   ```bash
   make build        # produces ./bin/govard
   ./bin/govard version
   ```

4. Explore the repository map in `AGENTS.md` (`internal/engine/` is framework-agnostic core, `internal/frameworks/<name>/` holds per-framework logic).

## Superpowers 3-Phase Workflow (AGENTS.md)

Every multi-step change to this repository **MUST** follow the Superpowers skill workflow defined in `AGENTS.md`, in order:

1. **brainstorming** — explore intent, requirements, and design before writing code. Record the outcome locally under `docs/superpowers/specs/` (gitignored).
2. **writing-plans** — turn the approved design into a task-by-task plan with exact test and implementation sketches under `docs/superpowers/plans/` (gitignored, transient — delete once the batch ships).
3. **executing-plans** — implement task by task with strict **TDD**: write a failing test first, verify RED, implement, verify GREEN, then commit that task before starting the next. Do not commit while tests are red.

Describe durable outcomes in the PR body instead of committing dated spec/plan files. When a superpowers-executed task completes, squash its per-task commits into one commit.

Do not skip ahead to implementation and do not bundle multiple TDD tasks into one commit during `executing-plans`.

## Branch Naming

Never commit directly to `master` (or `main`/`develop`). Start a feature branch per work session:

- `fix/<topic>` — bug fixes
- `feat/<topic>` — new features
- `docs/<topic>` — documentation-only changes

Rebase (not merge) when the base moves: `git fetch origin && git rebase origin/master`.

## Conventional Commits

All commit subjects **must** follow [Conventional Commits](https://www.conventionalcommits.org/) in imperative mood:

```
<type>(<scope>): <subject>

<body — why, not what>

Refs: #<issue>
```

- **Types (closed list):** `feat` `fix` `docs` `chore` `refactor` `perf` `test` `build` `ci` `revert`
- **Scope:** optional, e.g. `feat(engine):`, `fix(bootstrap):`, `docs(readme):`
- **Subject:** imperative, lowercase first word, ≤ 72 chars, no trailing period
- **Body:** explain *why* and trade-offs when non-trivial
- **Breaking changes:** `feat!: <subject>` plus a `BREAKING CHANGE:` footer

One TDD task = one commit while executing a plan; squash at merge time if the history reads better squashed.

## Validation

Run these before opening a PR (match depth to risk):

```bash
make test              # full suite: lint + fmt-check + vet + unit tests
make test-unit         # unit tests only
make test-integration  # integration tests (requires Docker + built bin/govard-test)
make build             # build CLI for current platform

# Direct equivalents
go test ./...                  # all unit tests
go test -tags integration ./...  # integration tests
go vet ./...                   # static analysis
gofmt -s -l .                  # should produce no output (no drift)
```

Additional checks when relevant:

```bash
# Blueprint versioning — see AGENTS.md
# Only bump internal/engine/render.go BlueprintVersion when Go rendering logic changes output without changing blueprint file bytes.

# Docs sync — docs/**/*.md auto-syncs to GitHub Wiki on push to master
```

Do not claim verified/done/clean without having actually run the checks — be ready to paste exact command output in the PR.

## Pull Requests

1. Push your branch and open a PR into `master`.
2. Fill out `.github/PULL_REQUEST_TEMPLATE.md` (Summary, Why, Changes, Validation, Linked Issues).
3. Link the PR to the plan that produced it when the Superpowers workflow was used.
4. Ensure CI (`make test` / `make build` via `.github/workflows/ci-pipeline.yml`) is green.
5. After any squash/force-push, re-read the PR description and linked issue and update them if the diff no longer matches.

## Code Standards

- Run `gofmt -s -w .` after Go edits.
- Keep code ASCII unless the file already requires Unicode.
- Prefer small pure helpers for parsing/formatting; do not swallow errors for critical flows (network, file, process).
- Never log secrets, tokens, private keys, or DB passwords.
- `internal/conventions/` holds only constants used by ≥2 packages or truly cross-cutting ones — verify with `grep -rl "conventions\.<Name>"` before adding a new framework-specific constant there.
- `internal/engine/` and `internal/cmd/` must not branch on hardcoded framework names (`cfg.Framework == "magento2"`) — expose behavior as a framework-registered capability instead (`internal/engine/framework_capabilities.go`).

## Package Visibility

This repository is public. Do not add private project names, client codenames, or secrets to code, docs, or commit messages.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](./CODE_OF_CONDUCT.md). By participating, you agree to its terms.

## Questions or Security Reports

- General questions: open a GitHub Discussion or issue at `https://github.com/ddtcorex/govard/discussions`.
- Contact maintainer: [kaido4492@gmail.com](mailto:kaido4492@gmail.com)
- Security vulnerabilities: use GitHub's private advisory reporting at `https://github.com/ddtcorex/govard/security/advisories` — do not file a public issue.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](./LICENSE).
