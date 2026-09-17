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
	"govard/internal/gateway"
)

// The docroot shapes a sandbox can be created with. They are the test lever for
// the publish strategy: govard resolves `auto` by looking at the target, so a
// sandbox whose docroot is absent or a symlink exercises the atomic swap, and one
// whose docroot is a real git checkout exercises the in-place publish.
const (
	SandboxDocRootAbsent  = "absent"
	SandboxDocRootSymlink = "symlink"
	SandboxDocRootReal    = "real"

	// SandboxRemoteName is the synthetic sandbox remote, resolved from live
	// Docker state rather than written anywhere.
	SandboxRemoteName = "sandbox"
	// SandboxDeployerLayout seeds a target that looks like one the other deploy
	// tool already owns.
	SandboxDeployerLayout = "deployer"
)

// govardProxyNetwork is the global network the SSH gateway and every other
// proxy-stack service ride (internal/blueprints/files/proxy.yml). Joining it
// is best-effort: the sandbox's own direct SSH path never depends on it.
const govardProxyNetwork = "govard-proxy"

// gatewayContainerName is the bastion whose POSIX accounts must exist before
// sshd will open a session for a routed username (see
// gateway.EnsureTargetAccount). It is best-effort like the rest of the
// registration block: when the gateway is absent, its own reconciler covers
// the account at gateway start.
const gatewayContainerName = "govard-proxy-sshd"

// gatewayContainerExec adapts the sandbox runtime to the gateway's narrow
// container-exec interface. The interface lives in internal/gateway (not
// here) because this package already imports internal/gateway -- declaring it
// here would be an import cycle.
type gatewayContainerExec struct{ runtime SandboxRuntime }

func (a gatewayContainerExec) ExecInContainer(ctx context.Context, name string, args ...string) (string, error) {
	return a.runtime.Exec(ctx, name, nil, args...)
}

// SandboxPortProbe waits until the sandbox is *deployable*, which is a login and
// not merely a listening port: Docker's proxy accepts a TCP connection before sshd
// has finished starting. It is a field on the request so the lifecycle is testable
// without a container: production runs the login, tests do not have to.
type SandboxPortProbe func(ctx context.Context, host string, port int, timeout time.Duration) error

// SandboxRequest is one `govard sandbox` invocation.
type SandboxRequest struct {
	ProjectRoot string
	ProjectName string
	Profile     string
	DocRoot     string
	// ReshapeDocRoot makes `up` re-shape the current path of a container that
	// already exists. It separates "make this target available" — the reuse half
	// of `up`, which is also how the mirror is refreshed before a new revision is
	// deployed — from "delete whatever is being served and lay the directory out
	// again", which `reset` exists for and which an operator asks for by naming a
	// shape.
	//
	// Without the guard, every `up` re-shaped the current path, and the default
	// shape is a symlink to `releases/0`: running `up` on a live in-place target
	// replaced the deployed application with a dangling link and every request
	// answered "File not found". Found on a real sandbox right after a successful
	// deploy, when the next revision was committed and `up` was run to refresh
	// the mirror.
	ReshapeDocRoot bool
	// ProfileExplicit records that the operator named a profile on the command
	// line rather than accepting the default. `--profile` has a default, so a
	// reused container cannot tell "no preference" from "asked for the default
	// one" by the value alone — and only an explicit choice may be refused when
	// it disagrees with the container that exists.
	ProfileExplicit bool
	Layout          string
	RemoteName      string
	// PHP is the series the image must provide, for example "8.4". Empty keeps
	// the base image's own version.
	PHP string
	// WebRoot is where inside the served path the web server serves from, from the
	// project's `stack.web_root` (`/pub` for a storefront served from a subdirectory). Empty serves the served path
	// itself, which is what a project with no web root has.
	WebRoot string
	// Requirements come from the framework recipe the command layer resolved.
	Requirements SandboxRequirements
	// Repository is the local checkout the mirror is refreshed from. Empty
	// means the project root.
	Repository string
	Recreate   bool
	Purge      bool
	// NoSeed skips the snapshot: the sandbox starts empty (no DB, no media,
	// no env file) and records no derivation.
	NoSeed bool
	// Volumes makes `down` delete the derived data volumes as well as the
	// container. Without it `down` stops and removes the container and keeps
	// every volume, so a rehearsal can resume tomorrow.
	Volumes bool
	// SeedOrigin names the origin project the snapshot is taken from and
	// recorded under. Empty means "no derivation claimed".
	SeedOrigin string
	// SeedBlueprintRev is the origin blueprint revision at seed time. Nothing
	// stamps one today (there is no single revision source to read it from),
	// so the field stays empty and DerivedFrom keeps the gap honestly instead
	// of inventing a value. Reserved for the day a source exists.
	SeedBlueprintRev string
	// SeedOriginRunning gates the seed: a stopped origin is a refusal, never
	// a silent empty sandbox. The caller (cmd) resolves it from the origin
	// env before calling SandboxUp.
	SeedOriginRunning bool
	// SeedDB names the origin database to dump; the sandbox user/database of
	// the same names are created before the import.
	SeedDBContainer string
	SeedDBUser      string
	SeedDBPassword  string
	SeedDBName      string
	// SeedAppContainer, SeedMediaSource and SeedMediaTarget stream one media
	// tree (tar through the caller) from the origin app container into the
	// sandbox. Empty source skips the copy.
	SeedAppContainer string
	SeedMediaSource  string
	SeedMediaTarget  string
	// SeedEnvSource and SeedEnvTarget copy one env file through EnvRewriter.
	// A nil rewriter skips the file (the registry wiring in cmd supplies the
	// framework implementation; see Task 5).
	SeedEnvSource  string
	SeedEnvTarget  string
	SeedEnvMapping map[string]string
	// EnvRewriter rewrites the env file for the sandbox. Nil skips the file.
	EnvRewriter SeedEnvRewriter
	Out         io.Writer
	Probe       SandboxPortProbe
}

