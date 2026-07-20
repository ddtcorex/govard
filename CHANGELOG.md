# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.58.0] - 2026-07-20

### ✨ New Features

- **PrestaShop Support:** Govard now auto-detects PrestaShop projects (via `config/defines.inc.php`) and applies framework-specific runtime defaults (PHP 8.1, MariaDB 10.11, no forced cache/search/queue). Adds a dedicated nginx blueprint template handling PrestaShop's friendly-URL product/category image rewrites, plus Docker Compose service wiring. `govard bootstrap` supports detecting and cloning/configuring an existing PrestaShop install (no fresh-install pipeline yet): it patches or fabricates `app/config/parameters.php` (reusing the remote's real encryption secrets and table prefix when available rather than generating fresh ones against a cloned database), points the shop's domain, SSL, and mail relay at the local govard environment, and syncs remote DB credentials automatically. `govard tool prestashop [command]` runs the PrestaShop CLI (Symfony console) inside the PHP container.

## [1.57.1] - 2026-07-17

### 🐛 Bug Fixes

- **`db sync` / `db import --stream-db` Progress Bar Accuracy:** The progress bar previously jumped to 100% as soon as bytes were streamed into mysql's stdin, even though mysql could keep committing for tens of minutes afterwards with no feedback. It also force-padded the counter to the pre-sync size estimate, which could show a false 100% next to a smaller real byte count, or silently freeze mid-transfer if the estimate undershot reality. The bar now tracks real bytes transferred independently of the estimate, caps the displayed percentage at 99% until the import process actually exits successfully, and stays alive through the finalize/commit phase (polling the target database's growing size) instead of being replaced by a disconnected spinner.

## [1.57.0] - 2026-07-17

### ✨ New Features

- **Custom Nginx/Apache Config Extension Point:** Added `.govard/nginx/custom/*.conf` and `.govard/apache/custom/*.conf` as new project extension directories. Files placed there are mounted read-only into the web server container and included directly inside the rendered `server {}` block (nginx) or `<VirtualHost>` block (Apache), so extra directives no longer require replacing Govard's entire generated nginx/Apache config. Both directories are fingerprinted alongside `.govard/docker-compose.override.yml`, so the next `env up` auto-re-renders when their contents change.

## [1.56.0] - 2026-07-15

### ✨ New Features

- **RabbitMQ CLI Shortcut:** Added `govard rabbitmq` (aliased under `govard env rabbitmq`), mirroring the existing `redis`/`varnish` command shape: `status` and `queues` run against `rabbitmqctl` inside the container, `cli` passes through arbitrary `rabbitmqctl` commands, and standard Docker Compose maintenance commands (`ps`, `logs`, `stop`, `start`, etc.) are supported.
- **Automatic Magento 2 AMQP Configuration:** `govard config auto` now configures Magento 2's AMQP connection (`setup:config:set --amqp-*`) automatically when `stack.services.queue: rabbitmq` is set, the same way it already auto-configures Redis cache/session backends.

### 🐛 Bug Fixes

- **RabbitMQ Guest-User Loopback Restriction:** RabbitMQ refuses `guest`/`guest` AMQP logins from anywhere but loopback by default. Since the PHP container reaches `rabbitmq` over the `govard-net` Docker network (not loopback), this previously meant any AMQP connection attempt failed with `ACCESS_REFUSED`. Fixed by mounting a `rabbitmq.conf` with `loopback_users.guest = false`, staged the same way as the existing Varnish VCL.

## [1.55.1] - 2026-07-15

### 🐛 Bug Fixes

- **Xdebug Crashes on PHP 8.1-8.4:** `docker/php/debug/Dockerfile` now installs Xdebug `3.4.5` for PHP 8.1-8.4 instead of `3.5.3`. The 3.5.x line has a crash bug in its rewritten debugger.c: a real debug client connection combined with internal-function-call breakpoint checks (e.g. exception breakpoints) segfaults in `zend_get_executed_lineno()`. PHP 8.5 has no 3.4.x release, so it stays on `3.5.3` and remains exposed until Xdebug ships a fix.

### 📚 Documentation

- Overhauled SEO for the docs site (govard.ddtcorex.com): unique title/description frontmatter on all EN/VI pages, an auto-generated sitemap.xml covering every resolved route, canonical/hreflang links between EN/VI page pairs, and Open Graph/Twitter Card/JSON-LD tags with an on-brand social preview image.

## [1.55.0] - 2026-07-15

### ✨ New Features

- **Custom Xdebug Version:** Added `stack.xdebug_version` to `.govard.yml`, letting projects override the PECL Xdebug version installed in the `php-debug` service instead of Govard's recommended default. Setting it forces a local image build (the version is baked into the image), since the override doesn't match any published `-debug` tag.

### 🐛 Bug Fixes

- **Xdebug Segfaults on Alpine:** Added a `ulimits.stack` override (8MB) to the `php-debug` service. Alpine/musl's default ~80KB thread stack, combined with Xdebug's native stack usage per call and `xdebug.start_with_request=yes`, could overflow and segfault on ordinary recursive PHP calls instead of raising a normal PHP error.
- **PHP 8.x Xdebug Compatibility:** `docker/php/debug/Dockerfile` now pins Xdebug to `3.5.3` for all PHP 8.x builds (previously installed whatever `pecl install xdebug` resolved to as "latest," which was not reproducible and could resolve to a build untested against a newer PHP release).

### 🔧 Maintenance

- **Blueprint Lifecycle:** Incremented internal `BlueprintVersion` to **1.46**, triggering automatic environment re-renders so existing `php-debug` services pick up the new stack ulimit.

### 📚 Documentation

- Documented `stack.xdebug_version` in the configuration reference (English and Vietnamese).
- Documented when to bump `BlueprintVersion` in `CLAUDE.md`.

## [1.54.7] - 2026-07-13

### ✨ New Features

- **Framework-Agnostic Container Skip Flag:** Renamed `bootstrap`'s `--skip-up` flag to `--no-up` for consistency with the other `--no-*` scope flags (`--no-db`, `--no-media`, `--no-composer`, `--no-admin`, etc). `--skip-up` is no longer accepted.

### 📚 Documentation

- Documented `--no-up` in the CLI commands reference (English and Vietnamese).

## [1.54.6] - 2026-07-09

### ✨ New Features

- **Elasticsearch/OpenSearch Host Access:** Projects with `stack.services.search` set to `elasticsearch` or `opensearch` are now automatically reachable from the host at `http://<project-domain>:9200`. Reuses the same Caddy Host-header routing that already serves each project's HTTPS domain, so no extra configuration or per-project port is needed.

### 📚 Documentation

- Documented the new `:9200` host-access behavior and the one-time `govard env up` re-render required for pre-existing projects (`docs/reference/configuration.md`, `docs/workflows/ssl-and-domains.md`).

## [1.54.5] - 2026-07-07

### 🛠 Improvements

- **Multi-Arch Docker Images:** `make images`/`make push` now build through a Buildx Bake wrapper that provisions a multi-platform builder and targets `linux/amd64,linux/arm64` on macOS hosts, fixing Govard image builds on Apple Silicon.
- **Local Image Architecture Fallback:** `govard doctor --fix`, `govard env images pull`, and `govard up` now detect locally cached Govard images built for the wrong architecture (e.g. an amd64 image on a Darwin/arm64 host) and rebuild them locally instead of failing at container start.

### 🐛 Bug Fixes

- **PHP Entrypoint UID Remap:** Fixed the PHP container entrypoint to re-enter as root before remapping the `www-data` UID/GID, preventing sudo failures once the original `www-data` UID no longer resolves.
- **Project Path Resolution:** Compose file paths and project registry lookups now resolve symlinks before comparison, preventing duplicate or mismatched project entries when a project path is accessed via a symlinked directory.

### 🔧 Maintenance

- **Dependabot Removal:** Removed the unused Dependabot configuration file.

## [1.54.4] - 2026-06-24

### 🐛 Bug Fixes

- **macOS Source Builds:** Fixed macOS source desktop builds by explicitly linking the `UniformTypeIdentifiers` framework.

### 🔧 Maintenance

- **Dead Code Removal:** Removed unused types and functions in desktop preferences (`ReadDesktopSettings`) and remote sync helpers (`SyncInput`).
- **Tests Cleanup:** Removed skipped integration tests for database snapshots that required running Docker containers.

## [1.54.3] - 2026-06-16

### 🛠 Improvements

- **Composer Compatibility:** Removed redundant project-level audit bypass configuration. Global composer config already handles this setting, and project-level config may fail when mounted as read-only.

## [1.54.2] - 2026-06-15

### 🛠 Improvements

- **Profile Shift Detection:** Improved detection logic to treat empty configuration values as "use profile default" rather than triggering a false shift. This prevents unnecessary reconfiguration when partial or default configs are used.
- **Base Version Comparison:** Profile shift now compares base versions (ignoring patch suffixes like `-p9` vs `-p10`), preventing false positives between Magento security patches.

### 🐛 Bug Fixes

- **Non-Interactive Mode:** Fixed issue where `ConfigureMagento` was called unnecessarily in non-interactive mode even when no profile shift was detected.

### 🔧 Maintenance

- **Dead Code Removal:** Removed the `patchMagentoElasticsearchSchemaForLibxml` function that was never called in production.
- **Enhanced Test Coverage:** Added tests for empty value handling in profile shift detection.

## [1.54.1] - 2026-06-04

### ✨ New Features

- **Magento Tuning Prompt:** Added interactive prompt asking user before running Magento auto-configuration during `env up`. Users can now choose whether to run tuning or skip it.
- **Profile Switch Warning:** Added warning message after profile switch to remind users to restart the environment.

### 🛠 Improvements

- **Simplified Profile Switch:** Profile switch now only switches the profile without auto-starting the environment. Users run `govard env up` manually after switching.
- **Framework-Agnostic Tuning Flag:** Renamed `--skip-magento-tuning` to `--no-tuning` to be framework-agnostic.
- **Profile Shift Detection:** Fixed false profile shift detection when config file doesn't have a `profile:` field. Now uses registry's Profile instead.

### 🐛 Bug Fixes

