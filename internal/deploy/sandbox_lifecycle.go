package deploy

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
)

// The docroot shapes a sandbox can be created with. They are the test lever for
// the publish strategy: govard resolves `auto` by looking at the target, so a
// sandbox whose docroot is absent or a symlink exercises the atomic swap, and one
// whose docroot is a real git checkout exercises the in-place publish.
const (
	SandboxDocRootAbsent  = "absent"
	SandboxDocRootSymlink = "symlink"
	SandboxDocRootReal    = "real"

	// SandboxRemoteName is the remote `up` writes and `down` removes.
	SandboxRemoteName = "sandbox"
	// SandboxDeployerLayout seeds a target that looks like one the other deploy
	// tool already owns.
	SandboxDeployerLayout = "deployer"
)

// SandboxPortProbe waits until the container's SSH port answers. It is a field
// on the request so the lifecycle is testable without a container: production
// dials, tests do not have to.
type SandboxPortProbe func(ctx context.Context, host string, port int, timeout time.Duration) error

// SandboxRequest is one `govard deploy sandbox` invocation.
type SandboxRequest struct {
	ProjectRoot string
	ProjectName string
	Profile     string
	DocRoot     string
	Layout      string
	RemoteName  string
	// PHP is the series the image must provide, for example "8.4". Empty keeps
	// the base image's own version.
	PHP string
	// Requirements come from the framework recipe the command layer resolved.
	Requirements SandboxRequirements
	// Repository is the local checkout the mirror is refreshed from. Empty
	// means the project root.
	Repository string
	Recreate   bool
	Purge      bool
	Out        io.Writer
	Probe      SandboxPortProbe
}

// SandboxState is what the sandbox commands report and what `down` needs to undo.
type SandboxState struct {
	Container   string
	Image       string
	Profile     string
	PHP         string
	Port        int
	Running     bool
	Exists      bool
	RemoteName  string
	RemoteSet   bool
	MirrorPath  string
	KeyPath     string
	DeployPath  string
	CurrentPath string
	Packages    string
}

func (r SandboxRequest) remoteName() string {
	if strings.TrimSpace(r.RemoteName) != "" {
		return strings.TrimSpace(r.RemoteName)
	}
	return SandboxRemoteName
}

func (r SandboxRequest) repository() string {
	if strings.TrimSpace(r.Repository) != "" {
		return strings.TrimSpace(r.Repository)
	}
	return r.ProjectRoot
}

func (r SandboxRequest) out() io.Writer {
	if r.Out == nil {
		return io.Discard
	}
	return r.Out
}

func (r SandboxRequest) docRoot() (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(r.DocRoot))
	switch normalized {
	case "":
		return SandboxDocRootSymlink, nil
	case SandboxDocRootAbsent, SandboxDocRootSymlink, SandboxDocRootReal:
		return normalized, nil
	default:
		return "", fmt.Errorf("unknown docroot %q; use %s, %s or %s",
			r.DocRoot, SandboxDocRootAbsent, SandboxDocRootSymlink, SandboxDocRootReal)
	}
}

// SandboxPaths are the fixed server-side paths a sandbox uses. They match the
// documented production layout so the pipeline sees nothing unusual.
type SandboxPaths struct {
	Home       string
	Current    string
	DeployPath string
}

// SandboxDefaultPaths returns the layout every sandbox uses.
func SandboxDefaultPaths() SandboxPaths {
	return SandboxPaths{
		Home:       SandboxHome,
		Current:    SandboxHome + "/public_html",
		DeployPath: SandboxHome + "/.deployer",
	}
}

