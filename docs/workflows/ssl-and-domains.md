---
title: Local HTTPS & .test Domains
description: Govard's built-in Caddy proxy and Root CA auto-trust give every local project a secure *.test HTTPS domain with zero configuration.
---

# SSL and Domains

Govard provides local HTTPS for `.test` domains through the shared Caddy proxy and its internal certificate authority.

---

## What Govard Handles Automatically

- Local `.test` DNS routing via `dnsmasq`
- Certificate issuance for all project domains
- Root CA export to `~/.govard/ssl/root.crt`
- System trust-store installation (best-effort)
- Browser NSS import when `certutil` is available
- PHP runtime trust refresh on `govard env up` / `govard env restart` when the exported Root CA exists

---

## DNS Configuration for `.test` Domains

Govard runs a built-in `dnsmasq` service that resolves `*.test` domains to your local environment. You need to tell your OS to forward `.test` queries to this service.

### Linux — systemd-resolved (Recommended)

Works on Ubuntu, Debian, Arch, Fedora:

```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' | sudo tee /etc/systemd/resolved.conf.d/govard-test.conf
[Resolve]
DNS=127.0.0.1
Domains=~test
EOF
sudo systemctl restart systemd-resolved
```

### Linux — resolvconf (Legacy Ubuntu/Debian)

```bash
sudo apt-get install resolvconf
echo "nameserver 127.0.0.1" | sudo tee /etc/resolvconf/resolv.conf.d/tail
sudo resolvconf -u
```

### macOS

```bash
sudo mkdir -p /etc/resolver
echo "nameserver 127.0.0.1" | sudo tee /etc/resolver/test
```

### Verify DNS Resolution

```bash
resolvectl query laravel.test
dig +short laravel.test
```

---

## Install Root CA Trust

`govard svc up` and `govard svc restart` auto-trust the Govard Root CA by default.

```bash
govard svc up         # Auto-trusts CA
govard doctor trust   # Manual trust (re-run anytime)
```

Skip auto-trust when needed:

```bash
govard svc up --no-trust
```

**What `doctor trust` does:**
1. Exports Root CA from Caddy to `~/.govard/ssl/root.crt`
2. Installs into system trust store (Linux/macOS)
3. Best-effort import into Chromium/Firefox NSS stores when `certutil` is available

::: tip TIP
On Linux, install `certutil` from the `libnss3-tools` package so Govard can import into browser NSS stores automatically:
```bash
sudo apt-get install libnss3-tools
```
:::

---

## Browser Trust Configuration

If the OS trust is installed but your browser still shows warnings:

1. Locate **`~/.govard/ssl/root.crt`**
2. Open browser certificate settings (e.g., `chrome://settings/certificates`)
3. Navigate to the **Authorities** tab → click **Import**
4. Select `root.crt` and mark it trusted for websites
5. Restart the browser

Once trusted, all `*.test` domains managed by Govard will show a "Green Lock" without further configuration.

---

## Domain Management

### Extra Domains

```bash
govard domain add brand-b.test
govard domain remove brand-b.test
govard domain list
```

Govard routes these domains through the same proxy and CA flow as the primary project domain.

### Elasticsearch/OpenSearch Host Access

When a project's `stack.services.search` is `elasticsearch` or `opensearch`, the shared Caddy proxy also routes `http://<your-domain>:9200` to that project's search container by hostname — the same Host-header routing used for the primary domain on `443`, just on a second, plain-HTTP port. This is automatic; no `linked_projects` or extra domain configuration is required. See [Configuration](/reference/configuration) for details, including the no-auth/no-TLS caveat.

### RabbitMQ Management UI Host Access

When a project's `stack.services.queue` is `rabbitmq`, the shared Caddy proxy also routes `http://<your-domain>:15672` to that project's RabbitMQ management UI by hostname — the same Host-header routing used for the search engine on `:9200`. This is automatic; no `linked_projects` or extra domain configuration is required. Same no-TLS caveat as above.

### Inter-Project Access From PHP Runtimes

By default, Govard projects are isolated. To allow one local PHP project to call another through the shared Caddy proxy, you must explicitly declare the dependency in your `.govard.yml` using the `linked_projects` field:

```yaml
linked_projects:
  - project-b
```

When a project is linked:
1.  **Isolation by Default**: Only projects explicitly linked will have their domains injected into the container's `/etc/hosts`.
2.  **Targeted Restarts**: When `project-b` starts, Govard will refresh only the projects that depend on it (like `project-a`), ensuring minimal downtime.
3.  **Automatic Resolution**: Listing a project name automatically maps its primary domain and all extra domains.

When `~/.govard/ssl/root.crt` is present, Govard also mounts that Root CA into `php` and `php-debug` and refreshes the container trust store during `govard env up` / `govard env restart`, so TLS verification works from inside the runtime.

This host alias list is refreshed on `govard env up`. If connectivity issues persist after linking, run:

```bash
govard doctor trust
govard env restart
```

### Multi-Store Magento

For Magento multi-site setups:
- Use `store_domains` to automatically route hostnames and set scoped base URLs
- Use object entries (`type: website` or `type: store`) for automatic `MAGE_RUN_CODE` / `MAGE_RUN_TYPE` injection
- Use `extra_domains` only for additional hostnames **not** already in `store_domains`

```yaml
store_domains:
  brand-b.test:
    code: brand_b
    type: store
```

You do **not** need manual `SetEnvIf` rules in `.htaccess` for the standard typed `store_domains` flow.

---

## How Routing Works

1. `govard env up` renders the project stack and registers all routes
2. `govard env start` and `govard env restart` re-apply routes + local host entries after lifecycle changes
3. Govard injects known Govard project domains into PHP runtimes for container-to-container HTTP calls
4. Caddy terminates HTTPS
5. Caddy forwards traffic to the project web container
6. Govard manages the local CA and exported root certificate

---

## Troubleshooting

### Browser says "Connection is not private"

Check in this order:

```bash
govard svc up               # Ensure global services are running
govard doctor trust         # Re-import Root CA
ls ~/.govard/ssl/root.crt   # Verify CA file exists
```

If still failing:
- Manually import `~/.govard/ssl/root.crt` into the browser
- Install `certutil` (Linux: `sudo apt-get install libnss3-tools`)
- Restart the browser

### Domain does not resolve

Check:
- `.test` resolver configuration (see [DNS Configuration](#dns-configuration-for-test-domains))
- `govard svc up` is running (includes the dnsmasq service)

```bash
govard svc up
resolvectl query myproject.test
```

### Certificate was not generated

```bash
govard env up
govard env logs
docker ps | grep caddy
```

### HTTPS not working after container restart

```bash
govard env restart    # Re-applies proxy routes + local domain entries
```

### `curl` inside `php` or `php-debug` says `unable to get local issuer certificate`

```bash
govard doctor trust
govard env restart
```

This re-exports the Govard Root CA, then recreates the PHP runtime with the CA mounted so `curl`, Composer, and other TLS clients trust `*.test` endpoints.

---

[Remotes and Sync](/workflows/remotes-and-sync) | [Desktop App](/workflows/desktop-app)
