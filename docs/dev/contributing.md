# Development Guide

Guide for contributing to Govard development.

## Architecture Overview

- **CLI (Cobra)**: Entry point in `cmd/` and `internal/cmd/`
- **Engine**: Business logic, Docker SDK interaction, and rendering in `internal/engine/`
- **UI**: Display logic using `pterm` in `internal/ui/`

## Project Structure

```
.
├── cmd/govard/          # Main CLI entry point
├── cmd/govard-desktop/  # Desktop entry point (Wails)
├── internal/
│   ├── cmd/             # CLI command definitions
│   ├── engine/          # Core business logic
│   ├── desktop/         # Desktop app bindings (Wails)
│   ├── proxy/           # Caddy proxy management
│   ├── ui/              # Terminal UI
│   └── updater/         # Self-update mechanism
├── desktop/             # Desktop app assets (Wails frontend/config)
├── blueprints/          # Docker Compose templates
├── docker/              # Docker image definitions
├── tests/               # Test files
├── scripts/             # Dev helper scripts (for example pre-push)
└── docs/                # Documentation
```

## Development Setup

### Prerequisites

- Go 1.24+
- Docker & Docker Compose
- Make

Verify toolchain:

```bash
go version
gofmt -h >/dev/null
```

If `go` or `gofmt` is not found, add Go bin to your shell path:

```bash
export PATH="$HOME/go/bin:$PATH"
```

### Build Commands

```bash
# Build for development
go build -o govard cmd/govard/main.go

# Build all platforms
make build

# Install locally
make install

# Build Docker images
make images
```

### Desktop Development (Optional)

The desktop app uses Wails. To run it in dev mode:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
govard desktop --dev
```

### Optional Services

Govard can include optional services via `stack.services`:
- `cache`: `redis` or `valkey`
- `search`: `opensearch` or `elasticsearch`
- `queue`: `rabbitmq`

Xdebug uses a dedicated `php-debug` container and routes requests based on the
`XDEBUG_SESSION` cookie. Configure the match value via `stack.xdebug_session`.

Example:

```yaml
stack:
  services:
    cache: redis
    search: opensearch
    queue: rabbitmq
  queue_version: "3.13.7"
  xdebug_session: "PHPSTORM,VSCODE"
  features:
    xdebug: true
```

### PHP Images (Build Args)

Govard uses a single PHP Dockerfile with build args to avoid version-specific folders.

Base PHP image:

```bash
docker build -f docker/php/Dockerfile -t govard/php:8.4 --build-arg PHP_VERSION=8.4 docker/php
```

Magento 2 PHP image (adds Node.js, Grunt, and n98-magerun):

```bash
docker build -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.4 --build-arg PHP_VERSION=8.4 docker/php
```

## Testing

### Run All Tests

```bash
# Frontend + unit + integration
make test
```

### Run Fast Local Tests (recommended before push)

```bash
# Frontend + unit (no integration)
make test-fast
```

### Run Unit Tests (without integration)

```bash
make test-unit
```

### Run Integration Tests (opt-in)

```bash
make test-integration
```

### Run Frontend Unit Tests

```bash
make test-frontend
```

## Dependency Mode

Govard uses Go modules. Always commit `go.mod` and `go.sum`.

If you need to force module mode (ignore `vendor/`):

```bash
go build -mod=mod -o govard cmd/govard/main.go
go test -mod=mod $(go list ./... | grep -v '^govard/tests/integration$') -v -short
```

### Run Specific Test

```bash
# Run discovery tests
go test ./tests -v -run TestMagentoDiscovery

# Run blueprint tests
go test ./tests -v -run TestRenderLaravelBlueprint

# Run integration CLI tests
go test -tags integration ./tests/integration/... -v -run TestCLI
```

### Test Structure

| Test File | Purpose |
|-----------|---------|
| `framework_detection_test.go` | Framework detection tests (9 frameworks) |
| `blueprint_content_test.go` | Blueprint rendering tests |
| `blueprint_workflow_test.go` | Full setup workflow tests |
| `domain_hosts_test.go` | Domain and hosts file tests |
| `init_command_test.go` | Init command tests |

### Test Fixtures

Test projects located in `tests/`:
- `tests/fixtures/` - Shared fixture files used by unit tests (for example runtime profile fixtures)
- `tests/integration/projects/` - Integration test project fixtures per framework/workflow

## Code Style

### Naming Conventions

- **Files**: `snake_case.go` (e.g., `config_manage.go`, `self_update.go`)
- **Structs**: PascalCase (e.g., `RecipeCommand`, `ProjectMetadata`)
- **Functions**: PascalCase for exported, camelCase for private
- **Variables**: camelCase (e.g., `rootCmd`, `errorFilter`)
- **Constants**: PascalCase (e.g., `Version = "1.0.0"`)

### Import Organization

```go
import (
    "fmt"
    "os"

    "github.com/pterm/pterm"
    "github.com/spf13/cobra"

    "govard/internal/engine"
    "govard/internal/ui"
)
```

Groups: stdlib → 3rd party → internal

### Error Handling

```go
if err != nil {
    pterm.Error.Printf("Failed to parse .govard.yml: %v\n", err)
    return
}
```

- Use `pterm.Error.Println()` for user-facing errors
- Use `pterm.Warning.Printf()` for non-fatal warnings
- Return errors up the call stack when appropriate
- Fatal errors call `os.Exit(1)` in `Execute()` function

### Formatting

- Standard `gofmt` formatting
- No trailing whitespace
- Go tabs via `gofmt` (do not manually align with spaces)
- Max line length: ~100 characters (soft limit)

## Adding New Commands

1. Create file in `internal/cmd/`:

```go
package cmd