- **Missing Warning on Default Profile Switch:** Fixed issue where warning was not shown when switching from default (empty) profile to a named profile.

### 📚 Documentation

- Updated CLI commands documentation with `--no-tuning` flag.
- Updated Vietnamese CLI commands documentation.

## [1.54.0] - 2026-06-03

### ✨ New Features

- **Global Services Section:** Added comprehensive documentation for built-in services (Mailpit, PHPMyAdmin, Portainer) with CLI shortcuts (`govard open mail`, `govard open db`, `govard open portainer`).
- **WordPress Support:** Added WordPress compatibility checks and WP-CLI installation during bootstrap.
- **Vietnamese Documentation:** Complete Vietnamese translations for getting started, reference guides, workflows, and FAQ sections.
- **Enhanced Profile Display:** Profile information is now shown in `profile` and `status` commands with improved formatting and feedback.
- **Previous Profile Handling:** Profile switching now tracks and handles previous profile state for better environment transitions.

### 🛠 Improvements

- **Profile Switch Feedback:** Improved feedback when switching to the default profile.
- **Lock File Update:** Deferred lock file updates to allow proper profile shift detection.
- **Documentation Sync:** Updated workflow documentation and wiki sync automation.

### 🐛 Bug Fixes

- **Profile Shift Detection:** Fixed issue where profile shift was not detected due to premature lock file updates.

### 📚 Documentation

- Added Global Services documentation section to README.md.
- Complete Vietnamese documentation across 10+ files.
- Updated CLI commands reference with Global Services section.
- Enhanced workflow documentation for remotes, sync, SSL, and desktop app.

## [1.53.1] - 2026-06-02

### ✨ New Features

- **Profile Switch Command:** `govard config profile switch <name>` to switch between environment profiles. Profiles are persisted per-project in `~/.govard/projects.json`.
- **Profile Clear Command:** `govard config profile clear` to reset to default profile.
- **Profile Display:** Shows active profile when running `env up/start/restart`.

### 🔄 Refactors

- **Profile Resolution:** Profile priority: `--profile` flag > project registry (last-used) > empty (default).
- **Subprocess Output:** Profile switch now runs `govard env up` as subprocess to show full startup output.

### 🐛 Bug Fixes

- **Duplicate Success Message:** Fixed duplicate success message when profile switch starts environment.
- **Profile Tuning Prompts:** Fixed profile tuning prompts not showing proper output.

### 📚 Documentation

- Updated CLI commands documentation with profile switch/clear commands.
- Updated configuration reference with profile commands.
- Updated design spec.

## [1.53.0] - 2026-06-01

### ✨ New Features

- **Custom Projects Without PHP:** Added ability to create custom framework projects without requiring a PHP container. Users can now select "none" as the PHP version during init, allowing for stacks that don't need PHP (e.g., pure Node.js projects).

### 🔄 Refactors

- **Database Configuration:** Replaced `DBType` with `Services.DB` in the configuration structure to streamline database service management. Updated all internal functions, methods, and tests to use the new `Services.DB` field.
- **CLAUDE.md Creation:** Created a comprehensive project documentation file (CLAUDE.md) to replace AGENTS.md, providing clear instructions for AI coding agents working in the govard codebase.

## [1.52.0] - 2026-05-29

### ✨ New Features

- **GitHub Actions CI/CD:** Added GitHub Actions workflow for deploying documentation to Cloudflare Pages.
- **Comprehensive Documentation:** Added complete documentation for frameworks, remote management, SSL, and Govard Desktop app.
- **Next.js Fallback:** Added fallback to yarn when npx/npm is missing for Next.js bootstrap.

### 🐛 Bug Fixes

- **Wiki Sync:** Fixed kebab-case filename preservation for correct GitHub Wiki URL matching.
- **Header Styling:** Multiple header styling improvements for desktop and mobile.
- **PHP-FPM:** Fixed Symfony environment by running php-fpm as remapped user.
- **Shopware:** Resolved directory permissions and correct domain mapping.
- **Container Security:** Fixed file permission errors for custom host UIDs.
- **Sitemap:** Updated change frequencies from quarterly to yearly.

### 🛠 Improvements

- **CI/CD:** Updated Cloudflare Pages deployment to use npx instead of wrangler-action.
- **Bootstrap:** Added best-effort CLI error ignoring and console output silencing.

## [1.51.1] - 2026-05-18

### ✨ New Features

- **Database Table Prefix Support:** Added comprehensive support for custom database table prefixes in Magento 1, Magento 2, and OpenMage environments, including a robust `SafeTablePrefix` helper function and a safe fallback to `demo_`.
- **Symfony Bootstrap Automation:** Implemented conditional execution of Doctrine commands (database creation and migrations) based on `composer.json` dependency detection, ensuring smoother onboarding for Symfony projects.

### 🛠 Improvements

- **Testing Infrastructure:** Added detailed unit and integration tests covering database table prefix handling, Symfony bootstrap conditional detection, database credentials, remote metadata, and migration synchronization.

## [1.51.0] - 2026-05-15

### ✨ New Features

- **Elasticsearch Support:** Added Elasticsearch support to framework configuration and incremented internal blueprint version for automated environment updates.
- **Profile Shift Detection:** Implemented proactive profile shift detection with user confirmation and integrated infrastructure cleanup to prevent container corruption during environment transitions.

### 🛠 Improvements

- **Profile Management:** Refactored engine apply logic to remove default profile assignments, ensuring more predictable environment identity.
- **Compose Orchestration:** Integrated project profiles into compose file path resolution for improved isolation.
- **Magento Lifecycle:** Hardened the Magento composer installation process and improved package dependency management with automated relaxation and composer compatibility checks.

### 📦 Dependencies

- Updated all Go packages and pinned dependencies to their latest versions.

## [1.50.0] - 2026-05-13

### ✨ New Features

- **Service Defaults:** Updated default infrastructure versions across all blueprints:
  - **PHP:** Bumped to **8.5**
  - **Valkey:** Bumped to **9.0** (replacing Redis defaults)
  - **Varnish:** Bumped to **8.0**
  - **RabbitMQ:** Bumped to **4.2**
  - **OpenSearch:** Bumped to **3.0**
- **Composer Security:** Replaced the generic `/root/.composer` volume mount with granular mapping of `auth.json` and `config.json` from the host. This prevents configuration leakage and ensures better isolation between the host and container environments.

### 🛠 Improvements

- **Remote Resolution:** Simplified remote environment resolution logic. Removed rigid auto-aliasing for custom remote names, providing more predictable behavior and better support for non-standard environment labels.

## [1.49.6] - 2026-05-04

### 🛠 Improvements

- **Search Engines:** Increased `indices.query.bool.max_clause_count` to **10240** in Govard blueprints for both Elasticsearch and OpenSearch services to prevent `too_many_nested_clauses` errors during complex search operations.
- **Blueprint Lifecycle:** Incremented internal `BlueprintVersion` to **1.41**, triggering automatic environment re-renders to ensure existing environments receive the updated search engine settings.

## [1.49.5] - 2026-04-22

### 🛠 Improvements

- **Diagnostics Engine:** Enhanced `govard doctor` and environment validation logic.
- SSH Agent: Now treats an "empty" agent (connected but no identities) as a healthy state, preventing unnecessary warnings.
- Multi-language Support: Refactored SSH agent responsiveness checks to use exit codes instead of English string matching, ensuring compatibility with non-English locales.
- Validation Bypass: Implemented a short-circuit for "advisory" profile sync warnings in `custom` and `generic` projects to avoid obstructive recommendations for non-standard frameworks.

## [1.49.4] - 2026-04-22

### ✨ New Features

- **Node.js Integration:** Implemented a robust "Hybrid" Node.js versioning strategy for all PHP containers.
- Pre-baked standard Node.js, NPM, Yarn, Grunt, and Gulp into the base PHP images for instant availability.
- Added dynamic Node.js version switching in `entrypoint.sh` that automatically downloads and installs the exact version specified in `.govard.yml` if it differs from the image default.
- Added `NODE_VERSION` environment variable propagation to all PHP services.

### 🛠 Improvements

- **Blueprint Lifecycle:** Incremented internal `BlueprintVersion` to **1.40**, triggering automatic environment re-renders to apply new Node.js environment variables.
- **Image Optimization:** Cleaned up redundant Node.js installation logic in `php-magento1` and `php-magento2` images as they now inherit from the centralized base image.

### 🐛 Bug Fixes

- **Environment:** Fixed missing Node.js/NPM environment in PHP containers for projects using the `custom` framework or non-Magento stacks.

## [1.49.3] - 2026-04-20

### 🐛 Bug Fixes

- **Magento Auto-Configuration:** Fixed an issue where `govard config auto` would return silently without performing any actions if no profile or PHP version shift was detected. The command now correctly forces a re-configuration of the environment as intended by the user.
- **Database Robustness:** Refactored the proactive search host fix SQL logic to safely handle empty databases or databases with custom table prefixes using MySQL `IF()` condition guards, preventing failures during initial project bootstrap or environment reset.

### 🚀 Improvements

- **CLI Orchestration:** Enhanced the internal subcommand runner to automatically propagate the current environment profile (`--profile`) to all nested CLI invocations, ensuring consistent behavior across complex, multi-profile development setups.

## [1.49.2] - 2026-04-17

### ✨ New Features

- **Magento Operations:** Added `magento_operation` to the framework manifest table list for improved synchronization control.
- **Environment Lifecycle:** Implemented automatic Magento cache cleanup and dependency synchronization triggered by PHP version or profile shifts.

### 🔄 Refactors

- **Database Credentials:** Centralized database credential management and introduced support for custom database configurations via remote settings.
- **Magento Shift Logic:** Optimized Magento profile shift detection and refined associated cleanup workflows.
- **Code Hygiene:** Standardized internal engine formatting and improved error handling across core test suites.

### 🧪 Testing

- **Integration Tests:** Standardized integration test tagging with `integration` build tags for `magento_shift` and `volume_check` suites.

## [1.49.1] - 2026-04-15

### 🐛 Bug Fixes

