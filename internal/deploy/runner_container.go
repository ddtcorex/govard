package deploy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// ErrContainerPathUnmappable reports a path that cannot be translated from this
// machine's project root into the project's root inside the container.
//
// There is no fallback on purpose. A build step that silently kept the host path
// would run against a directory the container does not have — or, worse, against
// one it does have that holds something else — so the answer is a refusal that
// names the path, the root it should have been under, and the two ways out.
var ErrContainerPathUnmappable = errors.New("path is not reachable from inside the project container")

// ErrContainerStdinUnsupported reports a container step that asked for stdin.
//
// RunOptions.Stdin exists so a secret can reach a command without appearing in
// its argv (visible in the target's process list) or in its command text
// (printed by --verbose and kept in CI logs). `docker exec` without -i has no
// stdin to give, so the alternative would be to put the secret in one of the two
// places the option was added to avoid. The caller is told that, rather than
// being handed the exec error, which would describe a symptom and not the reason.
var ErrContainerStdinUnsupported = errors.New("a container step cannot receive stdin")

// ContainerRunner runs a build command inside the project's own container
// through `docker exec`, instead of on the host shell. It is the third
// implementation of the same seam: a project whose private VCS credentials live
// in the container has nothing on the host that can fetch them, so its artifact
// cannot be assembled there at all.
type ContainerRunner struct {
	// Container is the container the step runs in, e.g. "sample-php-1".
	Container string
	// User is the account the step runs as. Empty leaves the container's own.
	User string
	// LocalRoot is this machine's project root: the directory the generated
	// build commands were written against. Nothing outside it can be mapped.
	LocalRoot string
	// ContainerRoot is where LocalRoot is mounted inside the container, e.g.
	// "/var/www/html".
	ContainerRoot string
}

// ContainerPath translates one path from this machine's project tree into the
// container's. An empty path is the project root itself, which is how a step
// with no working directory of its own is expressed.
func (r ContainerRunner) ContainerPath(local string) (string, error) {
	if strings.TrimSpace(r.ContainerRoot) == "" {
		return "", fmt.Errorf("%w: this runner has no container root configured, so no path can be mapped into it", ErrContainerPathUnmappable)
	}
	local = strings.TrimSpace(local)
	if local == "" {
		return path.Clean(r.ContainerRoot), nil
	}
	local = filepath.Clean(local)
	rel, err := filepath.Rel(filepath.Clean(r.LocalRoot), local)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s is outside the project root %s; move --output under the project root, or build on the host", ErrContainerPathUnmappable, local, r.LocalRoot)
	}
	return path.Join(r.ContainerRoot, filepath.ToSlash(rel)), nil
}

// Args builds the `docker exec` argv for one command, the same shape
// cli.RunInContainerAt uses for every other govard exec: exec, the account, the
// working directory, the container, then the command under `sh -c` because the
// steps are shell chains.
//
// workdir must already be a path inside the container; Run maps RunOptions.Dir
// through ContainerPath before it gets here. An empty workdir is the project
// root inside the container, the same answer ContainerPath gives an empty
// local path.
func (r ContainerRunner) Args(workdir, command string) ([]string, error) {
	if strings.TrimSpace(r.Container) == "" {
		return nil, fmt.Errorf("the container runner has no container name to exec into")
	}
	if strings.TrimSpace(workdir) == "" {
		workdir = r.ContainerRoot
	}
	args := []string{"exec"}
	if r.User != "" {
		args = append(args, "-u", r.User)
	}
	return append(args, "-w", workdir, r.Container, "sh", "-c", r.replaceLocalRoot(command)), nil
}