import (
    "github.com/spf13/cobra"
)

var myCmd = &cobra.Command{
    Use:   "mycommand [args]",
    Short: "Brief description",
    Run: func(cmd *cobra.Command, args []string) {
        // Implementation
    },
}
```

2. Register in `internal/cmd/root.go`:

```go
func init() {
    rootCmd.AddCommand(myCmd)
}
```

## Adding New Framework Support

1. **Create Blueprint**:
   - Add includes in `blueprints/includes/` or a framework-specific folder
   - Use existing templates as reference

2. **Update Detection**:
   - Edit `internal/engine/discovery.go`
   - Add package detection in `DetectFramework()`

3. **Add Test Project**:
   - Create/update fixture data under `tests/integration/projects/` and/or `tests/fixtures/`
   - Add `composer.json` or `package.json` with framework dependency

4. **Add Tests**:
   - Update `tests/framework_detection_test.go`
   - Update `tests/blueprint_content_test.go`

5. **Update Documentation**:
   - Add to `docs/frameworks/[framework].md`
   - Update `docs/user/configuration.md`
   - Update `docs/user/commands.md`

## Blueprint Creation

Blueprints use Go `text/template`. Use `{{ $.ImageRepository }}` for image names to support custom repositories.

```yaml
version: '3.8'

services:
  web:
    image: {{ $.ImageRepository }}/nginx:latest
    volumes:
      - .:/var/www/html
    networks: [govard-net]
    depends_on: [php]

  php:
    image: {{ if eq .Config.Recipe "magento2" }}{{ $.ImageRepository }}/php-magento2:{{ .Config.Stack.PHPVersion }}{{ else }}{{ $.ImageRepository }}/php:{{ .Config.Stack.PHPVersion }}{{ end }}
    volumes:
      - .:/var/www/html
    environment:
      XDEBUG_MODE: {{ if .Config.Stack.Features.Xdebug }}debug{{ else }}off{{ end }}
    networks: [govard-net]

networks:
  govard-net:
    driver: bridge
  govard-proxy:
    external: true
```

## CI/CD

GitHub Actions in `.github/workflows/`:

- **ci-pipeline.yml**: Unified CI workflow for push/PR on `main`, `master`, `develop`
  - `Quality Checks (Vet + Format)`: `make vet` + `gofmt -s -l .`
  - `Fast Tests (Frontend + Unit)`: `make test-fast`
  - `Integration Tests`: `make test-integration-ci`
  - `Build Binaries`: `make build`
  
- **release.yml**: Triggered on semantic version tags (`v*.*.*`)
  - Uses GoReleaser with `.goreleaser.yml`
- **codeql.yml**: Weekly + PR security scanning for Go and workflow files
- **govulncheck.yml**: Weekly Go vulnerability scan (SARIF upload)

### Troubleshooting CI Locally

If CI fails, run the same gates locally in this order:

```bash
# 1) Quality checks
make vet
gofmt -s -l .

# 2) Fast tests (frontend + unit)
make test-fast

# 3) Integration tests (optional, slower)
make test-integration-ci

# 4) Build artifacts
make build
```

Expected toolchain:

- Go: `1.24.x`
- Node.js: `20.x`

Quick environment verification:

```bash
go version
node --version
docker --version
```

Common fixes:

- `make test-fast` fails on frontend tests:
  - Ensure Node.js 20+ is installed.
  - Re-run: `node --test tests/frontend/*.test.mjs`
- `make test-unit` fails after dependency changes:
  - Run `go mod tidy` and commit `go.mod`/`go.sum`.
- Integration tests fail due to Docker:
  - Confirm Docker daemon is running and user has permission to access Docker.

## Release Process

1. Ensure tests pass (`make test-fast` minimum).
2. Create release tag: `git tag -a v1.x.x -m "Govard v1.x.x"`.
3. Push tag: `git push origin v1.x.x`.
4. GitHub Actions `release.yml` runs GoReleaser and publishes artifacts.

Notes:

- Release binaries inject the git tag into `govard version` via GoReleaser `ldflags`.
- `.goreleaser.yml` is the source of truth for build/archive settings.

## Submitting Changes

1. Fork the repository
2. Create feature branch: `git checkout -b feature/my-feature`
3. Make changes with tests
4. Ensure all tests pass: `make test` and `make test-frontend`
5. Format code: `gofmt -w .`
6. Commit with clear message
7. Push and create Pull Request

## Code Review Checklist

- [ ] Tests pass
- [ ] Code formatted with `gofmt`
- [ ] No unused imports
- [ ] Error handling appropriate
- [ ] Documentation updated
- [ ] Blueprint renders correctly (if applicable)
