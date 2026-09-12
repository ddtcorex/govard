package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

// A sandbox that cannot serve the release cannot rehearse the last step of a
// deploy: `deploy:verify` only checks a URL when the remote sets
// `deploy.verify.url`, and without a web server in the container that check had
// never run against anything but a stub. These tests state what the web tier is
// and that a sandbox turns the check on by itself.

func TestSandboxWebTierBelongsToTheProfilesWithPHP(t *testing.T) {
	for _, profile := range []string{deploy.SandboxProfileBasic} {
		if deploy.SandboxServesWeb(profile) {
			t.Errorf("the %s profile has no interpreter, so it must not serve web", profile)
		}
	}
	for _, profile := range []string{deploy.SandboxProfilePHP, deploy.SandboxProfileFull} {
		if !deploy.SandboxServesWeb(profile) {
			t.Errorf("the %s profile ships PHP and must serve it", profile)
		}
	}
}

func TestSandboxWebDocumentRootFollowsTheProjectWebRoot(t *testing.T) {
	cases := map[string]string{
		"":      "/home/deployer/public_html",
		"/":     "/home/deployer/public_html",
		"pub":   "/home/deployer/public_html/pub",
		"/pub":  "/home/deployer/public_html/pub",
		"/pub/": "/home/deployer/public_html/pub",
		" /pub": "/home/deployer/public_html/pub",
	}
	for webRoot, want := range cases {
		if got := deploy.SandboxWebDocumentRoot(webRoot); got != want {
			t.Errorf("SandboxWebDocumentRoot(%q) = %q, want %q", webRoot, got, want)
		}
	}
}

func TestSandboxDockerfileServesWebForAPHPProfile(t *testing.T) {
	spec := deploy.SandboxSpec{
		Profile: deploy.SandboxProfileFull,
		PHP:     "8.4",
		WebRoot: "/pub",
	}
	dockerfile, err := deploy.SandboxDockerfile(spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// The interpreter is the series the rest of the image uses, and the server
	// comes with it.
	for _, want := range []string{"'nginx'", "'php8.4-fpm'", "EXPOSE 22 80", "govard-sandbox-nginx.conf"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("a web-serving sandbox must contain %q, got:\n%s", want, dockerfile)
		}
	}
	// The entrypoint starts the tier through the service list, and the pool runs
	// as the deploy user so the application can write the directories
	// `deploy:writable` handed over.
	if !strings.Contains(dockerfile, "govard-sandbox-web") {
		t.Error("the web tier is not in the service list the entrypoint starts")
	}
	if !strings.Contains(dockerfile, "user = deployer") {
		t.Error("the FPM pool does not run as the deploy user")
	}
}

func TestSandboxDockerfileWithoutWebLeavesHTTPOff(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileBasic})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, absent := range []string{"nginx", "fpm", "govard-sandbox-web", "EXPOSE 22 80"} {
		if strings.Contains(dockerfile, absent) {
			t.Errorf("the basic profile must not carry the web tier, found %q", absent)
		}
	}
	if files := deploy.SandboxBuildFiles(deploy.SandboxSpec{Profile: deploy.SandboxProfileBasic}); len(files) != 0 {
		t.Errorf("the basic profile must copy no web files, got %v", files)
	}
}

func TestSandboxWebFilesAreTheServerAndTheInitScript(t *testing.T) {
	files := deploy.SandboxBuildFiles(deploy.SandboxSpec{
		Profile: deploy.SandboxProfilePHP,
		PHP:     "8.4",
		WebRoot: "/pub",
	})
	if len(files) != 2 {
		t.Fatalf("want the nginx server block and the init script, got %d files: %v", len(files), files)
	}

	var nginx string
	for _, content := range files {
		if strings.Contains(content, "server {") {
			nginx = content
		}
	}
	if nginx == "" {
		t.Fatal("no nginx server block among the build files")
	}
	// The two lines that make it a front controller rather than a file server.
	for _, want := range []string{
		"root /home/deployer/public_html/pub;",
		"try_files $uri $uri/ /index.php$is_args$args;",
		"fastcgi_pass unix:/run/php/govard-sandbox-fpm.sock;",
	} {
		if !strings.Contains(nginx, want) {
			t.Errorf("the server block must contain %q, got:\n%s", want, nginx)
		}
	}
}

