package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/gateway"
	"govard/internal/runtime"
)

// The sandbox ships three profiles; a fourth is a typo, not a new size.
func TestSandboxProfilesAreAClosedSet(t *testing.T) {
	for _, profile := range []string{"", "basic", "php", "full", "BASIC", " php "} {
		if _, err := deploy.ValidateSandboxProfile(profile); err != nil {
			t.Errorf("ValidateSandboxProfile(%q) = %v, want no error", profile, err)
		}
	}
	for _, profile := range []string{"minimal", "php8", "docker"} {
		if _, err := deploy.ValidateSandboxProfile(profile); err == nil {
			t.Errorf("ValidateSandboxProfile(%q) accepted an unknown profile", profile)
		}
	}
}

func TestSandboxDockerfileContainsWhatTheProfilePromises(t *testing.T) {
	cases := []struct {
		profile string
		want    []string
		absent  []string
	}{
		{
			profile: deploy.SandboxProfileBasic,
			want:    []string{"openssh-server", "rsync", "git", "deployer"},
			absent:  []string{"php-cli", "mariadb-server", "redis-server"},
		},
		{
			profile: deploy.SandboxProfilePHP,
			want:    []string{"openssh-server", "rsync", "git", "php-cli", "composer", "nodejs", "npm"},
			absent:  []string{"mariadb-server"},
		},
		{
			profile: deploy.SandboxProfileFull,
			want:    []string{"php-cli", "mariadb-server"},
			absent:  []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.profile, func(t *testing.T) {
			dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: tc.profile})
			if err != nil {
				t.Fatalf("render %s: %v", tc.profile, err)
			}
			for _, want := range tc.want {
				if !strings.Contains(dockerfile, want) {
					t.Errorf("the %s profile must provide %q", tc.profile, want)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(dockerfile, absent) {
					t.Errorf("the %s profile must not provide %q", tc.profile, absent)
				}
			}
		})
	}
}

func TestSandboxDeployerIsNotALockedAccount(t *testing.T) {
	// sshd refuses a locked account before it ever looks at authorized_keys, so
	// a `useradd` that leaves the password field as `!` produces a sandbox no
	// deploy can reach. The `*` field means no password can match, which is
	// exactly right with PasswordAuthentication off.
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileBasic})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(dockerfile, "-p '*'") {
		t.Fatal("the deployer account must be created with a password field sshd does not treat as locked")
	}
}

func TestSandboxMirrorIsASafeGitDirectory(t *testing.T) {
	// The mirror is bind-mounted from the host, so the container's deployer is
	// usually not its owner — git then refuses with "detected dubious
	// ownership" and the deploy cannot materialise a revision. It only shows up
	// when the host uid differs from the image's deployer uid, which is exactly
	// the case on a CI runner.
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileBasic})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(dockerfile, "git config --system --add safe.directory "+deploy.SandboxRepoPath) {
		t.Fatalf("the mounted mirror must be declared a safe git directory, got:\n%s", dockerfile)
	}
}

func TestSandboxDockerfileCarriesTheFrameworkRequirements(t *testing.T) {
	requirements := deploy.SandboxRequirements{
		Packages:   []string{"libxslt1-dev"},
		Extensions: []string{"bcmath", "intl"},
		Services:   []string{"mariadb"},
	}
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfilePHP, Requirements: requirements})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"libxslt1-dev", "php-bcmath", "php-intl"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("the Dockerfile must carry the framework's %q requirement", want)
		}
	}
	// The service list also carries the sandbox's own web tier, so the assertion
	// is that the recipe's service is *named in it*, not that it is alone.
	services := sandboxServicesLine(t, dockerfile)
	if !strings.Contains(services, "mariadb") {
		t.Errorf("GOVARD_SANDBOX_SERVICES = %q, want the framework's mariadb service", services)
	}
}

func TestSandboxDockerfileIsByteStable(t *testing.T) {
	requirements := deploy.SandboxRequirements{
		Extensions: []string{"intl", "bcmath"},
		Services:   []string{"redis", "mariadb"},
	}
	first, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileFull, Requirements: requirements})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	second, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileFull, Requirements: requirements})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if first != second {
		t.Fatal("two renders of the same spec differ, so the image tag would never be stable")
	}
	// The order requirements arrive in must not change the image either: two
	// recipes listing the same extensions in a different order are the same
	// sandbox.
	shuffled, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile: deploy.SandboxProfileFull,
		Requirements: deploy.SandboxRequirements{
			Extensions: []string{"bcmath", "intl"},
			Services:   []string{"mariadb", "redis"},
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if first != shuffled {
		t.Fatal("the Dockerfile depends on the order the requirements are listed in")
	}
}

func TestSandboxImageTagTracksTheDockerfile(t *testing.T) {
	spec := deploy.SandboxSpec{Project: "sample-project", Profile: deploy.SandboxProfilePHP}
	tag, err := deploy.SandboxImageTag(spec)
	if err != nil {
		t.Fatalf("tag: %v", err)
	}
	if !strings.HasPrefix(tag, "govard-sandbox:sample-project-php-") {
		t.Fatalf("tag = %q, want a project/profile prefix", tag)
	}
	same, err := deploy.SandboxImageTag(spec)
	if err != nil {
		t.Fatalf("tag: %v", err)
	}
	if tag != same {
		t.Fatalf("the tag is not deterministic: %q vs %q", tag, same)
	}

	other, err := deploy.SandboxImageTag(deploy.SandboxSpec{Project: "sample-project", Profile: deploy.SandboxProfileFull})
	if err != nil {
		t.Fatalf("tag: %v", err)
	}
	if other == tag {
		t.Fatal("two profiles share an image tag")
	}

	withRequirements, err := deploy.SandboxImageTag(deploy.SandboxSpec{
		Project:      "sample-project",
		Profile:      deploy.SandboxProfilePHP,
		Requirements: deploy.SandboxRequirements{Extensions: []string{"intl"}},
	})
	if err != nil {
		t.Fatalf("tag: %v", err)
	}
	if withRequirements == tag {
		t.Fatal("a framework requirement did not change the image tag")
	}
}

