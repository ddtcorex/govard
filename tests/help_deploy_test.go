package tests

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func renderDeployHelp(t *testing.T, args []string) string {
	t.Helper()
	root := cmd.RootCommandForTest()
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return output.String()
}

func TestDeployReadCommandsAcceptRemoteFlag(t *testing.T) {
	for _, sub := range []string{"status", "releases", "unlock"} {
		command, _, err := cmd.RootCommandForTest().Find([]string{"deploy", sub})
		if err != nil {
			t.Fatalf("find deploy %s: %v", sub, err)
		}
		if command.Flags().Lookup("remote") == nil {
			t.Fatalf("deploy %s has no --remote flag", sub)
		}
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"deploy", "status", "--remote", "staging"})
	if err := root.Execute(); err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("deploy status still rejects --remote: %v", err)
	}
}

func TestDeployLongDocumentsResumeFallback(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "deploy")
	if !strings.Contains(long, "unfinished release") || !strings.Contains(long, "--resume") {
		t.Fatalf("deploy Long omits the --resume fallback:\n%s", long)
	}
}

func TestDeployKeepUsageDocumentsDefault(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "deploy")
	if !strings.Contains(usage("keep"), "5") {
		t.Fatalf("--keep usage omits the default:\n%s", usage("keep"))
	}
}

func TestDeployUnlockLongDefinesRecent(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "deploy", "unlock")
	if !strings.Contains(long, "2h") {
		t.Fatalf("unlock Long leaves recent undefined:\n%s", long)
	}
}

func TestDeployPlanDocumentsOverrideLimits(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "deploy", "plan")
	if !strings.Contains(long, "overrides") {
		t.Fatalf("plan Long omits deploy-flag override limits:\n%s", long)
	}
}

func TestDeployCheckDocumentsJsonPath(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "deploy", "check")
	if !strings.Contains(long, "plan --json") {
		t.Fatalf("check Long does not point at plan --json:\n%s", long)
	}
}

func TestTunnelStopLongStatesScope(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "tunnel", "stop")
	if !strings.Contains(long, "every cloudflared") {
		t.Fatalf("tunnel stop Long understates its scope:\n%s", long)
	}
}

func TestTunnelStartTlsVerifyNote(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "tunnel", "start")
	if !strings.Contains(usage("no-tls-verify"), "--no-tls-verify=false") {
		t.Fatalf("--no-tls-verify usage has no security note:\n%s", usage("no-tls-verify"))
	}
}

func TestTunnelStartUrlExclusivity(t *testing.T) {
	_, long, use, _ := lookupHelpCommand(t, "tunnel", "start")
	if documented := long + " " + use; !strings.Contains(documented, "not both") {
		t.Fatalf("tunnel start does not document url exclusivity:\n%s", documented)
	}
}

func TestTunnelLongMatchesShort(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "tunnel")
	if !strings.Contains(long, "Manage local project tunnels") {
		t.Fatalf("tunnel Long diverges from its Short:\n%s", long)
	}
}

func TestRemoteTestLongMentionsRsync(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "remote", "test")
	if !strings.Contains(long, "rsync") {
		t.Fatalf("remote test Long omits the rsync probe:\n%s", long)
	}
}

func TestRemoteExecDocumentsCd(t *testing.T) {
	_, long, use, example := lookupHelpCommand(t, "remote", "exec")
	if documented := long + " " + use + " " + example; !strings.Contains(documented, "configured path") {
		t.Fatalf("remote exec does not document its cd behavior:\n%s", documented)
	}
}

func TestRemoteAddProtectedDocumentsAuto(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "remote", "add")
	if !strings.Contains(usage("protected"), "automatically") {
		t.Fatalf("--protected usage omits auto-protection:\n%s", usage("protected"))
	}
}

func TestRemoteAddCapabilitiesListsAll(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "remote", "add")
	if !strings.Contains(usage("capabilities"), "all") {
		t.Fatalf("--capabilities usage omits all:\n%s", usage("capabilities"))
	}
}

func TestCopyIdIdentityDocumentsDiscovery(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "remote", "copy-id")
	if !strings.Contains(usage("identity"), "id_ed25519") {
		t.Fatalf("--identity usage omits default discovery:\n%s", usage("identity"))
	}
}

func TestSandboxLongNamesDocker(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "sandbox")
	if !strings.Contains(long, "Docker") {
		t.Fatalf("sandbox help never names its Docker requirement:\n%s", long)
	}
}

func TestSandboxUpDocumentsReuse(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "sandbox", "up")
	if !strings.Contains(long, "--recreate") {
		t.Fatalf("sandbox up Long omits reuse semantics:\n%s", long)
	}
}

func TestSandboxPhpUsageNotesBasic(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "sandbox", "up")
	if !strings.Contains(usage("php"), "basic") {
		t.Fatalf("--php usage omits the basic-profile caveat:\n%s", usage("php"))
	}
}

func TestSandboxLayoutDocumentsValues(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "sandbox", "reset")
	if !strings.Contains(usage("layout"), "ignored") {
		t.Fatalf("--layout usage hides silent values:\n%s", usage("layout"))
	}
}

func TestSandboxSshStatesInteractive(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "sandbox", "ssh")
	if !strings.Contains(long, "nteractive") {
		t.Fatalf("sandbox ssh Long omits interactivity:\n%s", long)
	}
}

func TestGatewayKeyCommandsDocumentSemantics(t *testing.T) {
	_, allowLong, allowUse, allowExample := lookupHelpCommand(t, "gateway", "allow-key")
	if documented := allowLong + " " + allowUse + " " + allowExample; !strings.Contains(documented, "quot") {
		t.Fatalf("allow-key does not document quoting:\n%s", documented)
	}
	_, revokeLong, revokeUse, _ := lookupHelpCommand(t, "gateway", "revoke-key")
	if documented := revokeLong + " " + revokeUse; !strings.Contains(documented, "xact") {
		t.Fatalf("revoke-key does not document exact matching:\n%s", documented)
	}
}

func TestGatewayExampleMatchesDocs(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "gateway")
	if !strings.Contains(long, "127.0.0.1") {
		t.Fatalf("gateway Long omits the documented address form:\n%s", long)
	}
}

func TestTunnelHelpRenders(t *testing.T) {
	help := renderDeployHelp(t, []string{"tunnel", "stop", "--help"})
	if !strings.Contains(help, "cloudflared") {
		t.Fatalf("tunnel stop help missing:\n%s", help)
	}
}