func TestSandboxImageTagTracksTheWebRoot(t *testing.T) {
	// The copied files are part of the definition. Hashing only the Dockerfile
	// would reuse an image whose nginx serves the previous project's web root.
	tag := func(webRoot string) string {
		t.Helper()
		value, err := deploy.SandboxImageTag(deploy.SandboxSpec{
			Project: "sample-project",
			Profile: deploy.SandboxProfilePHP,
			WebRoot: webRoot,
		})
		if err != nil {
			t.Fatalf("tag for %q: %v", webRoot, err)
		}
		return value
	}
	pub, root, empty := tag("/pub"), tag("/"), tag("")
	if pub == root {
		t.Fatal("two web roots share an image tag")
	}
	if root != empty {
		t.Fatal("an unset web root must mean the project root, not another image")
	}
}

func TestSandboxUpPublishesTheWebPortAndPointsVerifyAtIt(t *testing.T) {
	root := sandboxProject(t)
	// A container that does not exist yet, because publishing the web port is part
	// of creating it: a reused container keeps the ports it was created with.
	fake := absentContainerFake()
	fake.fail["image inspect"] = "Error: No such image"

	state, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		PHP:         "8.4",
		WebRoot:     "/pub",
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if !fake.has("--publish 127.0.0.1::80") {
		t.Fatalf("the web port was not published: %v", fake.calls)
	}
	if state.WebPort == 0 {
		t.Fatal("the published web port was not read back")
	}

	remote, set, err := deploy.SandboxRemoteForTest(root, "sandbox")
	if err != nil {
		t.Fatalf("read the sandbox remote: %v", err)
	}
	if !set || remote.Deploy == nil {
		t.Fatal("up did not write a sandbox remote")
	}
	want := "http://127.0.0.1:" + itoa(state.WebPort) + "/"
	if got := remote.Deploy.Verify.URL; got != want {
		t.Fatalf("verify url = %q, want %q — without it the HTTP half of deploy:verify never runs", got, want)
	}
}

func TestSandboxUpWithoutWebPublishesNoHTTPPort(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	state, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfileBasic,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if fake.has("127.0.0.1::80") {
		t.Fatalf("a profile with no web tier must not publish an HTTP port: %v", fake.calls)
	}
	if state.WebPort != 0 {
		t.Fatalf("web port = %d, want 0", state.WebPort)
	}
	remote, _, err := deploy.SandboxRemoteForTest(root, "sandbox")
	if err != nil {
		t.Fatalf("read the sandbox remote: %v", err)
	}
	if remote.Deploy.Verify.URL != "" {
		t.Fatalf("verify url = %q, want none for a profile that serves nothing", remote.Deploy.Verify.URL)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

func TestSandboxStatusReportsTheServedURL(t *testing.T) {
	// `up` prints the URL once; `status` has to answer the same question later,
	// on a terminal that has since scrolled away.
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}

	state, err := deploy.SandboxStatus(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if state.WebPort == 0 {
		t.Fatal("status does not report the web port of a serving sandbox")
	}
	// A stopped container keeps its labels but answers nothing, and reporting a
	// URL nobody can reach would be worse than reporting none.
	stopped := sandboxFake()
	stopped.answers["image inspect"] = "sha256:abc\n"
	stopped.answers["inspect --format {{.State.Running}}"] = "false\n"
	state, err = deploy.SandboxStatus(context.Background(), deploy.NewDockerCLIForTest(stopped.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if state.Running {
		t.Fatal("the stopped fake reported a running container")
	}
}
