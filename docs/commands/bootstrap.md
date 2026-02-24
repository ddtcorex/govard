# govard bootstrap

Bootstrap a local environment for fast onboarding from remote or fresh install.

## Usage

```bash
govard bootstrap
govard bootstrap --clone --environment dev
govard bootstrap --fresh --version 2.4.8
```

## Core Flow

`govard bootstrap` can orchestrate:

1. `govard init` automatically if `govard.yml` is missing
2. `govard env up` (unless `--skip-up`)
3. Clone flow (`sync` + DB import + configure + admin setup)
4. Fresh install flow (`composer create-project` + setup install + optional sample/Hyva)

## Key Options

- `-c, --clone` Clone project from remote (default: true)
- `--code-only` Clone source only (skip DB/media)
- `--fresh` Fresh Magento install (auto-disables default clone; explicit `--clone` is rejected)
- `--include-sample` Install Magento sample data (fresh install)
- `-e, --environment` Remote source environment (default: `dev`)
- `--no-db` Skip DB import
- `--no-media` Skip media sync
- `--no-composer` Skip composer install
- `--no-admin` Skip admin user creation
- `--no-stream-db` Disable stream DB import and use sync DB flow instead
- `--db-dump` Import DB from local dump file
- `--include-product` Include `pub/media/catalog/product` images during media sync (Magento only)
- `-p, --meta-package` Magento package for fresh install (default: `magento/project-community-edition`)
- `--version` Magento version for fresh install
- `--hyva-install` Install Hyva default theme in fresh flow
- `--hyva-token` Hyva repo token
- `--mage-username` Magento marketplace username for auth bootstrap
- `--mage-password` Magento marketplace password for auth bootstrap
- `--fix-deps` Run `govard custom fix-deps` before bootstrap (auto-detects remote Magento version when cloning and `--version` is omitted)
- `--skip-up` Skip `govard env up` stage

## Notes

- Requires `remotes.<environment>` configured in `govard.yml` for clone flow.
- Fresh flow works as a single command (`govard bootstrap --fresh ...`) without needing `--clone=false`.
- `govard init` preserves existing `remotes` and `hooks`.
- Clone DB import:
  - Magento 2 remotes: probes remote `app/etc/env.php` for DB credentials.
  - Symfony/Laravel/Drupal/WordPress/Shopware/CakePHP remotes: probes remote `.env*` files for DB credentials.
  - Falls back to default credentials if probing fails.
- Symfony clone flow:
  - If composer fails but `vendor/autoload.php` exists, bootstrap continues.
  - Media sync is skipped gracefully when remote media path does not exist.
- `auth.json` handling:
  - Reuses project `auth.json` if present.
  - Can copy from `~/.composer/auth.json`.
  - Can generate from `--mage-username` / `--mage-password`.
- Media sync defaults:
  - Excludes `pub/media/catalog/product` by default for faster clones.
  - Use `--include-product` to include product images (still excludes `catalog/product/cache`).

## Examples

```bash
# Standard remote clone from dev
govard init
govard bootstrap

# Clone source only, no DB/media
govard bootstrap --clone --code-only --environment dev

# Clone from staging, skip composer and admin user creation
govard bootstrap --environment staging --no-composer --no-admin

# Clone from dev and include product images in media sync
govard bootstrap --include-product

# Fresh install + sample data
govard bootstrap --fresh --version 2.4.8 --include-sample
```
