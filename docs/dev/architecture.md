# System Architecture

Technical specification and architecture of Govard.

## 1. Project Identity & Philosophy

**Govard** (Go-based Versatile Runtime & Development) is a professional-grade local development orchestrator engineered to replace legacy bash-based tools with a high-performance, Go-native binary.

- **Target Frameworks**: Primarily optimized for Magento 2, extensible to Magento 1/OpenMage, Laravel, Next.js, Drupal, Symfony, Shopware, CakePHP, and WordPress
- **Core Principle**: "Simple - Efficient - High Performance"

## 2. System Architecture

### 2.1 Core Engine (Go)

- **Language**: Go 1.24+ for cross-platform support (macOS Intel/Silicon, Linux, Windows)
- **CLI Framework**: **Cobra CLI** for robust command structure
- **Orchestration**: Direct integration with **Docker SDK** to manage container lifecycles
- **Templating**: **Go `text/template`** engine for dynamic Docker Compose rendering from framework-specific Blueprints

### 2.2 Networking Layer

- **Reverse Proxy**: Integrated **Caddy** server as a global gateway
- **SSL/TLS**: Automated internal CA management via Caddy for HTTPS on `.test` domains
- **Service Mesh**: Unified Docker network (`govard-net`) allowing seamless communication between global proxy and project containers

## 3. Core Functional Modules

### 3.1 Initialization & Discovery (`internal/engine/discovery.go`)

**Logic:**
- Scans project root for `composer.json` or `package.json`
- Detects framework by checking package names:
  - Magento 1/OpenMage: `openmage/magento-lts` or `app/Mage.php`
  - Magento 2: `magento/product-community-edition`
  - Laravel: `laravel/framework`
  - Next.js: `next` dependency
  - Drupal: `drupal/core`
  - Symfony: `symfony/framework-bundle`
  - Shopware: `shopware/core`
  - CakePHP: `cakephp/cakephp`
  - WordPress: `johnpbloch/wordpress`

**Output:** Generates `.govard.yml` configuration file

### 3.2 Environment Liftoff (`internal/engine/render.go`)

**Process:**
1. `Detect` project framework context
2. `Validate` layered config and startup prerequisites (Docker/Compose, port, disk, network sanity)
3. `Render` per-project compose file in `~/.govard/compose/` from selected Blueprint
4. `Start` containers with `docker compose up -d`
5. `Verify` hosts/proxy wiring and post-up hooks

### 3.3 Blueprint Rendering

Blueprints are Go templates stored in `blueprints/`:

- Variables passed via `engine.Config` struct
- Conditional rendering based on feature flags
- Support for PHP version, database type, and optional services

### 3.4 Diagnostic Suite (`internal/engine/docker.go`)

Checks for common local development friction points:
- Docker Desktop/Daemon connectivity
- Docker Compose plugin availability
- Port conflicts on host machine (80, 443)
- Disk scratch write sanity
- Outbound network probe sanity

### 3.5 Automated Configuration (`internal/engine/magento.go`)

For Magento 2:
- Injects DB credentials, Redis hostnames, Varnish settings into `app/etc/env.php`
- Uses the configured runtime user (`stack.user_id:stack.group_id` when set, otherwise `www-data`) to preserve file permissions

### 3.6 Desktop App (Wails)

- **Entrypoint**: `cmd/govard-desktop`
- **Frontend**: `desktop/frontend`
- **Bindings**: `internal/desktop` exposes API methods to the UI
- **One-screen status**: dashboard summary cards plus environment list with start/stop/open controls
- **Action surface**: quick actions (start/stop/open, PHPMyAdmin, Xdebug toggle, health)
- **Project management surface**: dedicated workspace grouping environment list, quick actions, and onboarding
- **Observability**: logs with service filtering and live streaming
- **Command access**: shell launcher with project/service/user/shell selection
- **Preferences**: theme, proxy target, preferred browser, and per-project shell-user preferences persisted in desktop preferences
- **Frontend structure**: modular frontend split across feature modules and bridge/state services

## 4. Project Structure