- **Remote Synchronization:** Injected `RSYNC_OLD_ARGS=1` environment variable to ensure backward compatibility with `rsync` versions >= 3.2.4 by disabling the strict `--protect-args` default behavior, resolving path quoting failures during file and media sync operations.

## [1.49.0] - 2026-04-15

### ✨ New Features

- **Apache Configuration:** Added `index.php` rewrite rules and explicit `PATH_INFO` support for legacy Magento 1 environments.
- **Magento 1 Support:** Improved environment bootstrap logic and path resolution for legacy framework versions.
- **Configuration Registry:** Externalized framework configuration into a centralized JSON manifest and migrated synchronization logic to a high-performance engine-based provider.
- **Composer Management:** Implemented profile-aware raw configuration loading and dynamic Composer patch version resolution for improved stack accuracy.

### 🛠 Improvements

- **Environment Infrastructure:** Incremented internal `BlueprintVersion` to **1.39**, triggering automatic re-renders for improved Apache compatibility.
- **Security & Permissions:** Standardized file and directory permissions using a new bootstrap security package and implemented defense-in-depth shell injection protection for all database queries.

### 🔄 Refactors

- **Core Conventions:** Consolidated all hardcoded project naming, binary conventions, file permissions, and path constants into a new centralized `conventions` package.
- **Dynamic Paths:** Replaced static path definitions with dynamic `WorkDir` and `HomeWWWData` variables across the engine and Docker configurations for improved portability.

## [1.48.2] - 2026-04-14

### ✨ New Features

- **Apache Configuration:** Added shared Apache handling for legacy `index.php/` path patterns, including explicit `PATH_INFO` support so Magento 1 environments can use "pretty" URLs without dropping route segments or triggering 403 errors.

### 🛠 Improvements

- **Blueprint Lifecycle:** Incremented internal `BlueprintVersion` to **1.39**, triggering automatic environment re-renders to ensure existing Apache environments receive the new rewrite rules.

## [1.48.1] - 2026-04-11

### ✨ New Features

- **Profile Registry:** Implemented a new JSON-based profile registry (`profiles.json`) for dynamic framework and Magento version resolution, replacing hardcoded logic with a flexible, externalized configuration for improved technical accuracy.

### 🛠 Improvements

- **Framework Compatibility:** Unified and refined technical profiles for Laravel, Symfony, WordPress, and Drupal to ensure consistent local development environments across all supported stacks.
- **Legacy Support:** Optimized metadata mapping for legacy framework versions to maintain backward compatibility while transitioning to the new registry system.

## [1.48.0] - 2026-04-10

### ✨ New Features

- **Inter-Project Connectivity:** Implemented opt-in cross-project networking via a new `linked_projects` field in `.govard.yml`. Projects that declare dependencies are selectively restarted to join the network of a newly started service, replacing the disruptive global container restart approach.

### 🛠 Improvements

- **Volume Naming:** Standardized all Docker volume names to use hyphens (`-`) instead of underscores (`_`), aligning with the existing `-data` pattern used across core blueprints.
- **phpMyAdmin:** Pinned the phpMyAdmin service image to `5.2.2` for improved build reproducibility and stability.

### 🔄 Refactors

- **Testing Infrastructure:** Removed the real-environment integration test suite and all associated Makefile targets to reduce coupling to live Docker environments and streamline CI.

## [1.47.2] - 2026-04-10

### ✨ New Features

- **Runtime Host Sync:** Implement cross-project runtime host synchronization and inject Govard Root CA into PHP containers for seamless SSL connectivity between projects.

### 🔄 Refactors

- **Testing Infrastructure:** Refactored blueprint rendering tests to use `engine.RenderBlueprint` and centralized home directory management for improved test isolation and stability.

## [1.47.1] - 2026-04-10

### 🔄 Refactors

- **Core Engine:** Consolidated Magento engine logic, removed unused frontend modules, and pruned legacy test helpers for improved maintainability.

## [1.47.0] - 2026-04-09

### ✨ New Features

- **Migration Orchestration:** Added automated database volume migration support for Warden and DDEV during `govard init` via new `db clone-volume` command.
- **Smart Magento Config:** Detect and force-override "locked" Magento core config keys (`base_url`, `cookie_domain`, `offloader_header`) in `app/etc/env.php` to match local environment.
- **Interactive SSH Auth:** Proactive SSH public key detection and automated deployment when authentication fails during remote operations.

### 🛠 Improvements

- **Volume Hygiene:** Pre-create Docker volumes with official Compose labels to prevent warnings and improve environment stability.
- **Doctor Fix Fallback:** `govard doctor --fix` now supports local image building if pulling a runtime image fails.

## [1.46.1] - 2026-04-09

### 🐛 Bug Fixes

- **Magento Docker Images:** Resolved persistent build failures and runtime errors in Magento 1 and 2 environments. Implemented a robust, version-aware dependency installation strategy that uses a pinned stable legacy version (**v1.101.1**) for Magento 1 to ensure full compatibility with PHP 5.6/7.0.
- **PHP Dockerfiles:** Fixed global `npm` installation permission issues by enforcing the `--unsafe-perm` flag across all PHP-based Docker images.
- **Build Stability:** Optimized memory management during container image builds by disabling Zend allocation and using specific PHP configuration flags to prevent segmentation faults during heavy dependency resolution.

## [1.46.0] - 2026-04-09

### ✨ New Features

- **Bootstrap Interruption:** Added skippable subcommand execution during the `bootstrap` process. Users can now skip non-critical tasks (like large database imports or initial file syncs) by pressing **Ctrl+C** without terminating the entire command.
- **Granular Media Sync:** Introduced granular media synchronization modes (`minimal`, `none`, `all`). This allows for more precise control over asset transfers during bootstrap and sync operations, significantly reducing onboarding time for media-heavy projects.

### 🛠 Improvements

- **Sync Optimization:** Consolidated and standardized synchronization exclusion logic across Magento, WordPress, and Laravel. Implemented a robust set of wildcard patterns for cache, temporary, and junk files to improve sync performance and reduce network overhead.
- **Sync UX:** Replaced tactical `pterm` spinners with high-visibility standard info and success logs in the `sync` command to prevent terminal output collisions and provide a more stable progress overview.
- **Installer Robustness:** Enhanced the unified installer with improved binary conflict detection and more descriptive feedback during dependency verification.

### 🔄 Refactors

- **Media Sync Strategy:** Transitioned the media synchronization logic from a simple boolean flag to a granular, string-based mode system across all core command families and the Desktop backend bridge.
- **Magento Sync Logic:** Removed the legacy "catalog media" sync mode in favor of the new granular `minimal` mode, simplifying the synchronization workflow for Magento developers.

### 🐛 Bug Fixes

- **PHP Dockerfiles:** Added the `--unsafe-perm` flag to global `npm install` commands in PHP Dockerfiles to resolve permission issues when building images as non-root users.
- **Composer Performance:** Disabled Zend memory allocation for Composer operations in PHP Dockerfiles to prevent memory-related crashes during heavy dependency resolution.
- **Docker Bake:** Formally registered `php-magento1` and `php-magento1-debug` targets in `docker-bake.hcl` for improved build observability and lifecycle management.

## [1.45.1] - 2026-04-08

### 🐛 Bug Fixes

- **PHP Image Build:** Fixed `icu-data-full` package resolution by using a dynamic `apk search` instead of a hardcoded conditional. This ensures compatibility across all Alpine-based PHP images where the package name may vary.

## [1.45.0] - 2026-04-08

### ✨ New Features

- **Magento 2:** Implemented automated **LiveReload** support for Magento 2 via port mapping and `env.php` injection, enabling real-time frontend updates during development.
- **Magento 1 / OpenMage:** Added `magerun` alias support and enabled dedicated runtime images for legacy Magento 1 and OpenMage projects.
- **Apache:** Increased `LimitRequestLine` and `LimitRequestFieldSize` to **16384** in standard Apache blueprints to support large session headers common in complex ecommerce environments.
- **Varnish:** Added a service readiness check with transient crash tolerance and updated ACL configuration for improved startup reliability.

### 🛠 Improvements

- **Environment Lifecycle:** Incremented internal `BlueprintVersion` to **1.35**, triggering automatic environment re-renders to apply the latest Apache and Varnish infrastructure optimizations.
- **Composer:** Updated the default Composer version to the latest stable release for all new project initializations.

### 🐛 Bug Fixes

- **Varnish:** Enhanced service startup behavior to prevent false-negative readiness failures when using custom ACL configurations.

## [1.44.5] - 2026-04-07

### ✨ New Features

- **Diagnostics:** Set `GOVARD_ASSUME_YES=true` environment variable when the `-y` / `--yes` flag is enabled during `init`, ensuring consistent non-interactive behavior for internal diagnostic checks.

### 🐛 Bug Fixes

- **Environment:** Use architecture-aware Mailpit download URLs in the installer to prevent rate limiting and ensure compatibility across different CPU architectures.
- **Diagnostics:** Safely ignore the return value of `os.Setenv` during initialization to prevent unnecessary execution failures in environments with restricted environment variable access.

## [1.44.4] - 2026-04-06

### 🚀 New Features

- **Performance:** Optimized Composer version management by pre-baking common binaries (`1`, `2`, `2.2`) into the PHP image, significantly reducing environment startup time for standard projects.
- **Composer:** Implemented an intelligent, verified fallback download mechanism for arbitrary point releases (e.g., `2.7.2`) specified in `stack.composer_version`.

### 🛠 Improvements

- **Blueprint Lifecycle:** Incremented internal `BlueprintVersion` to `1.28`, triggering automatic environment re-renders to ensure all projects receive the latest configuration optimizations and infrastructure fixes.
- **CI/CD:** Hardened the integration test suite by enforcing non-interactive mode (`-y`) for all `govard init` calls, preventing pipeline hangs in non-TTY environments.

### 🐛 Bug Fixes

- **Environment:** Resolved a blueprint rendering issue where empty `volumes` mappings caused Docker Compose schema validation failures.

## [1.44.3] - 2026-04-06

### 🛠 Improvements

