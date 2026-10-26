package tests

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"

	"github.com/spf13/cobra"
)

func TestRootOwnershipRiskWarnsWhenRootOnUserProject(t *testing.T) {
	risk, err := engine.RootOwnershipRisk("/p", 0, func(string) (int, error) { return 1000, nil })
	if err != nil || !risk {
		t.Fatalf("want risk, got %v %v", risk, err)
	}
}

func TestRootOwnershipRiskSilentForRootOwnedProject(t *testing.T) {
	risk, _ := engine.RootOwnershipRisk("/p", 0, func(string) (int, error) { return 0, nil })
	if risk {
		t.Fatal("root on a root-owned project (CI containers) must not warn")
	}
}

func TestRootOwnershipRiskSilentForNonRoot(t *testing.T) {
	risk, _ := engine.RootOwnershipRisk("/p", 1000, func(string) (int, error) { return 1000, nil })
	if risk {
		t.Fatal("non-root must not warn")
	}
}

func TestWarnRootOwnershipRiskNamesDoctorFix(t *testing.T) {
	var out bytes.Buffer
	cmd.WarnRootOwnershipRiskForTest(&out, "/p", 0, func(string) (int, error) { return 1000, nil })
	if !strings.Contains(out.String(), "govard doctor --fix") {
		t.Fatalf("warning must name govard doctor --fix, got %q", out.String())
	}
	out.Reset()
	cmd.WarnRootOwnershipRiskForTest(&out, "/p", 1000, func(string) (int, error) { return 1000, nil })
	if out.Len() != 0 {
		t.Fatalf("non-root must print nothing, got %q", out.String())
	}
}

// Root cause of a root-owned .govard/: env start and restart ran compose up on
// the previously rendered file, so a bind source removed since the last render
// was recreated by the Docker daemon as root. The source must exist, created
// by the invoking user, before compose up runs.
func TestEnvRestartPreparesBindSourcesBeforeComposeUp(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: sample-project
domain: sample-project.test
framework: magento2
`)
	custom := []string{
		filepath.Join(tempDir, ".govard", "nginx", "custom"),
		filepath.Join(tempDir, ".govard", "apache", "custom"),
	}
	preparedAtUp := false
	restore := cmd.SetEnvDependenciesForTest(cmd.EnvDependenciesForTest{
		RunCompose: func(_ context.Context, opts engine.ComposeOptions) error {
			if len(opts.Args) > 0 && opts.Args[0] == "up" {
				for _, dir := range custom {
					if info, err := os.Stat(dir); err == nil && info.IsDir() {
						preparedAtUp = true
					}
				}
			}
			return nil
		},
		RegisterDomains:           func([]string, string) error { return nil },
		UnregisterDomain:          func(string) error { return nil },
		UnregisterSearchDomain:    func(string) error { return nil },
		UnregisterRabbitMQDomain:  func(string) error { return nil },
		AddHostsEntry:             func(string) error { return nil },
		RemoveHostsEntry:          func(string) error { return nil },
		IsDomainResolvableLocally: func(string) bool { return true },
		RunHooks:                  func(engine.Config, string, io.Writer, io.Writer) error { return nil },
		RefreshPMAActiveProjects:  func() error { return nil },
	})
	defer restore()

	command := &cobra.Command{}
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	if err := cmd.ProxyEnvToComposeForTest(command, []string{"restart"}); err != nil {
		t.Fatalf("execute env restart: %v", err)
	}
	if !preparedAtUp {
		t.Fatal("the web server custom config directory must exist before compose up")
	}
}