// replaceLocalRoot rewrites the paths a generated build command names in this
// machine's terms — `git -C '<checkout>'`, `tar -x -C '<output>'`, and the
// `cd {{release_path}}` every recipe build task opens with — so they name the
// same directories inside the container.
//
// Only the project root is mapped. A command that names some other host path —
// a project's own build hook may name one — keeps it and fails inside the
// container, which is the honest outcome; a guess would build the wrong tree and
// report success.
//
// The occurrence has to be a whole path: one that runs into a character able to
// continue a directory's name belongs to a different directory which merely
// starts with the same characters (`/srv/app-old` beside a project root
// `/srv/app`), and rewriting that would assemble the wrong tree.
func (r ContainerRunner) replaceLocalRoot(command string) string {
	local := strings.TrimSpace(r.LocalRoot)
	if local == "" {
		// A needle that matches everywhere is worse than no rewrite: an unset
		// root would otherwise insert the container root between every character
		// of every command.
		return command
	}
	local = filepath.Clean(local)
	container := filepath.ToSlash(filepath.Clean(r.ContainerRoot))
	if local == "." || local == string(filepath.Separator) {
		// Neither names a project: one is whatever the process happens to be in,
		// the other is under everything. Rewriting either would mangle the
		// command, so nothing is mapped until a real root is configured.
		return command
	}

	var out strings.Builder
	rest := command
	for {
		found := strings.Index(rest, local)
		if found < 0 {
			break
		}
		end := found + len(local)
		if (end < len(rest) && pathNameChar(rest[end])) || (found > 0 && pathNameChar(rest[found-1])) {
			// A longer name on either side is a different directory; keep
			// looking from one character further along.
			out.WriteString(rest[:found+1])
			rest = rest[found+1:]
			continue
		}
		out.WriteString(rest[:found])
		out.WriteString(container)
		rest = rest[end:]
	}
	out.WriteString(rest)
	return out.String()
}

// pathNameChar reports whether a byte can appear inside one path segment's name,
// which is what tells "/srv/app" followed by "-old" from "/srv/app" followed by
// "/vendor".
func pathNameChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	switch b {
	case '.', '_', '-', '+', '~':
		return true
	}
	return false
}

// Run implements Runner over `docker exec`.
//
// It refuses what a container cannot honour before it starts anything, then
// runs the step the way the local and SSH runners do: one bounded capture per
// stream, its own process group, and a WaitDelay so a killed client cannot hold
// the wait open.
func (r ContainerRunner) Run(ctx context.Context, command string, opts RunOptions) (Result, error) {
	if opts.Stdin != "" {
		return Result{}, fmt.Errorf("%w: %q would have to carry it in its argv or its command text, which is what RunOptions.Stdin exists to avoid", ErrContainerStdinUnsupported, command)
	}
	workdir := r.ContainerRoot
	if strings.TrimSpace(opts.Dir) != "" {
		mapped, err := r.ContainerPath(opts.Dir)
		if err != nil {
			return Result{}, err
		}
		workdir = mapped
	}
	args, err := r.Args(workdir, command)
	if err != nil {
		return Result{}, err
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	// The working directory that matters is the container's, carried by -w above.
	// Setting cmd.Dir would move the docker client's own working directory on
	// this machine, which is not the step's working directory.
	cmd := exec.CommandContext(ctx, "docker", args...)
	// The docker client is the process govard starts, so a cancelled step's
	// signal is aimed at a group govard created instead of govard's own. What
	// runs inside the container belongs to the daemon, not to this group, so
	// nothing here signals it.
	prepareProcessGroup(cmd, nil)
	// A killed process does not necessarily close the pipes its children
	// inherited, and Wait would then block until those children exit. WaitDelay
	// bounds that wait.
	cmd.WaitDelay = waitDelayAfterKill

	stdout, stderr := newBoundedBuffer(captureLimit), newBoundedBuffer(captureLimit)
	cmd.Stdout = streamTo(stdout, opts.Out)
	cmd.Stderr = streamTo(stderr, opts.Out)

	err = cmd.Run()
	if ctx.Err() != nil {
		// The step was stopped rather than finished, so anything still in the
		// client's group is killed outright.
		sweepProcessGroup(cmd)
	}
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	// The operator is told about the step they wrote, not about the `docker exec`
	// govard wrapped around it.
	return result, commandError(ctx, command, result.Stderr, err)
}