- **Diagnostics:** Improved database version preservation logic. `doctor --fix` now respects user-defined DB versions even when they don't match framework defaults, preventing unintended overwrites and data corruption.
- **Diagnostics:** Added `GOVARD_ASSUME_YES=true` environment variable to auto-confirm `doctor --fix` changes, improving support for automation and CI/CD pipelines.
- **Diagnostics:** `govard doctor` now displays warnings for database version mismatches against recommendations while avoiding automated upgrades for explicit user-defined configurations.

### 🐛 Bug Fixes

- **Environment:** Fixed logic separation between version checks and profile synchronization in the `doctor` command.
- **Testing:** Updated integration tests to be compatible with the new "Clean Config" behavior.

## [1.44.2] - 2026-04-06

### 🔄 Refactors

- **Configuration:** Implemented "Clean Config" persistence. Redundant default values (PHP, Node, DB versions, and capabilities) are now omitted from `.govard.yml` to streamline configuration and improve portability.

### 🛠 Improvements

- **Diagnostics:** Upgraded `doctor --fix` with **multi-pass execution loops**. Related cascades (e.g., changing a profile resulting in missing Docker images) are now resolved comprehensively in a single command run.
- **Diagnostics:** Enhanced PHP version preservation logic. `doctor --fix` now respects manual PHP configurations when a specific framework version profile is not found.
- **UI/UX:** Removed incorrect "out of sync" warnings in `doctor` when using manual version overrides on default frameworks.

## [1.44.1] - 2026-04-06

### 🐛 Bug Fixes

- **Environment:** Fail fast when PHP container exits during startup verification to avoid redundant checks.
- **PHP Runtime:** Avoid self-remap failures for users with UID 1001 by adjusting the permission handler.

## [1.44.0] - 2026-04-06

### ✨ New Features

- **Documentation:** Migrated documentation from `/docs` to `/wiki` and added automated wiki synchronization workflow.

### 🛠 Improvements

- **Environment:** Enhanced proxy verification by ensuring the PHP runtime is fully ready before execution.

## [1.43.0] - 2026-04-04

### ✨ New Features

- **Frameworks:** Added full support for the **Emdash** framework, including runtime configuration, bootstrap patching, and dynamic container execution targets.

### 🔄 Refactors

- **Bootstrap Engine:** Implemented **staged project initialization** to support frameworks that require empty directories during the initial bootstrap phase.
- **Environment Core:** Modularized the environment startup logic with **dependency injection** and added project identity uniqueness validation for improved stability.

## [1.42.2] - 2026-04-03

### 🐛 Bug Fixes

- **Warden Migration:** Fixed configuration noise where the `capabilities` field was incorrectly included as an empty object (`capabilities: {}`) in `.govard.yml`. It is now omitted unless explicitly configured.
- **Remote Policy:** Improved nil-safety in the capability engine to prevent potential pointer dereferences when `capabilities` is unset.
- **Integration Tests:** Corrected struct-to-pointer mismatches in multiple test suites related to remote configuration refactoring.


## [1.42.1] - 2026-04-03

### ✨ New Features

- **Remotes:** Added interactive SSH port configuration to the `remote add` command, ensuring users are prompted for non-standard ports during environment onboarding.
- **CLI:** Reordered `remote add` interactive prompts to follow a more logical connection flow: `host` -> `user` -> `port` -> `path`.

## [1.42.0] - 2026-04-03

### ✨ New Features

- **Configuration Audit:** Introduced the `project.config.audit` diagnostic check to proactively identify legacy configuration patterns and provide automated migration guidance.

### 🛠 Improvements

- **Standardized Configuration:** Transitioned to a service-centric configuration model where all infrastructure components (Database, Cache, Search, Queue) are defined within the `stack.services` block.
- **Automated Legacy Migration:** Enhanced `doctor --fix` to automatically and safely migrate legacy `.govard.yml` files, mapping old `db_type` and feature flags to the new standardized model.
- **Optimized Configuration Layout:** Standardized the logical field ordering in `.govard.yml`, prioritizing `web_root` and web server versions at the top of the file for better clarity.
- **Refined Doctor UX:** Improved interactive diagnostic feedback; skipping optional fixes is now reported as `INFO` (Skipped) instead of `ERROR`, resulting in a cleaner and more accurate health summary.

### 🧹 Cleanup

- **Codebase:** Removed all legacy backward-compatibility mapping logic for old configuration keys, ensuring a clean and modern engine architecture.

## [1.41.0] - 2026-04-03

### ✨ New Features

- **Performance:** Parallelized domain resolution checks during environment startup with configurable timeouts (3s default), significantly reducing wait times for projects with multiple domains.
- **Engine:** Introduced `ComposerVersion` field in runtime profiles with intelligent auto-detection logic to ensure the correct Composer version is always available in the PHP container.
- **Diagnostics:** Improved `govard doctor --fix` with automatic project profile synchronization and repository directory health repairs.
- **Auto-Tune:** Introduced non-interactive initialization via `govard init --yes` and `govard bootstrap --yes` for seamless environment containerization.
- **UX:** Integrated interactive remote environment prompting during bootstrap when no staging/dev remotes are configured.

### 🛠 Improvements

- **Search Engines:** Normalized search engine image tagging strategy (Major.Minor) and implemented dynamic build version resolution for Elasticsearch and OpenSearch in `docker-bake.hcl`.
- **UI/UX:** Replaced the legacy `deps` command with proactive synchronization warnings during bootstrap and automated `doctor` repairs for a more streamlined developer experience.

### 🔄 Refactors

- **CLI:** Removed the redundant `deps` command and consolidated dependency management logic into the `doctor` and `bootstrap` workflows.
- **Core:** Refactored domain verification to use concurrent workers for faster local resolution checks.

## [1.40.4] - 2026-04-02

### ✨ New Features

- **Installer:** Added automatic `cloudflared` detection and installation support in `install.sh`. It now automatically downloads and installs the latest `.deb` package on Linux and uses Homebrew on macOS.

### 🛠 Improvements

- **Installer:** Interactive prompts in `install.sh` now default to "Yes" (`Y/n`), allowing users to proceed by simply pressing Enter.
- **Installer:** Streamlined the installation process by removing redundant SSL trust calls, as they are already handled by the service initialization.

## [1.40.3] - 2026-04-01

### ✨ New Features

- **Database:** Implemented human-readable progress tracking (MB/GB) for database import and synchronization operations.
- **Bootstrap:** Added a silent command execution helper (`runGovardSubcommandSilent`) to suppress verbose terminal output during idempotent setup steps like admin user creation.
- **Tunnel:** Added explicit documentation and CLI help notes regarding the `cloudflared` binary installation requirement.

### 🛠 Improvements

- **Synchronization:** Refactored shell quoting logic into the engine package and improved `rsync` error handling with proactive permission fix suggestions for Magento 2.

## [1.40.2] - 2026-04-01

### ✨ New Features

- **Configuration:** Implemented framework-specific default `chown_dir_list`. These defaults are now implied and omitted from `.govard.yml` unless explicitly overridden by the user, leading to cleaner configuration files.

## [1.40.1] - 2026-04-01

### ✨ New Features

- **Diagnostics:** Added a new `host.govard.registry` doctor check to automatically detect and repair project registry directory corruption.

### 🛠 Improvements

- **Project Registry:** Refactored `ProjectRegistryPath` to use the centralized `GovardHomeDir` helper for consistent environment resolution.

### 🐛 Bug Fixes

- **phpMyAdmin:** Supported both hyphenated (`-`) and underscored (`_`) container naming patterns for automated configuration.
- **phpMyAdmin:** Improved internal database hostname resolution in the generated PHP configuration.

## [1.40.0] - 2026-04-01

### ✨ New Features

- **Snapshots:** Added remote database and media snapshot support with bidirectional transfers, remote listing, and production safety guards.

### 🛠 Improvements

- **Performance:** Optimized domain verification by replacing per-domain checks with a bulk API call to the proxy, reducing latency.

### 🐛 Bug Fixes
- **Desktop:** Resolved startup failures on Ubuntu 24.04 by persisting AppArmor user namespace restrictions via `sysctl`.

## [1.39.0] - 2026-03-31

### ✨ New Features
- **Tunnel:** Added framework-agnostic HostHeader support to Cloudflare tunnels via Caddy proxy aliases.
- **Portainer:** Added pre-configured admin password and improved snapshot creation logging.
- **Project Management:** Added support for listing and safely deleting orphaned Docker projects.

### 🛠 Improvements
- **Testing:** Consolidated all quality checks and tests under a single `make test` command.

### 🐛 Bug Fixes
- **Tooling:** Added follow-redirects and insecure flag to `n98-magerun` download command.

## [1.38.0] - 2026-03-31

### Added
- Multi-framework support for `govard upgrade` (Magento, Laravel, Symfony, WordPress).

### Changed
- Bumped Wails from 2.11.0 to 2.12.0.
- Improved Desktop UI contrast and vertical alignment for buttons.

### Fixed
- Fixed Mailpit email delivery configuration (added mandatory `-t` flag).
- Fixed CHANGELOG.md formatting issues from previous release.

## [1.37.3] - 2026-03-31

### 🐛 Bug Fixes

- **Mailpit Communication**: Added the mandatory `-t` flag to `sendmail_path` in the standard PHP configuration. This resolves an `InvalidArgumentException` in Symfony Mailer (used by Magento, Shopware, Laravel, etc.) and ensures reliable email delivery to Mailpit.

## [1.37.2] - 2026-03-30

### 🛠 Improvements

- **Release Automation**: Significantly improved the automated changelog generation in `.goreleaser.yml` to eliminate redundant bullet points and improve commit categorization.
- **Improved Grouping**: Commits using the `refactor:` prefix are now correctly grouped under the **Improvements** section.
- **Documentation Visibility**: Re-enabled the **Documentation** section in release notes by refining the global exclusion filters.
- **Changelog Hygiene**: Cleaned up the `chore:` exclusion list to only target release-specific commits.

## [1.37.1] - 2026-03-30

### 🐛 Bug Fixes

- **Nested Vendor Sync**: Resolved a bug where a nested `vendor/vendor` directory was created when fallback synchronization was triggered during project bootstrap.
- **Improved Directory Detection**: Significantly enhanced the `sync` command's directory detection logic to correctly handle `--path` arguments even if trailing slashes are omitted.

