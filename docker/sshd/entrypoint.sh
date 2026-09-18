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
# (see this task's header comment). It never removes an account, and it never
# modifies one either: a routed name that collides with a pre-existing system
# account is left unmodified with a warning (access still requires an
# allowlisted key), because the gateway never touches accounts it did not
# create.
reconcile_accounts() {
    targets=/govard-gateway/targets
    while true; do
        if [ -f "$targets" ]; then
            while read -r user _container _target_user; do
                [ -z "$user" ] && continue
                if id "$user" >/dev/null 2>&1; then
                    # Already exists: either a previous tick created it
                    # (uid 0, nothing to do) or it is a pre-existing system
                    # account the gateway must never touch. Warn loudly for
                    # the latter so the dead route is visible, not silent.
                    if [ "$(id -u "$user" 2>/dev/null)" != "0" ]; then
                        echo "gateway: username '$user' collides with an existing system account; leaving it unmodified (access still requires an allowlisted key)" >&2
                    fi
                    continue
                fi
                useradd --non-unique --uid 0 --gid 0 --no-create-home \
                    --home-dir /nonexistent --shell /bin/sh -- "$user" 2>/dev/null || true
            done < "$targets"
        fi
        sleep 5
    done
}
reconcile_accounts &

exec "$@"
