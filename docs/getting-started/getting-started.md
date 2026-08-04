---
title: Getting Started with Govard
description: The shortest path from installing Govard to running your first local project — initialize, detect framework, and bring your environment up.
---

# Getting Started

This guide walks you through the shortest path from installation to a working local Govard project.

---

## 1. Initialize a Project

Navigate to your project root and run:

```bash
cd /path/to/your/project
govard init
```

Govard inspects `composer.json` or `package.json`, detects the framework, and writes `.govard.yml`.

### Detected Frameworks

| Framework | Detection |
| :--- | :--- |
| Magento 2 | `composer.json` with `magento/magento2-base` |
| Mage-OS | `composer.json` with `mage-os/product-community-edition` or `mage-os/project-community-edition` |
| Magento 1 / OpenMage | `composer.json` patterns |
| Laravel | `artisan` file + `composer.json` |
| Next.js | `package.json` with `next` dependency |
| Emdash | Emdash project markers |
| Drupal | `composer.json` with `drupal/core` |
| Symfony | `symfony/framework-bundle` |
| Shopware | `shopware/core` |
| CakePHP | `cakephp/cakephp` |
| PrestaShop | `config/defines.inc.php` |
| Django | `manage.py` |
| WordPress | `wp-config.php` or `wp-login.php` |
| Custom | Interactive stack picker (`govard init --framework custom`) |

### Force a Specific Framework

```bash
govard init --framework magento2
govard init --framework custom
```

### Migrate from other tools

If you are transitioning from Warden or DDEV, Govard can automatically detect your setup and seamlessly copy your local database volume so you don't lose any data:

```bash
govard init --migrate-from warden
govard init --migrate-from ddev
```

---

## 2. Start the Environment

```bash
govard env up
```

This renders a per-project compose file under `~/.govard/compose/` and starts your specialized stack.

### Common Variants

```bash
govard up --quickstart           # Alias: govard env up
govard env up --pull             # Pull latest images first
govard env up --fallback-local-build  # Build images locally if pull fails
```

### Startup Pipeline

1. Detect framework context
2. Validate config, Docker, ports, and prerequisites
3. Render compose file into `~/.govard/compose/`
4. Start containers in detached mode
5. Verify proxy and host wiring

### Root Shortcuts

| Shortcut | Equivalent |
| :--- | :--- |
| `govard up` | `govard env up` |
| `govard down` | `govard env down` |
| `govard restart` | `govard env restart` |
| `govard ps` | `govard env ps` |
| `govard logs` | `govard env logs` |

---

## 3. Configure the App

### Magento 2 Projects

Auto-inject container settings into `app/etc/env.php`:

```bash
govard config auto
```

### View Current Config

```bash
govard config get php_version
govard config get stack.services.db
```

---

## 4. Enter the Workspace

```bash
govard shell
```

- **PHP frameworks** (Magento, Laravel, etc.): opens the `php` container at `/var/www/html`
- **Node-first frameworks** (Next.js, Emdash): opens the `web` container at `/app`

---

## 5. Access Your App and Global Services

Govard routes project domains through the shared Caddy proxy and provides built-in services for development:

### Project URLs

| Target | URL | Command |
| :--- | :--- | :--- |
| App URL | `https://<project>.test` | Open in browser |
| Admin panel | `https://<project>.test/admin` | `govard open admin` |
| Mail testing | `https://mail.govard.test` | `govard open mail` |
| Database admin | `https://pma.govard.test` | `govard open db` |
| Docker management | `https://portainer.govard.test` | `govard open portainer` |

### Global Services Credentials

| Service | URL | Credentials |
| :--- | :--- | :--- |
| Portainer | `https://portainer.govard.test` | `admin` / `AdminGovard123$` |
| PHPMyAdmin | `https://pma.govard.test` | Use project's DB credentials |
| Mailpit | `https://mail.govard.test` | No login required |

### Quick Commands

```bash
# Open services
govard open mail     # Mailpit
govard open db       # PHPMyAdmin
govard open admin    # Project admin panel

# Manage global services
govard svc up        # Start all global services
govard svc down      # Stop all global services
govard svc ps        # View service status
```

---

## 🔁 Daily Workflow

```bash
# Start work
govard up

# Follow logs
govard logs php -f

# Toggle Xdebug
govard debug on

# Enter shell
govard shell

# Stop work
govard down
```

---

## 🌐 Bootstrap a Remote Clone

To clone an existing environment from a remote server:

```bash
govard bootstrap --clone -e staging --no-pii --no-noise
```

For a fresh framework installation:

```bash
govard bootstrap --framework magento2 --fresh --framework-version 2.4.9
govard env up
govard open admin
```

---

## 🩺 First Troubleshooting Checks

```bash
govard doctor
govard doctor trust   # Fix browser SSL warnings
```

If your browser shows HTTPS trust warnings after setup, run `govard doctor trust` to re-import the Root CA.

---

## 📋 What's Next

| Topic | Link |
| :--- | :--- |
| All CLI commands | [CLI Commands](/reference/cli-commands) |
| Configuration options | [Configuration](/reference/configuration) |
| Framework-specific notes | [Frameworks](/reference/frameworks) |
| SSL and DNS setup | [SSL and Domains](/workflows/ssl-and-domains) |
| Remote environments | [Remotes and Sync](/workflows/remotes-and-sync) |
| Desktop app | [Desktop App](/workflows/desktop-app) |

---

**[← Installation](/getting-started/installation)** | **[CLI Commands →](/reference/cli-commands)**