### 🔄 Refactors

- **Testing Infrastructure**: Exported internal bootstrap and synchronization types to facilitate robust unit testing across project packages.

## [1.37.0] - 2026-03-30

### ✨ New Features

- **Automated Upgrade Pipelines**: Introduced dedicated upgrade workflows for Magento, WordPress, Laravel, and Symfony, enabling seamless version transitions within the environment.
- **Project Sync Status Tracking**: Implemented persistent tracking for synchronization status, providing better visibility into out-of-date remote data.
- **Improved Project Resolution**: Refactored internal project path resolution logic to handle complex symlinked and non-standard directory structures more reliably.
- **Composer Cache Configuration**: Added the `COMPOSER_CACHE_DIR` environment variable to all base configurations, ensuring optimized dependency persistence across container rebuilds.

### 🛠 Improvements

- **Blueprint Lifecycle**: Incremented internal `BlueprintVersion` to 1.27, triggering automatic environment re-renders to ensure all projects receive the latest configuration optimizations.

## [1.36.0] - 2026-03-29

### ✨ New Features

- **Desktop Auto-Update Notifications**: Implemented a comprehensive notification system for the desktop application to alert users when a new version is available.
- **Project Deletion**: Added the ability to delete projects directly from the Desktop UI with safety confirmation and backend cleanup.
- **Visual Sync Progress**: Introduced a dedicated UI module for remote synchronization with real-time progress bars and a live terminal console.
- **OS Terminal Support**: Integrated support for launching native OS terminals for service shells and remote operations.
- **Git Branch Visibility**: The dashboard now displays the active Git branch for each project in the environment details.
- **Desktop Doctor**: Added a `doctor` command to the Desktop app for automated troubleshooting and health checks.

### 🛠 Improvements

- **Project Selection Flow**: Refined the onboarding and project selection experience for better clarity and speed.
- **Sync Log Throttling**: Implemented event throttling for synchronization logs to improve UI responsiveness during high-volume data transfers.
- **Installer Compatibility**: Enhanced the system installer to support a wider range of Linux distributions and environment configurations.
- **Production Lock ID**: Switched the Desktop application's single-instance lock to a stable production identifier.

### 🔄 Refactors

- **Remote Configuration Management**: Implemented a new `RemoteConfigMap` type for deterministic YAML marshaling and priority-based sorting of remote environments.
- **UI Style Consolidation**: Removed redundant Tailwind CSS base styles and reset configurations, shifting to a more flexible and lightweight vanilla CSS architecture.
- **Project Name Normalization**: Standardized internal project naming conventions to ensure consistency across CLI and Desktop modules.

## [1.35.0] - 2026-03-28

### ✨ New Features

- **Real-time Desktop Logs**: Re-engineered the desktop log streaming engine to support carriage returns (`\r`). This enables real-time progress bar animations and spinner updates in the Desktop UI, providing immediate visual feedback for long-running synchronization tasks.
- **Enhanced Database Probing**: Extended the remote environment probe to support `MYSQL_` prefixed environment variables in `.env` files. This improves compatibility with Symfony, ensuring seamless database credentials resolution.

### 🛠 Improvements

- **Optimized Environment Actions**: The Desktop dashboard now automatically includes `--force-recreate` and `--remove-orphans` flags for all "Start" and "Restart" actions, ensuring a clean and predictable environment state.
- **Streamlined Sync Workflow**: Defaulted all automated "Pull" and "Sync" actions to use the `--yes` flag to bypass interactive prompts in background tasks. Removed the redundant "Assume Yes" option from the UI modals for a cleaner experience.

## [1.34.1] - 2026-03-28

### 🐛 Bug Fixes

- **Desktop Auto-Restart**: Fixed an issue where the desktop application failed to restart seamlessly after an update on Linux due to the `SingleInstanceLock`. The app now correctly yields the lock and relaunches via `gtk-launch` to preserve dock integration.
- **Update UI Alignment**: Fixed the flex alignment of the "Installing..." button in the desktop settings pane.

## [1.34.0] - 2026-03-28

### ✨ New Features

- **Store Domain Management**: Enhanced the `domain list` command with sorted tables and clear distinction between primary and extra domains for multi-store Magento environments.
- **Improved Onboarding UX**: Added explicit framework version selection (e.g., Magento 2.4.7, Laravel 11) to the project onboarding flow, ensuring the stack is correctly tuned from the start.

### 🛠 Improvements

- **Desktop Stability**: Implemented automatic panic recovery for desktop bridge proxies, preventing the application from crashing due to unexpected backend errors.
- **Image Fallback Engineering**: Refactored the local image fallback logic into the core engine, improving the reliability of environment startups in offline or air-gapped scenarios.

### 🔄 Refactors

- **Unified Synchronization Options**: Consolidated the legacy `sanitize` and `excludeLogs` options into a single, high-performance `--no-noise` flag for both CLI and Desktop sync operations. This simplifies the interface while providing more robust data protection and smaller transfer sizes.


## [1.33.0] - 2026-03-27

### ✨ New Features

- **Smart Data Synchronization**: Introduced the `--no-noise` flag for `bootstrap` and `sync` commands. It automatically excludes ephemeral data (caches, logs, sessions, and media thumbnails) for Magento, Laravel, and WordPress, drastically reducing transfer volume.
- **Centralized Credential Management**: Transitioned to using the global `~/.composer/auth.json` on the host, eliminating project-level `auth.json` files and stopping automatic `.gitignore` modifications for better security and hygiene.
- **Advanced Debugging**: Integrated Xdebug routing for Apache environments and added Varnish bypass logic for active debug sessions.

### 🛠 Improvements

- **Database Stream Compression**: Implemented automatic `gzip` streaming for all remote database transfers. This reduces network bandwidth usage by 80-90% during synchronization projects.
- **High-Performance SQL Sanitization**: Refactored the SQL sanitization engine using fast-path string matching, significantly reducing CPU overhead and memory usage during database imports.
- **SSH Transfer Tuning**: Standardized on the `aes128-ctr` cipher for remote operations and disabled redundant SSH-level compression to maximize throughput on high-speed networks.
- **Intelligent Progress Tracking**: Enhanced database import progress bars to distinguish between compressed file sizes and logical database volume for more accurate ETAs.

### 🐛 Bug Fixes

- **Stream Integrity**: Resolved edge cases involving data corruption during piped database dumps by ensuring atomic stream handling and proper gzip detection.

## [1.32.0] - 2026-03-27

### ✨ New Features

- **Automated Remote Selection**: `bootstrap` and `sync` commands now automatically prioritize `staging` then `dev` environments if not specified.
- **Blueprint Inspection**: New `blueprint` command for enhanced environment fingerprinting and template review.
- **Lock UX Enhancements**: Added `lock diff` command and `--update-lock` flag to `env up` for easier dependency tracking.
- **Compose Hygiene**: Introduced background cleanup for stale Docker Compose files and a new `env cleanup` command to manage directory saturation.

### 🛠 Improvements

- **Intelligent Update Notifier**: Refined update checks to suppress redundant warnings for development and pre-release builds.
- **Doctor Diagnostics**: Enhanced `govard doctor` with new checks for Docker Compose storage health.

### 🐛 Bug Fixes

- **CLI Stability**: Resolved various minor edge cases in command execution and internal configuration layering.

## [1.31.0] - 2026-03-27

### ✨ New Features

- **Comprehensive Web Server Support**: Introduced new Nginx and Apache templates with improved asset management and framework bootstrapping.
- **OpenMage Support**: Added dedicated support for the OpenMage framework and adjusted Magento 1 media paths.
- **Magento Cron Support**: Added `cronie` and `crond` to the PHP container for automated cron task execution.

### 🛠 Improvements

- **Nginx Backend Resolution**: Standardized PHP backend resolution and enhanced Varnish support across all web server modes.
- **Dashboard UI Refinement**: Modernized the desktop dashboard environment list using CSS Grid and optimized color contrast for status indicators.
- **HTTPS & TLS Enhancements**: Improved HTTPS detection for Magento 1 and added configurable TLS policies for local development domains.

### 🐛 Bug Fixes

- **Header Cleanup**: Removed redundant `X-Forwarded-Proto` directives for cleaner protocol detection.
- **Test Stability**: Enhanced integration test coverage and adjusted assertions for blueprint content verification.

## [1.30.0] - 2026-03-26

### ✨ New Features

- **Command Aliases**: Introduced command aliases and shortcuts for improved CLI usability, allowing users to run `up`, `down`, `ps`, and `logs` as top-level shortcuts.
- **Enhanced Service Management**: Added new flags and Portainer integration to the `svc` command for better observability of global services.

### 🛠 Improvements

- **DB Command Refactor**: Refactored `db` command flags for consistency. The `--no-pii` shorthand is now `-P`, and `--sanitize` is introduced as a `-S` alias.
- **Improved Help Output**: Enhanced help output by dynamically filtering compose flags and adding Govard-specific options.
- **Documentation Restructuring**: Restructured and consolidated documentation into broader topics for better navigation and clarity.
- **Framework Detection & Config**: Simplified framework detection logic and added profile-based config loading for more flexible environment setups.

## [1.29.0] - 2026-03-25

### ✨ New Features

- **Interactivity Control**: Introduced the `-y, --yes` flag for `bootstrap` and `sync` commands. In non-interactive environments (CI/CD), these commands now require the `--yes` flag to proceed, preventing unexpected hangs.
- **Improved Headers**: Redesigned all CLI headers with a bold, blue-boxed style and standardized vertical margins for better focus and readability.
- **Elasticsearch Alias**: Added the `opensearch` hostname alias to the `elasticsearch` service in blueprints to ensure backward compatibility for projects expecting the OpenSearch hostname.

### 🛠 Improvements

