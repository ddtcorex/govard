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

// purgeSandboxArtifacts removes the sandbox images, containers and database
// volumes this run created, leaving anything that was already on the machine.
// Best effort by design: a machine without Docker, or a Docker that is already
// stopping, must not turn a suite result into a cleanup error.
func purgeSandboxArtifacts(preExisting map[string]bool) {
	for _, container := range sandboxContainerIDs() {
		if !preExisting["container:"+container] {
			_ = exec.Command("docker", "rm", "-f", container).Run()
		}
	}
	for _, image := range sandboxImageTags() {
		if !preExisting["image:"+image] {
			_ = exec.Command("docker", "rmi", "-f", image).Run()
		}
	}
	// Volumes are data, and `sandbox down` without --purge keeps them so a
	// developer can inspect what a failed case left. The suite discards the ones
	// it made: they are named per project, so the next run would silently reuse a
	// database an earlier test wrote to.
	for _, volume := range sandboxVolumeNames() {
		if !preExisting["volume:"+volume] {
			_ = exec.Command("docker", "volume", "rm", "-f", volume).Run()
		}
	}
}

// snapshotSandboxArtifacts records what is already on the machine, keyed so an
// image, a container and a volume cannot shadow each other.
func snapshotSandboxArtifacts() map[string]bool {
	existing := make(map[string]bool)
	for _, container := range sandboxContainerIDs() {
		existing["container:"+container] = true
	}
	for _, image := range sandboxImageTags() {
		existing["image:"+image] = true
	}
	for _, volume := range sandboxVolumeNames() {
		existing["volume:"+volume] = true
	}
	return existing
}

// sandboxContainerIDs lists the sandbox containers on this machine, and answers
// nil when Docker cannot be reached at all.
func sandboxContainerIDs() []string {
	return dockerLines("ps", "-a", "--filter", "label=govard.sandbox=1", "--format", "{{.ID}}")
}

// sandboxImageTags lists the sandbox images on this machine.
func sandboxImageTags() []string {
	return dockerLines("image", "ls", "--format", "{{.Repository}}:{{.Tag}}", "govard-sandbox")
}

// sandboxVolumeNames lists the sandbox database volumes on this machine.
func sandboxVolumeNames() []string {
	return dockerLines("volume", "ls", "--filter", "label=govard.sandbox.db-volume", "--format", "{{.Name}}")
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
