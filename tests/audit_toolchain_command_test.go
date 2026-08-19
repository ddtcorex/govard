package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
)

func TestAuditToolchainCommandStatusIsReadOnly(t *testing.T) {
	withGovardMagelintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	localImage := wantLocalBuildImageForTest(t, contextDigest)
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != localImage {
				return audit.ImageInspection{}, fmt.Errorf("no such image")
			}
			return audit.ImageInspection{ID: "sha256:existinglocal", Labels: map[string]string{"io.govard.audit.context-digest": contextDigest}}, nil
		},
		pull: func(context.Context, string) error {
			return fmt.Errorf("status must never pull")
		},
		build: func(context.Context, string, string, map[string]string) error {
			return fmt.Errorf("status must never build")
		},
	}
	project := auditToolchainCommandDirectory(t)
	installAuditToolchainManager(t, docker)

	output, err := executeAuditCommand(t, project, []string{"audit", "toolchain", "status", "--format", "json"})
	if err != nil {
		t.Fatalf("audit toolchain status returned error: %v", err)
	}
	status := decodeAuditToolchainJSON(t, output)
	for _, field := range []string{"provider", "present", "context_digest", "local_build_image", "local_build_present", "toolchain"} {
		if _, ok := status[field]; !ok {
			t.Fatalf("status JSON is missing identity field %q: %s", field, output)
		}
	}
	toolchain, ok := status["toolchain"].(map[string]any)
	if !ok {
		t.Fatalf("status JSON toolchain identity = %#v, want an object", status["toolchain"])
	}
	for _, field := range []string{"image", "image_digest", "context_digest", "local_build"} {
		if _, ok := toolchain[field]; !ok {
			t.Fatalf("status JSON toolchain is missing identity field %q: %s", field, output)
		}
	}
	if toolchain["image"] != localImage {
		t.Fatalf("status image = %v, want %q", toolchain["image"], localImage)
	}
	if docker.PullCount() != 0 || docker.BuildCount() != 0 {
		t.Fatalf("pull count = %d and build count = %d, want 0 and 0: status must be read only", docker.PullCount(), docker.BuildCount())
	}
}

func TestAuditToolchainCommandStatusTextFormatShowsImageIdentity(t *testing.T) {
	// Text is the default format, so the identity must be readable there and
	// never rendered as a pointer address (fmt's "%+v" only dereferences a
	// top-level pointer, so a nested pointer field would print as hex).
	withGovardMagelintDigestForTest(t, testOfficialDigestHex)
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
	project := auditToolchainCommandDirectory(t)
	installAuditToolchainManager(t, docker)

	output, err := executeAuditCommand(t, project, []string{"audit", "toolchain", "status"})
	if err != nil {
		t.Fatalf("audit toolchain status returned error: %v", err)
	}
	rendered := string(output)
	if strings.Contains(rendered, "0x") {
		t.Fatalf("default text output renders a pointer address instead of the image identity: %s", rendered)
	}
	for _, want := range []string{localImage, "sha256:existinglocal", contextDigest} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("default text output is missing %q: %s", want, rendered)
		}
	}
}

func TestAuditToolchainCommandStatusRepairGuidanceCarriesNoSecrets(t *testing.T) {
	withGovardMagelintDigestForTest(t, testOfficialDigestHex)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".composer"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".composer", "auth.json"), []byte("{\"http-basic\":{}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(home, "agent.sock"))
	docker := &fakeToolchainDocker{
		inspect: func(context.Context, string) (audit.ImageInspection, error) {
			return audit.ImageInspection{}, fmt.Errorf("no such image")
		},
	}
	project := auditToolchainCommandDirectory(t)
	installAuditToolchainManager(t, docker)

	output, err := executeAuditCommand(t, project, []string{"audit", "toolchain", "status", "--format", "json"})
	if err != nil {
		t.Fatalf("audit toolchain status returned error: %v", err)
	}
	status := decodeAuditToolchainJSON(t, output)
	if status["present"] != false {
		t.Fatalf("status present = %v, want false: nothing is available locally", status["present"])
	}
	repair, ok := status["repair"].(string)
	if !ok || strings.TrimSpace(repair) == "" {
		t.Fatalf("status JSON carries no repair guidance: %s", output)
	}
	assertNoSecretRepairGuidance(t, repair)
	assertNoSecretRepairGuidance(t, string(output))
	if strings.Contains(string(output), home) {
		t.Fatalf("status output leaks a host credential path: %s", output)
	}
}

func TestAuditToolchainCommandPullNeverBuilds(t *testing.T) {
	withGovardMagelintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	imageRef := wantOfficialImageRefForTest(t)
	var inspectCalls atomic.Int32
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != imageRef {
				return audit.ImageInspection{}, fmt.Errorf("unexpected inspect image %q", image)
			}
			if inspectCalls.Add(1) == 1 {
				return audit.ImageInspection{}, fmt.Errorf("no such image")
			}
			return audit.ImageInspection{ID: "sha256:pulledofficial", Labels: validOfficialLabelsForTest(contextDigest)}, nil
		},
		build: func(context.Context, string, string, map[string]string) error {
			return fmt.Errorf("pull must never build")
		},
	}
	project := auditToolchainCommandDirectory(t)
	installAuditToolchainManager(t, docker)

	output, err := executeAuditCommand(t, project, []string{"audit", "toolchain", "pull", "--format", "json"})
	if err != nil {
		t.Fatalf("audit toolchain pull returned error: %v", err)
	}
	identity := decodeAuditToolchainJSON(t, output)
	if identity["image"] != imageRef {
		t.Fatalf("pull image = %v, want %q", identity["image"], imageRef)
	}
	if identity["local_build"] != false {
		t.Fatalf("pull local_build = %v, want false", identity["local_build"])
	}
	if docker.BuildCount() != 0 {
		t.Fatalf("build count = %d, want 0: pull must never build", docker.BuildCount())
	}
	if docker.PullCount() != 1 {
		t.Fatalf("pull count = %d, want 1", docker.PullCount())
	}
}