func TestSandboxImageTagIsADockerLegalName(t *testing.T) {
	// Docker refuses a repository name with uppercase or spaces, and a project
	// name comes from `.govard.yml` untouched.
	tag, err := deploy.SandboxImageTag(deploy.SandboxSpec{Project: "My Project", Profile: deploy.SandboxProfileBasic})
	if err != nil {
		t.Fatalf("tag: %v", err)
	}
	if !strings.HasPrefix(tag, "govard-sandbox:") {
		t.Fatalf("tag = %q, want the govard-sandbox:<slug>-<profile>-<hash> form (no \"deploy\")", tag)
	}
	for _, r := range tag {
		legal := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' || r == '/'
		if !legal {
			t.Fatalf("tag %q contains %q, which Docker rejects", tag, r)
		}
	}
}

func TestMagento2RecipeAsksForTheSandboxItNeeds(t *testing.T) {
	// Through the registry the command layer uses, so the test sees the recipe
	// a real deploy would run.
	recipe, ok := frameworks.DeployRecipe("magento2")
	if !ok {
		t.Fatal("magento2 has no deploy recipe")
	}
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfilePHP, Requirements: recipe.Sandbox})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"php-intl", "php-bcmath", "php-soap", "php-gd", "php-zip", "php-mysql", "mariadb"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("the application recipe's sandbox requirements are missing %q", want)
		}
	}
}

// fakeSandboxRuntime records what the sandbox asked the container runtime to do
// and answers from a script, so the whole command is exercised without Docker.
type fakeSandboxRuntime struct {
	calls   [][]string
	answers map[string]string
	fail    map[string]string
	streams []string
	// labels answers container label queries by label name.
	labels map[string]string
}

func newFakeSandboxRuntime() *fakeSandboxRuntime {
	return &fakeSandboxRuntime{answers: map[string]string{}, fail: map[string]string{}}
}

// containerProfile makes the fake describe a container built for one profile,
// which is what the real label says. Labels are answered by name rather than
// through the substring table: the generic `inspect --format {{index .Config.Labels`
// entry answers every label query, so a test that needs one label answered
// precisely cannot express that with a longer marker.
func (f *fakeSandboxRuntime) containerProfile(profile string) *fakeSandboxRuntime {
	if f.labels == nil {
		f.labels = map[string]string{}
	}
	f.labels["govard.sandbox.profile"] = profile + "\n"
	return f
}

// containerPHP makes the fake answer the container's PHP series label, which
// is what `up` labels the container it creates with. An empty series means a
// container built without one, so the synthetic resolution declares none.
func (f *fakeSandboxRuntime) containerPHP(series string) *fakeSandboxRuntime {
	if f.labels == nil {
		f.labels = map[string]string{}
	}
	f.labels["govard.sandbox.php"] = series + "\n"
	return f
}

