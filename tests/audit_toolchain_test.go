package tests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	auditmagento "govard/docker/audit"
	"govard/internal/audit"
)

const testOfficialDigestHex = "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2"

func TestLintToolchainManagerReusesLocalOfficialImage(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != wantOfficialImageRefForTest(t) {
				return audit.ImageInspection{}, fmt.Errorf("unexpected inspect image %q", image)
			}
			return audit.ImageInspection{ID: "sha256:localofficial", Labels: validOfficialLabelsForTest(contextDigest)}, nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	resolved, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if resolved.LocalBuild {
		t.Fatalf("resolved toolchain used local build: %#v", resolved)
	}
	if resolved.Image != wantOfficialImageRefForTest(t) {
		t.Fatalf("image = %q, want %q", resolved.Image, wantOfficialImageRefForTest(t))
	}
	if resolved.ContextDigest != contextDigest {
		t.Fatalf("context digest = %q, want %q", resolved.ContextDigest, contextDigest)
	}
	if docker.PullCount() != 0 {
		t.Fatalf("pull count = %d, want 0 for an already-local official image", docker.PullCount())
	}
	if docker.BuildCount() != 0 {
		t.Fatalf("build count = %d, want 0 when the official image resolves", docker.BuildCount())
	}
}

func TestLintToolchainManagerPullsAndInspectsOfficialImage(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	imageRef := wantOfficialImageRefForTest(t)
	var inspectCalls int
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			inspectCalls++
			if inspectCalls == 1 {
				return audit.ImageInspection{}, fmt.Errorf("no such image")
			}
			return audit.ImageInspection{ID: "sha256:pulledofficial", Labels: validOfficialLabelsForTest(contextDigest)}, nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	resolved, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if resolved.LocalBuild {
		t.Fatalf("resolved toolchain used local build: %#v", resolved)
	}
	if resolved.Image != imageRef {
		t.Fatalf("image = %q, want %q", resolved.Image, imageRef)
	}
	if docker.PullCount() != 1 {
		t.Fatalf("pull count = %d, want 1", docker.PullCount())
	}
	if docker.BuildCount() != 0 {
		t.Fatalf("build count = %d, want 0 when the official image resolves after pull", docker.BuildCount())
	}
}

func TestLintToolchainManagerFallsBackToEmbeddedBuildOnPullFailure(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	localImage := wantLocalBuildImageForTest(t, contextDigest)
	var built atomic.Bool
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image == localImage {
				if built.Load() {
					return audit.ImageInspection{ID: "sha256:builtlocal", Labels: map[string]string{"io.govard.audit.context-digest": contextDigest}}, nil
				}
				return audit.ImageInspection{}, fmt.Errorf("no such local image")
			}
			return audit.ImageInspection{}, fmt.Errorf("no such official image")
		},
		pull: func(context.Context, string) error {
			return fmt.Errorf("registry unreachable")
		},
		build: func(_ context.Context, _, image string, _ map[string]string) error {
			if image != localImage {
				return fmt.Errorf("unexpected build image %q", image)
			}
			built.Store(true)
			return nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	resolved, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if !resolved.LocalBuild {
		t.Fatalf("resolved toolchain did not fall back to a local build: %#v", resolved)
	}
	if resolved.Image != localImage {
		t.Fatalf("image = %q, want %q", resolved.Image, localImage)
	}
	if docker.BuildCount() != 1 {
		t.Fatalf("build count = %d, want 1", docker.BuildCount())
	}
}

