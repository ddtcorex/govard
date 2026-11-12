//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// warmupTimeout bounds one fixture's image build. A cold sandbox image pulls a
// base distribution and installs packages, so it is minutes, but a hung build
// must not eat the suite's own `go test -timeout` and abort the run with the
// cleanup unrun.
const warmupTimeout = 15 * time.Minute

// sandboxFixtureProjects names the projects the suite owns. Cleanup is scoped to
// them in both directions: an artifact this run created AND that belongs to one of
// these projects is removed. The ownership test is the one that protects a
// developer's machine, because a snapshot taken while the Docker daemon was
// starting returns nothing, and "everything is new" would otherwise mean "delete
// every sandbox image, container and database volume on this host".
var sandboxFixtureProjects = []string{"deploy-magento-stub", "deploy-code-only"}

// sandboxWarmups lists the fixture and profile pairs the suite's tests ask for.
// The image tag carries the project, the profile and a hash of the definition, so
// each pair is its own image and a pair missing here is a build inside a timed
// test instead.
var sandboxWarmups = []struct {
	fixture string
	profile string
}{
	{"deploy/magento-stub", "php"},
	{"deploy/magento-stub", "full"},
	// The DB-backup refusal test runs the same stub fixture on the lightest
	// profile, which is a different image because the tag carries the profile.
	{"deploy/magento-stub", "basic"},
	{"deploy/code-only", "basic"},
}

func TestMain(m *testing.M) {
	originalGovardHome, hadGovardHome := os.LookupEnv("GOVARD_HOME_DIR")

	tempGovardHome, err := os.MkdirTemp("", "govard-integration-home-*")
	if err != nil {
		panic(err)
	}

	if err := os.Setenv("GOVARD_HOME_DIR", tempGovardHome); err != nil {
		panic(err)
	}

	// What already existed on this machine, so the cleanup removes the artifacts
	// this run created and nothing else: a developer's own sandbox image,
	// container or database volume is not the suite's to delete, and the sandbox
	// database volume holds real data.
	preExisting := snapshotSandboxArtifacts()

	// Build every sandbox image the suite can reuse before any test is timed.
	// The image is cacheable by construction (its tag hashes the definition), but
	// the build used to land inside whichever test ran first, and every test used
	// to purge its image on the way out.
	warmSandboxImages()

	code := m.Run()

	purgeSandboxArtifacts(preExisting)

	_ = os.RemoveAll(tempGovardHome)
	if hadGovardHome {
		_ = os.Setenv("GOVARD_HOME_DIR", originalGovardHome)
	} else {
		_ = os.Unsetenv("GOVARD_HOME_DIR")
	}

	os.Exit(code)
}

// warmSandboxImages builds the sandbox images the suite's fixtures ask for, then
// removes the containers and leaves the images for the tests to reuse.
//
// The copy handed to `sandbox up` has to be a git repository: `up` refreshes the
// container's mirror from the project root before it builds anything, and a bare
// fixture copy makes that fetch fail, which turns the whole warm-up into a no-op
// whose images are then built inside the timed tests. Failures are reported
// rather than swallowed for the same reason: the tests build on demand, so a
// broken warm-up is invisible in a green run and shows up later as a budget
// breach on a cold host.
func warmSandboxImages() {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return
	}
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	binary := filepath.Join(projectRoot, "bin", "govard-test")
	buildGovardBinaryOnce.Do(func() {
		buildGovardBinaryErr = buildTestBinary(projectRoot, binary)
	})
	if buildGovardBinaryErr != nil {
		fmt.Fprintf(os.Stderr, "sandbox warm-up skipped: %v\n", buildGovardBinaryErr)
		return
	}

	for _, warmup := range sandboxWarmups {
		source := filepath.Join(projectRoot, "tests", "integration", "projects", filepath.FromSlash(warmup.fixture))
		workdir, err := os.MkdirTemp("", "govard-warmup-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "sandbox warm-up skipped for %s/%s: %v\n", warmup.fixture, warmup.profile, err)
			continue
		}
		failed := func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "sandbox warm-up failed for %s/%s: "+format+"\n",
				append([]any{warmup.fixture, warmup.profile}, args...)...)
			_ = os.RemoveAll(workdir)
		}

		if err := copyTree(source, workdir); err != nil {
			failed("%v", err)
			continue
		}
		if output, err := runInDir(workdir, "git", "init", "-q", "-b", "main"); err != nil {
			failed("git init: %v\n%s", err, output)
			continue
		}
		if output, err := runInDir(workdir, "git", "add", "-A"); err != nil {
			failed("git add: %v\n%s", err, output)
			continue
		}
		// --allow-empty: a developer's global gitignore can exclude the fixture's
		// .govard.yml (this machine's does, with ".govard*.yml"), and a warm-up copy
		// whose only file is ignored still needs the commit that makes the directory
		// a repository the mirror can fetch from.
		commit := exec.Command("git", "commit", "-q", "--allow-empty", "-m", "warm-up")
		commit.Dir = workdir
		commit.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Integration", "GIT_AUTHOR_EMAIL=integration@example.com",
			"GIT_COMMITTER_NAME=Integration", "GIT_COMMITTER_EMAIL=integration@example.com",
		)
		if output, err := commit.CombinedOutput(); err != nil {
			failed("git commit: %v\n%s", err, output)
			continue
		}

		// `down` without --purge is the point: the container goes, the image stays
		// for the tests to reuse.
		if output, err := runWithTimeout(warmupTimeout, workdir, binary, "sandbox", "up", "--profile", warmup.profile, "--no-seed"); err != nil {
			failed("sandbox up: %v\n%s", err, output)
			continue
		}
		if output, err := runWithTimeout(warmupTimeout, workdir, binary, "sandbox", "down"); err != nil {
			// The image is built by now, which is what the warm-up is for; a
			// container left behind is reported and swept by the suite purge.
			fmt.Fprintf(os.Stderr, "sandbox warm-up left a container for %s/%s: %v\n%s\n",
				warmup.fixture, warmup.profile, err, output)
		}
		_ = os.RemoveAll(workdir)
	}
}