// labelQuery is the label name in an `inspect --format {{index .Config.Labels "x"}}`
// invocation, empty for any other command.
func labelQuery(key string) string {
	const marker = "{{index .Config.Labels \""
	start := strings.Index(key, marker)
	if start < 0 {
		return ""
	}
	rest := key[start+len(marker):]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func (f *fakeSandboxRuntime) run(ctx context.Context, request deploy.SandboxCommand) (string, error) {
	f.calls = append(f.calls, request.Args)
	key := strings.Join(request.Args, " ")
	for marker, message := range f.fail {
		if strings.Contains(key, marker) {
			return "", &deploy.CommandError{Command: "docker " + key, ExitCode: 1, Stderr: message}
		}
	}
	if name := labelQuery(key); name != "" {
		if answer, ok := f.labels[name]; ok {
			if request.Out != nil {
				fmt.Fprint(request.Out, answer)
			}
			return answer, nil
		}
	}
	for marker, answer := range f.answers {
		if strings.Contains(key, marker) {
			if request.Out != nil {
				fmt.Fprint(request.Out, answer)
			}
			return answer, nil
		}
	}
	if request.Out != nil {
		fmt.Fprint(request.Out, key+"\n")
		f.streams = append(f.streams, key)
	}
	_ = ctx
	return "", nil
}

func (f *fakeSandboxRuntime) has(fragment string) bool {
	for _, call := range f.calls {
		if strings.Contains(strings.Join(call, " "), fragment) {
			return true
		}
	}
	return false
}

func (f *fakeSandboxRuntime) hasArg(flag, value string) bool {
	for _, call := range f.calls {
		for i, arg := range call {
			if arg == flag && i+1 < len(call) && call[i+1] == value {
				return true
			}
		}
	}
	return false
}

func (f *fakeSandboxRuntime) call(fragment string) []string {
	for _, call := range f.calls {
		if strings.Contains(strings.Join(call, " "), fragment) {
			return call
		}
	}
	return nil
}

func TestSandboxRunRequestPublishesADockerChosenPort(t *testing.T) {
	fake := newFakeSandboxRuntime()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	err := runtime.RunContainer(context.Background(), deploy.SandboxRunRequest{
		Name:        "govard-sample-project-sandbox",
		Image:       "govard-sandbox:sample-project-basic-abc123",
		MirrorPath:  "/tmp/sample/.govard/sandbox/repo.git",
		ProjectName: "sample-project",
	})
	if err != nil {
		t.Fatalf("run container: %v", err)
	}

	call := fake.call("run")
	if call == nil {
		t.Fatal("no docker run was issued")
	}
	joined := strings.Join(call, " ")
	// Docker allocates the host port; guessing one collides on a busy machine.
	if !strings.Contains(joined, "--publish "+deploy.SandboxPortBinding) {
		t.Fatalf("the run must publish an ephemeral loopback port, got %v", call)
	}
	if !strings.Contains(joined, "type=bind,source=/tmp/sample/.govard/sandbox/repo.git,target="+deploy.SandboxRepoPath+",readonly") {
		t.Fatalf("the mirror must be mounted read-only at %s, got %v", deploy.SandboxRepoPath, call)
	}
	if !strings.Contains(joined, "--name govard-sample-project-sandbox") {
		t.Fatalf("the container name is missing: %v", call)
	}
}

func TestSandboxRunContainerLabelsThePHPSeries(t *testing.T) {
	fake := newFakeSandboxRuntime()
	runtime := deploy.NewDockerCLIForTest(fake.run)
	if err := runtime.RunContainer(context.Background(), deploy.SandboxRunRequest{
		Name:        "govard-sample-sandbox-abc123",
		Image:       "govard-sandbox:sample-full-abc123",
		MirrorPath:  "/tmp/mirror",
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfileFull,
		PHP:         "8.4",
	}); err != nil {
		t.Fatalf("run container: %v", err)
	}
	if !fake.hasArg("--label", "govard.sandbox.php=8.4") {
		t.Fatal("the container must carry the PHP series as a label, the way profile and project already are")
	}
}

func TestSandboxRuntimeReadsBackThePublishedPort(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.answers["port govard-sample-project-sandbox 22/tcp"] = "127.0.0.1:49153\n"
	runtime := deploy.NewDockerCLIForTest(fake.run)

	port, err := runtime.PublishedPort(context.Background(), "govard-sample-project-sandbox", deploy.SandboxSSHPort)
	if err != nil {
		t.Fatalf("published port: %v", err)
	}
	if port != 49153 {
		t.Fatalf("port = %d, want 49153", port)
	}
}

func TestSandboxRuntimeReportsAnUnpublishedPort(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.fail["port "] = "Error: No public port '22/tcp' published"
	runtime := deploy.NewDockerCLIForTest(fake.run)

	if _, err := runtime.PublishedPort(context.Background(), "sandbox", deploy.SandboxSSHPort); err == nil {
		t.Fatal("want an error when the port is not published")
	}
}

func TestSandboxRuntimeDistinguishesAMissingImage(t *testing.T) {
	absent := newFakeSandboxRuntime()
	absent.fail["image inspect"] = "Error response from daemon: No such image: govard-sandbox:absent"
	exists, err := deploy.NewDockerCLIForTest(absent.run).ImageExists(context.Background(), "govard-sandbox:absent")
	if err != nil {
		t.Fatalf("image exists: %v", err)
	}
	if exists {
		t.Fatal("an image that is not local must report false, not an error")
	}

	present := newFakeSandboxRuntime()
	present.answers["image inspect"] = "sha256:abc\n"
	exists, err = deploy.NewDockerCLIForTest(present.run).ImageExists(context.Background(), "govard-sandbox:present")
	if err != nil {
		t.Fatalf("image exists: %v", err)
	}
	if !exists {
		t.Fatal("a local image must report true")
	}
}

func TestSandboxRuntimeSurfacesStderrFromAFailedCommand(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.fail["build"] = "the daemon is not running"
	runtime := deploy.NewDockerCLIForTest(fake.run)

	err := runtime.BuildImage(context.Background(), deploy.SandboxBuildRequest{
		Image:      "govard-sandbox:sample-project-basic",
		Dockerfile: "/tmp/sample/.govard/sandbox/Dockerfile",
		Context:    "/tmp/sample/.govard/sandbox",
	})
	if err == nil {
		t.Fatal("want an error when the build fails")
	}
	if !strings.Contains(err.Error(), "the daemon is not running") {
		t.Fatalf("the failure must carry the runtime's own message, got %q", err.Error())
	}
}

func TestSandboxRuntimeStreamsTheBuildOutput(t *testing.T) {
	fake := newFakeSandboxRuntime()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	var out strings.Builder
	err := runtime.BuildImage(context.Background(), deploy.SandboxBuildRequest{
		Image:      "govard-sandbox:sample-project-basic",
		Dockerfile: "/tmp/sample/.govard/sandbox/Dockerfile",
		Context:    "/tmp/sample/.govard/sandbox",
		Out:        &out,
	})
	if err != nil {
		t.Fatalf("build image: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("a build's progress must reach the operator, not a discarded buffer")
	}
}

func TestSandboxRuntimeReadsTheRecordedPackageVersions(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.answers["--entrypoint cat"] = "rsync=3.2.7-1\n"
	runtime := deploy.NewDockerCLIForTest(fake.run)

	versions, err := runtime.ImageFile(context.Background(), "govard-sandbox:sample", deploy.SandboxPackagesTxt)
	if err != nil {
		t.Fatalf("image file: %v", err)
	}
	if !strings.Contains(versions, "rsync=3.2.7-1") {
		t.Fatalf("recorded versions = %q", versions)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func TestSandboxKeyIsAnOpenSSHKeyPair(t *testing.T) {
	dir := t.TempDir()
	pair, err := deploy.EnsureSandboxKey(dir)
	if err != nil {
		t.Fatalf("ensure key: %v", err)
	}
	if !strings.HasPrefix(pair.PublicKey, "ssh-ed25519 ") {
		t.Fatalf("public key = %q, want an ssh-ed25519 line", pair.PublicKey)
	}
	info, err := os.Stat(pair.PrivatePath)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %v, want 0600", info.Mode().Perm())
	}
	private, err := os.ReadFile(pair.PrivatePath)
	if err != nil {
		t.Fatalf("read private key: %v", err)
	}
	if !strings.HasPrefix(string(private), "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Fatalf("private key is not in the OpenSSH format:\n%s", private)
	}
	if !strings.Contains(string(private), "-----END OPENSSH PRIVATE KEY-----") {
		t.Fatal("private key has no end marker")
	}

	// A second call reuses the pair rather than minting one the container's
	// authorized_keys no longer matches.
	again, err := deploy.EnsureSandboxKey(dir)
	if err != nil {
		t.Fatalf("ensure key again: %v", err)
	}
	if again.PublicKey != pair.PublicKey {
		t.Fatal("the second call generated a different key")
	}
}

func TestSandboxKeyIsReadableByOpenSSH(t *testing.T) {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed; the live sandbox run covers this")
	}
	dir := t.TempDir()
	pair, err := deploy.EnsureSandboxKey(dir)
	if err != nil {
		t.Fatalf("ensure key: %v", err)
	}
	output, err := exec.Command(sshKeygen, "-y", "-f", pair.PrivatePath).Output()
	if err != nil {
		t.Fatalf("ssh-keygen refused the generated key: %v", err)
	}
	// ssh-keygen prints `type base64 comment`; the comment is the key's own.
	parts := strings.SplitN(pair.PublicKey, " ", 3)
	expected := parts[0] + " " + parts[1]
	derived := strings.TrimSpace(string(output))
	if !strings.HasPrefix(derived, expected) {
		t.Fatalf("ssh-keygen derived\n%s\nwant a key starting with\n%s", derived, expected)
	}
}

// sandboxProject writes a minimal project that a sandbox can be attached to.
func sandboxProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, "app.txt"), "deployed\n")
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = root
		command.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("add", ".")
	run("commit", "-q", "-m", "initial")
	return root
}

