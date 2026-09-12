package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/frameworks"
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
			dockerfile, err := deploy.SandboxDockerfile(tc.profile, "", deploy.SandboxRequirements{})
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
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxProfileBasic, "", deploy.SandboxRequirements{})
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
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxProfileBasic, "", deploy.SandboxRequirements{})
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
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxProfilePHP, "", requirements)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"libxslt1-dev", "php-bcmath", "php-intl", `GOVARD_SANDBOX_SERVICES="mariadb"`} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("the Dockerfile must carry the framework's %q requirement", want)
		}
	}
}

func TestSandboxDockerfileIsByteStable(t *testing.T) {
	requirements := deploy.SandboxRequirements{
		Extensions: []string{"intl", "bcmath"},
		Services:   []string{"redis", "mariadb"},
	}
	first, err := deploy.SandboxDockerfile(deploy.SandboxProfileFull, "", requirements)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	second, err := deploy.SandboxDockerfile(deploy.SandboxProfileFull, "", requirements)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if first != second {
		t.Fatal("two renders of the same spec differ, so the image tag would never be stable")
	}
	// The order requirements arrive in must not change the image either: two
	// recipes listing the same extensions in a different order are the same
	// sandbox.
	shuffled, err := deploy.SandboxDockerfile(deploy.SandboxProfileFull, "", deploy.SandboxRequirements{
		Extensions: []string{"bcmath", "intl"},
		Services:   []string{"mariadb", "redis"},
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
	if !strings.HasPrefix(tag, "govard-deploy-sandbox:sample-project-php-") {
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
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxProfilePHP, "", recipe.Sandbox)
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
}

func newFakeSandboxRuntime() *fakeSandboxRuntime {
	return &fakeSandboxRuntime{answers: map[string]string{}, fail: map[string]string{}}
}

func (f *fakeSandboxRuntime) run(ctx context.Context, request deploy.SandboxCommand) (string, error) {
	f.calls = append(f.calls, request.Args)
	key := strings.Join(request.Args, " ")
	for marker, message := range f.fail {
		if strings.Contains(key, marker) {
			return "", &deploy.CommandError{Command: "docker " + key, ExitCode: 1, Stderr: message}
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
		Name:        "govard-sample-project-deploy-sandbox",
		Image:       "govard-deploy-sandbox:sample-project-basic-abc123",
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
	if !strings.Contains(joined, "--name govard-sample-project-deploy-sandbox") {
		t.Fatalf("the container name is missing: %v", call)
	}
}

func TestSandboxRuntimeReadsBackThePublishedPort(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.answers["port govard-sample-project-deploy-sandbox 22/tcp"] = "127.0.0.1:49153\n"
	runtime := deploy.NewDockerCLIForTest(fake.run)

	port, err := runtime.PublishedPort(context.Background(), "govard-sample-project-deploy-sandbox", deploy.SandboxSSHPort)
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
	absent.fail["image inspect"] = "Error response from daemon: No such image: govard-deploy-sandbox:absent"
	exists, err := deploy.NewDockerCLIForTest(absent.run).ImageExists(context.Background(), "govard-deploy-sandbox:absent")
	if err != nil {
		t.Fatalf("image exists: %v", err)
	}
	if exists {
		t.Fatal("an image that is not local must report false, not an error")
	}

	present := newFakeSandboxRuntime()
	present.answers["image inspect"] = "sha256:abc\n"
	exists, err = deploy.NewDockerCLIForTest(present.run).ImageExists(context.Background(), "govard-deploy-sandbox:present")
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
		Image:      "govard-deploy-sandbox:sample-project-basic",
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
		Image:      "govard-deploy-sandbox:sample-project-basic",
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

	versions, err := runtime.ImageFile(context.Background(), "govard-deploy-sandbox:sample", deploy.SandboxPackagesTxt)
	if err != nil {
		t.Fatalf("image file: %v", err)
	}
	if !strings.Contains(versions, "rsync=3.2.7-1") {
		t.Fatalf("recorded versions = %q", versions)
	}
}

func TestSandboxRemoteRoundTripsThroughTheRealLoader(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)

	remote := engine.RemoteConfig{
		Host:       "127.0.0.1",
		Port:       49153,
		User:       deploy.SandboxUser,
		Path:       "/home/deployer/public_html",
		DeployPath: "/home/deployer/.deployer",
		Repository: deploy.SandboxRepoPath,
		Sandbox:    true,
		Auth:       engine.RemoteAuth{KeyPath: ".govard/sandbox/id_ed25519"},
	}
	if err := deploy.WriteSandboxRemote(root, "sandbox", remote); err != nil {
		t.Fatalf("write sandbox remote: %v", err)
	}

	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	loaded, ok := cfg.Remotes["sandbox"]
	if !ok {
		t.Fatal("the sandbox remote did not survive the real config loader")
	}
	if loaded.Port != 49153 || loaded.Repository != deploy.SandboxRepoPath {
		t.Fatalf("loaded remote = %+v, want the port and repository written", loaded)
	}
	if !loaded.Sandbox {
		t.Fatal("the sandbox marker was lost, so the pipeline cannot refresh the mirror")
	}
	if loaded.Auth.KeyPath != ".govard/sandbox/id_ed25519" {
		t.Fatalf("key path = %q", loaded.Auth.KeyPath)
	}
}

func TestSandboxRemoteWriteKeepsTheProjectsOwnContent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	// A project may keep its own local remotes and comments here. Writing the
	// sandbox must not reformat or drop them.
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `# my own local overrides
remotes:
  staging-local:
    host: 10.0.0.5
    user: deploy
    path: /srv/staging
`)

	if err := deploy.WriteSandboxRemote(root, "sandbox", engine.RemoteConfig{
		Host: "127.0.0.1", Port: 1234, User: deploy.SandboxUser, Sandbox: true,
		Path: "/home/deployer/public_html", DeployPath: "/home/deployer/.deployer",
	}); err != nil {
		t.Fatalf("write sandbox remote: %v", err)
	}

	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, ok := cfg.Remotes["staging-local"]; !ok {
		t.Fatal("the project's own remote was dropped")
	}
	if _, ok := cfg.Remotes["sandbox"]; !ok {
		t.Fatal("the sandbox remote was not added")
	}
	content, err := os.ReadFile(filepath.Join(root, ".govard.local.yml"))
	if err != nil {
		t.Fatalf("read the local config: %v", err)
	}
	if !strings.Contains(string(content), "my own local overrides") {
		t.Fatalf("the project's comment was lost:\n%s", content)
	}

	// Rewriting replaces the entry instead of duplicating it.
	if err := deploy.WriteSandboxRemote(root, "sandbox", engine.RemoteConfig{
		Host: "127.0.0.1", Port: 2345, User: deploy.SandboxUser, Sandbox: true,
		Path: "/home/deployer/public_html", DeployPath: "/home/deployer/.deployer",
	}); err != nil {
		t.Fatalf("rewrite sandbox remote: %v", err)
	}
	cfg, _, err = engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.Remotes["sandbox"].Port != 2345 {
		t.Fatalf("port = %d, want the rewritten 2345", cfg.Remotes["sandbox"].Port)
	}
	// The bare `sandbox:` key, not the `sandbox: true` field of the remote.
	if matches := regexp.MustCompile(`(?m)^\s*sandbox:\s*$`).FindAllString(string(mustReadFile(t, filepath.Join(root, ".govard.local.yml"))), -1); len(matches) != 1 {
		t.Fatalf("the sandbox remote key appears %d times", len(matches))
	}
}

func TestSandboxRemoteRemovalLeavesTheRest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	writeFile(t, filepath.Join(root, ".govard.local.yml"), `remotes:
  staging-local:
    host: 10.0.0.5
    user: deploy
    path: /srv/staging
  sandbox:
    host: 127.0.0.1
    port: 1234
    user: deployer
    sandbox: true
`)
	if err := deploy.RemoveSandboxRemote(root, "sandbox"); err != nil {
		t.Fatalf("remove sandbox remote: %v", err)
	}
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, ok := cfg.Remotes["sandbox"]; ok {
		t.Fatal("the sandbox remote survived its own removal")
	}
	if _, ok := cfg.Remotes["staging-local"]; !ok {
		t.Fatal("removing the sandbox remote removed an unrelated one")
	}

	// Removing it twice is not an error: `down` must be repeatable.
	if err := deploy.RemoveSandboxRemote(root, "sandbox"); err != nil {
		t.Fatalf("second removal: %v", err)
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
	fake.answers["inspect --format {{.Config.Image}}"] = "govard-deploy-sandbox:sample-project-php-deadbeef\n"
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

func TestSandboxUpCreatesTheContainerAndWritesTheRemote(t *testing.T) {
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

	remote, set, err := deploy.SandboxRemoteForTest(root, "sandbox")
	if err != nil {
		t.Fatalf("read the sandbox remote: %v", err)
	}
	if !set {
		t.Fatal("up did not write the sandbox remote")
	}
	if remote.Port != 49153 || remote.Repository != deploy.SandboxRepoPath {
		t.Fatalf("remote = %+v, want the published port and the mounted mirror", remote)
	}
	if !remote.Sandbox {
		t.Fatal("the remote must be marked as a sandbox")
	}
	if _, err := os.Stat(deploy.SandboxMirrorPath(root)); err != nil {
		t.Fatalf("the mirror was not created: %v", err)
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
	content := mustReadFile(t, filepath.Join(root, ".govard.local.yml"))
	if strings.Count(string(content), "port:") != 1 {
		t.Fatalf("the sandbox remote was duplicated:\n%s", content)
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

func TestSandboxDownRemovesTheContainerAndTheRemote(t *testing.T) {
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
	if _, set, err := deploy.SandboxRemoteForTest(root, "sandbox"); err != nil || set {
		t.Fatalf("down left the sandbox remote behind (set=%v err=%v)", set, err)
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
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfileBasic,
		DocRoot:     deploy.SandboxDocRootReal,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("up --docroot=real: %v", err)
	}
	if !fake.has("git init -q") {
		t.Fatal("an in-place target has to be a git checkout")
	}

	basic := sandboxFake()
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
	remote, _, err := deploy.SandboxRemoteForTest(root, "sandbox")
	if err != nil {
		t.Fatalf("read the sandbox remote: %v", err)
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
	fake := sandboxFake()
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
	if !strings.HasPrefix(first, "govard-sample-project-deploy-sandbox-") {
		t.Fatalf("container name = %q, want the project slug and a path suffix", first)
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
