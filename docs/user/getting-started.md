# Getting Started

This guide will walk you through installing Govard and setting up your first project.

## Installation

### Install from Source (Linux/macOS)

Ensure you have Go 1.24+ installed:

```bash
go version
git clone https://github.com/ddtcorex/govard.git
cd govard
make install
```

If `go` is installed but not found, add your Go bin directory to `PATH` and reload your shell:

```bash
export PATH="$HOME/go/bin:$PATH"
```

## Basic Workflow

### 1. Initialize a Project

Navigate to your project root and run:

```bash
govard init
```

This scans your project (via `composer.json` or `package.json`) and generates a `govard.yml` configuration.

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
- Custom (interactive recipe)

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

- **[Configuration](configuration.md)** - Customize your `govard.yml`
- **[CLI Commands](commands.md)** - Full command reference
- **[SSL & HTTPS](ssl-https.md)** - Set up local HTTPS