// sandboxFake answers the lifecycle the way a healthy container runtime would.
func sandboxFake() *fakeSandboxRuntime {
	fake := newFakeSandboxRuntime()
	fake.answers["version --format"] = "29.8.0\n"
	fake.answers["port "] = "127.0.0.1:49153\n"
	fake.answers["inspect --format {{.State.Running}}"] = "true\n"
	fake.answers["inspect --format {{.Id}}"] = "abc123\n"
	fake.answers["inspect --format {{.Config.Image}}"] = "govard-sandbox:sample-project-php-deadbeef\n"
	fake.answers["inspect --format {{index .Config.Labels"] = "php\n"
	fake.answers["--entrypoint cat"] = "rsync=3.2.7-1\n"
	return fake
}

// absentContainerFake makes `docker inspect` behave as it does for a name that
// does not exist, which is how the lifecycle learns whether to create or reuse.
func absentContainerFake() *fakeSandboxRuntime {
	fake := sandboxFake()
	fake.fail["inspect"] = "Error: No such container"
	return fake
}

func TestSandboxUpCreatesAResolvableContainer(t *testing.T) {
	root := sandboxProject(t)
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"

	state, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if !fake.has("build --file") {
		t.Fatal("a missing image must be built")
	}
	if !fake.has("run --detach") {
		t.Fatal("a missing container must be created")
	}
	if state.Port != 49153 {
		t.Fatalf("port = %d, want the port Docker published", state.Port)
	}
	if !fake.has("--interactive") {
		t.Fatal("the public key must be installed with an interactive exec")
	}

	// The container `up` created answers the probes now: drop the absence the
	// fake was scripted with so the resolution below observes a running
	// sandbox rather than the missing one `up` started from.
	delete(fake.fail, "inspect")
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	remote, liveness, err := deploy.ResolveSyntheticSandboxRemoteForTest(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, root, "sample-project")
	if err != nil {
		t.Fatalf("resolve the sandbox remote: %v", err)
	}
	if liveness != deploy.SandboxLivenessRunning {
		t.Fatalf("liveness = %q, want running right after up", liveness)
	}
	if remote.Port != 49153 || remote.Deploy.Repository != deploy.SandboxRepoPath {
		t.Fatalf("remote = %+v, want the published port and the mounted mirror", remote)
	}
	if !remote.Sandbox {
		t.Fatal("the remote must be marked as a sandbox")
	}
	if _, err := os.Stat(deploy.SandboxMirrorPath(root)); err != nil {
		t.Fatalf("the mirror was not created: %v", err)
	}
}

func TestSandboxUpNeverTouchesTheLocalConfigFile(t *testing.T) {
	root := sandboxProject(t)
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".govard.local.yml")); !os.IsNotExist(err) {
		t.Fatalf(".govard.local.yml must never be created by sandbox up, stat err = %v", err)
	}
}

func TestSandboxUpReusesAnExistingSandbox(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	request := deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}
	first, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request)
	if err != nil {
		t.Fatalf("first up: %v", err)
	}

	secondFake := sandboxFake()
	secondFake.answers["image inspect"] = "sha256:abc\n"
	second, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(secondFake.run), deploy.LocalRunner{}, request)
	if err != nil {
		t.Fatalf("second up: %v", err)
	}
	if secondFake.has("build --file") {
		t.Fatal("a second up rebuilt an image that is already local")
	}
	if secondFake.has("run --detach") {
		t.Fatal("a second up created a second container")
	}
	if second.Port != first.Port {
		t.Fatalf("the second up moved the port from %d to %d", first.Port, second.Port)
	}
	// No remote is written anywhere: reusing must not create the file either.
	if _, err := os.Stat(filepath.Join(root, ".govard.local.yml")); !os.IsNotExist(err) {
		t.Fatalf(".govard.local.yml must never be created by sandbox up, stat err = %v", err)
	}
}

// `up` is the command an operator runs to *reuse* a sandbox — to refresh the
// mirror so a new revision is deployable, to read the published ports, to bring a
// stopped container back. It must not delete the application the target is
// serving.
//
// It did: the current path was re-shaped on every `up`, and the default shape is
// a symlink to `releases/0`, so `up` on a live in-place target replaced the
// deployed application with a dangling link and every request answered
// "File not found". Found on a real sandbox right after a successful deploy,
// when the next revision was committed and `up` was run to refresh the mirror.
func TestSandboxUpDoesNotReshapeAnExistingTarget(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	request := deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("first up: %v", err)
	}

	reuse := sandboxFake()
	reuse.answers["image inspect"] = "sha256:abc\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(reuse.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("second up: %v", err)
	}
	if reuse.has("rm -rf " + deploy.SandboxDefaultPaths().Current) {
		t.Fatalf("a reused sandbox had its current path deleted:\n%v", reuse.calls)
	}
}

// The shape is still applied where it is meaningful: when the container is
// created, and when the operator asks for it by naming one.
func TestSandboxUpShapesTheTargetOnCreateOrOnRequest(t *testing.T) {
	root := sandboxProject(t)
	created := absentContainerFake()
	created.fail["image inspect"] = "Error: No such image"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(created.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("first up: %v", err)
	}
	if !created.has("rm -rf " + deploy.SandboxDefaultPaths().Current) {
		t.Fatalf("a new sandbox must still be shaped:\n%v", created.calls)
	}

	reshaped := sandboxFake()
	reshaped.answers["image inspect"] = "sha256:abc\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(reshaped.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot:    root,
		ProjectName:    "sample-project",
		Profile:        deploy.SandboxProfilePHP,
		DocRoot:        deploy.SandboxDocRootReal,
		ReshapeDocRoot: true,
		Probe:          func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("reshaping up: %v", err)
	}
	if !reshaped.has("rm -rf " + deploy.SandboxDefaultPaths().Current) {
		t.Fatalf("an explicitly requested shape must be applied:\n%v", reshaped.calls)
	}
}

func TestSandboxRecreateRebuilds(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Recreate:    true,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("up --recreate: %v", err)
	}
	if !fake.has("rm --force --volumes") {
		t.Fatal("--recreate must remove the container it replaces")
	}
	if !fake.has("build --file") {
		t.Fatal("--recreate must rebuild the image")
	}
}

