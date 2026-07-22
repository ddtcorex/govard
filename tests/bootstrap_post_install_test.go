package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/conventions"
	"govard/internal/engine"

	"github.com/spf13/cobra"
)

func TestRunBootstrapMagentoSetupInstallForTestUsesDefaultAdminEmailWhenDomainMissing(t *testing.T) {
	var capturedArgs [][]string
	restore := cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		copied := append([]string(nil), args...)
		capturedArgs = append(capturedArgs, copied)
		return nil
	})
	defer restore()

	err := cmd.RunBootstrapMagentoSetupInstallForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   conventions.FrameworkMagento2,
	}, "remote", "")
	if err != nil {
		t.Fatalf("RunBootstrapMagentoSetupInstallForTest() error = %v", err)
	}

	var setupInstallArgs []string
	for _, args := range capturedArgs {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "setup:install") {
			setupInstallArgs = args
			break
		}
	}
	if len(setupInstallArgs) == 0 {
		t.Fatalf("expected magento setup:install subcommand, got %#v", capturedArgs)
	}

	joined := strings.Join(setupInstallArgs, " ")
	expected := "--admin-email=" + conventions.DefaultAdminEmail
	if !strings.Contains(joined, expected) {
		t.Fatalf("expected %q in setup args, got %q", expected, joined)
	}
}

func TestRunBootstrapMagentoSetupInstallForTestUsesConfigTablePrefix(t *testing.T) {
	var capturedArgs [][]string
	restore := cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		copied := append([]string(nil), args...)
		capturedArgs = append(capturedArgs, copied)
		return nil
	})
	defer restore()

	err := cmd.RunBootstrapMagentoSetupInstallForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   conventions.FrameworkMagento2,
		TablePrefix: "demo_",
	}, "remote", "")
	if err != nil {
		t.Fatalf("RunBootstrapMagentoSetupInstallForTest() error = %v", err)
	}

	joined := strings.Join(flattenArgs(capturedArgs), " ")
	if !strings.Contains(joined, "--db-prefix=demo_") {
		t.Fatalf("expected setup args to contain table prefix, got %q", joined)
	}
}

func TestRunBootstrapMagentoSetupInstallForTestUsesMagentoDBCredentialsForMagento2(t *testing.T) {
	var capturedArgs [][]string
	restore := cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		copied := append([]string(nil), args...)
		capturedArgs = append(capturedArgs, copied)
		return nil
	})
	defer restore()

	err := cmd.RunBootstrapMagentoSetupInstallForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   conventions.FrameworkMagento2,
	}, "remote", "")
	if err != nil {
		t.Fatalf("RunBootstrapMagentoSetupInstallForTest() error = %v", err)
	}

	joined := strings.Join(flattenArgs(capturedArgs), " ")
	if !strings.Contains(joined, "--db-name=magento") || !strings.Contains(joined, "--db-user=magento") || !strings.Contains(joined, "--db-password=magento") {
		t.Fatalf("expected magento db credentials in setup args, got %q", joined)
	}
}

func TestRunBootstrapMagentoSetupInstallForTestUsesMageOSDBCredentials(t *testing.T) {
	var capturedArgs [][]string
	restore := cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		copied := append([]string(nil), args...)
		capturedArgs = append(capturedArgs, copied)
		return nil
	})
	defer restore()

	err := cmd.RunBootstrapMagentoSetupInstallForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   conventions.FrameworkMageOS,
	}, "remote", "")
	if err != nil {
		t.Fatalf("RunBootstrapMagentoSetupInstallForTest() error = %v", err)
	}

	joined := strings.Join(flattenArgs(capturedArgs), " ")
	if !strings.Contains(joined, "--db-name=mageos") || !strings.Contains(joined, "--db-user=mageos") || !strings.Contains(joined, "--db-password=mageos") {
		t.Fatalf("expected mageos db credentials in setup args, got %q", joined)
	}
}

// TestRunBootstrapMagentoSetupInstallForTestUsesMageOSDBCredentialsForLegacyVersion covers the
// second (version-comparison) setupArgs block, which independently hardcoded magento DB
// credentials before this fix.
func TestRunBootstrapMagentoSetupInstallForTestUsesMageOSDBCredentialsForLegacyVersion(t *testing.T) {
	var capturedArgs [][]string
	restore := cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		copied := append([]string(nil), args...)
		capturedArgs = append(capturedArgs, copied)
		return nil
	})
	defer restore()

	err := cmd.RunBootstrapMagentoSetupInstallForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   conventions.FrameworkMageOS,
	}, "remote", "2.4.7")
	if err != nil {
		t.Fatalf("RunBootstrapMagentoSetupInstallForTest() error = %v", err)
	}

	joined := strings.Join(flattenArgs(capturedArgs), " ")
	if !strings.Contains(joined, "--db-name=mageos") || !strings.Contains(joined, "--db-user=mageos") || !strings.Contains(joined, "--db-password=mageos") {
		t.Fatalf("expected mageos db credentials in legacy-version setup args, got %q", joined)
	}
	if !strings.Contains(joined, "--search-engine=opensearch") {
		t.Fatalf("expected Mage-OS 1.x to use OpenSearch, got %q", joined)
	}
}

func flattenArgs(groups [][]string) []string {
	result := make([]string, 0)
	for _, group := range groups {
		result = append(result, group...)
	}
	return result
}