// runWithTimeout runs a command with a deadline, which is what keeps a hung
// Docker build from taking the suite's own timeout with it.
func runWithTimeout(timeout time.Duration, dir, binary string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), fmt.Errorf("%s timed out after %s", binary, timeout)
	}
	return string(output), err
}

// runInDir runs one command in dir and returns its combined output.
func runInDir(dir, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	return string(output), err
}

// copyTree copies a fixture directory without a *testing.T, so the warm-up can
// run before any test exists.
func copyTree(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// purgeSandboxArtifacts removes the sandbox artifacts this run created and that
// belong to the suite's own fixtures. Best effort by design: a machine without
// Docker, or a Docker that is already stopping, must not turn a suite result into
// a cleanup error.
func purgeSandboxArtifacts(preExisting map[string]bool) {
	// A snapshot that came back empty because Docker was unreachable says nothing
	// about what was already on the machine, so nothing is safe to delete.
	if preExisting == nil {
		fmt.Fprintln(os.Stderr, "sandbox cleanup skipped: the pre-run Docker snapshot was unavailable")
		return
	}
	for _, container := range ownedSandboxContainers() {
		if !preExisting["container:"+container] {
			_ = exec.Command("docker", "rm", "-f", container).Run()
		}
	}
	for _, image := range ownedSandboxImages() {
		if !preExisting["image:"+image] {
			_ = exec.Command("docker", "rmi", "-f", image).Run()
		}
	}
	// Volumes are data, and `sandbox down` without --purge keeps them so a
	// developer can inspect what a failed case left. The suite discards the ones
	// it made: they are named per project, so the next run would silently reuse a
	// database an earlier test wrote to.
	for _, volume := range ownedSandboxVolumes() {
		if !preExisting["volume:"+volume] {
			_ = exec.Command("docker", "volume", "rm", "-f", volume).Run()
		}
	}
}

// snapshotSandboxArtifacts records what is already on the machine, keyed so an
// image, a container and a volume cannot shadow each other. It answers nil when
// the Docker daemon cannot be reached, which is the signal the cleanup uses to
// stay away entirely.
func snapshotSandboxArtifacts() map[string]bool {
	if !dockerReachable() {
		return nil
	}
	existing := make(map[string]bool)
	for _, container := range ownedSandboxContainers() {
		existing["container:"+container] = true
	}
	for _, image := range ownedSandboxImages() {
		existing["image:"+image] = true
	}
	for _, volume := range ownedSandboxVolumes() {
		existing["volume:"+volume] = true
	}
	return existing
}

func dockerReachable() bool {
	return exec.Command("docker", "version", "--format", "{{.Server.Version}}").Run() == nil
}

// ownedSandboxContainers lists the sandbox containers of the suite's fixtures.
// The project label is the ownership test: a container another project created
// carries another project's name and is never listed here.
func ownedSandboxContainers() []string {
	ids := make([]string, 0)
	for _, project := range sandboxFixtureProjects {
		ids = append(ids, dockerLines("ps", "-a",
			"--filter", "label=govard.sandbox.project="+project,
			"--format", "{{.ID}}")...)
	}
	return ids
}

// ownedSandboxImages lists the sandbox images of the suite's fixtures. The tag is
// govard-sandbox:<project-slug>-<profile>-<definition hash>, so the project's
// padded slug is the ownership test.
func ownedSandboxImages() []string {
	tags := make([]string, 0)
	for _, tag := range dockerLines("image", "ls", "--format", "{{.Repository}}:{{.Tag}}", "govard-sandbox") {
		for _, project := range sandboxFixtureProjects {
			if strings.HasPrefix(tag, "govard-sandbox:"+project+"-") {
				tags = append(tags, tag)
				break
			}
		}
	}
	return tags
}

// ownedSandboxVolumes lists the database volumes whose recorded owner is one of
// the suite's containers.
func ownedSandboxVolumes() []string {
	names := make([]string, 0)
	for _, line := range dockerLines("volume", "ls",
		"--filter", "label=govard.sandbox.db-volume",
		"--format", `{{.Name}}	{{.Label "govard.sandbox.db-volume"}}`) {
		name, owner, found := strings.Cut(line, "\t")
		if !found {
			continue
		}
		for _, project := range sandboxFixtureProjects {
			if strings.HasPrefix(owner, "govard-"+project+"-sandbox") {
				names = append(names, name)
				break
			}
		}
	}
	return names
}

func dockerLines(args ...string) []string {
	output, err := exec.Command("docker", args...).Output()
	if err != nil {
		return nil
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(string(output), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