func TestSandboxDownRemovesTheContainer(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("up: %v", err)
	}

	downFake := sandboxFake()
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(downFake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
	}); err != nil {
		t.Fatalf("down: %v", err)
	}
	if !downFake.has("stop ") || !downFake.has("rm --force --volumes") {
		t.Fatal("down must stop and remove the container")
	}
	// `down` removed the container, so resolving it afterwards must report an
	// absent sandbox: script the removal into the fake, which cannot observe
	// its own `rm` the way Docker does.
	downFake.fail["inspect"] = "Error: No such container"
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	_, liveness, err := deploy.ResolveSyntheticSandboxRemoteForTest(context.Background(), deploy.NewDockerCLIForTest(downFake.run), deploy.LocalRunner{}, root, "sample-project")
	if err == nil {
		t.Fatal("resolving a removed sandbox must fail")
	}
	if liveness != deploy.SandboxLivenessAbsent {
		t.Fatalf("liveness = %q, want absent after down removed the container", liveness)
	}
	if _, err := os.Stat(deploy.SandboxStateDir(root)); err != nil {
		t.Fatalf("a plain down must keep the key and the mirror: %v", err)
	}

	// The remote must be gone from the configuration, not merely stale.
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, ok := cfg.Remotes["sandbox"]; ok {
		t.Fatal("the removed remote is still configured")
	}
}

func TestSandboxDownPurgeRemovesTheState(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("up: %v", err)
	}

	purgeFake := sandboxFake()
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(purgeFake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Purge:       true,
	}); err != nil {
		t.Fatalf("down --purge: %v", err)
	}
	if !purgeFake.has("image rm --force") {
		t.Fatal("--purge must remove the image")
	}
	if _, err := os.Stat(deploy.SandboxStateDir(root)); !os.IsNotExist(err) {
		t.Fatalf("--purge must remove the state directory (%v)", err)
	}
}

func TestSandboxResetSeedsADeployerOwnedTarget(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()

	if _, err := deploy.SandboxReset(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Layout:      deploy.SandboxDeployerLayout,
		DocRoot:     deploy.SandboxDocRootSymlink,
	}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !fake.has("deploy.lock") {
		t.Fatal("--layout=deployer must seed the other tool's lock")
	}
	if !fake.has("releases/1") {
		t.Fatal("--layout=deployer must seed a release directory the other tool owns")
	}
	if !fake.has("ln -sfn") {
		t.Fatal("the docroot shape was not applied")
	}
}

func TestSandboxResetRefusesWithoutAContainer(t *testing.T) {
	root := sandboxProject(t)
	fake := absentContainerFake()
	_, err := deploy.SandboxReset(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
	})
	if err == nil || !strings.Contains(err.Error(), "sandbox up") {
		t.Fatalf("err = %v, want a refusal naming the way out", err)
	}
}

func TestSandboxDocRootRealInitialisesAGitCheckout(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile(deploy.SandboxProfileBasic)
	fake.answers["image inspect"] = "sha256:abc\n"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfileBasic,
		DocRoot:     deploy.SandboxDocRootReal,
		// The CLI sets this whenever `--docroot` is named, which is what makes
		// re-shaping an existing target the operator's decision.
		ReshapeDocRoot: true,
		Probe:          func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("up --docroot=real: %v", err)
	}
	if !fake.has("git init -q") {
		t.Fatal("an in-place target has to be a git checkout")
	}

	basic := sandboxFake().containerProfile(deploy.SandboxProfileBasic)
	basic.answers["image inspect"] = "sha256:abc\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(basic.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfileBasic,
		DocRoot:     deploy.SandboxDocRootAbsent,
		Recreate:    true,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("up --docroot=absent: %v", err)
	}
	if !basic.has("rm -rf /home/deployer/public_html") {
		t.Fatal("--docroot=absent must leave no current path")
	}

	// The basic profile has no PHP, so naming one would fail every deploy.
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	remote, liveness, err := deploy.ResolveSyntheticSandboxRemoteForTest(context.Background(), deploy.NewDockerCLIForTest(basic.run), deploy.LocalRunner{}, root, "sample-project")
	if err != nil {
		t.Fatalf("resolve the sandbox remote: %v", err)
	}
	if liveness != deploy.SandboxLivenessRunning {
		t.Fatalf("liveness = %q, want running", liveness)
	}
	if remote.Deploy == nil {
		t.Fatal("the sandbox remote carries no deploy settings")
	}
	if _, ok := remote.Deploy.Settings["php_bin"]; ok {
		t.Fatal("a profile without PHP must not ask the target to probe one")
	}
}

func TestSandboxUpRejectsAnUnknownProfileOrDocRoot(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	if _, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root, ProjectName: "sample-project", Profile: "enormous",
	}); err == nil {
		t.Fatal("want an error for an unknown profile")
	}
	if _, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root, ProjectName: "sample-project", DocRoot: "sideways",
	}); err == nil {
		t.Fatal("want an error for an unknown docroot")
	}
}

func TestSandboxStatusReportsAStoppedSandbox(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile("full").containerPHP("8.4")
	fake.answers["inspect --format {{.State.Running}}"] = "false\n"

	state, err := deploy.SandboxStatus(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !state.Exists || state.Running {
		t.Fatalf("state = %+v, want an existing stopped sandbox", state)
	}
	if state.Image == "" {
		t.Fatal("status must report the image the container was built from")
	}
	if state.Profile != "full" || state.PHP != "8.4" {
		t.Fatalf("a stopped sandbox must keep profile+PHP from its labels, got profile=%q php=%q", state.Profile, state.PHP)
	}
}

func TestSandboxSSHArgsUseTheGeneratedKeyAndPort(t *testing.T) {
	root := sandboxProject(t)
	args := deploy.SandboxSSHArgs(deploy.SandboxRequest{ProjectRoot: root}, &deploy.SandboxState{Port: 49153})
	joined := strings.Join(args, " ")
	for _, want := range []string{"-i " + filepath.Join(root, ".govard", "sandbox", "id_ed25519"), "-p 49153", "deployer@127.0.0.1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("ssh args are missing %q: %v", want, args)
		}
	}
}

