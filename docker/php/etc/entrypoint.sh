#!/usr/bin/env sh
set -e

if [ -n "${PGID:-}" ]; then
  sudo groupmod -g "${PGID}" www-data 2>/dev/null || true
fi
if [ -n "${PUID:-}" ]; then
  sudo usermod -u "${PUID}" www-data 2>/dev/null || true
fi

# Ensure permissions for typical PHP directories
sudo chown -R www-data:www-data /var/log/php /var/lib/php 2>/dev/null || true

exec "$@"