// SandboxState is what the sandbox commands report and what `down` needs to undo.
type SandboxState struct {
	Container string
	Image     string
	Profile   string
	PHP       string
	Port      int
	// WebPort is the published HTTP port, zero for a profile with no web tier.
	WebPort     int
	Running     bool
	Exists      bool
	RemoteName  string
	RemoteSet   bool
	MirrorPath  string
	KeyPath     string
	DeployPath  string
	CurrentPath string
	Packages    string
	// DerivedFrom records the origin project this sandbox was seeded from.
	// Nil means nothing was ever seeded (a --no-seed sandbox, or one built
	// before derivation existed): old states without it stay valid.
	DerivedFrom *DerivedFrom `json:"derived_from,omitempty"`
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
	Current    string
	DeployPath string
}

// SandboxDefaultPaths returns the layout every sandbox uses.
func SandboxDefaultPaths() SandboxPaths {
	return SandboxPaths{
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

	// Whether the container already exists decides what the flags mean, so it is
	// asked before anything is written from them: reusing a sandbox must describe
	// the sandbox that exists, not the one today's flags would create.
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
	if exists {
		profile, php, err = sandboxReusedRuntime(ctx, runtime, request, container, profile, php)
		if err != nil {
			return nil, err
		}
	}

	spec := SandboxSpec{
		Project:      request.ProjectName,
		Profile:      profile,
		PHP:          php,
		WebRoot:      request.WebRoot,
		Requirements: request.Requirements,
	}
	dockerfile, err := SandboxDockerfile(spec)
	if err != nil {
		return nil, err
	}
	dockerfilePath := SandboxDockerfilePath(request.ProjectRoot)
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", dockerfilePath, err)
	}
	// The files the Dockerfile copies travel beside it in the build context.
	// They are written every time, so a run after a `stack.web_root` change builds
	// the image the new definition describes rather than reusing the old one.
	for name, content := range SandboxBuildFiles(spec) {
		if err := os.WriteFile(filepath.Join(SandboxStateDir(request.ProjectRoot), name), []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}

	image, err := SandboxImageTag(spec)
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

	servesWeb := SandboxServesWeb(profile)

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
			PHP:         php,
			Web:         servesWeb,
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

	authorizedKeys := []string{key.PublicKey}
	_, gatewayPublicLine, gatewayKeyErr := gateway.EnsureSecondHopKey()
	if gatewayKeyErr != nil {
		fmt.Fprintf(request.out(), "note: could not prepare the SSH gateway key: %v\n", gatewayKeyErr)
	} else {
		authorizedKeys = append(authorizedKeys, gatewayPublicLine)
	}
	if err := installSandboxAuthorizedKey(ctx, runtime, container, authorizedKeys...); err != nil {
		return nil, err
	}

	if connected, netErr := runtime.EnsureNetworkConnected(ctx, container, govardProxyNetwork); netErr != nil {
		fmt.Fprintf(request.out(), "note: could not join the %s network for the SSH gateway: %v\n", govardProxyNetwork, netErr)
	} else if connected && gatewayKeyErr == nil {
		reg, regErr := gateway.Load()
		if regErr != nil {
			fmt.Fprintf(request.out(), "note: could not load the SSH gateway registry: %v\n", regErr)
		} else if gwUser, ok := gateway.RouteUsername(request.ProjectName); !ok {
			fmt.Fprintf(request.out(), "note: project name %q cannot be an SSH gateway username (a-z, 0-9, -); skipping gateway registration\n", request.ProjectName)
		} else {
			if gwUser != request.ProjectName {
				fmt.Fprintf(request.out(), "SSH gateway target: %s@ssh.govard.test (project %q)\n", gwUser, request.ProjectName)
			}
			if addErr := reg.AddTarget(gwUser, gateway.Target{
				Container:  container,
				TargetUser: SandboxUser,
				Project:    request.ProjectName,
			}); addErr != nil {
				fmt.Fprintf(request.out(), "note: could not register the SSH gateway target: %v\n", addErr)
			} else if saveErr := reg.Save(); saveErr != nil {
				fmt.Fprintf(request.out(), "note: could not save the SSH gateway registry: %v\n", saveErr)
			} else if ensureErr := gateway.EnsureTargetAccount(ctx, gatewayContainerExec{runtime: runtime}, gatewayContainerName, gwUser); ensureErr != nil {
				fmt.Fprintf(request.out(), "note: could not create the SSH gateway account: %v\n", ensureErr)
			}
		}
	}
	// Shape a target that is being created, and one the operator explicitly asked
	// to re-shape. An existing target is left as the last deploy left it: `up` is
	// also how a stopped sandbox is started and how the mirror is refreshed
	// before the next revision is deployed, and neither may cost the application
	// currently being served.
	if !exists || request.ReshapeDocRoot {
		if err := prepareSandboxDocRoot(ctx, runtime, container, docRoot); err != nil {
			return nil, err
		}
	}

	probe := request.Probe
	if probe == nil {
		keyPath := key.PrivatePath
		probe = func(ctx context.Context, host string, port int, timeout time.Duration) error {
			return DialSandboxSSH(ctx, git, keyPath, host, port, timeout)
		}
	}
	if err := probe(ctx, "127.0.0.1", port, 30*time.Second); err != nil {
		return nil, err
	}

	// The web port is read back, not assumed: Docker chooses it, and a rehearsal
	// whose `verify.url` named the wrong port would fail at the last step of every
	// deploy for a reason that has nothing to do with the release. It is read
	// before the seed (not after) because the seed's base_url defaults to it.
	webPort := 0
	if servesWeb {
		if published, err := runtime.PublishedPort(ctx, container, sandboxWebPort); err == nil {
			webPort = published
		}
	}

	// The snapshot runs once, on a fresh container, against the running
	// services — never upfront (the container would not exist yet) and never
	// on reuse (an existing sandbox keeps its data; --recreate is the
	// refresh, and it arrives here as a fresh container). An empty SeedOrigin
	// means plain `sandbox up` with no derivation: no seed, no record.
	derived := NewDerivedFrom(request.SeedOrigin, request.SeedBlueprintRev)
	seeded := false
	if request.SeedOrigin != "" && !request.NoSeed && !exists {
		if err := runSandboxSeed(ctx, runtime, request.out(), container, webPort, request); err != nil {
			return nil, err
		}
		seeded = true
	}

	paths := SandboxDefaultPaths()

	// The image the container was actually built from. A reused sandbox must be
	// described as what it is rather than as the tag today's flags would build:
	// `up` with no flags on a `full` sandbox printed the default profile's tag,
	// which the container does not have and which the operator cannot act on.
	stateImage := image
	if exists {
		if actual, err := runtime.ContainerImage(ctx, container); err == nil && actual != "" {
			stateImage = actual
		}
	}

	packages, _ := runtime.ImageFile(ctx, image, SandboxPackagesTxt)
	state := &SandboxState{
		Container:   container,
		Image:       stateImage,
		Profile:     profile,
		PHP:         php,
		Port:        port,
		WebPort:     webPort,
		Running:     true,
		Exists:      true,
		RemoteName:  request.remoteName(),
		RemoteSet:   true,
		MirrorPath:  mirror,
		KeyPath:     key.PrivatePath,
		DeployPath:  paths.DeployPath,
		CurrentPath: paths.Current,
		Packages:    strings.TrimSpace(packages),
	}
	// Only a run that seeded records a derivation: reuse keeps whatever the
	// seeding run recorded (or nothing, for pre-derivation sandboxes), and a
	// --no-seed run claims no origin at all.
	if seeded {
		state.DerivedFrom = &derived
	}
	return state, nil
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

// SandboxRemoteConfig is the remote a running sandbox resolves to. It is
// exported because it is the contract between the sandbox and the deploy
// pipeline, and both are worth asserting on independently.
func SandboxRemoteConfig(profile, php string, port, webPort int, paths SandboxPaths, key SandboxKeyPair) engine.RemoteConfig {
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
	remote := engine.RemoteConfig{
		Host:      "127.0.0.1",
		Port:      port,
		User:      SandboxUser,
		Path:      paths.Current,
		Sandbox:   true,
		Protected: engine.BoolPtr(false),
		Auth: engine.RemoteAuth{
			Method:  engine.RemoteAuthMethodKeyfile,
			KeyPath: keyPath,
		},
		Deploy: &engine.DeployConfig{
			Repository: SandboxRepoPath,
			DeployPath: paths.DeployPath,
			Settings:   settings,
		},
	}

	// A sandbox that serves the release is the only place the HTTP half of
	// `deploy:verify` can be rehearsed, and a URL nobody wrote leaves it switched
	// off: the check reports the files in place and says nothing about whether the
	// application answers. The remote carries it as a per-remote override, which
	// is exactly what `deploy.verify.url` on a remote means.
	if webPort > 0 {
		remote.Deploy.Verify.URL = fmt.Sprintf("http://127.0.0.1:%d/", webPort)
	}
	return remote
}

// installSandboxAuthorizedKey installs the generated public key(s) for the
// container's deployer user (the sandbox's own key, and -- when available --
// the gateway's second-hop key). The image holds no key material, which is
// what keeps it cacheable across projects.
func installSandboxAuthorizedKey(ctx context.Context, runtime SandboxRuntime, container string, publicKeys ...string) error {
	joined := strings.Join(publicKeys, "\n")
	script := "umask 077 && mkdir -p " + SandboxHome + "/.ssh && cat > " + SandboxHome + "/.ssh/authorized_keys" +
		" && chown -R " + SandboxUser + ":" + SandboxUser + " " + SandboxHome + "/.ssh"
	// The container may accept exec a moment before it accepts an interactive
	// one; retry briefly rather than racing the entrypoint.
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if _, err = runtime.Exec(ctx, container, []byte(joined+"\n"), "sh", "-c", script); err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("install the sandbox public key(s) in %s: %w", container, err)
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

// DialSandboxSSH waits until a real login to the sandbox succeeds.
//
// The port answering a TCP connection is not the same thing as the sandbox being
// deployable: Docker's proxy accepts before sshd has finished starting, so a
// readiness check that only dials lets `up` return at the moment a deploy will
// fail. Observed under load — the integration suite running next to a live
// rehearsal — as the first step of the deploy reporting
// `the sandbox container behind remote "sandbox" is not running` (exit 255) for a
// container that was up and answered seconds later.
//
// The login is the deploy's own: batch mode, the sandbox key, the published port,
// and `true` as the command. Each attempt gets its own short timeout so a stalled
// handshake cannot consume the whole budget.
func DialSandboxSSH(ctx context.Context, runner Runner, keyPath, host string, port int, timeout time.Duration) error {
	if runner == nil {
		runner = LocalRunner{}
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	command := SandboxSSHCommand(keyPath, host, port)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		attempt, cancel := context.WithTimeout(ctx, sandboxProbeAttempt)
		_, err := runner.Run(attempt, command, RunOptions{Timeout: sandboxProbeAttempt})
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("the sandbox sshd did not answer on %s within %s: %w", address, timeout, lastErr)
}

// sandboxProbeAttempt bounds one login attempt of the readiness check.
const sandboxProbeAttempt = 5 * time.Second

// SandboxSSHCommand is the non-interactive login the readiness check uses: the
// options `sandbox ssh` offers, with batch mode and a command instead of a tty, so
// it can never block on a prompt or a host-key question.
func SandboxSSHCommand(keyPath, host string, port int) string {
	return "ssh -o BatchMode=yes -o LogLevel=ERROR -o StrictHostKeyChecking=no" +
		" -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5" +
		" -i " + conventions.ShellQuote(keyPath) +
		" -p " + strconv.Itoa(port) +
		" " + conventions.ShellQuote(SandboxUser+"@"+host) + " true"
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
		DeployPath:  sandboxRemoteDeployPath(config.Remote),
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
	// The web port is reported the same way, so `sandbox status` answers "where do
	// I point a browser at this rehearsal" without the operator reading `up`'s
	// output from a terminal that has since scrolled away.
	if SandboxServesWeb(state.Profile) {
		if webPort, err := runtime.PublishedPort(ctx, container, sandboxWebPort); err == nil {
			state.WebPort = webPort
		}
	}
	return state, nil
}

// sandboxReusedRuntime reports the profile and PHP series a sandbox container
// already has, and refuses to relabel it.
//
// A reused container is what it is: its image was built for one profile and one
// PHP series, and that is what the sandbox's remote has to declare. Without this,
// `up` with no flags rewrote the remote without `php_version` — after which
// `sandbox status` reported no series for a php-8.4 image, and an artifact deploy
// lost the check that catches a series mismatch — while `up --php 8.3` on a
// php-8.4 container relabelled the target without touching it.
//
// The profile is read from the container's own label and the series from the
// sandbox remote the creation wrote, both of which describe what exists.
// `--recreate` is the way to get a different one, and the refusal says so.
// sandboxReusedRuntime reports the profile and PHP series a sandbox container
// already has, and refuses to relabel it.
//
// A reused container is what it is: its image was built for one profile and one
// PHP series, and that is what the sandbox remote has to describe — the profile
// decides which settings the remote carries (a PHP binary, the web tier's verify
// URL) and the series is what an artifact deploy is checked against. Reading them
// from today's flags instead meant `up` with no flags erased `php_version` from a
// php-8.4 sandbox and described a `full` container as the default profile, with a
// computed image tag; on a host that did not have that tag it went further and
// built the image, and `--recreate` would then hand back a container without the
// services the first one had.
//
// The container's label is the record of its profile and the sandbox remote is the
// record of its series, both of which `sandbox status` already reads. An explicit
// flag that disagrees is refused rather than silently ignored: `--recreate` is the
// way to get a different one, and the refusal says so.
func sandboxReusedRuntime(ctx context.Context, runtime SandboxRuntime, request SandboxRequest, container, profile, php string) (string, string, error) {
	if declared := containerLabelOrEmpty(ctx, runtime, container, sandboxProfileLabel); declared != "" {
		if request.ProfileExplicit && !strings.EqualFold(strings.TrimSpace(request.Profile), declared) {
			return profile, php, fmt.Errorf(
				"the sandbox container %s was built for the %s profile; changing it means a new image, so run `govard sandbox up --profile %s --recreate`",
				container, declared, strings.TrimSpace(request.Profile))
		}
		profile = declared
	}
	config, err := LoadSandboxRemote(request.ProjectRoot, request.remoteName())
	if err != nil || !config.RemoteSet {
		return profile, php, nil
	}
	declaredPHP := sandboxRemotePHP(config.Remote)
	if php != "" && declaredPHP != "" && declaredPHP != php {
		return profile, php, fmt.Errorf(
			"the sandbox container ships PHP %s; changing it to %s means a new image, so run `govard sandbox up --php %s --recreate`",
			declaredPHP, php, php)
	}
	if php == "" {
		php = declaredPHP
	}
	return profile, php, nil
}

// sandboxRemotePHP is the series the sandbox remote declares, empty when it
// declares none.
func sandboxRemotePHP(remote engine.RemoteConfig) string {
	if remote.Deploy == nil {
		return ""
	}
	return settingsString(remote.Deploy.Settings, "php_version")
}

// sandboxRemoteDeployPath is the layout root the sandbox remote declares,
// empty when it declares none (a remote written before the nested form).
func sandboxRemoteDeployPath(remote engine.RemoteConfig) string {
	if remote.Deploy == nil {
		return ""
	}
	return remote.Deploy.DeployPath
}

// SandboxDown removes the container, so a stale sandbox cannot be deployed
// to by accident. The remote is synthetic — resolved from live Docker state,
// never written — so there is nothing to delete from the configuration.
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
	if request.ProjectName != "" {
		if reg, regErr := gateway.Load(); regErr == nil {
			reg.RemoveTarget(request.ProjectName)
			if err := reg.Save(); err != nil {
				fmt.Fprintf(request.out(), "note: could not prune the SSH gateway target: %v\n", err)
			}
		}
	}
	// Volumes are data, not runtime: plain `down` keeps them so a rehearsal
	// resumes tomorrow. Only an explicit --volumes deletes the derived
	// project's named volumes (matched by compose project label, so no caller
	// has to know their names).
	if request.Volumes && request.ProjectName != "" {
		if err := runtime.RemoveVolumesByLabel(ctx, "com.docker.compose.project", request.ProjectName); err != nil {
			return nil, err
		}
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
		return nil, fmt.Errorf("the sandbox container %s does not exist; run `govard sandbox up` first", state.Container)
	}
	if !state.Running {
		return nil, fmt.Errorf("the sandbox container %s is not running; run `govard sandbox up` first", state.Container)
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
	// The state learned the key path when the sandbox was created; a project
	// root alone re-derives the same default. Preferring the record keeps `ssh`
	// working when the two ever disagree.
	keyPath := filepath.Join(SandboxStateDir(request.ProjectRoot), SandboxKeyName)
	if state != nil && strings.TrimSpace(state.KeyPath) != "" {
		keyPath = state.KeyPath
	}
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

// LoadSandboxRemote resolves the project's sandbox remote from live Docker
// state. It never writes: `status` and `ssh` must work on a project whose
// sandbox was created by an earlier session.
func LoadSandboxRemote(projectRoot, name string) (SandboxConfig, error) {
	config, _, err := engine.LoadConfigFromDir(projectRoot, false)
	if err != nil {
		return SandboxConfig{}, fmt.Errorf("read the project configuration: %w", err)
	}
	result := SandboxConfig{ProjectName: config.ProjectName, Remotes: config.Remotes}
	if strings.ToLower(strings.TrimSpace(name)) != SandboxRemoteName {
		// Only "sandbox" is ever resolved synthetically; anything else keeps
		// reading .govard.local.yml as it always has (this helper is also
		// used, in principle, with whatever request.remoteName() returns,
		// which defaults to "sandbox" but can be overridden).
		remote, ok := config.Remotes[name]
		result.Remote, result.RemoteSet = remote, ok
		return result, nil
	}
	remote, liveness, err := resolveSyntheticSandboxRemoteFn(context.Background(), config.ProjectName)
	if liveness == SandboxLivenessRunning && err == nil {
		result.Remote, result.RemoteSet = remote, true
	}
	return result, nil
}

// sandboxProfileLabel is the container label the profile is recorded in. It is
// the container's own record of how it was built, which is what `status` should
// report rather than a value recomputed from today's configuration.
const sandboxProfileLabel = "govard.sandbox.profile"

// sandboxPHPLabel is the container label the PHP series is recorded in, for
// the same reason sandboxProfileLabel exists: the container's own record,
// read back by ResolveSyntheticSandboxRemote and sandboxReusedRuntime.
const sandboxPHPLabel = "govard.sandbox.php"

func containerLabelOrEmpty(ctx context.Context, runtime SandboxRuntime, container, label string) string {
	value, err := runtime.ContainerLabel(ctx, container, label)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}
