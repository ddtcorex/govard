---
title: Govard FAQ & Troubleshooting
description: Answers to common questions about installing, migrating to, and troubleshooting Govard local development environments.
---

# FAQ & Troubleshooting

Common questions, issues, and solutions for Govard.

---

## Installation Issues

### Q: The installer fails with a permission error

Try the `--local` flag to install without `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --local
```

This installs to `~/.local/bin` instead of `/usr/local/bin`.

### Q: I have conflicting binaries in `/usr/bin` and `/usr/local/bin`

You've mixed install channels. Pick one channel and clean up the other:

```bash
which govard           # Find which one is active
ls /usr/bin/govard /usr/local/bin/govard  # See both locations
```

Remove the one from the wrong channel manually. Do not use multiple channels on the same machine.

### Q: `govard self-update` fails with a permissions error

Govard needs write access to the installed binary path. If using system-wide paths:

```bash
sudo govard self-update
```

Or reinstall to a user-local path with `--local`.

---

## Docker Issues

### Q: `govard env up` fails pulling images

If image pull fails, try the local fallback:

```bash
govard env up --fallback-local-build
```

This builds missing Govard-managed images locally from embedded blueprints.

### Q: Port conflict when starting the environment

```bash
govard doctor       # Checks for port conflicts
govard env ps       # Check what containers are currently running
```

Ensure no other service is occupying ports 80, 443, or your project's mapped ports.

### Q: `govard env up` shows "project identity collision"

Another tracked project already uses the same `project_name` or `domain`.

```bash
govard project list    # See all tracked projects
```

Update `project_name` or `domain` in `.govard.yml` to a unique value.

---

## SSL / HTTPS Issues

### Q: Browser shows "Your connection is not private"

Run in this order:

```bash
govard svc up       # Ensure global services are running
govard doctor trust # Re-import Root CA
```

Then manually import `~/.govard/ssl/root.crt` into your browser if auto-import fails.

### Q: Auto-import doesn't work for my browser

Install `certutil` first:

```bash
# Ubuntu/Debian
sudo apt-get install libnss3-tools
# Then re-run:
govard doctor trust
```

### Q: HTTPS breaks after restarting project containers

```bash
govard env restart   # Re-applies proxy routes and host entries
```

### Q: `curl` inside `php` or `php-debug` fails with `unable to get local issuer certificate`

Run in this order:

```bash
govard doctor trust
govard env restart
```

This exports the Govard Root CA to `~/.govard/ssl/root.crt`, then recreates the PHP runtime with that CA mounted and trusted inside the container.

---

## DNS Issues

### Q: `myproject.test` doesn't resolve

1. Ensure systemd-resolved is configured:
   ```bash
   cat /etc/systemd/resolved.conf.d/govard-test.conf
   ```
2. Verify dnsmasq service is running:
   ```bash
   govard svc up
   ```
3. Test DNS resolution:
   ```bash
   resolvectl query myproject.test
   dig +short myproject.test
   ```

### Q: DNS resolved but no response (502 Bad Gateway)

```bash
govard env ps        # Check containers are actually running
govard env up        # Restart if needed
```

---

## Configuration Issues

### Q: My configuration changes are not taking effect

Govard re-renders the compose file on `env up`. Restart the environment:

```bash
govard env up
```

If you changed `stack.php_version` or other stack settings, containers need to be recreated.

### Q: `govard config set` doesn't update the right file

`govard config set` only writes to `.govard.yml` (the base config). Profile and local override files are read-only from the CLI.

### Q: How do I change the Composer version and which ones are fastest?

Modify `stack.composer_version` in `.govard.yml`. Govard optimizes for:
- `1`
- `2`
- `2.2` (LTS)

These versions are pre-baked in the image and switch instantly. Any other version (e.g. `2.7.2`) will be downloaded automatically on the first `env up`.

### Q: `doctor --fix` shows "skipped" for optional fixes

This is correct behavior — skipping optional fixes is reported as `INFO (Skipped)` instead of `ERROR`. Your environment is healthy.

---

## Remote / Sync Issues

### Q: `govard remote test` fails with "auth" failure

