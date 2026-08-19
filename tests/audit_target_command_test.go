package tests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks/types"
)

func TestResolveAuditPHPVersions(t *testing.T) {
	t.Run("project accepts supported active versions", func(t *testing.T) {
		for _, version := range []string{"7.4", "8.5"} {
			t.Run(version, func(t *testing.T) {
				project := auditCommandProjectWithPHPVersion(t, "magento2", version)
				backend := &commandLintBackend{}
				installAuditCommandDependencies(t, backend)
				if _, err := executeAuditCommand(t, project, []string{"audit", "run"}); err != nil {
					t.Fatal(err)
				}
				if got, want := backend.requests[0].SelectedPHPVersions, []string{version}; !reflect.DeepEqual(got, want) {
					t.Fatalf("selected PHP versions = %#v, want %#v", got, want)
				}
			})
		}
	})

	t.Run("project rejects unsupported active versions before provider construction", func(t *testing.T) {
		for _, version := range []string{"8.0", "7.3", "8.6"} {
			t.Run(version, func(t *testing.T) {
				project := auditCommandProjectWithPHPVersion(t, "magento2", version)
				backend := &commandLintBackend{}
				installAuditCommandDependencies(t, backend)
				_, err := executeAuditCommand(t, project, []string{"audit", "run"})
				if err == nil || !strings.Contains(err.Error(), "unsupported_php") {
					t.Fatalf("error = %v, want unsupported_php", err)
				}
				if backend.runs != 0 {
					t.Fatalf("backend runs = %d, want 0", backend.runs)
				}
			})
		}
	})

	t.Run("project override must equal active version", func(t *testing.T) {
		project := auditCommandProjectWithPHPVersion(t, "magento2", "8.4")
		backend := &commandLintBackend{}
		installAuditCommandDependencies(t, backend)
		_, err := executeAuditCommand(t, project, []string{"audit", "run", "--php", "8.5"})
		if err == nil || !strings.Contains(err.Error(), "active project PHP") {
			t.Fatalf("error = %v, want active project PHP mismatch", err)
		}
		if backend.runs != 0 {
			t.Fatalf("backend runs = %d, want 0", backend.runs)
		}
	})

	t.Run("standalone defaults and permits a narrowed matrix", func(t *testing.T) {
		module := standaloneAuditModule(t, "vendor/audit-module")
		for _, test := range []struct {
			name      string
			arguments []string
			want      []string
			complete  bool
		}{
			{name: "default", arguments: []string{"audit", "run", "--mode", "standalone"}, want: []string{"8.1", "8.2", "8.3", "8.4", "8.5"}, complete: true},
			{name: "narrowed", arguments: []string{"audit", "run", "--mode", "standalone", "--php", "8.1,8.5"}, want: []string{"8.1", "8.5"}, complete: false},
		} {
			t.Run(test.name, func(t *testing.T) {
				backend := &commandLintBackend{}
				installAuditCommandDependencies(t, backend)
				if _, err := executeAuditCommand(t, module, test.arguments); err != nil {
					t.Fatal(err)
				}
				request := backend.requests[0]
				if got := request.SelectedPHPVersions; !reflect.DeepEqual(got, test.want) {
					t.Fatalf("selected PHP versions = %#v, want %#v", got, test.want)
				}
				if request.MatrixComplete != test.complete {
					t.Fatalf("matrix complete = %t, want %t", request.MatrixComplete, test.complete)
				}
			})
		}
	})

	t.Run("standalone rejects unavailable versions", func(t *testing.T) {
		for _, version := range []string{"7.4", "8.0"} {
			t.Run(version, func(t *testing.T) {
				module := standaloneAuditModule(t, "vendor/audit-module")
				backend := &commandLintBackend{}
				installAuditCommandDependencies(t, backend)
				_, err := executeAuditCommand(t, module, []string{"audit", "run", "--mode", "standalone", "--php", version})
				if err == nil || !strings.Contains(err.Error(), "unsupported_php") {
					t.Fatalf("error = %v, want unsupported_php", err)
				}
			})
		}
	})
}