func TestRefreshSandboxMirrorMakesLocalBranchesDeployable(t *testing.T) {
	root := sandboxProject(t)
	mirror := filepath.Join(t.TempDir(), "repo.git")
	if err := deploy.RefreshSandboxMirrorForTest(context.Background(), deploy.LocalRunner{}, root, mirror); err != nil {
		t.Fatalf("refresh mirror: %v", err)
	}
	// An unpushed local commit has to be reachable in the mirror, which is the
	// whole reason the sandbox does not point at the real repository.
	head := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	output := runGitOutput(t, mirror, "rev-parse", "refs/heads/main")
	if strings.TrimSpace(output) != head {
		t.Fatalf("mirror main = %q, want the local HEAD %q", strings.TrimSpace(output), head)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// Two checkouts of the same project must not share a sandbox container: the
// container's mirror is a bind mount into one project's state directory, and the
// second `up` reusing it fails much later with "revision is not present in the
// deploy mirror".
func TestSandboxContainerNameIsPerProjectPath(t *testing.T) {
	first := deploy.SandboxContainerName("sample-project", "/home/dev/one/sample-project")
	second := deploy.SandboxContainerName("sample-project", "/home/dev/two/sample-project")
	if first == second {
		t.Fatalf("two checkouts of the same project share a container name: %q", first)
	}
	if !strings.HasPrefix(first, "govard-sample-project-sandbox-") {
		t.Fatalf("container name = %q, want the govard-<slug>-sandbox-<hash> prefix (no \"deploy\")", first)
	}
	// Stable for the same path, so `status` and `down` address the same container.
	if again := deploy.SandboxContainerName("sample-project", "/home/dev/one/sample-project"); again != first {
		t.Fatalf("container name is not stable: %q then %q", first, again)
	}
	// Docker accepts the name.
	for _, r := range first {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789_.-", r) {
			t.Fatalf("container name %q holds %q, which Docker rejects", first, r)
		}
	}
}

// sandboxServicesLine is the GOVARD_SANDBOX_SERVICES value the entrypoint reads.
func sandboxServicesLine(t *testing.T, dockerfile string) string {
	t.Helper()
	const marker = "GOVARD_SANDBOX_SERVICES="
	index := strings.Index(dockerfile, marker)
	if index < 0 {
		t.Fatalf("the Dockerfile sets no service list:\n%s", dockerfile)
	}
	rest := dockerfile[index+len(marker):]
	end := strings.IndexByte(rest, '\n')
	if end < 0 {
		end = len(rest)
	}
	return strings.Trim(rest[:end], `"`)
}

// `up` on a stopped sandbox is the other half of reuse: start the container and
// leave the target exactly as the last deploy left it. The current path is as
// much the application here as it is for a running container.
func TestSandboxUpStartsAStoppedContainerWithoutReshaping(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["inspect --format {{.State.Running}}"] = "false\n"
	fake.answers["image inspect"] = "sha256:abc\n"

	state, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if !fake.has("start ") {
		t.Fatalf("a stopped sandbox must be started: %v", fake.calls)
	}
	if fake.has("rm -rf " + deploy.SandboxDefaultPaths().Current) {
		t.Fatalf("starting a stopped sandbox must not reshape its target: %v", fake.calls)
	}
	if !state.Running {
		t.Fatal("the state must report the container as running after up started it")
	}
}

// flakyRunner fails the first `failures` commands and then succeeds, which is what
// a sandbox that is still starting its sshd looks like.
type flakyRunner struct {
	failures int
	attempts int
	commands []string
}

func (r *flakyRunner) Run(_ context.Context, command string, _ deploy.RunOptions) (deploy.Result, error) {
	r.attempts++
	r.commands = append(r.commands, command)
	if r.attempts <= r.failures {
		return deploy.Result{}, &deploy.CommandError{Command: command, ExitCode: 255, Stderr: "Connection refused"}
	}
	return deploy.Result{}, nil
}

// The readiness check has to prove a login, not a listening port: Docker's proxy
// accepts a TCP connection before sshd is ready, and a deploy started in that
// window fails its first step with exit 255.
func TestDialSandboxSSHWaitsForARealLogin(t *testing.T) {
	runner := &flakyRunner{failures: 2}
	if err := deploy.DialSandboxSSH(context.Background(), runner, "/keys/id_ed25519", "127.0.0.1", 49153, 10*time.Second); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if runner.attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (two refusals then a login)", runner.attempts)
	}
	command := runner.commands[0]
	for _, want := range []string{"BatchMode=yes", "-p 49153", "deployer@127.0.0.1", "/keys/id_ed25519", " true"} {
		if !strings.Contains(command, want) {
			t.Errorf("the probe command is missing %q:\n%s", want, command)
		}
	}
	if strings.Contains(command, "-t ") {
		t.Errorf("a readiness probe must not allocate a tty:\n%s", command)
	}
}

func TestDialSandboxSSHReportsWhatKeptFailing(t *testing.T) {
	runner := &flakyRunner{failures: 1 << 30}
	err := deploy.DialSandboxSSH(context.Background(), runner, "/keys/id_ed25519", "127.0.0.1", 49153, 400*time.Millisecond)
	if err == nil {
		t.Fatal("want an error when the sandbox never answers")
	}
	if !strings.Contains(err.Error(), "did not answer") || !strings.Contains(err.Error(), "127.0.0.1:49153") {
		t.Fatalf("err = %v, want the address and the fact that it never answered", err)
	}
}

// sshNotReadyRunner refuses the first `failures` ssh commands and delegates
// everything else to the real local runner: the sandbox mirror is still built by
// real git, and only the target's sshd is late.
type sshNotReadyRunner struct {
	failures int
	attempts int
	commands []string
	inner    deploy.Runner
}

func (r *sshNotReadyRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	r.commands = append(r.commands, command)
	if strings.HasPrefix(command, "ssh ") {
		r.attempts++
		if r.attempts <= r.failures {
			return deploy.Result{}, &deploy.CommandError{Command: command, ExitCode: 255, Stderr: "Connection refused"}
		}
		// Answered: the probe is what is under test, so no real login is made.
		return deploy.Result{}, nil
	}
	return r.inner.Run(ctx, command, opts)
}

