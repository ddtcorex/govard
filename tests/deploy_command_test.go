package tests

import (
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/runtime"
)

func cliCodeForTest(err error) int {
	return cli.Code(err)
}

func TestDeployFlagsAreTheDocumentedSet(t *testing.T) {
	command := cmd.DeployCommand()
	for _, name := range []string{
		"remote", "branch", "revision", "tag", "publish", "keep", "verify", "lock",
		"ignore-deployer-lock", "command-timeout", "force", "resume", "yes", "json", "verbose",
	} {
		if command.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s flag", name)
		}
	}
	// These were declared in the initial commit and never read anywhere: they
	// described a strategy that never existed.
	for _, dead := range []string{"strategy", "deployer", "deployer-config", "locales"} {
		if command.Flags().Lookup(dead) != nil {
			t.Errorf("--%s must be gone: it was never read", dead)
		}
	}
}

func TestDeployCapabilityContract(t *testing.T) {
	if got := runtime.Requires(cmd.DeployCommand()); len(got) != 2 || got[0] != runtime.CapSSH || got[1] != runtime.CapRsync {
		t.Fatalf("govard deploy requires %v, want [ssh rsync]", got)
	}
	if got := runtime.Requires(cmd.DeployPlanCommand()); len(got) != 1 || got[0] != runtime.CapNone {
		t.Fatalf("govard deploy plan requires %v, want [none]", got)
	}
	if got := runtime.Requires(cmd.DeployCheckCommand()); len(got) != 1 || got[0] != runtime.CapSSH {
		t.Fatalf("govard deploy check requires %v, want [ssh]", got)
	}
	if got := runtime.Requires(cmd.DeployUnlockCommand()); len(got) != 1 || got[0] != runtime.CapSSH {
		t.Fatalf("govard deploy unlock requires %v, want [ssh]", got)
	}
}

func TestDeployUnlockCarriesAForceFlag(t *testing.T) {
	if cmd.DeployUnlockCommand().Flags().Lookup("force") == nil {
		t.Fatal("unlock needs --force: a recent lock must not be released by accident")
	}
	parent := cmd.DeployUnlockCommand().Parent()
	if parent == nil || parent.Name() != "deploy" {
		t.Fatalf("unlock's parent = %v, want the deploy group", parent)
	}
}

func TestDeployCommandsAreRegisteredUnderTheDeployGroup(t *testing.T) {
	parent := cmd.DeployPlanCommand().Parent()
	if parent == nil || parent.Name() != "deploy" {
		t.Fatalf("plan's parent = %v, want the deploy group", parent)
	}
	if cmd.DeployCheckCommand().Parent() == nil {
		t.Fatal("check is not attached to the deploy group")
	}
}

func TestDeployRejectsCombinedSourceSelectors(t *testing.T) {
	_, err := cmd.DeployOverridesForTest([]string{"--branch", "main", "--revision", "abc"})
	if err == nil {
		t.Fatal("want a usage error when --branch and --revision are combined")
	}
	if code := cliCodeForTest(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (%v)", code, err)
	}
}

func TestDeployRejectsAnUnknownPublishStrategy(t *testing.T) {
	_, err := cmd.DeployOverridesForTest([]string{"--publish", "teleport"})
	if err == nil {
		t.Fatal("want a usage error for an unknown publish strategy")
	}
	if code := cliCodeForTest(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (%v)", code, err)
	}
}

func TestDeployAcceptsEachSourceSelectorAlone(t *testing.T) {
	for _, args := range [][]string{
		{"--branch", "main"},
		{"--revision", "abc123"},
		{"--tag", "v1.2.3"},
	} {
		if _, err := cmd.DeployOverridesForTest(args); err != nil {
			t.Errorf("DeployOverridesForTest(%v) = %v, want no error", args, err)
		}
	}
}
