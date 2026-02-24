# Govard Documentation

Welcome to the Govard documentation. This directory is organized by audience and purpose.

## Quick Navigation

### For Users
- **[Getting Started](user/getting-started.md)** - Install, init, up, and basic workflow
- **[Configuration](user/configuration.md)** - `.govard.yml` and blueprints
- **[CLI Commands](user/commands.md)** - Complete command reference
- **[SSL & HTTPS](user/ssl-https.md)** - Local HTTPS setup

### For Framework Users
- **[Magento 1 (OpenMage)](frameworks/magento1.md)** - Magento 1/OpenMage support
- **[Magento 2](frameworks/magento2.md)** - Magento-specific features and auto-configuration
- **[Framework Support Matrix](frameworks/support-matrix.md)** - Runtime defaults and version-specific profile overrides
- **[Custom](frameworks/custom.md)** - Build your own service mix

### For Developers
- **[Architecture](dev/architecture.md)** - System design and technical specifications
- **[Contributing](dev/contributing.md)** - Build, test, and development workflow
- **[Desktop](desktop/README.md)** - Desktop app roadmap and integration notes

### Product Planning
- **[Support Matrix](product/support-matrix.md)** - Supported platforms, toolchain, and framework coverage
- **[Config Contract](product/config-contract.md)** - Layering and ownership rules for Govard config
- **[Persona Journeys](product/persona-journeys.md)** - Developer, PM, and tester usage paths

---

## What is Govard?

Govard is a professional-grade local development orchestrator built in Go. It replaces legacy bash-based tools with a high-performance, native binary that manages complex containerized environments.

**Key Features:**
- Automatic framework detection (Magento 1/OpenMage, Magento 2, Laravel, Next.js, Drupal, Symfony, Shopware, CakePHP, WordPress)
- One-command environment setup with `govard env up`
- Built-in HTTPS support via Caddy
- Xdebug integration
- Dedicated `php-debug` container with cookie-based routing
- Optional RabbitMQ support
- Multi-platform support (macOS, Linux, Windows)

---

## Getting Started (TL;DR)

```bash
# 1. Install Govard
git clone https://github.com/ddtcorex/govard.git
cd govard
make install

# 2. Initialize your project
cd /path/to/your/project
govard init

# 3. Start the environment
govard env up

# 4. Access your site
# Open https://your-project.test in browser
```

For detailed instructions, see [Getting Started](user/getting-started.md).
