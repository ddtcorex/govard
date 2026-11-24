---
title: Govard release changelog
description: Full version history of Govard, following Semantic Versioning, with details on new features, fixes, and blueprint changes.
---

# Changelog

Govard follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and keeps a detailed changelog in the repository.

---

## Full Changelog

The complete release history is maintained in the repository:

**[CHANGELOG.md on GitHub](https://github.com/ddtcorex/govard/blob/master/CHANGELOG.md)**

---

## Latest Releases

For the latest releases, release notes, and download links, visit the:

**[GitHub Releases Page](https://github.com/ddtcorex/govard/releases)**

---

## Release Artifacts

Each tagged release (`vX.Y.Z`) publishes:

| Artifact | Description |
| :--- | :--- |
| `govard_<version>_Linux_amd64.tar.gz` | CLI binary archive (Linux x86_64) |
| `govard_<version>_Linux_arm64.tar.gz` | CLI binary archive (Linux ARM64) |
| `govard_<version>_Darwin_amd64.tar.gz` | CLI binary archive (macOS Intel) |
| `govard_<version>_Darwin_arm64.tar.gz` | CLI binary archive (macOS Apple Silicon) |
| `govard_<version>_linux_amd64.deb` | Linux installer (CLI + Desktop) |
| `govard_<version>_Darwin_arm64.pkg` | macOS installer (CLI + Desktop) |
| `checksums.txt` | SHA-256 checksums for all artifacts |

---

## Stay Up to Date

```bash
# Check current version
govard version

# Update to latest
govard self-update
```

`govard self-update` downloads the platform-specific release artifact, verifies the SHA-256 checksum, and atomically replaces the installed binaries.

---

[FAQ](/more/faq) | [Getting Started](/getting-started/getting-started)