func TestAuditToolchainCommandPullFailureGuidanceCarriesNoSecrets(t *testing.T) {
	withGovardMagelintDigestForTest(t, testOfficialDigestHex)
	docker := &fakeToolchainDocker{
		inspect: func(context.Context, string) (audit.ImageInspection, error) {
			return audit.ImageInspection{}, fmt.Errorf("no such image")
		},
		pull: func(context.Context, string) error {
			return fmt.Errorf("registry unreachable")
		},
		build: func(context.Context, string, string, map[string]string) error {
			return fmt.Errorf("pull must never build")
		},
	}
	project := auditToolchainCommandDirectory(t)
	installAuditToolchainManager(t, docker)

	_, err := executeAuditCommand(t, project, []string{"audit", "toolchain", "pull"})
	if err == nil {
		t.Fatal("audit toolchain pull did not fail when the official image was unavailable")
	}
	if !strings.Contains(err.Error(), "govard audit toolchain build") {
		t.Fatalf("error = %v, want actionable repair guidance", err)
	}
	assertNoSecretRepairGuidance(t, err.Error())
	if docker.BuildCount() != 0 {
		t.Fatalf("build count = %d, want 0", docker.BuildCount())
	}
}

func TestAuditToolchainCommandBuildNeverPulls(t *testing.T) {
	withGovardMagelintDigestForTest(t, testOfficialDigestHex)
	contextDigest := auditLintContextDigestForTest(t)
	localImage := wantLocalBuildImageForTest(t, contextDigest)
	var built atomic.Bool
	docker := &fakeToolchainDocker{
		inspect: func(_ context.Context, image string) (audit.ImageInspection, error) {
			if image != localImage {
				return audit.ImageInspection{}, fmt.Errorf("unexpected inspect image %q", image)
			}
			if built.Load() {
				return audit.ImageInspection{ID: "sha256:builtlocal", Labels: map[string]string{"io.govard.audit.context-digest": contextDigest}}, nil
			}
			return audit.ImageInspection{}, fmt.Errorf("no such local image")
		},
		pull: func(context.Context, string) error {
			return fmt.Errorf("build must never pull")
		},
		build: func(_ context.Context, _, image string, _ map[string]string) error {
			if image != localImage {
				return fmt.Errorf("unexpected build image %q", image)
			}
			built.Store(true)
			return nil
		},
	}
	project := auditToolchainCommandDirectory(t)
	installAuditToolchainManager(t, docker)

	output, err := executeAuditCommand(t, project, []string{"audit", "toolchain", "build", "--format", "json"})
	if err != nil {
		t.Fatalf("audit toolchain build returned error: %v", err)
	}
	identity := decodeAuditToolchainJSON(t, output)
	if identity["image"] != localImage {
		t.Fatalf("build image = %v, want %q", identity["image"], localImage)
	}
	if identity["local_build"] != true {
		t.Fatalf("build local_build = %v, want true", identity["local_build"])
	}
	if identity["context_digest"] != contextDigest {
		t.Fatalf("build context_digest = %v, want %q", identity["context_digest"], contextDigest)
	}
	if docker.PullCount() != 0 {
		t.Fatalf("pull count = %d, want 0: build must never pull", docker.PullCount())
	}
	if docker.BuildCount() != 1 {
		t.Fatalf("build count = %d, want 1", docker.BuildCount())
	}
}

func TestAuditToolchainCommandsRunOutsideAGovardProject(t *testing.T) {
	withGovardMagelintDigestForTest(t, "")
	docker := &fakeToolchainDocker{
		inspect: func(context.Context, string) (audit.ImageInspection, error) {
			return audit.ImageInspection{}, fmt.Errorf("no such image")
		},
	}
	// A directory with no .govard.yml and no framework markers: the toolchain
	// commands manage a machine-wide image and must never resolve a target.
	outside := t.TempDir()
	installAuditToolchainManager(t, docker)

	if _, err := executeAuditCommand(t, outside, []string{"audit", "toolchain", "status", "--format", "json"}); err != nil {
		t.Fatalf("audit toolchain status outside a project returned error: %v", err)
	}
}

func auditToolchainCommandDirectory(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func installAuditToolchainManager(t *testing.T, docker audit.DockerClient) {
	t.Helper()
	cacheRoot := t.TempDir()
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{
		ToolchainFactory: func() (*audit.ToolchainManager, error) {
			return audit.NewToolchainManager(docker, cacheRoot), nil
		},
	})
	t.Cleanup(restore)
}

func decodeAuditToolchainJSON(t *testing.T, output []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("output is not a single undecorated JSON object: %v; output=%q", err, output)
	}
	return decoded
}
