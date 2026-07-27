package tests

import (
	"reflect"
	"testing"

	"govard/internal/cmd"

	"github.com/spf13/cobra"
)

func TestRunBootstrapHyvaInstallForTestRunsExpectedComposerCalls(t *testing.T) {
	calls := make([][]string, 0, 3)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := make([]string, len(args))
		copy(captured, args)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.RunBootstrapHyvaInstallForTest(&cobra.Command{}, "token-123")
	if err != nil {
		t.Fatalf("RunBootstrapHyvaInstallForTest() error = %v", err)
	}

	want := [][]string{
		{"tool", "composer", "config", "http-basic.hyva-themes.repo.packagist.com", "token", "token-123"},
		{"tool", "composer", "config", "repositories.hyva-themes", "composer", "https://hyva-themes.repo.packagist.com/app-hyva-test-dv1dgx/"},
		{"tool", "composer", "require", "-n", "hyva-themes/magento2-default-theme"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("composer calls = %#v, want %#v", calls, want)
	}
}

func TestRunBootstrapSampleDataForTestRunsAllSteps(t *testing.T) {
	calls := make([][]string, 0, 4)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := make([]string, len(args))
		copy(captured, args)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.RunBootstrapSampleDataForTest(&cobra.Command{})
	if err != nil {
		t.Fatalf("RunBootstrapSampleDataForTest() error = %v", err)
	}

	want := [][]string{
		{"tool", "magento", "sample:deploy"},
		{"tool", "magento", "setup:upgrade"},
		{"tool", "magento", "indexer:reindex"},
		{"tool", "magento", "cache:flush"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("sample data calls = %#v, want %#v", calls, want)
	}
}
