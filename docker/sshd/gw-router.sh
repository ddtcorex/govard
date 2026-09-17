#!/bin/sh
set -eu

user="$1"
targets=/govard-gateway/targets

route=""
if [ -f "$targets" ]; then
    route="$(awk -v u="$user" '$1 == u { print $2, $3; exit }' "$targets")"
fi

if [ -z "$route" ]; then
    echo "unknown target '$user' -- it is not registered with the gateway" >&2
    exit 255
fi

container="$(echo "$route" | cut -d' ' -f1)"
target_user="$(echo "$route" | cut -d' ' -f2)"

ssh_opts="-i /govard-gateway/id_ed25519 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 -p 22"

# A real sftp client requests the "sftp" subsystem, which carries no
# SSH_ORIGINAL_COMMAND and (like a plain interactive shell) allocates no pty
# (no SSH_TTY). A plain shell request also carries no SSH_ORIGINAL_COMMAND,
# but does get a pty. This is the only signal available to a forced command
# (OpenSSH does not expose "this was the sftp subsystem" any other way); the
# real proof is Task 8's live sftp put/get through this exact path.
if [ -n "${SSH_ORIGINAL_COMMAND:-}" ]; then
    # shellcheck disable=SC2086
    set +e
    ssh $ssh_opts "$target_user@$container" -- "$SSH_ORIGINAL_COMMAND"
    rc=$?
    set -e
    if [ "$rc" -eq 255 ]; then
        echo "sandbox '$user' is not reachable -- run \`govard sandbox up\` to start it" >&2
        exit 255
    fi
    exit "$rc"
elif [ -n "${SSH_TTY:-}" ]; then
    # shellcheck disable=SC2086
    set +e
    ssh -tt $ssh_opts "$target_user@$container"
    rc=$?
    set -e
    if [ "$rc" -eq 255 ]; then
        echo "sandbox '$user' is not reachable -- run \`govard sandbox up\` to start it" >&2
        exit 255
    fi
    exit "$rc"
else
    # shellcheck disable=SC2086
    set +e
    ssh $ssh_opts -s "$target_user@$container" sftp
    rc=$?
    set -e
    if [ "$rc" -eq 255 ]; then
        echo "sandbox '$user' is not reachable -- run \`govard sandbox up\` to start it" >&2
        exit 255
    fi
    exit "$rc"
fi
