#!/bin/sh
set -e

HOST_KEY_DIR=/etc/ssh/keys
mkdir -p "$HOST_KEY_DIR"
if [ ! -f "$HOST_KEY_DIR/ssh_host_ed25519_key" ]; then
    ssh-keygen -q -t ed25519 -N "" -f "$HOST_KEY_DIR/ssh_host_ed25519_key"
fi
ln -sf "$HOST_KEY_DIR/ssh_host_ed25519_key" /etc/ssh/ssh_host_ed25519_key
ln -sf "$HOST_KEY_DIR/ssh_host_ed25519_key.pub" /etc/ssh/ssh_host_ed25519_key.pub

# Every routed username must be a real POSIX account before sshd will open a
# session for it -- AuthorizedKeysCommand only decides which key is accepted,
# not whether the account exists. This loop creates one, sharing UID 0
# (--non-unique) so the forced command can read the gateway's private key
# (see this task's header comment). It never removes an account: an entry
# whose registry route is gone is inert, because gw-router.sh checks
# /govard-gateway/targets itself on every connection.
reconcile_accounts() {
    targets=/govard-gateway/targets
    while true; do
        if [ -f "$targets" ]; then
            while read -r user _container _target_user; do
                [ -z "$user" ] && continue
                id "$user" >/dev/null 2>&1 || \
                    useradd --non-unique --uid 0 --gid 0 --no-create-home \
                        --home-dir /nonexistent --shell /bin/sh -- "$user" 2>/dev/null || true
            done < "$targets"
        fi
        sleep 5
    done
}
reconcile_accounts &

exec "$@"
