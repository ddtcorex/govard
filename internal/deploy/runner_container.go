package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/runtime"
)

// containerInterruptTimeout bounds the second `docker exec` that tears a stopped
// step down. It runs after the step was already cancelled, so it must never hold
// the CLI: the teardown script waits at most a few seconds for the record and
// for the group to leave, and this is the ceiling above that.
const containerInterruptTimeout = 20 * time.Second

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
	return r.argv(workdir, r.replaceLocalRoot(command))
}

// argv is Args without the path rewrite, for a script govard wrote itself: the
// wrapper and the teardown name a record under /tmp that is the container's own
// path, and rewriting it would be wrong on a host whose project root is /tmp.
func (r ContainerRunner) argv(workdir, script string) ([]string, error) {
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
	return append(args, "-w", workdir, r.Container, "sh", "-c", script), nil
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
//
// Killing the docker client stops nothing inside the container: the step belongs
// to the daemon. So the step records its own pid first (a `docker exec` process
// is its own session and process group leader, measured on a real daemon:
// pid == pgid == sid), and a stopped step is torn down by a second exec that
// signals that group, the same shape SSHRunner uses for work on another machine.
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
	pidFile := containerInterruptPIDFile()
	args, err := r.argv(workdir, recordContainerProcessGroup(r.replaceLocalRoot(command), pidFile))
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
	// runs inside the container is not in this group; the teardown below is
	// what reaches it.
	prepareProcessGroup(cmd, nil)
	// A killed process does not necessarily close the pipes its children
	// inherited, and Wait would then block until those children exit. WaitDelay
	// bounds that wait.
	cmd.WaitDelay = waitDelayAfterKill

	stdout, stderr := newBoundedBuffer(captureLimit), newBoundedBuffer(captureLimit)
	cmd.Stdout = streamTo(stdout, opts.Out)
	cmd.Stderr = streamTo(stderr, opts.Out)

	err = cmd.Run()
	stopped := ctx.Err() != nil
	var teardownErr error
	if stopped {
		// The step was stopped rather than finished, so anything still in the
		// client's group is killed outright, and the step inside the container
		// is signalled before this returns: a build that is aborted must not keep
		// writing into --output behind the operator's back, and a re-run with
		// --force would otherwise clear that directory under it.
		sweepProcessGroup(cmd)
		teardownErr = r.terminateInContainer(pidFile)
	}
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	// The operator is told about the step they wrote, not about the `docker exec`
	// and the wrapper govard put around it.
	failure := commandError(ctx, command, result.Stderr, err)
	var commandErr *CommandError
	if stopped && errors.As(failure, &commandErr) {
		commandErr.Note = containerTeardownNote(r.Container, teardownErr)
	}
	return result, failure
}

// terminateInContainer runs the teardown script through a second, short-lived
// `docker exec` as the same account, so it may signal the step's processes.
func (r ContainerRunner) terminateInContainer(pidFile string) error {
	args, err := r.argv(r.ContainerRoot, processGroupTeardownScript(pidFile))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), containerInterruptTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.WaitDelay = waitDelayAfterKill
	output, err := cmd.CombinedOutput()
	if err != nil {
		if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
			return fmt.Errorf("%w: %s", err, trimmed)
		}
		return err
	}
	return nil
}

// containerTeardownNote tells the operator that the teardown was attempted and
// whether its exec completed, because only the teardown can reach a step inside
// the container. A completed exec means the script ran: it signals the group
// the step recorded, and finds nothing to signal when the step never started.
func containerTeardownNote(container string, teardownErr error) string {
	if teardownErr == nil {
		return fmt.Sprintf("teardown attempted: govard ran a second docker exec in container %s to signal the step's process group", container)
	}
	return fmt.Sprintf("teardown attempted but failed: the docker exec in container %s that signals the step's process group did not complete (%v); the step may still be running there", container, teardownErr)
}

// recordContainerProcessGroup prefixes a step with the pid of the shell docker
// exec started, which leads the step's process group, and removes that record
// when the step ends by itself. The step runs in a subshell for the reason
// recordRemoteProcessGroup gives: an `exec` or an EXIT trap in the step cannot
// then skip the cleanup. The newline before the closing parenthesis keeps a
// trailing comment in the step from commenting it out.
func recordContainerProcessGroup(command, pidFile string) string {
	return fmt.Sprintf("printf '%%s\\n' $$ > %s; ( %s\n); rc=$?; rm -f %s; exit $rc",
		conventions.ShellQuote(pidFile), command, pidFile)
}

