package tests

import (
	"context"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"

	"github.com/spf13/cobra"
)

func TestConfirmProtectedRemoteReadsTheSyntheticProtectedField(t *testing.T) {
	restore := cmd.StubSandboxResolverForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{Protected: engine.BoolPtr(false), Sandbox: true}, deploy.SandboxLivenessRunning, nil
	})
	defer restore()

	err := cmd.ConfirmProtectedRemoteForTest(&cobra.Command{}, engine.Config{ProjectName: "sample-project"}, "sandbox", deploy.Options{Yes: false}, "Deploy")
	if err != nil {
		t.Fatalf("confirmProtectedRemote must not block a sandbox: %v", err)
	}
}
