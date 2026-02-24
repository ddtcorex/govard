# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
