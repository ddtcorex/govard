package tests

import (
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
)

func TestBuildMagentoFreshCreateProjectCommandUsesMagentoRepositoryForMagento2(t *testing.T) {
	commandLine := bootstrap.BuildMagentoFreshCreateProjectCommand(bootstrap.Magento2Variant, bootstrap.Options{
		MetaPackage: "magento/project-community-edition",
	})
	if !strings.Contains(commandLine, "https://repo.magento.com") {
		t.Errorf("expected magento2 fresh-install command to reference https://repo.magento.com, got: %s", commandLine)
	}
	if strings.Contains(commandLine, "repo.mage-os.org") {
		t.Errorf("expected magento2 fresh-install command to NOT reference repo.mage-os.org, got: %s", commandLine)
	}
}

func TestBuildMagentoFreshCreateProjectCommandUsesMageOSRepositoryForMageOS(t *testing.T) {
	commandLine := bootstrap.BuildMagentoFreshCreateProjectCommand(bootstrap.MageOSVariant, bootstrap.Options{
		MetaPackage: "mage-os/project-community-edition",
	})
	if !strings.Contains(commandLine, "https://repo.mage-os.org") {
		t.Errorf("expected mageos fresh-install command to reference https://repo.mage-os.org, got: %s", commandLine)
	}
	if strings.Contains(commandLine, "repo.magento.com") {
		t.Errorf("expected mageos fresh-install command to NOT reference repo.magento.com, got: %s", commandLine)
	}
}

func TestBuildMagentoFreshCreateProjectCommandIncludesVersionWhenSet(t *testing.T) {
	commandLine := bootstrap.BuildMagentoFreshCreateProjectCommand(bootstrap.Magento2Variant, bootstrap.Options{
		MetaPackage: "magento/project-community-edition",
		Version:     "2.4.8",
	})
	if !strings.Contains(commandLine, "'magento/project-community-edition' /tmp/govard-create-project '2.4.8'") {
		t.Errorf("expected version to be appended to the create-project command, got: %s", commandLine)
	}
}

// TestBuildMagentoFreshCreateProjectCommandBuildsExpectedCommand covers the
// same ground as the pre-migration
// TestRunBootstrapFreshCreateProjectForTestBuildsExpectedCommand (deleted
// from tests/bootstrap_fresh_install_test.go along with
// cmd.RunBootstrapFreshCreateProjectForTest in Task 3) - the full composer
// invocation (including the -n --ignore-platform-reqs flags, which are
// easy to silently drop in a refactor since they're not exercised by any
// other assertion here) and the /tmp/govard-create-project cleanup step.
func TestBuildMagentoFreshCreateProjectCommandBuildsExpectedCommand(t *testing.T) {
	commandLine := bootstrap.BuildMagentoFreshCreateProjectCommand(bootstrap.Magento2Variant, bootstrap.Options{
		MetaPackage: "magento/project-community-edition",
		Version:     "2.4.8",
	})

	if !strings.Contains(commandLine, "composer create-project -n --ignore-platform-reqs --repository-url=https://repo.magento.com 'magento/project-community-edition' /tmp/govard-create-project '2.4.8'") {
		t.Fatalf("unexpected create-project command: %s", commandLine)
	}
	if !strings.Contains(commandLine, "rm -rf /tmp/govard-create-project") {
		t.Fatalf("expected cleanup commands in shell line: %s", commandLine)
	}
}

func TestBuildMagentoFreshCreateProjectCommandOmitsVersionWhenEmpty(t *testing.T) {
	commandLine := bootstrap.BuildMagentoFreshCreateProjectCommand(bootstrap.Magento2Variant, bootstrap.Options{
		MetaPackage: "magento/project-community-edition",
	})
	if !strings.Contains(commandLine, "'magento/project-community-edition' /tmp/govard-create-project && ") {
		t.Errorf("expected no version suffix on the create-project command, got: %s", commandLine)
	}
}

func TestBuildMagentoSetupInstallArgsUsesMagentoDBCredentialsForMagento2(t *testing.T) {
	args := bootstrap.BuildMagentoSetupInstallArgs(bootstrap.Magento2Variant, "", "admin@sample.test", "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--db-name=magento") || !strings.Contains(joined, "--db-user=magento") || !strings.Contains(joined, "--db-password=magento") {
		t.Fatalf("expected magento db credentials in setup args, got %q", joined)
	}
	if !strings.Contains(joined, "--admin-email=admin@sample.test") {
		t.Fatalf("expected admin email in setup args, got %q", joined)
	}
}

func TestBuildMagentoSetupInstallArgsUsesMageOSDBCredentials(t *testing.T) {
	args := bootstrap.BuildMagentoSetupInstallArgs(bootstrap.MageOSVariant, "", "admin@sample.test", "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--db-name=mageos") || !strings.Contains(joined, "--db-user=mageos") || !strings.Contains(joined, "--db-password=mageos") {
		t.Fatalf("expected mageos db credentials in setup args, got %q", joined)
	}
}

func TestBuildMagentoSetupInstallArgsUsesConfigTablePrefix(t *testing.T) {
	args := bootstrap.BuildMagentoSetupInstallArgs(bootstrap.Magento2Variant, "", "admin@sample.test", "demo_")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--db-prefix=demo_") {
		t.Fatalf("expected setup args to contain table prefix, got %q", joined)
	}
}

func TestBuildMagentoSetupInstallArgsUsesElasticsearch7ForLegacyMagento2Version(t *testing.T) {
	args := bootstrap.BuildMagentoSetupInstallArgs(bootstrap.Magento2Variant, "2.4.7", "admin@sample.test", "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--search-engine=elasticsearch7") {
		t.Fatalf("expected elasticsearch7 engine for legacy versions, args: %s", joined)
	}
	if strings.Contains(joined, "--search-engine=opensearch") {
		t.Fatalf("did not expect opensearch args for legacy version: %s", joined)
	}
}

func TestBuildMagentoSetupInstallArgsUsesOpenSearchForRecentMagento2Version(t *testing.T) {
	args := bootstrap.BuildMagentoSetupInstallArgs(bootstrap.Magento2Variant, "2.4.8", "admin@sample.test", "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--search-engine=opensearch") {
		t.Fatalf("expected opensearch args for 2.4.8+, got: %s", joined)
	}
}

func TestBuildMagentoSetupInstallArgsUsesOpenSearchForMageOSRegardlessOfVersion(t *testing.T) {
	args := bootstrap.BuildMagentoSetupInstallArgs(bootstrap.MageOSVariant, "1.3.0", "admin@sample.test", "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--search-engine=opensearch") {
		t.Fatalf("expected Mage-OS 1.x to use OpenSearch regardless of version (pre-existing asymmetry, not fixed by this migration), got %q", joined)
	}
	if strings.Contains(joined, "--search-engine=elasticsearch7") {
		t.Fatalf("expected Mage-OS to never hit the elasticsearch7 branch (magento2-only), got %q", joined)
	}
}
