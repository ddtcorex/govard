---
title: Supported Frameworks — Magento, Mage-OS, Laravel, Symfony, WordPress & More
description: Govard auto-detects Magento 1/2, Mage-OS, Laravel, Symfony, Drupal, Shopware, CakePHP, PrestaShop, WordPress, Next.js, and applies framework-specific runtime defaults.
---

# Frameworks

Govard detects supported frameworks and applies runtime defaults plus version-aware overrides.

---

## Support Matrix

| Framework | Auto-Detection | Version-Aware Profile | Default Web Root |
| :--- | :---: | :---: | :--- |
| Magento 2 | ✅ | ✅ | `/pub` |
| Mage-OS | ✅ | framework defaults | `/pub` |
| Magento 1 / OpenMage | ✅ | framework defaults | project root |
| Laravel | ✅ | ✅ | `/public` |
| Next.js | ✅ | framework defaults | project root |
| Emdash | ✅ | framework defaults | project root |
| Drupal | ✅ | ✅ | `/web` |
| Symfony | ✅ | ✅ | `/public` |
| Shopware | ✅ | framework defaults | `/public` |
| CakePHP | ✅ | framework defaults | `/webroot` |
| PrestaShop | ✅ | framework defaults | project root |
| WordPress | ✅ | ✅ | `/` |
| Django | ✅ | framework defaults | project root |
| Custom | manual | manual | project root |

---

## Runtime Defaults

| Framework | PHP | Node | Python | DB | Cache | Search | Queue |
| :--- | :---: | :---: | :---: | :--- | :--- | :--- | :--- |
| Magento 2 | 8.4 | 24 | — | mariadb 11.4 | valkey 8.0.0 | opensearch 2.19.0 | none |
| Mage-OS | 8.4 | 24 | — | mariadb 11.8 | redis 7.4 | opensearch 3.0 | none |
| Magento 1 / OpenMage | 8.1 | — | — | mariadb 10.11 | none | none | none |
| Laravel | 8.4 | — | — | mariadb 11.4 | none | none | none |
| Next.js | — | 24 | — | none | none | none | none |
| Emdash | — | 22 | — | none | none | none | none |
| Drupal | 8.4 | — | — | mariadb 11.4 | none | none | none |
| Symfony | 8.4 | — | — | mariadb 11.4 | none | none | none |
| Shopware | 8.4 | — | — | mariadb 11.4 | none | none | none |
| CakePHP | 8.4 | — | — | mariadb 11.4 | none | none | none |
| PrestaShop | 8.1 | — | — | mariadb 10.11 | none | none | none |
| WordPress | 8.3 | — | — | mariadb 11.4 | none | none | none |
| Django | — | — | 3.12 | postgres 16 | none | none | none |
| Custom | 8.4 | — | — | mariadb 11.4 | none | none | none |

`—` means Govard does not force a default for that stack component.

---

## Version-Aware Overrides

| Framework | Version | PHP Override | Other |
| :--- | :--- | :--- | :--- |
| Laravel | 10 | 8.2 | |
| Laravel | 11 | 8.3 | |
| Laravel | 12 | 8.4 | |
| Symfony | 6 | 8.2 | |
| Symfony | 7 | 8.3 | |
| Drupal | 10 | 8.3 | |
| Drupal | 11 | 8.4 | |
| WordPress | 6 | 8.3 | |
| Magento 2 | 2.4.9+ | 8.4 | MariaDB 11.4, Redis 7.2, OpenSearch 3.0.0, RabbitMQ 4.1.0 |
| Magento 2 | 2.4.8 | 8.4 | MariaDB 11.4, Redis 7.2, OpenSearch 2.19.0 or 3.0.0 |
| Magento 2 | 2.4.7 | 8.3 | MariaDB 10.6 or 10.11, Redis 7.2, OpenSearch 2.12.0-2.19.0 |
| Magento 2 | 2.4.6 | 8.2 | MariaDB 10.6 or 10.11, Redis 7.0-7.2, OpenSearch 2.5.0-2.19.0 |

```bash
# Inspect the resolved profile
govard config profile --json
govard config profile --framework laravel --framework-version 11 --json
```

---

## 🧱 Magento 2

Magento 2 is the deepest supported workflow in Govard.

### Key Features

- `govard config auto` injects DB, cache, search, Varnish, and base URLs into `app/etc/env.php`
- `govard tool magento [command]` runs Magento CLI (`bin/magento`) inside the PHP container
- `govard tool magerun [command]` (Shortcut: `mr`) runs `n98-magerun2` inside the PHP container
- `govard tool magento cron:install` installs crontabs inside the container
- Optional Selenium/MFTF support (`mftf: true` in features)
- Optional LiveReload for Grunt/Vite workflows (`livereload: true` in features)
- Dedicated `php-debug` routing when Xdebug is enabled