// containerInterruptPIDFile names the per-step record inside the container. It
// is built from a pid and a nanosecond clock, so every character is one this
// package chose.
func containerInterruptPIDFile() string {
	return fmt.Sprintf("/tmp/.govard-build-interrupt-%d-%d.pid", os.Getpid(), time.Now().UnixNano())
}

// missingTools reports which of tools the container cannot find on its PATH.
// Each name is checked on its own, so the answer names exactly what is missing
// rather than the first failure.
func (r ContainerRunner) missingTools(ctx context.Context, tools ...string) ([]string, error) {
	words := make([]string, 0, len(tools))
	for _, tool := range tools {
		words = append(words, conventions.ShellQuote(tool))
	}
	script := fmt.Sprintf(`for tool in %s; do command -v "$tool" >/dev/null 2>&1 || printf '%%s\n' "$tool"; done`, strings.Join(words, " "))
	result, err := r.Run(ctx, script, RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, line := range strings.Split(result.Stdout, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			missing = append(missing, name)
		}
	}
	return missing, nil
}

// nodeCapability names the toolchain a frontend step needs. It is not one of the
// capabilities a command can declare: it is reported inside the same exit-3
// error so the envelope, the exit code and the hint read like every other
// missing requirement.
const nodeCapability runtime.Capability = "node"

// frontendDirsVariable is the setting every shipped recipe's frontend step loops
// over; an empty list is how a project says it has no frontend to build.
const frontendDirsVariable = "settings.frontend_dir_args"

// preflightContainerNode refuses a container build whose frontend step would
// run in a container without Node. The app container is a PHP container, so
// `npm ci` there fails with `sh: npm: not found` at the fourth step, after the
// vendors and the compile have already spent their time. Asking up front costs
// one exec, and only when the frontend step will actually run.
func preflightContainerNode(ctx context.Context, runner Runner, req BuildRequest) error {
	container, ok := runner.(ContainerRunner)
	if !ok {
		return nil
	}
	step, ok := frontendBuildStep(req)
	if !ok {
		return nil
	}
	missing, err := container.missingTools(ctx, "node", "npm")
	if err != nil {
		return fmt.Errorf("check container %s for the Node toolchain %s needs: %w", container.Container, step.ID, err)
	}
	if len(missing) == 0 {
		return nil
	}
	return &runtime.MissingError{
		Caps: []runtime.Capability{nodeCapability},
		Detail: fmt.Sprintf("container %s has no %s, which %s needs; --runner container runs every build task in the project's app container, which carries only its own toolchain",
			container.Container, strings.Join(missing, ", "), step.ID),
		Hint: "build with --runner host on a machine that has Node, or leave deploy.settings.frontend_dir empty to skip the frontend build",
	}
}

// frontendBuildStep returns the frontend step when a build would run it: it is
// in the build stage's plan, it has something to run, it is not left to the
// target, and, when it loops over the frontend directories, at least one is
// configured. A recipe step that does not name the directory list runs whenever
// it is implemented.
func frontendBuildStep(req BuildRequest) (Step, bool) {
	plan, err := BuildPlan(req.Recipe, req.Hooks)
	if err != nil {
		// The same error stops the build tasks with its own message; the
		// preflight has nothing to add to it.
		return Step{}, false
	}
	for _, step := range plan.ForBuildMode(BuildServer).Steps {
		if step.ID != TaskFrontend || step.Kind != StepTask || step.Stage != StageBuild {
			continue
		}
		if !step.Implemented() || step.NeedsApplication || step.NeedsMigration {
			return Step{}, false
		}
		placeholder := "{{" + frontendDirsVariable + "}}"
		if !strings.Contains(step.Command, placeholder) {
			return step, true
		}
		dirs, err := req.Vars.Expand(placeholder)
		if err != nil || strings.TrimSpace(dirs) == "" {
			return Step{}, false
		}
		return step, true
	}
	return Step{}, false
}
