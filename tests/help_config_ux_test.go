package tests

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func renderConfigUxHelp(t *testing.T, dir string, args []string) (string, error) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	root := cmd.RootCommandForTest()
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err = root.Execute()
	return output.String(), err
}

func TestVscodeToolHelpOutsideProject(t *testing.T) {
	help, err := renderConfigUxHelp(t, t.TempDir(), []string{"vscode", "php", "--help"})
	if err != nil {
		t.Fatalf("vscode php --help outside a project must show help, got: %v", err)
	}
	if !strings.Contains(help, "Usage:") {
		t.Fatalf("vscode php --help shows no Usage:\n%s", help)
	}
}

func TestConfigProfileShortAndArgs(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"config", "profile"})
	if err != nil {
		t.Fatalf("find config profile: %v", err)
	}
	if strings.Contains(command.Short, "(show") || strings.Contains(command.Short, "show,") {
		t.Fatalf("profile Short advertises a show subcommand: %q", command.Short)
	}
	_, err = renderConfigUxHelp(t, t.TempDir(), []string{"config", "profile", "show"})
	if err == nil {
		t.Fatal("config profile show must fail, the subcommand does not exist")
	}
}

func TestConfigGetSetDocumentKeys(t *testing.T) {
	for _, sub := range []string{"get", "set"} {
		_, long, _, example := lookupHelpCommand(t, "config", sub)
		if documented := long + " " + example; !strings.Contains(documented, "stack.php_version") {
			t.Fatalf("config %s documents no accepted keys:\n%s", sub, documented)
		}
	}
}

func TestConfigAutoDocumentsScope(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "config", "auto")
	if !strings.Contains(long, "framework") || !strings.Contains(long, "container") {
		t.Fatalf("config auto Long omits its framework/container scope:\n%s", long)
	}
}

func TestUpgradeDocumentsVersionRequired(t *testing.T) {
	usage, _, _, example := lookupHelpCommand(t, "upgrade")
	if !strings.Contains(usage("version"), "required") {
		t.Fatalf("--version usage does not say required:\n%s", usage("version"))
	}
	if !strings.Contains(example, "upgrade --version") {
		t.Fatalf("upgrade has no --version example:\n%s", example)
	}
}

func TestUpgradeNoDbUpgradeMagentoOnly(t *testing.T) {
	usage, _, _, _ := lookupHelpCommand(t, "upgrade")
	if !strings.Contains(usage("no-db-upgrade"), "Magento") {
		t.Fatalf("--no-db-upgrade usage omits Magento-only:\n%s", usage("no-db-upgrade"))
	}
}

func TestOpenDocumentsTargetLocality(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "open")
	if !strings.Contains(long, "local only") {
		t.Fatalf("open Long annotates no target locality:\n%s", long)
	}
}

func TestFrontendStartDocumentsPrereqs(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "frontend", "start")
	if !strings.Contains(long, "env up") {
		t.Fatalf("frontend start Long omits prerequisites:\n%s", long)
	}
}

func TestFrontendLogsDocumentsDefault(t *testing.T) {
	_, long, use, _ := lookupHelpCommand(t, "frontend", "logs")
	if documented := long + " " + use; !strings.Contains(documented, "sync") {
		t.Fatalf("frontend logs documents no default service:\n%s", documented)
	}
}

func TestFrontendStopClarifiesScope(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "frontend", "stop")
	if !strings.Contains(long, "volumes") {
		t.Fatalf("frontend stop Long hides volume/route behavior:\n%s", long)
	}
}

func TestTrustDocumentsPrivilege(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "trust")
	if !strings.Contains(long, "sudo") {
		t.Fatalf("trust Long omits the privilege requirement:\n%s", long)
	}
}

func TestDomainAddRemoveNoteEnvUp(t *testing.T) {
	for _, sub := range []string{"add", "remove"} {
		_, long, _, _ := lookupHelpCommand(t, "domain", sub)
		if !strings.Contains(long, "env up") {
			t.Fatalf("domain %s Long omits the env up step:\n%s", sub, long)
		}
	}
}

func TestBlueprintGroupScope(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"blueprint"})
	if err != nil {
		t.Fatalf("find blueprint: %v", err)
	}
	if strings.Contains(command.Short, "components") {
		t.Fatalf("blueprint Short overclaims components: %q", command.Short)
	}
}

func TestCustomDocumentsLocations(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "custom")
	if !strings.Contains(long, ".govard/commands") {
		t.Fatalf("custom Long names no command directory:\n%s", long)
	}
}

func TestVscodeSetupNames(t *testing.T) {
	_, long, _, _ := lookupHelpCommand(t, "vscode", "setup")
	if !strings.Contains(long, "Listen for Xdebug (Govard)") {
		t.Fatalf("vscode setup Long misquotes the launch config:\n%s", long)
	}
}

func TestVscodeParentExamplesUseRealWrappers(t *testing.T) {
	_, _, _, example := lookupHelpCommand(t, "vscode")
	if !strings.Contains(example, ".govard/bin/govard-php") {
		t.Fatalf("vscode examples use placeholder wrapper names:\n%s", example)
	}
}

func TestProjectShortCoversDelete(t *testing.T) {
	root := cmd.RootCommandForTest()
	command, _, err := root.Find([]string{"project"})
	if err != nil {
		t.Fatalf("find project: %v", err)
	}
	if !strings.Contains(command.Short, "delete") && !strings.Contains(command.Short, "Manage") {
		t.Fatalf("project Short understates the group: %q", command.Short)
	}
}