### Typical Flow

```bash
govard env up
govard config auto
govard tool magento cache:clean
govard test phpunit
```

### 🏎️ LiveReload & Frontend Development

Govard supports the standard Magento 2 Grunt-based LiveReload workflow.

1.  **Enable the feature** in `.govard.yml`:
    ```yaml
    stack:
      features:
        livereload: true
    ```
2.  **Apply changes**: Run `govard env up`. This exposes port `35729` to your host machine.
3.  **Start the watcher**:
    ```bash
    govard shell
    # Inside the shell:
    grunt watch
    ```
4.  **Verify Configuration**: This is set up **automatically** if you run `govard config auto`. It injects the following snippet into your `app/etc/env.php`:
    ```html
    <script src="http://localhost:35729/livereload.js?snipver=1"></script>
    ```
5.  **Browser Setup**: Simply install the [LiveReload Browser Extension](http://livereload.com/extensions/) or rely on the automated script injection above.

::: tip TIP
Manual injection via `default.xml` is no longer needed. Everything is handled by Govard's auto-configuration engine via `env.php`.
:::

::: info NOTE
Since port `35729` is mapped directly to your host, you can only run `livereload: true` for one project at a time. If you have multiple projects, ensure only the active one has this feature enabled.
:::

### Native Upgrade Pipeline

```bash
# Test upgrade in an isolated profile
cp .govard.yml .govard.upgrade-test.yml
GOVARD_ENV=upgrade-test govard upgrade --version 2.4.8-p4 --dry-run
GOVARD_ENV=upgrade-test govard upgrade --version 2.4.8-p4
```

What `govard upgrade` does for Magento 2:
- Resolves correct PHP/MariaDB/Search versions for the target
- Smart Composer merge (preserves your modules and custom repos)
- Automatically relaxes version constraints for dev tools (`phpunit`, `phpmd`)
- Handles `composer update`, `setup:upgrade`, and static content compilation

### Multi-Website / Multi-Store Setup

```yaml
framework: "magento2"
domain: "primary.test"
store_domains:
  store-a.test:
    code: base
    type: website
  store-b.test:
    code: store_b
    type: store
```

```bash
govard domain add store-a.test
govard domain add store-b.test
govard config auto
govard tool magento cache:flush
```

**What Govard handles automatically:**
- Routes all domains through the shared proxy with HTTPS
- Sets global base URL from `domain`
- Runs scoped `bin/magento config:set` for each `store_domains` entry
- Emits `MAGE_RUN_CODE` / `MAGE_RUN_TYPE` host mappings (object form with explicit `type`)

**What you still need to do:**
- Create websites, stores, and store views in Magento admin
- Clear config/cache after changing store mappings

---

## 🌱 Mage-OS

Mage-OS is a community-maintained, drop-in fork of Magento 2 Open Source. Govard detects it via `mage-os/product-community-edition` or `mage-os/project-community-edition` in `composer.json`, and reuses Magento 2's Docker image, nginx template, and Varnish/compose stack — all Magento 2 tooling above (`govard tool magento`, `govard tool magerun`, `govard config auto`, multi-site routing) applies unchanged.

Default runtime: PHP 8.4.

### Fresh Bootstrap & Native Upgrade Pipeline

```bash
govard bootstrap --framework mageos --fresh
govard upgrade --version 1.3.1
```

`govard bootstrap`/`govard upgrade` use `mage-os/project-community-edition` and Mage-OS's public repository (`https://repo.mage-os.org`) instead of Magento's private repository.

---

## 🛒 Magento 1 / OpenMage

```bash
govard tool magerun [command]
```

Default runtime: PHP 8.1 + MariaDB 10.11. No optional cache/search/queue services forced.

### Native Upgrade Pipeline

```bash
govard upgrade --version <version>
```

Handles: Composer sync, cache purge (`var/cache`, `var/session`, etc.), compiler maintenance, and DB migration via `n98-magerun`.

### Multi-Store with Typed Routing

```yaml
framework: "magento1"
domain: "primary.test"
store_domains:
  store-a.test:
    code: base
    type: website
  store-b.test:
    code: store_b
    type: store
  store-c.test: store_c   # scalar = legacy behavior (try both website + store code)
```

Object form with explicit `type` causes Govard to inject host-based `MAGE_RUN_CODE` / `MAGE_RUN_TYPE` into nginx/Apache automatically — no manual `.htaccess` `SetEnvIf` rules needed.

---

## 🎨 Laravel

```bash
govard tool artisan [command]
```

Defaults: web root `/public`, MariaDB 11.4, version-aware PHP.

### Native Upgrade Pipeline

```bash
govard upgrade --version 12
```

- Updates `composer.json` framework constraint
- Runs full `composer update`
- Runs `php artisan migrate --force`

---

## 🌐 Drupal

```bash
govard tool drush [command]
```

Defaults: web root `/web`, MariaDB 11.4, version-aware PHP.

---

## ⚡ Symfony

```bash
govard tool symfony [command]
```

Defaults: web root `/public`, MariaDB 11.4, version-aware PHP.

### Native Upgrade Pipeline

```bash
govard upgrade --version 7
```

- Updates `symfony/framework-bundle` constraints
- Runs `composer update`
- Runs `doctrine:migrations:migrate`
- Runs `cache:clear`

---

## 🛍️ Shopware

```bash
govard tool shopware [command]
```

Defaults: web root `/public`, MariaDB 11.4, PHP 8.4.

---

## 🍰 CakePHP

```bash
govard tool cake [command]
```

Defaults: web root `/webroot`, MariaDB 11.4.

---

## 🏪 PrestaShop

```bash
govard tool prestashop [command]
```

Defaults: web root project root, MariaDB 10.11, PHP 8.1. Govard auto-detects PrestaShop projects and clones/configures existing installs; there is no fresh-install bootstrap or native upgrade pipeline for PrestaShop yet.

---

## 📰 WordPress

```bash
govard tool wp [command]
```

Defaults: web root `/`, MariaDB 11.4, PHP 8.3.

### Fresh Bootstrap

WordPress fresh bootstrap downloads core from `wordpress.org` and installs via PHP bootstrap scripts — `wp-cli` is **not** required for initial setup.

```bash
govard bootstrap --framework wordpress --fresh
```

### Native Upgrade Pipeline

```bash
govard upgrade --version 6.7
```

- `wp core update --version=<version>`
- `wp core update-db`
- `wp cache flush`

---

## ⚡ Next.js

```bash
govard shell   # opens web container at /app
govard tool npm [command]
govard tool npx [command]
```

Defaults: Node 24, no DB forced. Project-root web serving.

---

## 🔵 Emdash

Node-first local runtime: Node 22, no managed PHP/DB/cache/search/queue.

```bash
govard shell           # web container at /app
govard tool pnpm [command]
govard open admin      # opens /_emdash/admin
```

Fresh install:

```bash
govard bootstrap --framework emdash --fresh
govard env up
```

**Package manager auto-detection:** Govard reads `package.json` (`packageManager` field), `pnpm-workspace.yaml`, and lockfiles.

> Current scope is local Node + SQLite + local uploads. Govard does not yet automate Cloudflare D1/R2 flows.

---

## 🐍 Django

Python-first local runtime: Python 3.12 (configurable via `stack.python_version`), PostgreSQL 16, no managed PHP/cache/search/queue.

```bash
govard shell           # web container at /app
govard tool manage [command]   # python manage.py [command]
govard db connect               # psql into the postgres db
```

Fresh install (scaffold a brand-new project from scratch):

```bash
mkdir myproject && cd myproject
govard init --framework django
govard bootstrap --fresh --framework django --framework-version 5.1
```

Fresh install (clone an existing project, then bootstrap it):

```bash
git clone <your-django-repo> myproject && cd myproject
govard init --framework django
govard env up
govard bootstrap --framework django
```

**Detection:** any project with a `manage.py` file at its root.

> Current scope is `requirements.txt` + `pip` only (no Poetry/`pyproject.toml`), PostgreSQL only (no SQLite/MySQL option), and `manage.py runserver` for local dev (no Gunicorn). Both workflows run `pip install` + `manage.py migrate` automatically. `--fresh` scaffolds via `django-admin startproject config .` and wires `settings.py` to the Postgres container Govard already provisions, plus `ALLOWED_HOSTS`/`CSRF_TRUSTED_ORIGINS` for the project's configured domain.

---

## 🔧 Custom Stack

```bash
govard init --framework custom
```

Interactive picker for:
- Web server (`nginx`, `apache`, `hybrid`)
- Database engine and version
- Cache service
- Search engine
- Queue service
- Optional Varnish

---

## Contributing a New Framework

Want to add a framework not listed here? See [Adding a New Framework](/developer/adding-a-framework) for the internal registry structure and a file-by-file guide.

---

[← Configuration](/reference/configuration) | [Remotes and Sync →](/workflows/remotes-and-sync)