// `up` must not report a sandbox ready before it can be deployed to. The probe it
// uses by default is a login over the sandbox key; this asserts the wiring, which
// the unit test above cannot see.
func TestSandboxUpProvesTheTargetAnswersBeforeItReportsReady(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"
	runner := &sshNotReadyRunner{failures: 1, inner: deploy.LocalRunner{}}

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), runner, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
	}); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	logins := 0
	for _, command := range runner.commands {
		if strings.HasPrefix(command, "ssh ") && strings.Contains(command, "BatchMode=yes") {
			logins++
		}
	}
	if logins != 2 {
		t.Fatalf("up must retry the login until the sandbox answers, saw %d of %d commands:\n%v", logins, len(runner.commands), runner.commands)
	}
}

// A reused sandbox is described by its container, not by the flags of the command
// that reached it.
//
// `up` with no profile used to write the *default* profile's settings and a
// computed image tag for a container built for a different one: on a host that did
// not have that tag it built the image, and `--recreate` would then have handed
// back a container without the services the first one had. Found live — a `full`
// sandbox reported itself as the `php` profile whose image tag did not exist.
func TestSandboxUpDescribesAReusedContainerAsWhatItIs(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake().containerProfile(deploy.SandboxProfileBasic)
	fake.answers["image inspect"] = "sha256:abc\n"
	const actualImage = "govard-sandbox:sample-project-basic-deadbeef"
	fake.answers["inspect --format {{.Config.Image}}"] = actualImage + "\n"
	probe := func(context.Context, string, int, time.Duration) error { return nil }

	state, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Probe:       probe,
	})
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if state.Profile != deploy.SandboxProfileBasic {
		t.Fatalf("profile = %q, want the profile the container was built for", state.Profile)
	}
	if state.Image != actualImage {
		t.Fatalf("image = %q, want the container's own image %q", state.Image, actualImage)
	}
	if fake.has("build --file") {
		t.Fatalf("a reused sandbox must not build another profile's image:\n%v", fake.calls)
	}
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapDocker)
	defer restore()
	remote, liveness, err := deploy.ResolveSyntheticSandboxRemoteForTest(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, root, "sample-project")
	if err != nil {
		t.Fatalf("resolve the sandbox remote: %v", err)
	}
	if liveness != deploy.SandboxLivenessRunning {
		t.Fatalf("liveness = %q, want running", liveness)
	}
	if _, ok := remote.Deploy.Settings["php_bin"]; ok {
		t.Fatal("a profile with no PHP must not declare a PHP binary")
	}

	// The default profile is what the CLI passes when the flag is not named, and it
	// must not conflict with the container: "no preference" and "asked for the
	// default" are the same value. Only an explicit choice is refused.
	named := sandboxFake().containerProfile(deploy.SandboxProfileFull)
	named.answers["image inspect"] = "sha256:abc\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(named.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.DefaultSandboxProfile,
		Probe:       probe,
	}); err != nil {
		t.Fatalf("an unnamed profile must adopt the container's: %v", err)
	}

	explicit := sandboxFake().containerProfile(deploy.SandboxProfileBasic)
	explicit.answers["image inspect"] = "sha256:abc\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(explicit.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot:     root,
		ProjectName:     "sample-project",
		Profile:         deploy.SandboxProfileFull,
		ProfileExplicit: true,
		Probe:           probe,
	}); err == nil || !strings.Contains(err.Error(), "--recreate") {
		t.Fatalf("err = %v, want a refusal naming --recreate", err)
	}
}

// A Composer install that extracts a dist shells out to `patch` when the project
// patches a dependency. The image had `unzip` and `git` but not `patch`, so
// `composer install --prefer-dist` aborted on a real project's patch while the
// same command with `--prefer-source` succeeded — that path uses `git apply`.
func TestSandboxInstallsThePatcherComposerShellsOutTo(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfilePHP, PHP: "8.4"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"'patch'", "'unzip'"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("the image does not install %s:\n%s", want, firstLineContaining(dockerfile, "apt-get install"))
		}
	}
}

func TestDockerCLIEnsureNetworkConnectedIsIdempotent(t *testing.T) {
	fake := newFakeSandboxRuntime()
	// No scripted answer needed: an unconfigured fake command succeeds by
	// default (see fakeSandboxRuntime.run) -- setting fake.fail[...] to an
	// empty string here would still trigger the fail branch and make this
	// assert the opposite of what it says.
	cli := deploy.NewDockerCLIForTest(fake.run)

	connected, err := cli.EnsureNetworkConnected(context.Background(), "sandbox-ctr", "govard-proxy")
	if err != nil || !connected {
		t.Fatalf("first connect: connected=%v err=%v", connected, err)
	}
	if !fake.has("network connect govard-proxy sandbox-ctr") {
		t.Fatal("expected a `docker network connect govard-proxy sandbox-ctr` call")
	}
}

func TestDockerCLIEnsureNetworkConnectedTreatsAlreadyConnectedAsSuccess(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.fail["network connect"] = "Error response from daemon: endpoint with name sandbox-ctr already exists in network govard-proxy"
	cli := deploy.NewDockerCLIForTest(fake.run)

	connected, err := cli.EnsureNetworkConnected(context.Background(), "sandbox-ctr", "govard-proxy")
	if err != nil || !connected {
		t.Fatalf("already-connected case: connected=%v err=%v, want connected=true err=nil", connected, err)
	}
}

func TestDockerCLIEnsureNetworkConnectedTreatsMissingNetworkAsSoftFailure(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.fail["network connect"] = "Error response from daemon: network govard-proxy not found"
	cli := deploy.NewDockerCLIForTest(fake.run)

	connected, err := cli.EnsureNetworkConnected(context.Background(), "sandbox-ctr", "govard-proxy")
	if err != nil {
		t.Fatalf("missing network must not be a hard error (the gateway is optional): %v", err)
	}
	if connected {
		t.Fatal("expected connected=false when the network does not exist")
	}
}

func TestSandboxUpJoinsTheGatewayNetworkAndRegistersTheTarget(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"
	request := deploy.SandboxRequest{
		ProjectRoot: sandboxProject(t),
		ProjectName: "shop",
		Profile:     deploy.SandboxProfileBasic,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}

	_, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request)
	if err != nil {
		t.Fatalf("SandboxUp: %v", err)
	}
	if !fake.has("network connect govard-proxy") {
		t.Fatal("expected SandboxUp to join the govard-proxy network")
	}

	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("gateway.Load: %v", err)
	}
	target, ok := reg.Targets["shop"]
	if !ok {
		t.Fatal("expected SandboxUp to register a gateway target for the project")
	}
	if target.TargetUser != deploy.SandboxUser {
		t.Fatalf("target user = %q, want %q", target.TargetUser, deploy.SandboxUser)
	}
	if !fake.has("exec govard-proxy-sshd useradd") {
		t.Fatal("expected SandboxUp to create the gateway account synchronously")
	}
}

