# Magento 1 (OpenMage)

Govard supports Magento 1/OpenMage projects with a lightweight stack and simple nginx template.

## Requirements

- PHP 7.4 or 8.1 (default 8.1)
- MariaDB 10.11
- A Magento 1/OpenMage codebase (e.g. OpenMage LTS or a legacy Magento 1 project)

## Detection

Govard detects Magento 1/OpenMage when one of the following is present:

- `openmage/magento-lts` in `composer.json`
- `magento-hackathon/magento-composer-installer` in `composer.json`
- `app/Mage.php`
- `app/etc/local.xml`

## Default Stack

- Web: nginx (`magento1.conf`)
- PHP: `8.1`
- DB: MariaDB `10.11`
- Cache: Redis (optional)

## Commands

Use `n98-magerun` for Magento 1 CLI tasks:

```bash
govard tool magerun cache:flush
```
