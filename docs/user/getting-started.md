# Getting Started

This guide will walk you through installing Govard and setting up your first project.

## Installation

### 1. One-Line Installation

The easiest way to install Govard is using the unified installer script:

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash
```

This will automatically:

1. Detect your OS and architecture.
2. Check for Docker and Git dependencies.
3. Download and install the latest binary to `/usr/local/bin`.

### 2. Source Installation (with Go Bootstrap)

If you prefer to build from source or need to install Go 1.24+ automatically:

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --source
```

The script will offer to install Go 1.24 if your system Go version is missing or outdated (< 1.24).

### 3. Manual Build

You can also build from a local clone:

```bash
git clone https://github.com/ddtcorex/govard.git
cd govard
./install.sh --source
```

### 4. Install from Release Packages (CLI + Desktop)

Tagged releases publish installer packages that install both `govard` and `govard-desktop`.

- Linux: `govard_<version>_linux_<arch>.deb`
- macOS: `govard_<version>_Darwin_<arch>.pkg`

Linux example:

```bash
sudo dpkg -i govard_<version>_linux_amd64.deb
```

macOS example:

```bash
sudo installer -pkg govard_<version>_Darwin_arm64.pkg -target /
```

## Basic Workflow

### 1. Initialize a Project

Navigate to your project root and run:

```bash
govard init
```

This scans your project (via `composer.json` or `package.json`) and generates a `.govard.yml` configuration.

**Supported Frameworks:**

- Magento 1 (OpenMage)
- Magento 2
- Laravel
- Next.js
- Drupal
- Symfony
- Shopware
- CakePHP
- WordPress
- Custom (interactive framework)

### 2. Start the Environment

```bash
govard env up
```

This executes a startup pipeline (`Detect -> Validate -> Render -> Start -> Verify`),
renders a project-specific compose file under `~/.govard/compose/`, and starts your specialized stack in detached mode.

For faster first-run on large projects, use:

```bash
govard env up --quickstart
```

### 3. Configure the Stack (Magento 2)

For Magento 2 projects, auto-inject the container settings:

```bash
govard config auto
```

### 4. Enter the Workspace

Access the application container:

```bash
govard shell
```

### 5. (Optional) Launch Desktop

```bash
govard desktop
```

## Essential Commands

### Environment Control

```bash
govard env up          # Start environment
govard env stop        # Stop project containers
govard env down        # Tear down project containers and networks
govard status          # List running environments across workspace
```

### Development

```bash
govard shell       # Open bash shell in PHP container
govard env logs        # View container logs (-e for errors only)
govard debug on    # Enable Xdebug
govard debug off   # Disable Xdebug
```

### Diagnostics

```bash
govard doctor      # Check system requirements
govard doctor trust       # Install Root CA for HTTPS
```

## Next Steps

- **[Configuration](configuration.md)** - Customize your `.govard.yml`
- **[CLI Commands](commands.md)** - Full command reference
- **[SSL & HTTPS](ssl-https.md)** - Set up local HTTPS