```
.
├── cmd/govard/          # Main entry point
├── cmd/govard-desktop/  # Desktop entry point (Wails)
├── internal/
│   ├── cmd/             # CLI command definitions (Cobra)
│   ├── engine/          # Business logic, Docker SDK, rendering
│   ├── desktop/         # Desktop app bindings (Wails)
│   ├── proxy/           # Caddy proxy management
│   ├── ui/              # Terminal UI (pterm)
│   └── updater/         # Self-update mechanism
├── desktop/             # Desktop app assets (frontend/config)
├── blueprints/          # Docker Compose templates
├── docker/              # Docker image definitions
└── tests/               # Test files and fixtures
```

### 4.1 CLI Architecture

Commands organized by domain:
- **Environment**: `up`, `stop`, `status`
- **Development**: `shell`, `logs`, `debug`
- **Frameworks**: `magento`, `artisan`, `composer`, `npm`, etc.
- **Services**: `db`, `redis`, `varnish`, `elasticsearch`
- **Diagnostics**: `doctor`, `trust`

### 4.2 Engine Architecture

Key packages:
- `config.go` - Configuration structures
- `discovery.go` - Framework detection
- `render.go` - Blueprint rendering
- `docker.go` - Docker SDK integration
- `hosts.go` - Host file management
- `magento.go` - Magento-specific configuration
- `proxy.go` - Caddy integration
- `trust.go` - SSL certificate handling

## 5. Stack Components

### 5.1 Web Server (Nginx/Apache/Hybrid)

- Nginx (default), Apache, or Hybrid via `stack.services.web_server`
- Serves from project root
- Proxy to PHP-FPM via FastCGI (Nginx) or `ProxyPass` (Apache)
- Hybrid mode: nginx handles edge traffic and proxies requests to an internal apache service

### 5.2 PHP-FPM

- Multi-version support (8.1, 8.3, 8.4)
- Pre-configured extensions
- Xdebug 3 integration with `host.docker.internal`
- Cookie-based routing to dedicated `php-debug` container when `stack.features.xdebug` is enabled

### 5.3 Database (MariaDB/MySQL)

- Default: MariaDB 11.4
- Configurable version
- Auto-created database and user

### 5.4 Optional Services

- **Varnish 7.x**: Full-page caching
- **Redis**: Cache and session storage
- **Elasticsearch/OpenSearch**: Search engine
- **RabbitMQ**: Queue service

## 6. Networking

### 6.1 Docker Networks

- `govard-net`: Internal project network (bridge)
- `govard-proxy`: External shared network for Caddy

### 6.2 Service Discovery

Services accessible by hostname within `govard-net`:
- `web`: Nginx
- `apache`: Apache sidecar (hybrid mode only)
- `php`: PHP-FPM
- `php-debug`: Xdebug-enabled PHP-FPM (only when enabled)
- `db`: Database
- `varnish`: Varnish (if enabled)
- `redis`: Redis cache and sessions (if enabled)
- `elasticsearch`: Search engine (if enabled)
- `rabbitmq`: Queue service (if enabled)

### 6.3 Domain Routing

- Project domain → Caddy → Web container
- Pattern: `*.test` domains
- Automatic HTTPS via Caddy CA

## 7. Security

### 7.1 SSL/TLS

- Caddy internal PKI
- Root CA generated once, shared across projects
- Wildcard certificate support

### 7.2 Container Security

- Non-root users where possible
- Read-only volumes for configuration
- Isolated networks per project

## 8. Extensibility

### 8.1 Adding New Frameworks

1. Create blueprint includes in `blueprints/includes/` or a framework-specific folder
2. Add detection logic in `internal/engine/discovery.go`
3. Add test fixtures in `tests/init-projects/[framework]-init/`
4. Update documentation

### 8.2 Custom Blueprints

Blueprints use Go `text/template` syntax:

```go
{{ if eq .Config.Stack.Services.Cache "redis" }}
redis:
  image: redis:7.0-alpine
{{ end }}
```

Available variables:
- `{{ .Config.ProjectName }}`
- `{{ .Config.Recipe }}`
- `{{ .Config.Domain }}`
- `{{ .Config.Stack.PHPVersion }}`
- `{{ .Config.Stack.Features.Xdebug }}`
- `{{ .Config.Stack.Services.Cache }}`
- etc.
