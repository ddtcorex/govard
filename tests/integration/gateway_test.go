//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGatewayEndToEnd is the shared SSH gateway's regression suite: a real
// gateway container, a real sandbox, and a real `ssh`/`sftp` client talking
// to 127.0.0.1:2222 -- proof that AuthorizedKeysCommand's key dispatch, the
// forced command's username substitution, the dynamic account reconciler,
// and the pty/exec/sftp detection in gw-router.sh all work together, not
// just that each shell script parses.
func TestGatewayEndToEnd(t *testing.T) {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed")
	}
	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("ssh is not installed")
	}
	sftpBin, err := exec.LookPath("sftp")
	if err != nil {
		t.Skip("sftp is not installed")
	}

	env := NewTestEnvironment(t)
	projectDir := env.CreateProjectFromFixture(t, "deploy/code-only", "gateway-sandbox")

	// `sandbox up` refreshes its mirror from the project checkout, so the
	// fixture must be a git checkout first -- the same seeding every
	// deploy-sandbox test performs (repo behavior wins over the brief here).
	origin, _ := seedDeployRevisions(t, 2)
	seedSandboxCheckout(t, projectDir, origin)

	up := env.RunGovard(t, projectDir, "svc", "up", "-d")
	if up.ExitCode != 0 {
		t.Fatalf("svc up failed (%d)\nstdout: %s\nstderr: %s", up.ExitCode, up.Stdout, up.Stderr)
	}

	clientKeyDir := t.TempDir()
	clientKeyPath := filepath.Join(clientKeyDir, "id_ed25519")
	if out, err := exec.Command(sshKeygen, "-q", "-t", "ed25519", "-N", "", "-f", clientKeyPath).CombinedOutput(); err != nil {
		t.Fatalf("generate client key: %v\n%s", err, out)
	}
	clientPub, err := os.ReadFile(clientKeyPath + ".pub")
	if err != nil {
		t.Fatalf("read client public key: %v", err)
	}

	allow := env.RunGovard(t, projectDir, "gateway", "allow-key", strings.TrimSpace(string(clientPub)))
	allow.AssertSuccess(t)

	sandboxUp := env.RunGovard(t, projectDir, "sandbox", "up", "--profile", "basic", "--no-seed")
	if sandboxUp.ExitCode != 0 {
		t.Fatalf("sandbox up failed (%d)\nstdout: %s\nstderr: %s", sandboxUp.ExitCode, sandboxUp.Stdout, sandboxUp.Stderr)
	}
	t.Cleanup(func() {
		env.RunGovard(t, projectDir, "sandbox", "down", "--purge")
		env.RunGovard(t, projectDir, "svc", "down")
	})

	// The gateway routes by registered ProjectName, and the deploy/code-only
	// fixture pins project_name: deploy-code-only
	// (tests/integration/projects/deploy/code-only/.govard.yml:1), so the
	// SSH/SFTP username is deploy-code-only -- NOT the checkout dir name.
	const gwUser = "deploy-code-only"
	gwAddr := gwUser + "@127.0.0.1"

	sshArgs := []string{
		"-p", "2222", "-i", clientKeyPath,
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
	}

	// One-shot exec through the gateway, second hop, into the sandbox.
	execOut, err := exec.Command(sshBin, append(append([]string{}, sshArgs...), gwAddr, "echo", "hello-from-sandbox")...).CombinedOutput()
	if err != nil {
		t.Fatalf("ssh exec through the gateway failed: %v\n%s", err, execOut)
	}
	if !strings.Contains(string(execOut), "hello-from-sandbox") {
		t.Fatalf("expected the sandbox's echo, got:\n%s", execOut)
	}

	// sftp put/get through the same forced command, exercising the
	// subsystem-detection branch of gw-router.sh. Note sftp's port flag is
	// uppercase -P (lowercase -p preserves mtimes), so the port flag cannot
	// be shared with sshArgs.
	localFile := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(localFile, []byte("gateway sftp round trip\n"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	downloaded := filepath.Join(t.TempDir(), "downloaded.txt")
	batch := filepath.Join(t.TempDir(), "batch.txt")
	if err := os.WriteFile(batch, []byte("put "+localFile+" /tmp/upload.txt\nget /tmp/upload.txt "+downloaded+"\n"), 0o644); err != nil {
		t.Fatalf("write sftp batch file: %v", err)
	}
	sftpArgs := []string{
		"-P", "2222", "-i", clientKeyPath,
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-b", batch, gwAddr,
	}
	if out, err := exec.Command(sftpBin, sftpArgs...).CombinedOutput(); err != nil {
		t.Fatalf("sftp through the gateway failed: %v\n%s", err, out)
	}
	roundTripped, err := os.ReadFile(downloaded)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(roundTripped) != "gateway sftp round trip\n" {
		t.Fatalf("round-tripped content = %q", roundTripped)
	}

	// A never-registered username gets a standard SSH refusal -- no oracle
	// distinguishing "wrong key" from "unknown user" (spec section 6).
	unknownArgs := append(append([]string{}, sshArgs...), "never-registered-project@127.0.0.1", "echo", "should-not-run")
	if out, err := exec.Command(sshBin, unknownArgs...).CombinedOutput(); err == nil {
		t.Fatalf("expected ssh to a never-registered username to fail, got:\n%s", out)
	}

	// Phase 4a: stop the sandbox container DIRECTLY (no `sandbox down` yet,
	// so the route stays) -- the router must report the target as
	// not reachable and fail fast (rc=255), not hang.
	psOut, err := exec.Command("docker", "ps", "--format", "{{.Names}}", "--filter", "label=govard.sandbox.project="+gwUser).CombinedOutput()
	if err != nil {
		t.Fatalf("list sandbox container: %v\n%s", err, psOut)
	}
	sandboxContainers := strings.Fields(string(psOut))
	if len(sandboxContainers) == 0 {
		t.Fatalf("no running sandbox container labelled govard.sandbox.project=%s", gwUser)
	}
	if out, err := exec.Command("docker", append([]string{"stop"}, sandboxContainers...)...).CombinedOutput(); err != nil {
		t.Fatalf("docker stop %s: %v\n%s", strings.Join(sandboxContainers, " "), err, out)
	}
	unreachOut, err := exec.Command(sshBin, append(append([]string{}, sshArgs...), gwAddr, "echo", "should-not-run")...).CombinedOutput()
	if err == nil {
		t.Fatalf("expected ssh to a stopped sandbox to fail, got:\n%s", unreachOut)
	}
	if !strings.Contains(string(unreachOut), "not reachable") {
		t.Fatalf("expected the gateway's own not-reachable message, got:\n%s", unreachOut)
	}

	// Phase 4b: `sandbox down` prunes the route end-to-end -- the router
	// must now report an unknown target.
	down := env.RunGovard(t, projectDir, "sandbox", "down")
	down.AssertSuccess(t)
	downOut, err := exec.Command(sshBin, append(append([]string{}, sshArgs...), gwAddr, "echo", "should-not-run")...).CombinedOutput()
	if err == nil {
		t.Fatalf("expected ssh to a removed sandbox to fail, got:\n%s", downOut)
	}
	if !strings.Contains(string(downOut), "unknown target") {
		t.Fatalf("expected the gateway's unknown-target message, got:\n%s", downOut)
	}
}