// SandboxUp brings the sandbox to a state a deploy can run against, and is
// idempotent: a second call reuses the key, the image, the container and the
// port it already has.
func SandboxUp(ctx context.Context, runtime SandboxRuntime, git Runner, request SandboxRequest) (*SandboxState, error) {
	// The request is validated before the runtime is probed: a flag that cannot
	// name a sandbox is a usage error, and reporting it should not depend on
	// whether this machine happens to have a container runtime.
	profile, err := ValidateSandboxProfile(request.Profile)
	if err != nil {
		return nil, err
	}
	docRoot, err := request.docRoot()
	if err != nil {
		return nil, err
	}
	php, err := ValidateSandboxPHP(request.PHP)
	if err != nil {
		return nil, err
	}
	if err := runtime.Available(ctx); err != nil {
		return nil, err
	}

	stateDir := SandboxStateDir(request.ProjectRoot)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", stateDir, err)
	}

	dockerfile, err := SandboxDockerfile(profile, php, request.Requirements)
	if err != nil {
		return nil, err
	}
	dockerfilePath := SandboxDockerfilePath(request.ProjectRoot)
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", dockerfilePath, err)
	}

	image, err := SandboxImageTag(SandboxSpec{
		Project:      request.ProjectName,
		Profile:      profile,
		PHP:          php,
		Requirements: request.Requirements,
	})
	if err != nil {
		return nil, err
	}

	key, err := EnsureSandboxKey(stateDir)
	if err != nil {
		return nil, err
	}
	mirror := SandboxMirrorPath(request.ProjectRoot)
	if err := RefreshSandboxMirror(ctx, git, request.repository(), mirror); err != nil {
		return nil, err
	}

	container := SandboxContainerName(request.ProjectName, request.ProjectRoot)
	exists, err := runtime.ContainerExists(ctx, container)
	if err != nil {
		return nil, err
	}
	if exists && request.Recreate {
		fmt.Fprintf(request.out(), "recreating %s\n", container)
		if err := runtime.RemoveContainer(ctx, container); err != nil {
			return nil, err
		}
		exists = false
	}

	imageExists, err := runtime.ImageExists(ctx, image)
	if err != nil {
		return nil, err
	}
	if !imageExists || request.Recreate {
		fmt.Fprintf(request.out(), "building the %s sandbox image %s\n", profile, image)
		if err := runtime.BuildImage(ctx, SandboxBuildRequest{
			Image:      image,
			Dockerfile: dockerfilePath,
			Context:    stateDir,
			Out:        request.out(),
		}); err != nil {
			return nil, err
		}
	}

	if !exists {
		if err := runtime.RunContainer(ctx, SandboxRunRequest{
			Name:        container,
			Image:       image,
			MirrorPath:  mirror,
			ProjectName: request.ProjectName,
			Profile:     profile,
		}); err != nil {
			return nil, err
		}
	} else {
		running, err := runtime.ContainerRunning(ctx, container)
		if err != nil {
			return nil, err
		}
		if !running {
			if err := runtime.StartContainer(ctx, container); err != nil {
				return nil, err
			}
		}
	}

	port, err := runtime.PublishedPort(ctx, container, SandboxSSHPort)
	if err != nil {
		return nil, err
	}

	if err := installSandboxAuthorizedKey(ctx, runtime, container, key.PublicKey); err != nil {
		return nil, err
	}
	if err := prepareSandboxDocRoot(ctx, runtime, container, docRoot); err != nil {
		return nil, err
	}

	probe := request.Probe
	if probe == nil {
		probe = DialSandboxPort
	}
	if err := probe(ctx, "127.0.0.1", port, 30*time.Second); err != nil {
		return nil, err
	}

	paths := SandboxDefaultPaths()
	// The remote carries the branch the checkout is on. A branch-less remote is
	// not deployable with no flags at all — the option resolver refuses a
	// remote with neither a branch nor an explicit revision — so "defaults to
	// the local HEAD" is only true once the branch is named here.
	remote := SandboxRemoteConfig(profile, php, port, paths, key)
	remote.Branch = localBranch(ctx, git, request.repository())
	if err := WriteSandboxRemote(request.ProjectRoot, request.remoteName(), remote); err != nil {
		return nil, err
	}

	packages, _ := runtime.ImageFile(ctx, image, SandboxPackagesTxt)
	return &SandboxState{
		Container:   container,
		Image:       image,
		Profile:     profile,
		PHP:         php,
		Port:        port,
		Running:     true,
		Exists:      true,
		RemoteName:  request.remoteName(),
		RemoteSet:   true,
		MirrorPath:  mirror,
		KeyPath:     key.PrivatePath,
		DeployPath:  paths.DeployPath,
		CurrentPath: paths.Current,
		Packages:    strings.TrimSpace(packages),
	}, nil
}

