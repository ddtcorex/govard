---
title: "Global services: proxy, Mailpit, phpMyAdmin, Portainer"
description: "Govard's shared global services stack: reverse proxy, mail catcher, database admin UI, and container management available to every project."
---

# Global Services

Govard provides a suite of built-in global services that are shared across all projects. These services run as Docker containers managed by a dedicated compose stack.

---

## Available Services

| Service | URL | Purpose |
| :--- | :--- | :--- |
| **Caddy Proxy** | — | Routes traffic for all `.test` domains |
| **DNSMasq** | — | Resolves `*.test` domains to local |
| **Mailpit** | `https://mail.govard.test` | Catch outgoing emails for development |
| **PHPMyAdmin** | `https://pma.govard.test` | Web-based MySQL database management |
| **Portainer** | `https://portainer.govard.test` | Docker container management UI |

---

## Accessing Services

### Via CLI Commands

```bash
govard open mail     # Open Mailpit
govard open db       # Open PHPMyAdmin
govard open portainer # Open Portainer
```

### Via Direct URLs

Open these URLs in your browser (HTTPS with automatic SSL):

| Service | URL |
| :--- | :--- |
| Mailpit | `https://mail.govard.test` |
| PHPMyAdmin | `https://pma.govard.test` |
| Portainer | `https://portainer.govard.test` |

---

## Service Credentials

### Portainer

| Field | Value |
| :--- | :--- |
| URL | `https://portainer.govard.test` |
| Username | `admin` |
| Password | `AdminGovard123$` |

### PHPMyAdmin

PHPMyAdmin uses the same database credentials as your project. Connect using:

- **Host**: `mysql` (from within Docker network)
- **Host**: `127.0.0.1` (from host, check `govard db info`)
- **Username/Password**: From your project's `.govard.yml` or `env.php`

### Mailpit

No authentication required. All outgoing mail from your project containers is captured and viewable at `https://mail.govard.test`.

#### Using Mailpit as SMTP Server

Mailpit acts as a SMTP server that catches all outgoing emails instead of sending them to real recipients. This is perfect for development.

| Setting | Value |
| :--- | :--- |
| SMTP Host | `mail` (from container) or `mail.govard.test` (from host) |
| SMTP Port | `1025` |
| Username | (empty) |
| Password | (empty) |
| SSL/TLS | Disable |

**Host resolution difference:**
- **From PHP container**: Use `mail` (Docker internal DNS resolves container names)
- **From host machine**: Use `mail.govard.test` (requires DNS resolution via dnsmasq)

#### Magento 2 Configuration

**Option 1: Configure via `app/etc/env.php`**

Add or modify the `system` section:

```php
'system' => [
    'default' => [
        'system' => [
            'smtp' => [
                'host' => 'mail',
                'port' => '1025',
            ],
        ],
    ],
],
```

Or if using a SMTP module (like Mageplaza SMTP, Aheadworks SMTP, etc.):

**Option 2: Module Admin Configuration**

Most SMTP modules have settings in **Stores → Configuration → General → SMTP**:

| Field | Value |
| :--- | :--- |
| Host | `mail` |
| Port | `1025` |
| Username | (leave empty) |
| Password | (leave empty) |
| SSL/TLS | Disable |

#### Laravel Configuration

In `.env`:

```env
MAIL_MAILER=smtp
MAIL_HOST=mail
MAIL_PORT=1025
MAIL_USERNAME=null
MAIL_PASSWORD=null
MAIL_ENCRYPTION=null
MAIL_FROM_ADDRESS=noreply@example.com
MAIL_FROM_NAME="${APP_NAME}"
```

#### Symfony Configuration

In `.env`:

```env
MAILER_DSN=smtp://mail:1025
```

Or in `config/packages/mailer.yaml`:

```yaml
framework:
    mailer:
        dsn: 'smtp://mail:1025'
```

#### Testing SMTP Connection

```bash
# Test with swaks (Swiss Army Knife for SMTP)
swaks --to test@example.com --server mail --port 1025

# Test with telnet
telnet mail 1025
```

Example SMTP session:
```
HELO localhost
MAIL FROM:<sender@example.com>
RCPT TO:<recipient@example.com>
DATA
Subject: Test Email

Hello, this is a test!
.
QUIT
```

After sending, check `https://mail.govard.test` to see the captured email.

---

## Managing Global Services

### Start/Stop Services

```bash
# Start all global services
govard svc up

# Stop all global services
govard svc down

# Restart all services
govard svc restart
```

### View Service Status

```bash
govard svc ps
```

### View Logs

```bash
govard svc logs
govard svc logs --tail 50
govard svc logs mail
```

### Sleep/Wake Workflow

Pause all running projects to free up resources:

```bash
govard svc sleep   # Stop all running project containers
govard svc wake    # Resume all paused projects
```

---

## Advanced Options

### Startup Flags

```bash
# Pull latest images before starting
govard svc up --pull

# Skip Root CA trust installation
govard svc up --no-trust

# Disable automatic local build fallback
govard svc up --no-fallback
```

### Access Raw Compose Commands

`govard svc` proxies to Docker Compose for the global services stack:

```bash
govard svc pull
govard svc logs -f
govard svc ps
```

---

## Troubleshooting

### Port Conflicts

If global services fail to start, check for port conflicts:

```bash
govard doctor
```

Common conflicts:
- Port 80/443: Other web servers (Apache, Nginx)
- Port 53: Other DNS servers

### Browser SSL Warnings

If you see SSL warnings after starting services:

```bash
govard doctor trust
```

### Service Not Responding

Check container status:

```bash
govard svc ps
docker ps | grep govard-proxy
```

View logs to identify issues:

```bash
govard svc logs --tail 100
```

---

## Architecture

Global services run from `~/.govard/proxy/docker-compose.yml` with the compose project name `proxy`.

Services are registered via Caddy routes:
- `mail.govard.test` → `govard-proxy-mail:8025`
- `pma.govard.test` → `govard-proxy-pma:80`
- `portainer.govard.test` → `govard-proxy-portainer:9000`

---

**[← SSL and Domains](/workflows/ssl-and-domains)** | **[Configuration →](/reference/configuration)**