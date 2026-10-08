package tests

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"

	"github.com/spf13/cobra"
)

func TestEnvRestartReappliesProjectDomainsAfterComposeUp(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: wordpress
domain: wordpress.test
framework: wordpress
`)

	var composeArgs [][]string
	var registeredDomains []string
	var registeredTarget string
	var mappedHosts []string

	restore := cmd.SetEnvDependenciesForTest(cmd.EnvDependenciesForTest{
		RunCompose: func(_ context.Context, opts engine.ComposeOptions) error {
			captured := append([]string{}, opts.Args...)
			composeArgs = append(composeArgs, captured)
			return nil
		},
		RegisterDomains: func(domains []string, target string) error {
			registeredDomains = append([]string{}, domains...)
			registeredTarget = target
			return nil
		},
		UnregisterDomain: func(string) error { return nil },
		// Both of these default to the real proxy implementation, which reaches
		// the Caddy admin API through `docker exec govard-proxy-caddy curl`. A
		// unit test must not shell into whichever containers happen to be
		// running on the developer's machine: it costs seconds per call, and it
		// makes the result depend on host state — with no proxy up, fetchCaddyConfig
		// errors and the route is never even looked for.
		UnregisterSearchDomain:   func(string) error { return nil },
		UnregisterRabbitMQDomain: func(string) error { return nil },
		AddHostsEntry: func(domain string) error {
			mappedHosts = append(mappedHosts, domain)
			return nil
		},
		RemoveHostsEntry:          func(string) error { return nil },
		IsDomainResolvableLocally: func(string) bool { return false },
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

	wantComposeArgs := [][]string{
		{"stop"},
		{"up", "-d"},
	}
	if !reflect.DeepEqual(composeArgs, wantComposeArgs) {
		t.Fatalf("compose args = %#v, want %#v", composeArgs, wantComposeArgs)
	}

	if !reflect.DeepEqual(registeredDomains, []string{"wordpress.test"}) {
		t.Fatalf("registered domains = %#v, want %#v", registeredDomains, []string{"wordpress.test"})
	}
	if registeredTarget != "wordpress-web-1" {
		t.Fatalf("registered target = %q, want %q", registeredTarget, "wordpress-web-1")
	}
	if !reflect.DeepEqual(mappedHosts, []string{"wordpress.test"}) {
		t.Fatalf("mapped hosts = %#v, want %#v", mappedHosts, []string{"wordpress.test"})
	}
}

func TestEnvRestartRegistersAndUnregistersSearchDomainWhenSearchEnabled(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: search-demo
domain: search-demo.test
framework: wordpress
stack:
  services:
    search: opensearch
`)

	var registeredSearchDomains []string
	var registeredSearchTarget string
	var unregisteredSearchDomains []string

	restore := cmd.SetEnvDependenciesForTest(cmd.EnvDependenciesForTest{
		RunCompose: func(_ context.Context, opts engine.ComposeOptions) error {
			return nil
		},
		RegisterDomains:  func([]string, string) error { return nil },
		UnregisterDomain: func(string) error { return nil },
		RegisterSearchDomains: func(domains []string, target string) error {
			registeredSearchDomains = append([]string{}, domains...)
			registeredSearchTarget = target
			return nil
		},
		UnregisterSearchDomain: func(domain string) error {
			unregisteredSearchDomains = append(unregisteredSearchDomains, domain)
			return nil
		},
		// The RabbitMQ unregister defaults to a real `docker exec` into the proxy
		// container; this test is about the search route, so stub it rather than
		// shell into whatever the developer happens to be running.
		UnregisterRabbitMQDomain:  func(string) error { return nil },
		AddHostsEntry:             func(string) error { return nil },
		RemoveHostsEntry:          func(string) error { return nil },
		IsDomainResolvableLocally: func(string) bool { return false },
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

	if !reflect.DeepEqual(registeredSearchDomains, []string{"search-demo.test"}) {
		t.Fatalf("registered search domains = %#v, want %#v", registeredSearchDomains, []string{"search-demo.test"})
	}
	if registeredSearchTarget != "search-demo-elasticsearch-1" {
		t.Fatalf("registered search target = %q, want %q", registeredSearchTarget, "search-demo-elasticsearch-1")
	}
	if !reflect.DeepEqual(unregisteredSearchDomains, []string{"search-demo.test"}) {
		t.Fatalf("unregistered search domains = %#v, want %#v", unregisteredSearchDomains, []string{"search-demo.test"})
	}
}

func TestEnvRestartRegistersAndUnregistersRabbitMQDomainWhenQueueEnabled(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: mq-demo
domain: mq-demo.test
framework: wordpress
stack:
  services:
    queue: rabbitmq
`)

	var registeredRabbitMQDomains []string
	var registeredRabbitMQTarget string
	var unregisteredRabbitMQDomains []string

	restore := cmd.SetEnvDependenciesForTest(cmd.EnvDependenciesForTest{
		RunCompose: func(_ context.Context, opts engine.ComposeOptions) error {
			return nil
		},
		RegisterDomains:  func([]string, string) error { return nil },
		UnregisterDomain: func(string) error { return nil },
		RegisterRabbitMQDomains: func(domains []string, target string) error {
			registeredRabbitMQDomains = append([]string{}, domains...)
			registeredRabbitMQTarget = target
			return nil
		},
		UnregisterRabbitMQDomain: func(domain string) error {
			unregisteredRabbitMQDomains = append(unregisteredRabbitMQDomains, domain)
			return nil
		},
		// The search unregister defaults to a real `docker exec` into the proxy
		// container; this test is about the RabbitMQ route, so stub it rather than
		// shell into whatever the developer happens to be running.
		UnregisterSearchDomain:    func(string) error { return nil },
		AddHostsEntry:             func(string) error { return nil },
		RemoveHostsEntry:          func(string) error { return nil },
		IsDomainResolvableLocally: func(string) bool { return false },
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

	if !reflect.DeepEqual(registeredRabbitMQDomains, []string{"mq-demo.test"}) {
		t.Fatalf("registered rabbitmq domains = %#v, want %#v", registeredRabbitMQDomains, []string{"mq-demo.test"})
	}
	if registeredRabbitMQTarget != "mq-demo-rabbitmq-1" {
		t.Fatalf("registered rabbitmq target = %q, want %q", registeredRabbitMQTarget, "mq-demo-rabbitmq-1")
	}
	if !reflect.DeepEqual(unregisteredRabbitMQDomains, []string{"mq-demo.test"}) {
		t.Fatalf("unregistered rabbitmq domains = %#v, want %#v", unregisteredRabbitMQDomains, []string{"mq-demo.test"})
	}
}

func TestInitRejectsDuplicateProjectIdentity(t *testing.T) {
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:        "/workspace/existing-wordpress",
		ProjectName: "wordpress",
		Domain:      "wordpress.test",
		Framework:   "wordpress",
	}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	parentDir := t.TempDir()
	projectDir := filepath.Join(parentDir, "wordpress")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	chdirForTest(t, projectDir)

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--framework", "wordpress", "--yes"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected duplicate project identity error")
	}
	if got := err.Error(); got == "" || !strings.Contains(strings.ToLower(got), "project_name wordpress is already used") {
		t.Fatalf("unexpected duplicate identity error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(projectDir, ".govard.yml")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no .govard.yml to be written, stat err = %v", statErr)
	}
}

// A service that left the compose file (Xdebug switched off) keeps running as
// an orphan; `down` must take it with the rest of the project.
func TestEnvDownRemovesOrphanContainers(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, `project_name: orphan-demo
domain: orphan-demo.test
framework: wordpress
`)

	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"plain", []string{"down"}, []string{"down", "--remove-orphans"}},
		{"with volumes", []string{"down", "-v"}, []string{"down", "-v", "--remove-orphans"}},
		{"already asked", []string{"down", "--remove-orphans"}, []string{"down", "--remove-orphans"}},
		{"stop is untouched", []string{"stop"}, []string{"stop"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			restore := cmd.SetEnvDependenciesForTest(cmd.EnvDependenciesForTest{
				RunCompose: func(_ context.Context, opts engine.ComposeOptions) error {
					got = append([]string{}, opts.Args...)
					return nil
				},
				UnregisterDomain:         func(string) error { return nil },
				UnregisterSearchDomain:   func(string) error { return nil },
				UnregisterRabbitMQDomain: func(string) error { return nil },
				RemoveHostsEntry:         func(string) error { return nil },
				RunHooks:                 func(engine.Config, string, io.Writer, io.Writer) error { return nil },
			})
			defer restore()
			command := &cobra.Command{}
			command.SetOut(io.Discard)
			command.SetErr(io.Discard)
			if err := cmd.ProxyEnvToComposeForTest(command, tc.args); err != nil {
				t.Fatalf("%v: %v", tc.args, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("compose args = %v, want %v", got, tc.want)
			}
		})
	}
}