func TestLintToolchainManagerFallsBackToEmbeddedBuildOnWrongLabels(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	imageRef := wantOfficialImageRefForTest(t)
	localImage := wantLocalBuildImageForTest(t, contextDigest)
	var built atomic.Bool
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image == imageRef {
				wrongLabels := validOfficialLabelsForTest(contextDigest)
				wrongLabels["io.govard.audit.context-digest"] = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
				return audit.ImageInspection{ID: "sha256:wronglabels", Labels: wrongLabels}, nil
			}
			if built.Load() {
				return audit.ImageInspection{ID: "sha256:builtlocal", Labels: map[string]string{"io.govard.audit.context-digest": contextDigest}}, nil
			}
			return audit.ImageInspection{}, fmt.Errorf("no such local image")
		},
		build: func(_ context.Context, _, image string, _ map[string]string) error {
			if image != localImage {
				return fmt.Errorf("unexpected build image %q", image)
			}
			built.Store(true)
			return nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	resolved, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if !resolved.LocalBuild {
		t.Fatalf("resolved toolchain did not fall back to a local build on wrong labels: %#v", resolved)
	}
	if resolved.Image != localImage {
		t.Fatalf("image = %q, want %q", resolved.Image, localImage)
	}
	if docker.PullCount() != 0 {
		t.Fatalf("pull count = %d, want 0 when labels are wrong (digest pinning makes re-pulling pointless)", docker.PullCount())
	}
	if docker.BuildCount() != 1 {
		t.Fatalf("build count = %d, want 1", docker.BuildCount())
	}
}

func TestLintToolchainManagerFallsBackToEmbeddedBuildOnMalformedDigest(t *testing.T) {
	// A malformed release digest is baked in at build time by Task 9's ldflags
	// injection, not a transient runtime condition. It must still degrade to
	// the embedded build exactly like every other official-path failure
	// (pull failure, inspect failure, wrong labels) rather than bricking the
	// audit feature for the entire life of a release with no workaround.
	withGovardGlintDigestForTest(t, "not-a-valid-sha256-digest")
	contextDigest := auditLintContextDigestForTest(t)
	localImage := wantLocalBuildImageForTest(t, contextDigest)
	var built atomic.Bool
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != localImage {
				return audit.ImageInspection{}, fmt.Errorf("unexpected inspect image %q: official path must not be attempted", image)
			}
			if built.Load() {
				return audit.ImageInspection{ID: "sha256:builtlocal", Labels: map[string]string{"io.govard.audit.context-digest": contextDigest}}, nil
			}
			return audit.ImageInspection{}, fmt.Errorf("no such local image")
		},
		pull: func(_ context.Context, image string) error {
			return fmt.Errorf("unexpected pull of %q: official path must not be attempted", image)
		},
		build: func(_ context.Context, _, image string, _ map[string]string) error {
			if image != localImage {
				return fmt.Errorf("unexpected build image %q", image)
			}
			built.Store(true)
			return nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	resolved, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if !resolved.LocalBuild {
		t.Fatalf("resolved toolchain did not fall back to a local build on a malformed digest: %#v", resolved)
	}
	if resolved.Image != localImage {
		t.Fatalf("image = %q, want %q", resolved.Image, localImage)
	}
	if docker.PullCount() != 0 {
		t.Fatalf("pull count = %d, want 0: a malformed digest must never attempt a pull", docker.PullCount())
	}
	if docker.BuildCount() != 1 {
		t.Fatalf("build count = %d, want 1", docker.BuildCount())
	}
}

