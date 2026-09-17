package gateway

import (
	"context"
	"fmt"
	"strings"
)

// ContainerExec runs a command inside a container (no stdin).
type ContainerExec interface {
	ExecInContainer(ctx context.Context, name string, args ...string) (string, error)
}

// EnsureTargetAccount creates the gateway's POSIX account for a routed
// username inside the bastion container, synchronously, at registration
// time.
//
// sshd refuses to open a session for a username with no POSIX account --
// AuthorizedKeysCommand only decides which key is accepted -- so a target
// registered moments ago must already own its account when the first ssh
// lands. The container's own reconciler loop (docker/sshd/entrypoint.sh,
// every 5s) stays as the crash-recovery backstop; this call is what makes an
// immediate post-up ssh work instead of racing that poll. The argv below is
// an EXACT mirror of entrypoint.sh's reconciler -- any drift between the two
// breaks the up-to-ssh guarantee the integration test pins.
//
// A useradd that fails because the account already exists is the idempotent
// re-up case and reports success; every other error is wrapped and returned.
func EnsureTargetAccount(ctx context.Context, execer ContainerExec, container, user string) error {
	_, err := execer.ExecInContainer(ctx, container,
		"useradd", "--non-unique", "--uid", "0", "--gid", "0",
		"--no-create-home", "--home-dir", "/nonexistent",
		"--shell", "/bin/sh", "--", user)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "already exists") {
		return nil
	}
	return fmt.Errorf("create the SSH gateway account %q in %s: %w", user, container, err)
}