// localBranch names the branch a checkout is on, and is empty for a detached
// HEAD: there is then no branch a branch-less fetch could use, and the operator
// passes --revision instead.
func localBranch(ctx context.Context, git Runner, repository string) string {
	if git == nil {
		git = LocalRunner{}
	}
	result, err := git.Run(ctx,
		"git -C "+conventions.ShellQuote(repository)+" rev-parse --abbrev-ref HEAD",
		RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(result.Stdout)
	if branch == "HEAD" {
		return ""
	}
	return branch
}

// SandboxRemoteConfig is the remote `up` writes. It is exported because it is
// the contract between the sandbox and the deploy pipeline, and both are worth
// asserting on independently.
func SandboxRemoteConfig(profile, php string, port int, paths SandboxPaths, key SandboxKeyPair) engine.RemoteConfig {
	settings := map[string]any{
		// The image's deployer identity is fixed, so the ownership path is
		// exercised rather than bypassed.
		"owner":                fmt.Sprintf("%d:%d", SandboxUserUID, SandboxUserGID),
		"writable_mode":        writableModeChmodChown,
		"writable_permissions": "0775",
	}
	// Only a profile that ships PHP gets a PHP binary to probe: naming one the
	// image does not have would fail every deploy for the wrong reason.
	if profile == SandboxProfilePHP || profile == SandboxProfileFull {
		settings["php_bin"] = "php"
		settings["composer_bin"] = "composer"
		// A deploy refuses to run when the target's PHP does not match the
		// declared series, so the sandbox declares what it actually shipped.
		// Without this, asking for 8.4 would pin the image correctly and then
		// fail the very first check.
		if php != "" {
			settings["php_version"] = php
		}
	}

	keyPath := filepath.ToSlash(filepath.Join(".govard", "sandbox", SandboxKeyName))
	return engine.RemoteConfig{
		Host:       "127.0.0.1",
		Port:       port,
		User:       SandboxUser,
		Path:       paths.Current,
		DeployPath: paths.DeployPath,
		Repository: SandboxRepoPath,
		Sandbox:    true,
		Protected:  engine.BoolPtr(false),
		Auth: engine.RemoteAuth{
			Method:  engine.RemoteAuthMethodKeyfile,
			KeyPath: keyPath,
		},
		Deploy: &engine.DeployConfig{Settings: settings},
	}
}

// installSandboxAuthorizedKey installs the generated public key for the
// container's deployer user. The image holds no key material, which is what
// keeps it cacheable across projects.
func installSandboxAuthorizedKey(ctx context.Context, runtime SandboxRuntime, container, publicKey string) error {
	script := "umask 077 && mkdir -p " + SandboxHome + "/.ssh && cat > " + SandboxHome + "/.ssh/authorized_keys" +
		" && chown -R " + SandboxUser + ":" + SandboxUser + " " + SandboxHome + "/.ssh"
	// The container may accept exec a moment before it accepts an interactive
	// one; retry briefly rather than racing the entrypoint.
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if _, err = runtime.Exec(ctx, container, []byte(publicKey+"\n"), "sh", "-c", script); err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("install the sandbox public key in %s: %w", container, err)
}

// prepareSandboxDocRoot shapes the target's current path so it implies the
// publish strategy the operator asked for.
func prepareSandboxDocRoot(ctx context.Context, runtime SandboxRuntime, container, docRoot string) error {
	paths := SandboxDefaultPaths()
	switch docRoot {
	case SandboxDocRootAbsent:
		_, err := runtime.Exec(ctx, container, nil, "sh", "-c", "rm -rf "+paths.Current+sandboxChownTail)
		return err
	case SandboxDocRootReal:
		// In-place publishing resets the docroot to an exact revision, so the
		// docroot has to be a git checkout for that to be possible at all — and a
		// target that publishes in place is one that is *already running* the
		// application: the maintenance window opens on the served application, and
		// requests never land in the release directory. An empty checkout would
		// therefore be a target no recipe can deploy to, which is why the working
		// tree is seeded from the mirror: the sandbox becomes a re-deploy target.
		script := "rm -rf " + paths.Current + " && mkdir -p " + paths.Current +
			" && git init -q " + paths.Current +
			" && git -C " + paths.Current + " fetch -q " + SandboxRepoPath + " HEAD" +
			" && git -C " + paths.Current + " reset -q --hard FETCH_HEAD" +
			sandboxChownTail
		_, err := runtime.Exec(ctx, container, nil, "sh", "-c", script)
		return err
	default:
		// A symlink, even dangling, is what the auto strategy reads as "this
		// target publishes by swapping".
		script := "rm -rf " + paths.Current + " && ln -sfn " + paths.DeployPath + "/releases/0 " + paths.Current + sandboxChownTail
		_, err := runtime.Exec(ctx, container, nil, "sh", "-c", script)
		return err
	}
}

// sandboxChownTail hands everything the setup created back to the deployer user.
//
// These commands run through `docker exec` as root, and a root-owned release
// directory is a sandbox the next deploy cannot write into — which is exactly
// the kind of failure a sandbox exists to catch, not to cause.
const sandboxChownTail = " ; chown -R " + SandboxUser + ":" + SandboxUser + " " + SandboxHome

// DialSandboxPort waits for the container's published SSH port to answer.
func DialSandboxPort(ctx context.Context, host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	address := net.JoinHostPort(host, strconv.Itoa(port))
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("the sandbox sshd did not answer on %s within %s: %w", address, timeout, lastErr)
}

// RefreshSandboxMirror makes the local bare mirror hold every local branch, so
// any commit — including one that was never pushed — is deployable.
func RefreshSandboxMirror(ctx context.Context, git Runner, repository, mirror string) error {
	if git == nil {
		git = LocalRunner{}
	}
	if _, err := git.Run(ctx, "git init --bare -q "+conventions.ShellQuote(mirror), RunOptions{Timeout: shortCommandTimeout}); err != nil {
		return fmt.Errorf("create the sandbox mirror %s: %w", mirror, err)
	}
	fetch := "git -C " + conventions.ShellQuote(mirror) + " fetch --prune --quiet " +
		conventions.ShellQuote(repository) + " " + conventions.ShellQuote("+refs/heads/*:refs/heads/*")
	if _, err := git.Run(ctx, fetch, RunOptions{Timeout: DefaultCommandTimeout}); err != nil {
		return fmt.Errorf("refresh the sandbox mirror from %s: %w", repository, err)
	}

	// A fresh bare repository points HEAD at a branch that may not exist, and a
	// branch-less `git fetch` against such a mirror fails with "couldn't find
	// remote ref HEAD". Pointing HEAD at the branch the checkout is on makes the
	// mirror look like the repository it mirrors.
	if branch := localBranch(ctx, git, repository); branch != "" {
		head := "git -C " + conventions.ShellQuote(mirror) + " symbolic-ref HEAD " +
			conventions.ShellQuote("refs/heads/"+branch)
		if _, err := git.Run(ctx, head, RunOptions{Timeout: shortCommandTimeout}); err != nil {
			return fmt.Errorf("point the sandbox mirror's HEAD at %s: %w", branch, err)
		}
	}
	return nil
}

// RefreshSandboxMirrorForTest exposes RefreshSandboxMirror to the tests/ package.
func RefreshSandboxMirrorForTest(ctx context.Context, git Runner, repository, mirror string) error {
	return RefreshSandboxMirror(ctx, git, repository, mirror)
}

// SandboxStatus reports what the sandbox is, without changing it.
func SandboxStatus(ctx context.Context, runtime SandboxRuntime, request SandboxRequest) (*SandboxState, error) {
	config, err := LoadSandboxRemote(request.ProjectRoot, request.remoteName())
	if err != nil {
		return nil, err
	}
	project := request.ProjectName
	if project == "" {
		project = config.ProjectName
	}
	container := SandboxContainerName(project, request.ProjectRoot)

	state := &SandboxState{
		Container:  container,
		RemoteName: request.remoteName(),
		RemoteSet:  config.RemoteSet,
		Port:       config.Remote.Port,
		MirrorPath: SandboxMirrorPath(request.ProjectRoot),
		KeyPath:    filepath.Join(SandboxStateDir(request.ProjectRoot), SandboxKeyName),
		Profile:    containerLabelOrEmpty(ctx, runtime, container, sandboxProfileLabel),
		// The series the image was built with, read back from the remote the
		// creation wrote: `status` must report what exists, not what a flag would
		// now ask for.
		PHP:         sandboxRemotePHP(config.Remote),
		DeployPath:  config.Remote.DeployPath,
		CurrentPath: config.Remote.Path,
	}

	exists, err := runtime.ContainerExists(ctx, container)
	if err != nil {
		return nil, err
	}
	state.Exists = exists
	if !exists {
		return state, nil
	}
	// The image the container was actually built from, so `down --purge` removes
	// that one rather than a tag recomputed from today's configuration.
	if image, err := runtime.ContainerImage(ctx, container); err == nil {
		state.Image = image
	}
	running, err := runtime.ContainerRunning(ctx, container)
	if err != nil {
		return nil, err
	}
	state.Running = running
	if !running {
		return state, nil
	}
	port, err := runtime.PublishedPort(ctx, container, SandboxSSHPort)
	if err == nil {
		state.Port = port
	}
	return state, nil
}

// sandboxRemotePHP reads the PHP series out of a sandbox remote's deploy
// settings, and is empty for a remote that does not record one.
func sandboxRemotePHP(remote engine.RemoteConfig) string {
	if remote.Deploy == nil {
		return ""
	}
	return settingsString(remote.Deploy.Settings, "php_version")
}

// SandboxDown removes the container and the remote `up` wrote, so a stale
// sandbox remote cannot be deployed to by accident.
func SandboxDown(ctx context.Context, runtime SandboxRuntime, request SandboxRequest) (*SandboxState, error) {
	state, err := SandboxStatus(ctx, runtime, request)
	if err != nil {
		return nil, err
	}
	if state.Exists {
		if state.Running {
			if err := runtime.StopContainer(ctx, state.Container); err != nil {
				return nil, err
			}
		}
		if err := runtime.RemoveContainer(ctx, state.Container); err != nil {
			return nil, err
		}
	}
	if err := RemoveSandboxRemote(request.ProjectRoot, request.remoteName()); err != nil {
		return nil, err
	}
	state.Running = false
	state.Exists = false
	state.RemoteSet = false

	if !request.Purge {
		return state, nil
	}
	if state.Image != "" {
		_ = runtime.RemoveImage(ctx, state.Image)
	}
	// The rendered Dockerfile and the mirror are derived state; the key is the
	// one thing here that cannot be regenerated into an already-running
	// container, which is why it only goes with --purge.
	if err := os.RemoveAll(SandboxStateDir(request.ProjectRoot)); err != nil {
		return nil, fmt.Errorf("remove %s: %w", SandboxStateDir(request.ProjectRoot), err)
	}
	return state, nil
}

// SandboxReset wipes the target's deploy directories and re-creates the docroot
// shape, optionally seeding a target the other deploy tool already owns.
func SandboxReset(ctx context.Context, runtime SandboxRuntime, request SandboxRequest) (*SandboxState, error) {
	state, err := SandboxStatus(ctx, runtime, request)
	if err != nil {
		return nil, err
	}
	if !state.Exists {
		return nil, fmt.Errorf("the sandbox container %s does not exist; run `govard deploy sandbox up` first", state.Container)
	}
	if !state.Running {
		return nil, fmt.Errorf("the sandbox container %s is not running; run `govard deploy sandbox up` first", state.Container)
	}
	docRoot, err := request.docRoot()
	if err != nil {
		return nil, err
	}

	paths := SandboxDefaultPaths()
	script := "rm -rf " + paths.DeployPath + " && mkdir -p " + paths.DeployPath + "/releases " + paths.DeployPath + "/shared " + paths.DeployPath + "/.dep"
	if _, err := runtime.Exec(ctx, state.Container, nil, "sh", "-c", script); err != nil {
		return nil, err
	}

	if strings.EqualFold(strings.TrimSpace(request.Layout), SandboxDeployerLayout) {
		seed := "mkdir -p " + paths.DeployPath + "/releases/1 " + paths.DeployPath + "/shared/app/etc" +
			" && printf '%s\\n' '{\"user\":\"deployer\",\"created_at\":\"seed\"}' > " + paths.DeployPath + "/.dep/deploy.lock"
		if _, err := runtime.Exec(ctx, state.Container, nil, "sh", "-c", seed); err != nil {
			return nil, err
		}
	}

	if err := prepareSandboxDocRoot(ctx, runtime, state.Container, docRoot); err != nil {
		return nil, err
	}
	return state, nil
}

// SandboxSSHArgs builds the argv for an interactive session with the sandbox.
// It is asserted rather than executed by the tests: the point is that the
// sandbox is reached exactly the way a real remote is.
func SandboxSSHArgs(request SandboxRequest, state *SandboxState) []string {
	keyPath := filepath.Join(SandboxStateDir(request.ProjectRoot), SandboxKeyName)
	args := []string{
		"-t",
		"-o", "LogLevel=ERROR",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-i", keyPath,
	}
	if state != nil && state.Port > 0 {
		args = append(args, "-p", strconv.Itoa(state.Port))
	}
	args = append(args, SandboxUser+"@127.0.0.1")
	return args
}

// SandboxConfig is the project configuration plus what the sandbox remote says.
type SandboxConfig struct {
	ProjectName string
	Remotes     map[string]engine.RemoteConfig
	Remote      engine.RemoteConfig
	RemoteSet   bool
}

// LoadSandboxRemote reads the project configuration to find the sandbox remote.
// It never writes: `status` and `ssh` must work on a project whose sandbox was
// created by an earlier session.
func LoadSandboxRemote(projectRoot, name string) (SandboxConfig, error) {
	config, _, err := engine.LoadConfigFromDir(projectRoot, false)
	if err != nil {
		return SandboxConfig{}, fmt.Errorf("read the project configuration: %w", err)
	}
	result := SandboxConfig{ProjectName: config.ProjectName, Remotes: config.Remotes}
	remote, ok := config.Remotes[name]
	result.Remote, result.RemoteSet = remote, ok
	return result, nil
}

// sandboxProfileLabel is the container label the profile is recorded in. It is
// the container's own record of how it was built, which is what `status` should
// report rather than a value recomputed from today's configuration.
const sandboxProfileLabel = "govard.sandbox.profile"

func containerLabelOrEmpty(ctx context.Context, runtime SandboxRuntime, container, label string) string {
	value, err := runtime.ContainerLabel(ctx, container, label)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

// SandboxRemoteForTest exposes the remote lookup to the tests/ package.
func SandboxRemoteForTest(projectRoot, name string) (engine.RemoteConfig, bool, error) {
	loaded, err := LoadSandboxRemote(projectRoot, name)
	if err != nil {
		return engine.RemoteConfig{}, false, err
	}
	return loaded.Remote, loaded.RemoteSet, nil
}
