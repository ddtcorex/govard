# Framework Runtime Support Matrix

This matrix defines how Govard chooses runtime defaults and version-aware overrides.
Use it together with `govard config profile` when validating project compatibility.

## Selection Rules

1. Govard auto-detects framework and version from project manifests.
2. `--framework` and `--framework-version` overrides always win.
3. If no version-specific override exists, Govard falls back to framework defaults.
4. `govard config profile --json` exposes the selected profile, source, notes, and warnings.

## Framework Defaults

| Framework | Web Root | PHP | Node | DB | Cache | Search | Queue |
|---|---|---:|---:|---|---|---|---|
| magento2 | `/pub` | 8.4 | 24 | mariadb 11.4 | valkey 8.0.0 | opensearch 2.19.0 | none |
| magento1 | project root | 8.1 | - | mariadb 10.11 | none | none | none |
| laravel | `/public` | 8.4 | - | mariadb 11.4 | none | none | none |
| nextjs | project root | - | 24 | none | none | none | none |
| drupal | `/web` | 8.4 | - | mariadb 11.4 | none | none | none |
| symfony | `/public` | 8.4 | - | mariadb 11.4 | none | none | none |
| shopware | `/public` | 8.4 | - | mariadb 11.4 | none | none | none |
| cakephp | `/webroot` | 8.4 | - | mariadb 11.4 | none | none | none |
| wordpress | `/wordpress` | 8.3 | - | mariadb 11.4 | none | none | none |
| custom | project root | 8.4 | - | mariadb 11.4 | none | none | none |

`-` means no default value is forced for that stack component.

## Version-Specific Overrides

| Framework | Version | Override |
|---|---|---|
| laravel | 10 | php 8.2 |
| laravel | 11 | php 8.3 |
| laravel | 12 | php 8.4 |
| symfony | 6 | php 8.2 |
| symfony | 7 | php 8.3 |
| drupal | 10 | php 8.3 |
| drupal | 11 | php 8.4 |
| wordpress | 6 | php 8.3 |
| magento2 | 2.4.9+ | php 8.4, mariadb 11.4, redis 7.2, opensearch 3.0.0, rabbitmq 4.1.0 |
| magento2 | 2.4.8 | php 8.4, mariadb 11.4, redis 7.2, opensearch 2.19.0/3.0.0 (patch-level), rabbitmq 4.1.0 |
| magento2 | 2.4.7 | php 8.3, mariadb 10.6/10.11 (patch-level), redis 7.2, opensearch 2.12.0/2.19.0 (patch-level), rabbitmq 3.13.7/4.1.0 (patch-level) |
| magento2 | 2.4.6 | php 8.2, mariadb 10.6/10.11 (patch-level), redis 7.0/7.2 (patch-level), opensearch 2.5.0/2.12.0/2.19.0 (patch-level), rabbitmq 3.9.0/3.12.0/3.13.7/4.1.0 (patch-level) |
| magento2 | 2.4.0 - 2.4.5 | php 7.4-8.1 (version-dependent), mariadb 10.4-10.6, redis 5.0-7.2, elasticsearch/opensearch (version-dependent), rabbitmq 3.8.0-4.1.0 |
| magento2 | 2.0.0 - 2.3.7 | php 7.0-7.4 (version-dependent), mariadb 10.0-10.4, redis 3.0-5.0, elasticsearch 1.7-7.9, rabbitmq 3.5.0-3.8.0 |

## Verification Commands

```bash
# Inspect selected profile with explainable payload
govard config profile --json

# Force framework/version selection explicitly
govard config profile --framework laravel --framework-version ^11.0 --json

# Apply selected profile into .govard.yml
govard config profile apply --framework laravel --framework-version 11
```

## Notes

- The matrix reflects current runtime mappings in `internal/engine/profile.go`.
- Cache/search/queue columns list default selections only. Optional services can still be enabled via `stack.services.*`.
- When Govard cannot parse a major version, it keeps framework defaults and reports a warning.
- If framework detection is uncertain, prefer explicit overrides to avoid accidental mismatch.
