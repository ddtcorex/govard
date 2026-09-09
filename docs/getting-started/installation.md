---
title: Install Govard on Linux & macOS
description: Install Govard via .deb package, make install, or self-update. Covers supported platforms, install channels, and how to avoid binary conflicts.
---

# Installation

This page covers all methods to install Govard on Linux and macOS.

::: warning IMPORTANT
Do not mix install channels on the same machine (e.g., `.deb` + `make install` + `self-update` across different paths). Use **one channel only** to avoid conflicting binaries in `/usr/bin` and `/usr/local/bin`.
:::

---

## 🚀 One-Line Install (Linux/macOS)

Install the latest release binary with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash
```

Using `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash
```

### Common Install Options

```bash
# Install to ~/.local/bin (no sudo required)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --local

# Build from source (auto-installs Go 1.25 if needed)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --source

# Install CLI only (required on Ubuntu 20.04)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --cli-only
```

By default, this installs `govard` (CLI) and, where `WebKitGTK 4.1` is available, `govard-desktop` to `/usr/local/bin` and:
- Auto-detects/installs missing system dependencies
- Starts global services
- Configures SSL trust
- On Linux, falls back to the separate `govard-desktop` `.deb` package if a standalone archive is not in the release

Ubuntu 20.04 does not provide WebKitGTK 4.1. The installer detects this and installs CLI only automatically; use `--cli-only` to explicitly skip Desktop on any platform. Govard Desktop requires Ubuntu 22.04+ or another Linux distribution with WebKitGTK 4.1.

---

## 📦 Release Installers

Every tagged release publishes two Linux packages:

From the [releases page](https://github.com/ddtcorex/govard/releases):

### Linux (`.deb`)

CLI only (including Ubuntu 20.04):

```bash
sudo apt install ./govard_<version>_linux_<arch>.deb
```

CLI + Desktop (WebKitGTK 4.1 / Ubuntu 22.04+):

```bash
sudo apt install ./govard_<version>_linux_<arch>.deb ./govard-desktop_<version>_linux_<arch>.deb
```

### macOS (`.pkg`)

```bash
sudo installer -pkg govard_<version>_Darwin_arm64.pkg -target /
```

---

## 🔧 Build from Source

### Prerequisites

Ensure you have the following installed:

| Tool | Required Version |
| :--- | :--- |
| Go | `1.25+` |
| Node.js | `20+` |
| Yarn | v1.x |
| golangci-lint | v2.11+ |
| Docker + Docker Compose | latest |
| Wails | `v2.11+` (desktop development only) |

### Source Install

```bash
git clone https://github.com/ddtcorex/govard.git
cd govard
./install.sh --source
```

### Local Developer Setup

1. **Install Go 1.25+** from [go.dev](https://go.dev/dl/)

2. **Enable Yarn** via Corepack:
   ```bash
   corepack enable
   ```

3. **Install golangci-lint**:
   ```bash
   curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
   ```

4. **Install Wails** (for desktop development):
   ```bash
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   wails version
   ```

No `sudo` required — install everything locally and update your `PATH`.

---

## 🐳 Docker Images

Govard uses a single PHP Dockerfile with build args instead of versioned folders.

```bash
# Standard PHP image
docker build -f docker/php/Dockerfile \
  -t ddtcorex/govard-php:8.4 \
  --build-arg PHP_VERSION=8.4 \
  docker/php

# Magento 2 optimized PHP image
docker build -f docker/php/magento2/Dockerfile \
  -t ddtcorex/govard-php-magento2:8.4 \
  --build-arg PHP_VERSION=8.4 \
  docker/php
```

---

## 📦 Package Managers & Containers

One command per platform — pick one channel and stick to it:

```bash
# npm (Node 20+, any OS, CLI only)
npm i -g @ddtcorex/govard
# Pin for reproducible environments:
npm i -g @ddtcorex/govard@<version>

# Homebrew (macOS + Linuxbrew, CLI only)
brew install ddtcorex/tap/govard

# Docker (no install needed — CI-friendly)
docker run --rm ghcr.io/ddtcorex/govard:<version> version

# Snap (Linux, CLI only — classic confinement is required:
# Govard orchestrates Docker and writes system paths)
sudo snap install govard --classic

# Debian/Ubuntu via CloudSmith (one-time repo setup, then install)
curl -1sLf https://dl.cloudsmith.io/public/ddtcorex/govard-deb/setup.deb.sh | sudo -E bash
sudo apt install govard

# Fedora/RHEL via CloudSmith (one-time repo setup, then install)
curl -1sLf https://dl.cloudsmith.io/public/ddtcorex/govard-rpm/setup.rpm.sh | sudo -E bash
sudo dnf install govard

# Alpine via CloudSmith (one-time repo setup, then install)
curl -1sLf https://dl.cloudsmith.io/public/ddtcorex/govard-apk/setup.alpine.sh | sudo -E bash
sudo apk add govard
```

Replace `<version>` with a tag from the [releases page](https://github.com/ddtcorex/govard/releases).

Installs from npm, Homebrew, or Docker record their install source
(check with `govard doctor`). `govard self-update` on those installs
defers to the owning package manager instead of overwriting the binary.

For CI pipelines, use the `ddtcorex/setup-govard` GitHub Action
(see [CI Integration](/workflows/ci-integration)).

---

## 🖥️ Shell Completions

Release archives bundle completion scripts under `completion/`, and the
`.deb` package installs them automatically (bash, fish, zsh). For
other install channels, generate them from the binary:

```bash
govard completion bash > /etc/bash_completion.d/govard   # root
govard completion zsh > "${fpath[1]}/_govard"
govard completion fish > ~/.config/fish/completions/govard.fish
govard completion powershell | Out-String | Invoke-Expression
```

---

## 🔄 Updating Govard

```bash
govard self-update
```

`self-update` downloads the platform-specific release artifact, **verifies the SHA-256 checksum**, and atomically replaces installed binaries (`govard` + `govard-desktop`).

---

## ✅ Verify Installation

```bash
govard version
govard doctor
```

`govard doctor` runs system diagnostics including Docker, DNS, ports, and SSL trust checks.

Release downloads (`install.sh` and npm) are sha256-verified against
`checksums.txt` before anything is installed; on mismatch the install
aborts. There is no bypass flag for this check by design.

---

**[← Home](/)** | **[Getting Started →](/getting-started/getting-started)**
