# Govard Support Matrix

## Runtime and Tooling

| Component | Supported |
|---|---|
| Go | 1.24+ |
| Docker Engine | 24+ |
| Docker Compose Plugin | v2 |
| SSH Client | OpenSSH compatible |
| rsync | 3.x |

## Operating Systems

| OS | Status |
|---|---|
| Linux | Supported |
| macOS | Supported |
| Windows (WSL2 workflow) | Supported with Linux-based runtime |

## Framework Support (Current Baseline)

| Framework | Detection | Version-Aware Profile |
|---|---|---|
| Magento 2 | Yes | Yes (major-level) |
| Magento 1 / OpenMage | Yes | Framework defaults |
| Laravel | Yes | Yes (major-level) |
| Next.js | Yes | Framework defaults |
| Drupal | Yes | Yes (major-level) |
| Symfony | Yes | Yes (major-level) |
| Shopware | Yes | Framework defaults |
| CakePHP | Yes | Framework defaults |
| WordPress | Yes | Yes (major-level) |
| Custom | Manual | Manual |

## Command Contract Notes

- `govard doctor` is diagnostics-only.
- `govard doctor fix-deps` checks required host dependencies (`docker`, `docker compose`, `ssh`, `rsync`).
- `govard config profile` is read-only.
- `govard config profile apply` writes runtime choices into `govard.yml` only.