func TestResolveAuditPHPVersionsTrustsStoppedRuntimeAndRejectsRuntimeDrift(t *testing.T) {
	for _, test := range []struct {
		name     string
		version  string
		running  bool
		wantErr  string
		wantRuns int
	}{
		{name: "stopped environment trusts normalized config", running: false, wantRuns: 1},
		{name: "running environment matches config", version: "8.4", running: true, wantRuns: 1},
		{name: "running environment mismatch is infrastructure error", version: "8.5", running: true, wantErr: `configured PHP "8.4" differs from running PHP "8.5"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := auditCommandProjectWithPHPVersion(t, "magento2", "8.4")
			backend := &commandLintBackend{}
			installAuditCommandDependenciesWithRuntimeProbe(t, backend, func(context.Context, types.AuditTarget, engine.Config) (string, bool, error) {
				return test.version, test.running, nil
			})
			_, err := executeAuditCommand(t, project, []string{"audit", "run"})
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) || !strings.Contains(err.Error(), "infrastructure") {
					t.Fatalf("error = %v, want infrastructure mismatch %q", err, test.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if backend.runs != test.wantRuns {
				t.Fatalf("backend runs = %d, want %d", backend.runs, test.wantRuns)
			}
		})
	}
}

func TestAuditCommandResolvesStandaloneTarget(t *testing.T) {
	module := standaloneAuditModule(t, "vendor/audit-module")
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	output, err := executeAuditCommand(t, module, []string{"audit", "run", "--mode", "standalone", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var result audit.RunResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	if result.ProjectID == "" || result.Source.Digest == "" {
		t.Fatalf("standalone result = %#v, want stable project and source identities", result)
	}
	request := backend.requests[0]
	if request.Target.Mode != types.AuditTargetStandalone || request.Target.TargetPath != module {
		t.Fatalf("target = %#v, want standalone module %q", request.Target, module)
	}
	if request.ProjectRoot != module {
		t.Fatalf("project root = %q, want canonical target %q", request.ProjectRoot, module)
	}

	backend = &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	secondOutput, err := executeAuditCommand(t, module, []string{"audit", "run", "--mode", "standalone", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var second audit.RunResult
	if err := json.Unmarshal(secondOutput, &second); err != nil {
		t.Fatal(err)
	}
	if second.ProjectID != result.ProjectID || second.Source != result.Source {
		t.Fatalf("standalone identity changed: first=%#v second=%#v", result, second)
	}
}

func TestAuditCommandRerunUsesPersistedTargetPHPSettings(t *testing.T) {
	project := auditCommandProjectWithPHPVersion(t, "magento2", "8.4")
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	output, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var first audit.RunResult
	if err := json.Unmarshal(output, &first); err != nil {
		t.Fatal(err)
	}
	config := "project_name: audit-shop\ndomain: audit-shop.test\nframework: magento2\nstack:\n  php_version: 8.0\n"
	if err := os.WriteFile(filepath.Join(project, ".govard.yml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAuditCommand(t, project, []string{"audit", "rerun", "--session", first.SessionID}); err != nil {
		t.Fatal(err)
	}
	last := backend.requests[len(backend.requests)-1]
	if got, want := last.SelectedPHPVersions, []string{"8.4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rerun selected PHP versions = %#v, want %#v", got, want)
	}
	if !last.MatrixComplete || last.Target.Mode != types.AuditTargetProject {
		t.Fatalf("rerun target settings = %#v, want persisted project matrix", last)
	}
}

func TestAuditCommandPreservesAuditTargetResolverErrors(t *testing.T) {
	project := auditCommandProjectWithPHPVersion(t, "magento2", "8.4")
	if err := os.WriteFile(filepath.Join(project, "composer.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	_, err := executeAuditCommand(t, project, []string{"audit", "run"})
	if err == nil || !strings.Contains(err.Error(), "parse Composer manifest") {
		t.Fatalf("error = %v, want malformed Composer resolver error", err)
	}
	if backend.runs != 0 {
		t.Fatalf("backend runs = %d, want 0", backend.runs)
	}
}

func TestAuditCommandUsesFrameworkOwnedTargetIdentity(t *testing.T) {
	for _, test := range []struct {
		framework string
		want      string
	}{
		{framework: "magento2", want: "magento2"},
		{framework: "mageos", want: "mageos"},
	} {
		t.Run(test.framework, func(t *testing.T) {
			project := auditCommandProjectWithPHPVersion(t, test.framework, "8.4")
			backend := &commandLintBackend{}
			installAuditCommandDependencies(t, backend)
			if _, err := executeAuditCommand(t, project, []string{"audit", "run"}); err != nil {
				t.Fatal(err)
			}
			if got := backend.requests[0].Target.Framework; got != test.want {
				t.Fatalf("target framework = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAuditCommandRejectsConfigFrameworkIncompatibleWithTarget(t *testing.T) {
	project := auditCommandProjectWithPHPVersion(t, "magento2", "8.4")
	config := "project_name: audit-shop\ndomain: audit-shop.test\nframework: mageos\nstack:\n  php_version: 8.4\n"
	if err := os.WriteFile(filepath.Join(project, ".govard.yml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	_, err := executeAuditCommand(t, project, []string{"audit", "run"})
	if err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("error = %v, want incompatible target/config frameworks", err)
	}
	if backend.runs != 0 {
		t.Fatalf("backend runs = %d, want 0", backend.runs)
	}
}

func TestAuditCommandNormalizesStandalonePHPVersions(t *testing.T) {
	module := standaloneAuditModule(t, "vendor/audit-module")
	backend := &commandLintBackend{}
	installAuditCommandDependencies(t, backend)
	if _, err := executeAuditCommand(t, module, []string{"audit", "run", "--mode", "standalone", "--php", " 8.1 , 8.5 "}); err != nil {
		t.Fatal(err)
	}
	if got, want := backend.requests[0].SelectedPHPVersions, []string{"8.1", "8.5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("standalone PHP versions = %#v, want %#v", got, want)
	}
}

func TestAuditReadOnlyCommandsDoNotProbeRuntimePHP(t *testing.T) {
	project := auditCommandProjectWithPHPVersion(t, "magento2", "8.4")
	backend := &commandLintBackend{}
	store := audit.NewStore(filepath.Join(project, "audit-store"))
	installAuditDependenciesWithStoreAndProbe(t, backend, store, func(context.Context, types.AuditTarget, engine.Config) (string, bool, error) {
		return "", false, nil
	})
	output, err := executeAuditCommand(t, project, []string{"audit", "run", "--format", "json"})
	if err != nil {
		t.Fatal(err)
	}
	var run audit.RunResult
	if err := json.Unmarshal(output, &run); err != nil {
		t.Fatal(err)
	}

	probes := 0
	installAuditDependenciesWithStoreAndProbe(t, backend, store, func(context.Context, types.AuditTarget, engine.Config) (string, bool, error) {
		probes++
		return "", false, errors.New("runtime probe must not run")
	})
	for _, arguments := range [][]string{
		{"audit", "status", "--session", run.SessionID},
		{"audit", "result", "--session", run.SessionID, "--run", run.RunID},
		{"audit", "cleanup", "--older-than", "1ns"},
	} {
		if _, err := executeAuditCommand(t, project, arguments); err != nil {
			t.Fatalf("%v: %v", arguments, err)
		}
	}
	if probes != 0 {
		t.Fatalf("runtime PHP probe calls = %d, want 0", probes)
	}
}

func standaloneAuditModule(t *testing.T, packageName string) string {
	t.Helper()
	module := t.TempDir()
	manifest := "{\"name\":\"" + packageName + "\",\"type\":\"magento2-module\"}"
	if err := os.WriteFile(filepath.Join(module, "composer.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return module
}

func installAuditCommandDependenciesWithRuntimeProbe(t *testing.T, backend audit.LintBackend, probe func(context.Context, types.AuditTarget, engine.Config) (string, bool, error)) {
	t.Helper()
	installAuditDependenciesWithStoreAndProbe(t, backend, audit.NewStore(audit.DefaultStoreRoot(engine.GovardHomeDir())), probe)
}

func installAuditDependenciesWithStoreAndProbe(t *testing.T, backend audit.LintBackend, store *audit.Store, probe func(context.Context, types.AuditTarget, engine.Config) (string, bool, error)) {
	t.Helper()
	restore := cmd.SetAuditDependenciesForTest(cmd.AuditDependenciesForTest{
		RunnerFactory: func(_ cmd.AuditRunnerRequest) (*audit.Runner, error) {
			return audit.NewRunner(audit.RunnerOptions{
				Store:       store,
				LintBackend: backend,
				Resources:   audit.Resources{CPU: 4, MemoryMB: 4096},
			}), nil
		},
		RuntimePHPProbe: probe,
	})
	t.Cleanup(restore)
}