func TestLintToolchainManagerReturnsErrorWhenPullAndBuildBothFail(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	docker := &fakeToolchainDocker{
		inspect: func(context.Context, string) (audit.ImageInspection, error) {
			return audit.ImageInspection{}, fmt.Errorf("no such image")
		},
		pull: func(context.Context, string) error {
			return fmt.Errorf("registry unreachable")
		},
		build: func(context.Context, string, string, map[string]string) error {
			return fmt.Errorf("build toolchain failed")
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	_, err := manager.Ensure(context.Background())
	if err == nil {
		t.Fatal("Ensure did not return an error when both pull and build failed")
	}
	if !strings.Contains(err.Error(), "build toolchain failed") {
		t.Fatalf("error = %v, want it to mention the build failure", err)
	}
}

func TestLintToolchainManagerReusesLocalBuildImage(t *testing.T) {
	withGovardGlintDigestForTest(t, "")
	contextDigest := auditLintContextDigestForTest(t)
	localImage := wantLocalBuildImageForTest(t, contextDigest)
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != localImage {
				return audit.ImageInspection{}, fmt.Errorf("unexpected inspect image %q", image)
			}
			return audit.ImageInspection{ID: "sha256:existinglocal", Labels: map[string]string{"io.govard.audit.context-digest": contextDigest}}, nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	resolved, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if !resolved.LocalBuild {
		t.Fatalf("resolved toolchain did not use the local build: %#v", resolved)
	}
	if resolved.Image != localImage {
		t.Fatalf("image = %q, want %q", resolved.Image, localImage)
	}
	if docker.BuildCount() != 0 {
		t.Fatalf("build count = %d, want 0 when the local build image already exists", docker.BuildCount())
	}
	if docker.PullCount() != 0 {
		t.Fatalf("pull count = %d, want 0 with no configured official digest", docker.PullCount())
	}
}

func TestLintToolchainManagerCancellationDoesNotFallBack(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	started := make(chan struct{})
	docker := &fakeToolchainDocker{
		inspect: func(context.Context, string) (audit.ImageInspection, error) {
			return audit.ImageInspection{}, fmt.Errorf("no such image")
		},
		pull: func(ctx context.Context, _ string) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := manager.Ensure(ctx)
		done <- err
	}()
	<-started
	cancel()

	select {
	case err := <-done:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("Ensure error = %v, want a wrapped context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Ensure did not return after cancellation")
	}
	if docker.BuildCount() != 0 {
		t.Fatalf("build count = %d, want 0: cancellation must not trigger the embedded build fallback", docker.BuildCount())
	}
}

func TestLintToolchainManagerPullNeverBuilds(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	imageRef := wantOfficialImageRefForTest(t)
	var inspectCalls int
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != imageRef {
				return audit.ImageInspection{}, fmt.Errorf("unexpected inspect image %q: pull must only touch the official path", image)
			}
			inspectCalls++
			if inspectCalls == 1 {
				return audit.ImageInspection{}, fmt.Errorf("no such image")
			}
			return audit.ImageInspection{ID: "sha256:pulledofficial", Labels: validOfficialLabelsForTest(contextDigest)}, nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	resolved, err := manager.Pull(context.Background())
	if err != nil {
		t.Fatalf("Pull returned error: %v", err)
	}
	if resolved.LocalBuild || resolved.Image != imageRef {
		t.Fatalf("resolved toolchain = %#v, want the pinned official image %q", resolved, imageRef)
	}
	if docker.PullCount() != 1 {
		t.Fatalf("pull count = %d, want 1", docker.PullCount())
	}
	if docker.BuildCount() != 0 {
		t.Fatalf("build count = %d, want 0: pull must never build", docker.BuildCount())
	}
}

func TestLintToolchainManagerPullFailsWithoutBuildingWhenOfficialImageIsUnusable(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	docker := &fakeToolchainDocker{
		inspect: func(context.Context, string) (audit.ImageInspection, error) {
			return audit.ImageInspection{}, fmt.Errorf("no such image")
		},
		pull: func(context.Context, string) error {
			return fmt.Errorf("registry unreachable")
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	_, err := manager.Pull(context.Background())
	if err == nil {
		t.Fatal("Pull did not return an error when the official image could not be obtained")
	}
	if docker.BuildCount() != 0 {
		t.Fatalf("build count = %d, want 0: a failed pull must never fall back to a build", docker.BuildCount())
	}
	assertNoSecretRepairGuidance(t, err.Error())
}

func TestLintToolchainManagerPullRefusesUnusableConfiguredDigests(t *testing.T) {
	for name, digest := range map[string]string{
		"unset":     "",
		"malformed": "not-a-valid-sha256-digest",
	} {
		t.Run(name, func(t *testing.T) {
			withGovardGlintDigestForTest(t, digest)
			docker := &fakeToolchainDocker{}
			manager := audit.NewToolchainManager(docker, t.TempDir())

			if _, err := manager.Pull(context.Background()); err == nil {
				t.Fatal("Pull did not return an error for an unusable configured digest")
			}
			if docker.PullCount() != 0 {
				t.Fatalf("pull count = %d, want 0", docker.PullCount())
			}
			if docker.BuildCount() != 0 {
				t.Fatalf("build count = %d, want 0", docker.BuildCount())
			}
		})
	}
}

func TestLintToolchainManagerBuildNeverPulls(t *testing.T) {
	// A configured official digest must not tempt Build into the pull path.
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	localImage := wantLocalBuildImageForTest(t, contextDigest)
	var built atomic.Bool
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != localImage {
				return audit.ImageInspection{}, fmt.Errorf("unexpected inspect image %q: build must only touch the local build path", image)
			}
			if built.Load() {
				return audit.ImageInspection{ID: "sha256:builtlocal", Labels: map[string]string{"io.govard.audit.context-digest": contextDigest}}, nil
			}
			return audit.ImageInspection{}, fmt.Errorf("no such local image")
		},
		build: func(_ context.Context, _, image string, _ map[string]string) error {
			if image != localImage {
				return fmt.Errorf("unexpected build image %q", image)
			}
			built.Store(true)
			return nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	resolved, err := manager.Build(context.Background())
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if !resolved.LocalBuild || resolved.Image != localImage {
		t.Fatalf("resolved toolchain = %#v, want the local build image %q", resolved, localImage)
	}
	if docker.BuildCount() != 1 {
		t.Fatalf("build count = %d, want 1", docker.BuildCount())
	}
	if docker.PullCount() != 0 {
		t.Fatalf("pull count = %d, want 0: build must never pull", docker.PullCount())
	}
}

func TestLintToolchainManagerStatusIsReadOnly(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	localImage := wantLocalBuildImageForTest(t, contextDigest)
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != localImage {
				return audit.ImageInspection{}, fmt.Errorf("no such image")
			}
			return audit.ImageInspection{ID: "sha256:existinglocal", Labels: map[string]string{"io.govard.audit.context-digest": contextDigest}}, nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !status.Present || !status.LocalBuildPresent {
		t.Fatalf("status = %#v, want an already-present local build", status)
	}
	if status.OfficialUsable {
		t.Fatalf("status = %#v, want the absent official image reported as unusable", status)
	}
	if status.Toolchain.Image != localImage || !status.Toolchain.LocalBuild {
		t.Fatalf("status toolchain = %#v, want the local build image %q", status.Toolchain, localImage)
	}
	if status.ContextDigest != contextDigest {
		t.Fatalf("context digest = %q, want %q", status.ContextDigest, contextDigest)
	}
	if status.OfficialImage != wantOfficialImageRefForTest(t) {
		t.Fatalf("official image = %q, want %q", status.OfficialImage, wantOfficialImageRefForTest(t))
	}
	if docker.PullCount() != 0 || docker.BuildCount() != 0 {
		t.Fatalf("pull count = %d and build count = %d, want 0 and 0: status must be read only", docker.PullCount(), docker.BuildCount())
	}
}

func TestLintToolchainManagerStatusReportsNothingPresentWithoutPullingOrBuilding(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	docker := &fakeToolchainDocker{
		inspect: func(context.Context, string) (audit.ImageInspection, error) {
			return audit.ImageInspection{}, fmt.Errorf("no such image")
		},
		pull: func(context.Context, string) error {
			return fmt.Errorf("status must never pull")
		},
		build: func(context.Context, string, string, map[string]string) error {
			return fmt.Errorf("status must never build")
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.Present || status.LocalBuildPresent || status.OfficialUsable {
		t.Fatalf("status = %#v, want nothing reported as present", status)
	}
	if docker.PullCount() != 0 || docker.BuildCount() != 0 {
		t.Fatalf("pull count = %d and build count = %d, want 0 and 0", docker.PullCount(), docker.BuildCount())
	}
}

func TestLintToolchainManagerStatusPrefersUsableOfficialImage(t *testing.T) {
	withGovardGlintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	imageRef := wantOfficialImageRefForTest(t)
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != imageRef {
				return audit.ImageInspection{}, fmt.Errorf("no such image")
			}
			return audit.ImageInspection{ID: "sha256:localofficial", Labels: validOfficialLabelsForTest(contextDigest)}, nil
		},
	}
	manager := audit.NewToolchainManager(docker, t.TempDir())

	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !status.Present || !status.OfficialUsable {
		t.Fatalf("status = %#v, want the official image reported as usable", status)
	}
	if status.LocalBuildPresent {
		t.Fatalf("status = %#v, want no local build reported", status)
	}
	if status.Toolchain.LocalBuild || status.Toolchain.Image != imageRef {
		t.Fatalf("status toolchain = %#v, want the official image %q", status.Toolchain, imageRef)
	}
}

// --- fakes and helpers ---

// assertNoSecretRepairGuidance keeps credential-shaped strings out of operator
// guidance: a repair hint is printed to a terminal and pasted into issues, so it
// must never carry a Composer auth path, an agent socket, or a token.
func assertNoSecretRepairGuidance(t *testing.T, guidance string) {
	t.Helper()
	for _, forbidden := range []string{"auth.json", "SSH_AUTH_SOCK", "ssh-agent", "COMPOSER_AUTH", "password", "token", "--user"} {
		if strings.Contains(strings.ToLower(guidance), strings.ToLower(forbidden)) {
			t.Fatalf("guidance %q contains secret-shaped argument %q", guidance, forbidden)
		}
	}
}

type fakeToolchainDocker struct {
	mu sync.Mutex

	inspect func(context.Context, string) (audit.ImageInspection, error)
	pull    func(context.Context, string) error
	build   func(context.Context, string, string, map[string]string) error

	pulls  []string
	builds []string
}

func (docker *fakeToolchainDocker) Pull(ctx context.Context, image string) error {
	docker.mu.Lock()
	docker.pulls = append(docker.pulls, image)
	fn := docker.pull
	docker.mu.Unlock()
	if fn != nil {
		return fn(ctx, image)
	}
	return nil
}

func (docker *fakeToolchainDocker) Inspect(ctx context.Context, image string) (audit.ImageInspection, error) {
	docker.mu.Lock()
	fn := docker.inspect
	docker.mu.Unlock()
	if fn != nil {
		return fn(ctx, image)
	}
	return audit.ImageInspection{}, fmt.Errorf("image %q not found", image)
}

func (docker *fakeToolchainDocker) Build(ctx context.Context, contextDir, image string, labels map[string]string) error {
	docker.mu.Lock()
	docker.builds = append(docker.builds, image)
	fn := docker.build
	docker.mu.Unlock()
	if fn != nil {
		return fn(ctx, contextDir, image, labels)
	}
	return nil
}

func (docker *fakeToolchainDocker) Run(context.Context, audit.ContainerRunRequest, io.Writer) error {
	return fmt.Errorf("toolchain manager must never run a container")
}

func (docker *fakeToolchainDocker) Stop(context.Context, string, time.Duration) error {
	return fmt.Errorf("toolchain manager must never stop a container")
}

func (docker *fakeToolchainDocker) Remove(context.Context, string) error {
	return fmt.Errorf("toolchain manager must never remove a container")
}

func (docker *fakeToolchainDocker) PullCount() int {
	docker.mu.Lock()
	defer docker.mu.Unlock()
	return len(docker.pulls)
}

func (docker *fakeToolchainDocker) BuildCount() int {
	docker.mu.Lock()
	defer docker.mu.Unlock()
	return len(docker.builds)
}

func withGovardGlintDigestForTest(t *testing.T, digest string) {
	t.Helper()
	previous := audit.GovardGlintDigest
	audit.GovardGlintDigest = digest
	t.Cleanup(func() { audit.GovardGlintDigest = previous })
}

func auditLintContextDigestForTest(t *testing.T) string {
	t.Helper()
	digest, err := auditmagento.ContextDigest()
	if err != nil {
		t.Fatalf("compute embedded audit lint context digest: %v", err)
	}
	return digest
}

func validOfficialLabelsForTest(contextDigest string) map[string]string {
	return map[string]string{
		"org.opencontainers.image.revision": "abcdef1234567890",
		"io.govard.version":                 "1.62.2",
		"io.govard.audit.context-digest":    contextDigest,
		"io.govard.audit.report-schema":     strconv.Itoa(auditmagento.ReportSchemaVersion),
		"io.govard.audit.php-versions":      strings.Join(auditmagento.PHPVersions(), ","),
	}
}

func wantOfficialImageRefForTest(t *testing.T) string {
	t.Helper()
	return "ghcr.io/ddtcorex/govard-glint@sha256:" + testOfficialDigestHex
}

func wantLocalBuildImageForTest(t *testing.T, contextDigest string) string {
	t.Helper()
	trimmed := strings.TrimPrefix(contextDigest, "sha256:")
	if len(trimmed) < 16 {
		t.Fatalf("context digest %q is too short", contextDigest)
	}
	return "govard-local/glint:" + trimmed[:16]
}