- **Bootstrap Flow**: Reordered the bootstrap execution flow to display the full synchronization plan review *before* starting environment containers, giving users a clear overview of the intended operations.
- **Sync Plan Visibility**: The synchronization plan review now explicitly lists endpoints, scopes (files, media, db), risk assessment, and detailed rsync/shell steps.
- **Sync Progress UI**: Integrated a new live-scrolling 10-line window for `rsync` progress during file and media synchronization, providing better real-time feedback without overwhelming the terminal.
- **Single File Sync**: Improved `--path` handling in `sync` to correctly distinguish between single files and directories, ensuring precise `rsync` behavior.
- **Non-Interactive Self-Update**: The `self-update` command now intelligently skips heavy system dependency checks when running in CI/non-interactive mode.

### 🐛 Bug Fixes

- **Integration Test Stability**: Resolved multiple test hangs in CI by enforcing the `--yes` flag and disabling terminal color (`NO_COLOR=1`) for consistent assertion matching.
- **Varnish CI Path Fix**: Corrected Varnish VCL path references in integration tests to align with the decentralized engine storage architecture.
- **Rsync Path Sanitization**: Correctly handles trailing slashes in sync operations to prevent duplicated subdirectories when syncing specific paths.

## [1.28.1] - 2026-03-24

### Fixed

- **SSH Key Mounting**: Fixed an issue where SSH keys from the host were missing in the container when a safe SSH configuration copy was being used.
- **Architectural Update**: Incremented internal blueprint version to ensure all project environments automatically receive the SSH mounting fix during the next `env up`.

## [1.28.0] - 2026-03-24

### Added

- **CLI Flag `--force-recreate`**: Added to `govard env up` and `govard svc up` commands, allowing users to force container recreation.
- **Apache Hybrid Mode Improvements**: Configured essential Apache modules (`mod_alias`, `mod_remoteip`, `mod_expires`, etc.) and added `X-Backend-Server: apache` header for easier debugging of hybrid environments.

### Improved

- **Global Nginx Proxy Support**: Updated all Nginx framework templates to include `fastcgi_param HTTPS 'on';`, ensuring correct HTTPS detection for all project types when running behind Govard's reverse proxy.
- **Service Management**: `govard svc up` now correctly parses additional arguments and always runs in detached mode (`-d`).
- **Magento 1 Bootstrap**: `RunMagento1SetConfigSQL` now sets `web/secure/offloader_header` to `X-Forwarded-Proto`, ensuring Magento 1 trusts the forwarded protocol header from Govard's proxy — a prerequisite for correct HTTPS detection.

## [1.27.0] - 2026-03-24

### Added

- **phpMyAdmin Database Access**: Database containers are now connected to the `govard-proxy` network, enabling phpMyAdmin (`pma.govard.test`) to directly reach all running project databases without additional configuration.
- **Magento 1 HTTPS Fix**: The `magento1.conf` Nginx template now includes `fastcgi_param HTTPS 'on';` so Magento 1 correctly identifies HTTPS requests behind Govard's Caddy reverse proxy, eliminating infinite redirect loops.
- **Composer Version Config**: Added `composer_version` field to `.govard.yml` stack config, allowing projects to pin a specific Composer version (e.g. `2.2`, `2`, `latest`).

### Improved

- **Auto Composer Downgrade**: Govard automatically selects Composer 2.2 LTS for projects running PHP < 7.2.5 when `composer_version` is not explicitly set, preventing plugin-blocking errors on legacy stacks.
- **DB Import Validation**: Improved `db import` command to correctly validate flag combinations (`--drop`, `--local`) and restrict incompatible options (`--no-noise`, `--no-pii`).

### Fixed

- **Self-Update CI Safety**: Avoided system dependency checks in non-interactive/CI environments to prevent test hangs and improve `TestSelfUpdateAutoConfirmViaEnv` reliability.

## [1.26.0] - 2026-03-24

### Added

- **Enhanced Magento 1 Support**: Dedicated bootstrap logic for Magento 1 / OpenMage with automated `local.xml` generation, admin user creation, and base URL configuration. Added support for `--no-noise` and `--no-pii` flags in Magento 1 database dumps.
- **Improved Mailpit Persistence**: Added a dedicated `mail_data` volume to the global Mailpit service and configured the `mail` network alias for reliable internal mail routing.

### Changed

- **Blueprint Architecture Standardizing**: Refactored blueprints to centralize shared service definitions (Redis, Varnish, RabbitMQ, etc.) into unified includes for better consistency.
- **Blueprint Version Update**: Updated internal blueprints to version 2 (V2 architecture) with optimized networking.

### Improved

- **Caddy Stability**: Enabled `--resume` flag for the global Caddy proxy, ensuring routes persist across container or host restarts.
- **Self-update Robustness**: Improved dependency installation checks and error handling in the `self-update` command.

### Fixed

- **Network Isolation**: Fully isolated PHP and Database networks from the global `govard-proxy` to resolve hostname conflicts and improve security.
- **Mail Connectivity**: Fixed mail routing issues by using `host-gateway` for the `mail` alias in all project environments.

## [1.25.0] - 2026-03-23

### Added

- **Optimized Self-Update**: The `govard self-update` command now includes automated system dependency checks (Linux-specific), post-update global service refreshes, and SSL trust verification, ensuring a consistent state after upgrades.
- **Debian Package Priority**: On Ubuntu/Debian, `self-update` now prioritizes using `.deb` packages for better dependency management and parity with the installation script.

### Fixed

- **Mail Server Connectivity**: Resolved an issue where PHP containers were isolated from the global Mailpit service, causing mail sending failures in projects like Magento 2.
- **Database Networking**: Added `govard-proxy` network to database services in blueprints for consistent phpMyAdmin access.
- **Magento 1 Credentials**: Restored the default database password in Magento 1 blueprints.

## [1.24.0] - 2026-03-23

### Added

- **Global HTTP Redirect**: Implemented a global 308 Permanent Redirect from port 80 to 443 in the Caddy proxy. All `.test` and `.govard.test` domains (including Mailpit and phpMyAdmin) now force HTTPS by default.
- **Zero-Config Installer**:
    - Automatic detection and installation of `libnss3-tools` (certutil) and `libwebkit2gtk-4.1-0` on Linux.
    - Post-installation hooks: Automatically starts global services (`svc up`) and configures SSL trust (`doctor trust`) for a "Green Lock" experience immediately after install.
    - Pipe compatibility: Optimized `install.sh` for `curl | bash` execution with `/dev/tty` redirection for interactive prompts.
- **WordPress Remote Support**: Added dedicated SSH-based database credential probing and site URL auto-correction for WordPress projects.
- **Framework Detection**: Added WordPress to the default list of auto-detected frameworks.

### Improved

- **Bootstrap Hygiene**: The `bootstrap` command now defaults to `--remove-orphans`, ensuring a clean environment state without requiring manual flags.
- **Package Integrity**: Elevated `libwebkit2gtk-4.1-0` and `libnss3-tools` to mandatory dependencies in the `.deb` package for seamless offline installation.
- **phpMyAdmin Reliability**: Switched to permanent directory-based mounting for the project registry, resolving "stale data" issues in phpMyAdmin.
- **Remote Shell Robustness**: Improved remote command execution with `bash -l` login shell invocation and `sh` fallback.
- **Database Tooling**: Added Gzip compression for `db dump` output and removed environment restrictions for the `--local` flag.

### Fixed

- **Installer Path Resolution**: Fixed a `BASH_SOURCE` edge case in `install.sh` when executing via pipes.
- **PHPMyAdmin Visibility**: Resolved inode-related search failures in phpMyAdmin by using more stable Docker volume configurations.

## [1.23.0] - 2026-03-23

### Added

- **Independent Monitoring Flags**: Separated `--no-noise` (`-N`) and `--no-pii` (`-S`) flags. They are now independent and can be used individually or together to fine-tune database synchronization and dumps.
- **Canonical Remote Name Display**: The CLI now consistently resolves and displays the canonical remote name (from `remotes` config) in all output messages, even when using aliases or environment names.

### Improved

- **Bootstrap & Sync Visibility**: Added clear, immediate `INFO` messages at the start of `bootstrap` and `sync` operations to provide instant feedback on the source, destination, and scope of the action.
- **Remote DB Dump Feedback**: Database dump operations now explicitly include the target remote file path in the success message for better traceability.

### Fixed

- **Remote Path Expansion**: Fixed an issue where paths starting with `~/` were not expanded on remote servers due to shell quoting. Introduced a `quoteRemotePath` helper to safely handle home-relative paths.
- **PHPMyAdmin Visibility**: Resolved a race condition where `pma.govard.test` would not show project databases after a fresh `bootstrap` or `env up`. Implemented an explicit active project refresh in the verification stage.
- **Remote Identity Resolution**: Refactored `ensureRemoteKnown` to consistently resolve and return the confirmed remote name across all CLI commands.

## [1.22.1] - 2026-03-21

### Added

- **Local Image Build Fallback**: Govard now automatically attempts to build missing Docker images from embedded blueprints if pulling fails.
- **Dependency-Aware Image Building**: Implemented a resolution algorithm for local image builds that correctly handles parent-child image dependencies.

### Changed

- **Command Refactoring**: Centralized Docker Compose execution logic in the `engine` package for better maintainability and consistency.
- **Standardized CLI Signature**: Unified `RunE` signatures and context handling across various command implementations.
- **Bootstrap Stability**: Improved Magento bootstrap reliability by clearing generated code and simplifying autoloader generation.
- **Log Management**: Refined log tailing and follow logic in `env logs` commands for more predictable behavior.

### Fixed

- **Composer Workflow**: Removed the `-o` (optimize) flag from `composer dump-autoload` in development flows to align with development best practices and resolve test failures.
- **Documentation Paths**: Corrected documentation links and existence checks in test suites.

## [1.22.0] - 2026-03-20

### Added

- **Snapshot Compression**: Database snapshots are now automatically compressed using Gzip, reducing disk usage by 70-90%.
- **Enhanced Tunnel Management**:
  - Added `tunnel stop` and `tunnel status` commands.
  - Automatic base URL update/revert for all frameworks when a tunnel starts or stops.
- **Database Operations**:
  - New `db top` command for real-time database process monitoring.
  - Real-time progress bar for database imports (`db import`, `bootstrap`) and `sync` operations.
- **Project Testing Integration**:
  - New `test` command with subcommands for `phpunit`, `phpstan`, `mftf`, and `integration` tests.
  - Standardized container execution and user resolution across all test types.
