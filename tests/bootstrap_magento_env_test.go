package tests

import (
	"fmt"
	"strings"
	"testing"

	"govard/internal/conventions"
	"govard/internal/frameworks/magento2"
)

func TestBuildBootstrapMagentoEnvPHPForTestUsesDefaultDBHost(t *testing.T) {
	got := magento2.BuildBootstrapEnvironment(
		"test-crypt-key",
		magento2.BootstrapEnvironmentDatabase{Database: "sample_db", Username: "sample_user", Password: "sample_pass"},
		"",
	)

	expectedHost := fmt.Sprintf("'host' => %q", conventions.DefaultMagentoDBHost)
	if count := strings.Count(got, expectedHost); count != 2 {
		t.Fatalf("expected Magento env.php to use %s twice, got %d occurrences in:\n%s", expectedHost, count, got)
	}

	if strings.Contains(got, `"+conventions.DefaultMagentoDBHost+"`) {
		t.Fatalf("expected rendered env.php to not contain broken literal interpolation, got:\n%s", got)
	}

	for _, expected := range []string{
		"'key' => \"test-crypt-key\"",
		"'dbname' => \"sample_db\"",
		"'username' => \"sample_user\"",
		"'password' => \"sample_pass\"",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected rendered env.php to contain %q, got:\n%s", expected, got)
		}
	}
}

func TestBuildBootstrapMagentoEnvPHPForTestUsesTablePrefix(t *testing.T) {
	got := magento2.BuildBootstrapEnvironment(
		"test-crypt-key",
		magento2.BootstrapEnvironmentDatabase{Database: "sample_db", Username: "sample_user", Password: "sample_pass"},
		"demo_",
	)

	if !strings.Contains(got, "'table_prefix' => \"demo_\"") {
		t.Fatalf("expected rendered env.php to contain table prefix, got:\n%s", got)
	}
}