func TestSandboxUpMapsUnderscoredProjectToGatewayUsername(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"
	request := deploy.SandboxRequest{
		ProjectRoot: sandboxProject(t),
		ProjectName: "my_shop",
		Profile:     deploy.SandboxProfileBasic,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("SandboxUp: %v", err)
	}

	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("gateway.Load: %v", err)
	}
	if _, ok := reg.Targets["my-shop"]; !ok {
		t.Fatal("expected SandboxUp to register gateway target my-shop for project my_shop")
	}
	if _, ok := reg.Targets["my_shop"]; ok {
		t.Fatal("the raw project name my_shop must not be registered as a gateway target")
	}
	if !fake.has("exec govard-proxy-sshd useradd") {
		t.Fatal("expected SandboxUp to create the gateway account synchronously")
	}
	if !fake.hasArg("--", "my-shop") {
		t.Fatal("expected the gateway account to be created for my-shop, not the raw project name")
	}
}

func TestSandboxDownPrunesTheGatewayTarget(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"
	request := deploy.SandboxRequest{
		ProjectRoot: sandboxProject(t),
		ProjectName: "shop",
		Profile:     deploy.SandboxProfileBasic,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("SandboxUp: %v", err)
	}

	// The container `up` created answers the probes now: drop the absence the
	// fake was scripted with so `down` observes a running sandbox.
	delete(fake.fail, "inspect")
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(fake.run), request); err != nil {
		t.Fatalf("SandboxDown: %v", err)
	}

	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("gateway.Load: %v", err)
	}
	if _, ok := reg.Targets["shop"]; ok {
		t.Fatal("expected SandboxDown to prune the gateway target")
	}
}

func TestSandboxDownPrunesGatewayTargetWhenProjectNameEmpty(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := sandboxProject(t)
	// The empty-name fallback resolves through the project configuration,
	// so the fixture must claim the name `up` registers under.
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: prune-empty-name
framework: generic
domain: prune-empty-name.test
`)
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"
	upReq := deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "prune-empty-name",
		Profile:     deploy.SandboxProfileBasic,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, upReq); err != nil {
		t.Fatalf("SandboxUp: %v", err)
	}

	// The container `up` created answers the probes now: drop the absence the
	// fake was scripted with so `down` observes a running sandbox.
	delete(fake.fail, "inspect")
	// CLI invoked without explicit name; must still prune via resolved name.
	downReq := upReq
	downReq.ProjectName = ""
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(fake.run), downReq); err != nil {
		t.Fatalf("SandboxDown: %v", err)
	}

	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("gateway.Load: %v", err)
	}
	if _, ok := reg.Targets["prune-empty-name"]; ok {
		t.Fatal("expected SandboxDown to prune the gateway target even when ProjectName is empty")
	}
}

func TestSandboxDownPrunesNormalizedGatewayTarget(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"
	upReq := deploy.SandboxRequest{
		ProjectRoot: sandboxProject(t),
		ProjectName: "My_Project",
		Profile:     deploy.SandboxProfileBasic,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, upReq); err != nil {
		t.Fatalf("SandboxUp: %v", err)
	}

	// A pre-normalization entry under the raw name must not survive either.
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("gateway.Load: %v", err)
	}
	if _, ok := reg.Targets["my-project"]; !ok {
		t.Fatal("expected SandboxUp to register gateway target my-project for project My_Project")
	}
	reg.Targets["My_Project"] = reg.Targets["my-project"]
	if err := reg.Save(); err != nil {
		t.Fatalf("reg.Save: %v", err)
	}

	// The container `up` created answers the probes now: drop the absence the
	// fake was scripted with so `down` observes a running sandbox.
	delete(fake.fail, "inspect")
	downReq := upReq
	downReq.ProjectName = "my_project"
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(fake.run), downReq); err != nil {
		t.Fatalf("SandboxDown: %v", err)
	}

	reg, err = gateway.Load()
	if err != nil {
		t.Fatalf("gateway.Load: %v", err)
	}
	if _, ok := reg.Targets["my-project"]; ok {
		t.Fatal("expected SandboxDown to prune the normalized gateway target my-project")
	}
	if _, ok := reg.Targets["My_Project"]; ok {
		t.Fatal("expected SandboxDown to prune the legacy raw gateway target My_Project")
	}
}

func TestSandboxDownRemovesRawGatewayTargetForUnroutableProjectName(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	if _, ok := gateway.RouteUsername("my.project"); ok {
		t.Fatal("RouteUsername(my.project) = ok, want false: the dot keeps it off the gateway")
	}
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"
	upReq := deploy.SandboxRequest{
		ProjectRoot: sandboxProject(t),
		ProjectName: "my.project",
		Profile:     deploy.SandboxProfileBasic,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, upReq); err != nil {
		t.Fatalf("SandboxUp: %v", err)
	}

	// An unroutable name registers nothing: seed the raw entry a
	// pre-normalization `up` may have left behind.
	reg, err := gateway.Load()
	if err != nil {
		t.Fatalf("gateway.Load: %v", err)
	}
	if _, ok := reg.Targets["my.project"]; ok {
		t.Fatal("expected SandboxUp to skip gateway registration for my.project")
	}
	reg.Targets["my.project"] = gateway.Target{Container: "govard-my.project-sandbox-basic", TargetUser: deploy.SandboxUser, Project: "my.project"}
	if err := reg.Save(); err != nil {
		t.Fatalf("reg.Save: %v", err)
	}

	// The container `up` created answers the probes now: drop the absence the
	// fake was scripted with so `down` observes a running sandbox.
	delete(fake.fail, "inspect")
	if _, err := deploy.SandboxDown(context.Background(), deploy.NewDockerCLIForTest(fake.run), upReq); err != nil {
		t.Fatalf("SandboxDown: %v", err)
	}

	reg, err = gateway.Load()
	if err != nil {
		t.Fatalf("gateway.Load: %v", err)
	}
	if _, ok := reg.Targets["my.project"]; ok {
		t.Fatal("expected SandboxDown to remove the raw gateway target my.project")
	}
}
