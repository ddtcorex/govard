package tests

import (
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func cliCodeForTest(err error) int {
	return cli.Code(err)
}

func TestDeployFlagsAreTheDocumentedSet(t *testing.T) {
	command := cmd.DeployCommand()
	for _, name := range []string{
		"remote", "branch", "revision", "tag", "build", "artifact-dir", "publish", "keep", "verify", "db-backup", "lock",
		"ignore-deployer-lock", "command-timeout", "force", "resume", "from", "yes", "json", "verbose",
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
	// Reading a target is ssh-only work: an operator must be able to inspect a
	// server from a machine with no Docker and no rsync.
	if got := runtime.Requires(cmd.DeployReleasesCommand()); len(got) != 1 || got[0] != runtime.CapSSH {
		t.Fatalf("govard deploy releases requires %v, want [ssh]", got)
	}
	if got := runtime.Requires(cmd.DeployStatusCommand()); len(got) != 1 || got[0] != runtime.CapSSH {
		t.Fatalf("govard deploy status requires %v, want [ssh]", got)
	}
	if got := runtime.Requires(cmd.DeployRollbackCommand()); len(got) != 2 || got[0] != runtime.CapSSH || got[1] != runtime.CapRsync {
		t.Fatalf("govard deploy rollback requires %v, want [ssh rsync]", got)
	}
}

func TestDeployRollbackFlagsAreTheDocumentedSet(t *testing.T) {
	command := cmd.DeployRollbackCommand()
	for _, name := range []string{"remote", "to", "with-db", "yes"} {
		if command.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s flag on deploy rollback", name)
		}
	}
}

func TestDeployRollbackIsRegisteredUnderTheDeployGroup(t *testing.T) {
	for _, command := range []*cobra.Command{cmd.DeployRollbackCommand(), cmd.DeployReleasesCommand(), cmd.DeployStatusCommand()} {
		parent := command.Parent()
		if parent == nil || parent.Name() != "deploy" {
			t.Errorf("%s is not attached to the deploy group", command.Name())
		}
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

func TestDeployRejectsAnUnknownBuildMode(t *testing.T) {
	_, err := cmd.DeployOverridesForTest([]string{"--build", "cloud"})
	if err == nil {
		t.Fatal("want a usage error for an unknown build mode")
	}
	if code := cliCodeForTest(err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (%v)", code, err)
	}
}

func TestDeployAcceptsEachBuildModeAlone(t *testing.T) {
	// `--build=artifact` is accepted here on purpose: whether an artifact
	// directory exists is only knowable once the configuration is loaded.
	for _, args := range [][]string{
		{"--build", "auto"},
		{"--build", "server"},
		{"--build", "artifact", "--artifact-dir", "artifacts"},
	} {
		over, err := cmd.DeployOverridesForTest(args)
		if err != nil {
			t.Errorf("DeployOverridesForTest(%v) = %v, want no error", args, err)
			continue
		}
		if over.ArtifactDir != "" && over.Build != "artifact" {
			t.Errorf("DeployOverridesForTest(%v) = %+v, want the artifact dir preserved", args, over)
		}
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

func TestDeployBuildFlagsAreTheDocumentedSet(t *testing.T) {
	command := cmd.DeployBuildCommand()
	for _, name := range []string{"remote", "output", "branch", "revision", "tag", "force", "json", "command-timeout"} {
		if command.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s flag on deploy build", name)
		}
	}
	parent := command.Parent()
	if parent == nil || parent.Name() != "deploy" {
		t.Fatalf("deploy build is not attached to the deploy group: %v", parent)
	}
}

func TestDeployBuildNeedsNeitherSSHNorRsync(t *testing.T) {
	// The whole point of the split: the build job has the project's toolchain
	// and the deploy job has govard, ssh and rsync. A build must run on a host
	// with no container runtime and no target connectivity.
	got := runtime.Requires(cmd.DeployBuildCommand())
	if len(got) != 1 || got[0] != runtime.CapNone {
		t.Fatalf("govard deploy build requires %v, want [none]", got)
	}
}

func TestDeployFlagUsageStringsCarryNoBackquotes(t *testing.T) {
	// cobra reads a backquoted token in a usage string as the flag's value
	// placeholder, so `--artifact-dir ` + "`govard deploy build`" + ` would print the
	// command as its own type instead of "string".
	for _, command := range []*cobra.Command{
		cmd.DeployCommand(), cmd.DeployPlanCommand(), cmd.DeployCheckCommand(),
		cmd.DeployBuildCommand(), cmd.DeployRollbackCommand(), cmd.DeployReleasesCommand(),
		cmd.DeployStatusCommand(), cmd.DeployUnlockCommand(),
	} {
		command.Flags().VisitAll(func(flag *pflag.Flag) {
			if strings.Contains(flag.Usage, "`") {
				t.Errorf("%s --%s: backquotes in the usage string become the value placeholder", command.Name(), flag.Name)
			}
		})
	}
}