- **Log Service Filtering**: `govard env logs` now supports an optional `<service>` argument to stream logs from a specific service only.
- **Capability Scopes**: Added `cache` capability for remote environments to explicitly allow/deny Redis operations.

### Changed

- **Redis Command Refactoring**: Migrated `redis` command to a structured subcommand pattern: `redis cli`, `redis flush`, and `redis info`.
  - Added support for both Redis and Valkey providers.
  - Added remote execution support for Redis commands via SSH.
- **Debug Command Refactoring**: Migrated `debug` command to a structured subcommand pattern: `debug on`, `debug off`, `debug status`, and `debug shell` (inspired by Warden).

### Fixed

- **Proxy Networking**: Remedied an issue where framework-specific compose overrides (such as `magento2/services.yml`) accidentally detached the `web` service from the `govard-proxy` network, resulting in 502 Bad Gateway errors.
- **Undefined References**: Fixed multiple lint errors and unreachable code in `bootstrap`, `debug`, and `open` commands by exporting and unifying container execution helpers.

## [1.21.1] - 2026-03-20

### Fixed

- **Visual Assets**: Updated the desktop application icon to use the new branding logo in the Linux package.

## [1.21.0] - 2026-03-20

### Added

- **Sync Data Obfuscation Flags**: Implemented `--no-noise` (`-N`) and `--no-pii` (`-S`) flags for `govard sync` and `govard bootstrap`.
  - `--no-noise`: Excludes ephemeral/noise tables (cron schedules, caches, sessions, logs) from `mysqldump`.
  - `--no-pii`: Excludes sensitive/PII tables (customers, orders, passwords, etc.) and implies `--no-noise`.
  - Supports framework-specific table lists: Magento 2, Laravel, and WordPress.
- **Sync Shortcut Flags**: Added `-s` / `-d` as short aliases for `--source` / `--destination` in `govard sync`.
- **Smart Remote Name Resolution**: `--source`, `--destination` (sync) and `--environment` (bootstrap) now resolve remote name aliases automatically (e.g. `-s dev` → matches a remote named `development`, or any remote whose name normalizes to `dev`).
- **SSH Agent Diagnostics**: `govard doctor` now checks SSH agent connectivity and reports the number of loaded keys with remediation guidance.
- **Secure SSH Config Mounting**: SSH config files with overly broad permissions are now copied to a safe temporary file with restricted permissions (`0600`) before mounting into containers, preventing SSH warnings in remote operations.
- **Magento Search Auto-fix**: Bootstrap now automatically configures the search engine host and port (`es`, `opensearch`) from container labels, and unblocks read-only Elasticsearch/OpenSearch indices to prevent post-install search failures.

### Changed

- **Sync Endpoint Display**: Removed redundant "environment" field from remote/sync endpoint summaries. Environment context is now derived from the remote name itself.
- **Bootstrap Examples Updated**: Updated `-e` flag description in help text to clarify it accepts remote name aliases.

### Fixed

- **GoReleaser Changelog Grouping**: Refined changelog group configuration in `.goreleaser.yml` for cleaner release notes.

## [1.20.0] - 2026-03-19


### Added

- **Enhanced Database Dump Flags**: Replaced `--exclude-sensitive-data` with `--no-noise` (`-N`) and `--no-pii` (`-S`) flags for `govard db dump`.
  - `--no-noise`: Excludes ephemeral tables (cron, cache, session, logs, etc.).
  - `--no-pii`: Excludes PII/sensitive tables (customers, orders, etc.) and implies `--no-noise`.
- **Locale Auto-detection**: Implemented automatic locale detection for `govard deploy` in Magento 2 projects when the `--locales` flag is omitted.
- **Magento 1 / OpenMage Improvements**:
  - Secure bootstrap with randomly generated 32-hex crypt keys in `local.xml`.
  - Automated post-clone setup (base URL configuration and admin user creation).
  - Remote database credential probing from `local.xml` via SSH.
- **Desktop Application Enhancements**:
  - New theme support with **Dark Mode**.
  - Added "Run in Background" preference and standard Quit functionality.
  - Refactored service action buttons to be icon-only with detailed tooltips.
- **Comprehensive Documentation**: Added dedicated documentation files for all supported frameworks (Laravel, Symfony, Drupal, WordPress, Shopware, CakePHP, Next.js).
- **SVG Logo**: Updated project logo to a modern SVG format.

### Changed

- **Go Version Upgrade**: Updated project Go requirement to **1.25.0**.
- **Linter Evolution**: Upgraded `golangci-lint` to **v2.11.3** and modernized configuration.
- **Refined Documentation**: Major updates to `README.md` and CLI command references.

### Improved

- **Database Reliability**: Enhanced local DB operations with larger max packet size and foreign key check safeguards.
- **Remote Stability**: Improved SSH connection handling and remote probe accuracy.

### Fixed

- **Elasticsearch Safety**: Fixed "read-only" index block issues during post-install operations.
- **Code Quality**: Fixed various linter warnings and improved code structure across the bootstrap engine.

## [1.19.0] - 2026-03-04

### Added

- **Environment Pull**: Introduced `govard env pull` command to pull Docker images for the current environment.

### Changed

- **Database Proxy**: Refactored `open db` and generic database access to support a shared containerized proxy, improving connectivity and client-url resolution.

### Testing

- **Desktop & Integration**: Added comprehensive integration tests for environment compose flows, database client URL resolution, and PMA proxy configurations.
- **Frontend**: Expanded unit tests for dashboard actions and global services modules.

## [1.18.0] - 2026-03-04

### Fixed

- **Updater**: Unified desktop and self-update binary handling and hardened permissions.
- **Update Targets**: Refined desktop update target resolution to prefer sibling binaries and support explicit environment overrides.

## [1.17.0] - 2026-03-04

### Added

- **Service Start/Restart Flow**: Reworked global service start/restart flow to include proxy readiness, route registration, and enhanced UI feedback with message summarization.

### Improved

- **UI Feedback**: Improved user feedback during service lifecycle operations with summarized messages and real-time status updates.
- **Service Stability**: Enhanced reliability of service startup by ensuring proxy readiness before route registration.

### Testing

- **Backend Tests**: Added comprehensive integration tests for desktop global services and service startup logic.
- **Frontend Tests**: Added core tests for global services frontend module.
- **Mocking**: Introduced additional test helpers for mocking backend services in tests.

## [1.16.1] - 2026-03-04

### Added

- **Update Formatting**: Unified update message formatting across the desktop app.

### Improved

- **Notifier UX**: Synchronized update notifier visibility with the settings drawer to avoid UI overlapping.

### Fixed

- **Redundant Config**: Removed unnecessary Darwin build configuration from GoReleaser.

## [1.16.0] - 2026-03-04

### Added

- **Self-Update Logic**: Implemented desktop application self-update functionality.
- **Log Management**: Introduced desktop log export and management features.
- **Caddy Stability**: Implemented Caddy command retry logic for better proxy reliability.

### Improved

- **UI Responsiveness**: Improved desktop UI layout responsiveness and log display for better readability.
- **Diagnostics**: Added additional test helpers and integration tests for desktop services and update logic.

## [1.15.0] - 2026-03-04

### Added

- **Git Project Onboarding**: Introduced the ability to clone Git repositories directly during the project onboarding process.
- **Onboarding UI Enhancements**: Added support for Git URL, branch selection, and progress tracking for repository cloning.
- **Terminal Integration Improvements**: Enhanced terminal output and progress monitoring for long-running onboarding operations.

### Improved

- **Testing Infrastructure**: Expanded integration and frontend tests to cover Git cloning and complex onboarding flows.
- **Desktop UI**: Refined onboarding styles and logic for a smoother repository setup experience.

### Fixed

- **Remotes Cleaning**: Fixed issues where some remote environment references were not correctly cleaned up in the UI.

## [1.14.1] - 2026-03-04

### Improved

- **Linux App Icons**: Optimized SVG logo size and distributed multiple hicolor icon sizes (16x16 up to 256x256) in the Debian package to ensure correct visual weight and visibility in the Ubuntu launcher.

## [1.14.0] - 2026-03-03

### Added

- **Desktop UI Revamp**: Major overhaul of global services controls and header UX for a more premium experience.
- **Modular Controllers**: Introduced `global-services.js` for better state management and dynamic service status handling.
- **Enhanced Bridge Proxies**: Improved backend-to-frontend communication for workspace-wide services.

## [1.13.0] - 2026-03-03

### Added

- **Local Image Fallback**: Introduced the `--fallback-local-build` flag for `up`, `svc up`, and `svc restart` commands. This allows Govard to automatically build missing ddtcorex/govard images locally from embedded blueprints if they cannot be pulled from Docker Hub, ensuring environments can start even without internet access or registry availability.

### Improved

- **`open db` Command UX**: Updated the default behavior of `govard open db` to launch phpMyAdmin (PMA) in the browser for a more immediate visual experience. A new `--client` flag was added to explicitly launch external database client protocols (e.g., `mysql://`).

### Quality & Testing

- **Additional Test Gates**: Added comprehensive tests for image reference parsing, local build spec resolution, and command-level flag existence to maintain high stability.

## [1.12.0] - 2026-03-03

### Added

- **Sync Presets Remote Capabilities**: Added support for remote synchronization presets, enhancing cross-environment workflow flexibility.
- **SSL Auto-Trust**: Automated root CA trust for `svc` lifecycle commands, simplifying SSL management on Linux systems.

### Improved

- **Log Stream Sanitization**: Significantly improved terminal output reliability by stripping ANSI escape codes, control characters, and invalid UTF-8 from log streams.
- **Desktop Stability**: Refined "open" actions and default configurations for a more consistent desktop experience.
- **Sync Options**: Refined synchronization options for better control over remote environment updates.

### Fixed

- **ANSI Fragment Cleaning**: Resolved issues with trailing or orphan ANSI fragments disrupting stream output.

### Quality & Documentation

- **Test Coverage**: Expanded command runtime coverage and refreshed the integration test suite.
- **Project Documentation**: Updated README to highlight remote management features and core differentiators.

## [1.11.0] - 2026-03-02

### Added

