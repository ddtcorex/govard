#!/usr/bin/env sh
set -e

# UID/GID remaps change the passwd entry for www-data. Do the remap from a
# root re-entry while the original www-data UID still resolves for sudo.
if [ "${GOVARD_ENTRYPOINT_ROOT:-}" != "1" ] && [ "$(id -u)" != "0" ] && { [ -n "${PUID:-}" ] || [ -n "${PGID:-}" ]; }; then
  exec sudo -E -H env GOVARD_ENTRYPOINT_ROOT=1 /usr/local/bin/entrypoint.sh "$@"
fi

# ─── Synchronization ───────────────────────────────────────────────────────

# Ensure www-data group changes happen before UID changes. Once the current
# process rewrites its own UID mapping, later sudo calls may stop resolving.
if [ -n "${PGID:-}" ]; then
  CURRENT_GID=$(id -g www-data)
  if [ "${CURRENT_GID}" != "${PGID}" ]; then
    echo "Updating www-data GID to ${PGID}..."
    if ! sudo groupmod -g "${PGID}" www-data; then
      echo "Warning: could not update www-data GID to ${PGID}; continuing with GID ${CURRENT_GID}." >&2
    fi
  fi
fi

# Add a fallback group (uid 1000) for internal image files if needed
if [ -n "${PUID:-}" ] && [ "${PUID}" != "1000" ]; then
  if ! getent group govard-legacy >/dev/null; then
    sudo groupadd -g 1000 govard-legacy || true
    sudo usermod -aG govard-legacy www-data || true
  fi
fi

UID_REMAP_CHANGED=0
if [ -n "${PUID:-}" ]; then
  CURRENT_UID=$(id -u www-data)
  if [ "${CURRENT_UID}" != "${PUID}" ]; then
    echo "Updating www-data UID to ${PUID}..."
    if ! sudo usermod -u "${PUID}" www-data; then
      echo "Warning: could not update www-data UID to ${PUID}; continuing with UID ${CURRENT_UID}." >&2
    else
      UID_REMAP_CHANGED=1
    fi
  fi
fi

# Apply recursive chown if requested. On some bind-mount backends (e.g.
# Docker Desktop for Mac), chown can be refused for individual paths owned by
# a UID that doesn't match the host session, for reasons outside this
# script's control. That must not abort the whole entrypoint: combined with
# `set -e` above, an unguarded failure here would prevent php-fpm from ever
# starting, so any such failure is logged as a warning instead.
if [ -n "${CHOWN_DIR_LIST:-}" ]; then
  for dir in ${CHOWN_DIR_LIST}; do
    if [ -d "${dir}" ]; then
      echo "Fixing permissions for ${dir}..."
      sudo chown -R www-data:www-data "${dir}" || \
        echo "Warning: could not fix ownership for some paths under ${dir}; continuing." >&2
    fi
  done
fi

# Ensure specific PHP directories are correct
sudo chown -R www-data:www-data /var/log/php /var/lib/php 2>/dev/null || true

# Refresh the container trust store when Govard mounts its local Root CA.
if [ -f "/usr/local/share/ca-certificates/govard.crt" ] && command -v update-ca-certificates >/dev/null 2>&1; then
  if ! sudo update-ca-certificates >/dev/null 2>&1; then
    echo "Warning: could not refresh container CA trust for Govard Root CA." >&2
  fi
fi

# Start cron so Magento cron:install entries can execute inside the container.
if command -v crond >/dev/null 2>&1; then
  sudo crond 2>/dev/null || true
fi