```bash
govard remote copy-id staging    # Copy your SSH key to the remote
ssh-add ~/.ssh/id_rsa            # Ensure key is loaded in SSH agent
```

### Q: Sync takes forever or times out

- Use `--no-compress` if CPU is a bottleneck:
  ```bash
  govard sync -s staging --full --no-compress
  ```
- Check file exclusions — `--no-noise` can significantly reduce transfer size:
  ```bash
  govard sync -s staging --db --no-noise
  ```

### Q: Getting "permission denied" during rsync

Govard will suggest a permission fix for Magento 2 when this occurs. For manual fix:

```bash
govard remote exec staging -- chmod -R 755 /var/www/app/var
```

### Q: `~/` paths in remote flags are expanded by my local shell

Quote the path to prevent local expansion:

```bash
govard remote add staging --host host.example.com --user deploy --path '~/public_html'
#                                                                          ^-- single quotes
```

### Q: Where does `govard deploy` fit in, and how do I rehearse it?

`govard deploy` publishes a git revision to a remote over SSH and rsync. Before the
first server is involved, rehearse the whole pipeline against a container on your
machine:

```bash
govard deploy sandbox up --profile full --php 8.3
govard deploy --remote sandbox --yes
govard deploy sandbox down --purge
```

- Engine reference, publish strategies, rollback: [Deployment](/workflows/deployment)
- Worked setups (Luma, Hyvä, several themes, developer vs production mode, symlinked
  vs real webroot): [Deployment case studies](/workflows/deploy-case-studies)

`govard deploy check staging` answers "is this remote ready" before anything is
created, and `govard deploy plan staging` prints the whole task list without
connecting.

---

## Database Issues

### Q: `db import` fails with "table doesn't exist" errors

Use `--drop` to safely reset before import:

```bash
govard db import --file backup.sql --drop
```

### Q: Database password is wrong after bootstrap

Run auto-config to inject the correct credentials:

```bash
govard config auto   # Magento 2: rebuilds env.php with container DB settings
```

### Q: PHPMyAdmin doesn't show my project's database

Run a full up to re-register the project:

```bash
govard env up
```

Then visit `govard open db`.

---

## Xdebug Issues

### Q: Xdebug is not connecting to my IDE

1. Check Xdebug is enabled: `govard debug status`
2. Ensure the cookie `XDEBUG_SESSION` matches `stack.xdebug_session` in `.govard.yml` (default: `PHPSTORM`)
3. Check your IDE is listening on port 9003

### Q: Xdebug slows down my site even when not debugging

Set a specific Xdebug session name and only trigger it via the cookie/browser extension. Xdebug routes to `php-debug` **only** when the session cookie is present.

---

## Desktop Issues

### Q: Desktop app crashes on Ubuntu 24.04 startup

This is a known AppArmor user namespace restriction issue. The installer handles this automatically, but you can apply manually:

```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
```

### Q: Desktop shows mock data instead of real projects

You're viewing the frontend directly as a file (no backend active). Start the desktop properly:

```bash
govard desktop
# or for dev mode:
DISPLAY=:1 govard desktop --dev
```

---

## Update Issues

### Q: `govard self-update` skips dependency checks in CI

This is intentional — self-update detects non-interactive environments and skips heavy system checks to prevent CI timeouts.

### Q: After `self-update`, the Desktop app still shows the old version

The desktop binary is also updated by `self-update`. If the old version persists, restart the desktop app completely.

---

## General Tips

### Check system health

```bash
govard doctor           # Full system diagnostics
govard doctor --json    # Machine-readable output
govard doctor --pack    # Bundle diagnostics for bug reports
```

### View what's running

```bash
govard status           # All running Govard environments
govard env ps           # Current project containers
govard project list     # All tracked projects
```

### Clean up stale environments

```bash
govard env cleanup      # Remove stale compose files
govard project list --orphans  # Find orphaned Docker projects
govard project delete <name>   # Remove a project completely
```

### Reset a project without losing source code

```bash
govard env down -v      # Stop + remove volumes (databases)
govard env up           # Fresh start
govard config auto      # Re-inject app configuration (Magento 2)
```

---

[Contributing](/developer/contributing) | [Changelog](/more/changelog)