- **Embedded Frontend Assets**: The desktop application now embeds all frontend assets (CSS, JS, Fonts) directly into the binary, enabling standalone distribution and simplified packaging.
- **Terminal Integration**: Integrated Terminal with backend PTY support for a full interactive shell experience directly within the desktop application.
- **Debian Packaging Support**: Official support for generating `.deb` packages for Linux distributions, complete with application menu integration and icons.

### Improved

- **Asset Resolution**: Enhanced the path resolution logic to automatically fall back to embedded assets when running in production/installed mode.
- **Development Environment Setup**: Significant documentation updates for local toolchain installation and prerequisites.
- **Frontend Bundle Consolidation**: Merged third-party CSS (Inter, Material Symbols, Terminal) into a single optimized bundle for faster loading.

### Fixed

- **Production Asset Loading**: Fixed a critical issue where the desktop application failed to locate assets after being installed via package managers.

## [1.10.0] - 2026-03-02

### Added

- **Tailwind CSS Integration**: Fully migrated the frontend to Tailwind CSS for improved maintainability and modern aesthetics.
- **Shared Reference Architecture**: Implemented a shared `refs` object across all frontend controllers, ensuring resilient DOM binding and immediate UI updates for dynamically injected elements.
- **Tab Selection Persistence**: The desktop app now preserves the active tab (e.g., Logs & Shell) when switching between environments.

### Fixed

- **Terminal Mounting**: Resolved the "Terminal requires a parent element" error by standardizing controller initialization with live DOM references.
- **Log Service Filtering**: Corrected state management in `main.js` to ensure log output is correctly filtered for the selected service.
- **Process Management**: Improved development server stability by adding robust cleanup logic for redundant `wails` and `govard` instances.

### Improved

- **Testing Infrastructure**: Expanded integration test suite for environment selection and log retrieval.
- **Build Automation**: Enhanced release workflows for multi-platform distribution.

## [1.9.0] - 2026-03-01

### Added

- **Portainer Integration**: Portainer is now available as a new global service, integrated with the `svc` and `open` commands.
- **Streaming Toasts**: Introduced streaming toasts and enhanced desktop UI messages for improved clarity and user feedback.

### Changed

- **Remote Environments**: Removed direct remote connection management from the UI and backend, adding a plan-only bootstrap option instead.
- **System Improvements**: Various system improvements and technical debt reduction.

## [1.8.0] - 2026-02-28

### Added

- **Interactive SSH Sessions**: `govard open shell` now supports fully interactive terminal handoff for remote environments using `syscall.Exec`.
- **Image Pulling Support**: New `govard env pull` command and added `--pull` / `--remove-orphans` flags to `govard env up`.
- **Search Engine Health**: Automatic detection and resolution for Elasticsearch/OpenSearch "read-only" index blocks caused by low disk space.
- **Environment Scopes (Profiles)**: Run isolated environment variants with `--profile <name>`. Config layers merge as Base → Profile (`.govard.<profile>.yml`) → Local (`.govard.local.yml`). Each profile gets its own Docker Compose file and database volumes for full isolation.
- **Network Isolation Mode**: Set `isolated: true` in `features` to prevent containers from reaching the internet.
- **MFTF & Selenium Support**: Set `mftf: true` in `features` to auto-start a Selenium Standalone Chrome container.
- **Frontend LiveReload / Watcher**: Set `livereload: true` in `features` to start a dedicated Node.js watcher container.
- **`govard open mftf`**: New target to open the Selenium VNC viewer in-browser.

### Improved

- **Remote Environments**: Added support for deriving environment configurations from name and making protected status optional.
- **Embedded Terminal**: Consistently handle sync operations and interactive commands within the UI.

## [1.7.0] - 2026-02-27

### Added

- **dnsmasq Service**: Built-in `dnsmasq` service for automatic `.test` domain resolution.
- **Interactive Recipe Selection**: Prompt for recipe (framework) selection during `init` if detection fails.
- **Restructured CLI**: Organized environment commands under a unified `env` subcommand (`govard env up`, `govard env stop`, etc.).
- **Multi-domain Support**: Enhanced support for multiple domains per project.

### Changed

- **Standardized Terminology**: Consistent use of "recipe" and "framework" across CLI and documentation.
- **Bootstrap Logic**: Refined to auto-clone if no source is present and improved remote connectivity tests.
- **Flag Renames**:
  - Renamed `--version` to `--framework-version` in `bootstrap`.
  - Renamed `--framework` to `--recipe` in `profile`.

### Fixed

- **Bootstrap Remote Sync**: Fixed issues with remote synchronization when source already exists.
- **CI Reliability**: Fixed potential CI recursion issues in bootstrap flows.

### Improved

- **Makefile**: Added `fmt-check` for better code quality control.
- **Proxy Naming**: Standardized proxy container naming for better visibility.

## [1.6.0] - 2026-02-26

### Added

- **Composer Cache**: Share the host's Composer cache directory with PHP containers to vastly speed up dependency installation.
- **Log Tailing**: Added `--tail` flag to `govard env logs` and `govard svc logs` for better control over log output.
- **Snapshot Management**: New `snapshot export` and `snapshot delete` subcommands.
- **CI Support**: Added `--no-tty` flag to `govard shell` for non-interactive environments.
- **Docker Images**: Use a non-alpine base image for `varnish:6.0` to resolve libc compatibility issues.
- **Route Revival**: Automatically re-registers domains for all running project containers when global services (`svc`) start or restart.

### Changed

- **Bootstrap Command**: `--clone` is now disabled by default. Running `govard bootstrap` will import DB/media and start containers without downloading the remote source. Pass `--clone` to override.
- **Docker Prefix**: Updated default Docker image prefix to `ddtcorex/govard-`.
- **PHP Redis**: Pinned Redis extension version specifically for PHP 7.1, 7.2, & 7.3 to resolve compilation failures.

### Fixed

- **Docker Error Messages**: Made Docker daemon connection errors significantly clearer and easier to diagnose.
- **Proxy Stability**: Improved `govard svc up` to handle stopped proxy containers and provide better port conflict diagnostics.
- **Configuration**: Support for dot notation (nested keys) in `govard config set` (e.g., `stack.php_version`).
- **SSL Trust**: Fixed diagnostics and instructions for Linux system trust store.

## [1.5.0] - 2026-02-25

### Added

- **Database Commands**: Added `db query` and `db info` commands for easier direct database interaction.
- **Enhanced Integration Tests**: Comprehensive realenv integration tests for bootstrap, sync, db, and open commands.
- **Improved Warden Migration**: Support for modern remotes and stack versions.

### Changed

- **Docker Organization**: Introduced `DOCKER_ORG` variable for better flexibility in image naming.
- **Help Documentation**: Major refactor of help commands with detailed examples and case studies.

## [1.4.0] - 2026-02-25

### Added

- **Migration Suite**: New automatic migration from DDEV and Warden environments during `govard init`.
- **Embedded Blueprints**: Blueprints are now embedded directly into the binary, enabling standalone installation without external dependencies.

### Changed

- **Rename Configuration File**: Standardized on `.govard.yml` (previously `govard.yml`) for consistent hidden file convention.

### Fixed

- **Docker Build Stability**: Enhanced PHP Dockerfiles with version-specific capping for PECL extensions (`redis`, `imagick`, `amqp`) and standard shell compatibility for older PHP versions (7.1, 7.2, 7.3).
- **Integration Test Reliability**: Synchronized test suite with embedded blueprints and updated file naming expectations.

## [1.3.0] - 2026-02-24

### Added

- **Local Snapshots**: Introduced the `snapshot` command to create, list, and restore database and media snapshots for rapid environment switching.
- **Public Tunnels**: New `tunnel` command with Cloudflare Tunnel integration to securely expose local projects to the internet.
- **Project Extensions**: Support for project-specific custom commands and extensions in `.govard/commands`.
- **Enhanced SSL Trust**: Automated root CA management and browser trust via `govard doctor trust`.
- **Deployment & Sync**: Improved `deploy` and `sync` commands for better remote environment orchestration.
- **Observability**: New events tracking for better CLI audit and telemetry.

### Changed

- **Proxy Architecture**: Significant refactor of Caddy route management for better performance and flexibility.
- **Framework Discovery**: Refined detection logic and runtime profiles for Magento 2, Laravel, Symfony, and WordPress.
- **CLI Robustness**: Added comprehensive input validation and clearer error messaging across all commands.

### Fixed

- Improved handling of `sudo` requirements for certificate installation.
- Fixed numerous edge cases in Docker Compose template rendering and service dependency management.
- Corrected various stability issues in remote environment synchronization.

### Quality & Testing

- **Massive Test Suite Expansion**: Added over 50,000 lines of unit, integration, and frontend tests.
- **Automated Quality Gates**: Integrated comprehensive coverage analysis and enhanced CI pipelines.

## [1.2.0] - 2026-02-24

### Added

- **Centralized Docker Image Management**: Introduced custom Elasticsearch and OpenSearch Docker images optimized for Magento 2.
- **Extended Magento 2 Support**: Enhanced Magento directory setup and refactored database import functions.
- **`tool` Subcommand**: New command for runtime wrappers to execute framework-specific CLI tools within project containers.
- **Desktop App Transformation**:
  - Full UI redesign with updated styling and modular dashboard.
  - Added Log viewer, Remote management, and Onboarding modules.
  - Integrated Remote Shell and Sync-plan wiring.
  - Redesigned Toast notification system with message deduplication logic.

### Changed

- **Command Architecture**: Restructured command groups and refactored `config` and `bootstrap` for consistency.
- **Desktop Layout**: Migrated from embedded Mailpit to a dedicated project workspace layout.
- **Docker Efficiency**: Optimized container listing performance for port availability checks.

### Fixed

- Fixed YAML syntax error in Elasticsearch blueprint.
- Corrected test expectations and CLI completion defaults.

## [1.1.2] - 2026-02-20

- Patch release with minor stability fixes.

## [1.1.0] - 2026-02-20

- Added initial Desktop app framework support.
- Enhanced framework discovery for Laravel and Next.js.

## [1.0.0] - 2026-02-20

- Initial professional-grade release of Govard.
