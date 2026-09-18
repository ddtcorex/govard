#!/bin/sh
set -eu

user="$1"
case "$user" in ''|*[!a-z0-9-]* ) exit 0 ;; esac
keys=/govard-gateway/keys
[ -f "$keys" ] || exit 0

while IFS=' ' read -r fingerprint keytype keydata comment; do
    [ -z "$fingerprint" ] && continue
    # $user is this script's own already-expanded argument, substituted here
    # in the emitted line -- a literal %u in the forced command would never
    # be re-expanded by sshd; only the AuthorizedKeysCommand directive's own
    # arguments go through %-expansion.
    printf 'command="/usr/local/bin/gw-router.sh %s",no-agent-forwarding,no-X11-forwarding,no-port-forwarding %s %s %s\n' \
        "$user" "$keytype" "$keydata" "$comment"
done < "$keys"