# Ensure specific Composer version is active if requested
if [ -n "${COMPOSER_VERSION:-}" ] && [ "${COMPOSER_VERSION}" != "latest" ]; then
  # Exact version check can be tricky due to build dates in --version output
  # If it doesn't look like a direct match, we do a re-baseline install
  CURRENT_COMPOSER_VERSION=$(composer --version 2>/dev/null | cut -d' ' -f3)
  if [ "${CURRENT_COMPOSER_VERSION}" != "${COMPOSER_VERSION}" ]; then
    COMPOSER_BIN=$(which composer 2>/dev/null || echo "/usr/local/bin/composer")
    
    # Try pre-baked version first
    LOCAL_BIN=""
    case "${COMPOSER_VERSION}" in
      1) [ -f "/usr/local/bin/composer1" ] && LOCAL_BIN="/usr/local/bin/composer1" ;;
      2) [ -f "/usr/local/bin/composer2" ] && LOCAL_BIN="/usr/local/bin/composer2" ;;
      2.2) [ -f "/usr/local/bin/composer2lts" ] && LOCAL_BIN="/usr/local/bin/composer2lts" ;;
    esac

    if [ -n "${LOCAL_BIN}" ]; then
      echo "Using pre-baked Composer version ${COMPOSER_VERSION}..."
      sudo ln -sf "${LOCAL_BIN}" "${COMPOSER_BIN}"
      echo "Composer version $(composer --version | head -n1) is now active."
    else
      # Falling back to download for non-standard or specific point versions
      echo "Ensuring Composer version ${COMPOSER_VERSION} (downloading)..."
      DOWNLOAD_URL="https://getcomposer.org/composer-stable.phar"
      case "${COMPOSER_VERSION}" in
        1) DOWNLOAD_URL="https://getcomposer.org/composer-1.phar" ;;
        2) DOWNLOAD_URL="https://getcomposer.org/composer-2.phar" ;;
        2.2) DOWNLOAD_URL="https://getcomposer.org/download/latest-2.2.x/composer.phar" ;;
        *.*) DOWNLOAD_URL="https://getcomposer.org/download/${COMPOSER_VERSION}/composer.phar" ;;
      esac

      if sudo curl -sSfL "${DOWNLOAD_URL}" -o "${COMPOSER_BIN}.tmp"; then
        sudo chmod +x "${COMPOSER_BIN}.tmp"
        sudo mv "${COMPOSER_BIN}.tmp" "${COMPOSER_BIN}"
        echo "Composer version $(composer --version | head -n1) is now active."
      else
        echo "Warning: failed to download Composer version ${COMPOSER_VERSION}; falling back to default image version." >&2
      fi
    fi
  fi
fi

# Ensure specific Node.js version is active if requested
if [ -n "${NODE_VERSION:-}" ]; then
  CURRENT_NODE_VERSION=$(node -v 2>/dev/null | sed 's/v//')
  if ! echo "${CURRENT_NODE_VERSION}" | grep -q "^${NODE_VERSION}"; then
    echo "Ensuring Node.js version ${NODE_VERSION} (downloading)..."
    
    # Detect architecture
    ARCH="x64"
    case "$(uname -m)" in
      aarch64|arm64) ARCH="arm64" ;;
      x86_64) ARCH="x64" ;;
    esac
    
    # Download and install specific version from unofficial MUSL builds (for Alpine)
    mkdir -p /tmp/node-install
    URL="https://unofficial-builds.nodejs.org/download/release/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${ARCH}-musl.tar.xz"
    if sudo curl -sSfL "${URL}" -o /tmp/node.tar.xz; then
      echo "Extracting Node.js ${NODE_VERSION}..."
      sudo tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1 --no-same-owner
      rm /tmp/node.tar.xz
      echo "Node.js version $(node -v) is now active."
    else
      # Try official build as a fallback (though it usually needs glibc)
      URL_OFFICIAL="https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${ARCH}.tar.xz"
      echo "Warning: MUSL build not found, attempting official build..."
      if sudo curl -sSfL "${URL_OFFICIAL}" -o /tmp/node.tar.xz; then
        sudo tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1 --no-same-owner
        rm /tmp/node.tar.xz
        echo "Node.js version $(node -v) is now active."
      else
        echo "Warning: failed to download Node.js version ${NODE_VERSION}; falling back to default image version." >&2
      fi
    fi
  fi
fi

if [ "$(id -u)" = "0" ]; then
  unset GOVARD_ENTRYPOINT_ROOT
  exec sudo -E -H -u www-data "$@"
fi

unset GOVARD_ENTRYPOINT_ROOT
exec "$@"
