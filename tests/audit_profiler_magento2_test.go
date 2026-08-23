package tests

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func TestAuditProfilerMagento2LifecycleUsesActiveGovardWebServer(t *testing.T) {
	for _, testCase := range []struct {
		name              string
		webServer         string
		configRelativeDir string
		configNeedle      string
		reload            []string
	}{
		{
			name:              "nginx",
			webServer:         "nginx",
			configRelativeDir: filepath.Join(".govard", "nginx", "custom", "audit-profiler"),
			configNeedle:      "fastcgi_param MAGE_PROFILER csvfile;",
			reload:            []string{"exec", "audit-shop-web-1", "nginx", "-s", "reload"},
		},
		{
			name:              "apache",
			webServer:         "apache",
			configRelativeDir: filepath.Join(".govard", "apache", "custom"),
			configNeedle:      "ProxyFCGISetEnvIf \"true\" MAGE_PROFILER \"csvfile\"",
			reload:            []string{"exec", "audit-shop-web-1", "httpd", "-k", "graceful"},
		},
		{
			name:              "hybrid configures apache",
			webServer:         "hybrid",
			configRelativeDir: filepath.Join(".govard", "apache", "custom"),
			configNeedle:      "ProxyFCGISetEnvIf \"true\" MAGE_PROFILER \"csvfile\"",
			reload:            []string{"exec", "audit-shop-apache-1", "httpd", "-k", "graceful"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			envPath := filepath.Join(projectRoot, "app", "etc", "env.php")
			if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(envPath, []byte("<?php return ['sentinel' => true];\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			definition, ok := frameworks.Get("magento2")
			if !ok {
				t.Fatal("Magento 2 definition is not registered")
			}
			config := &engine.Config{ProjectName: "audit-shop", Framework: "magento2"}
			config.Stack.Services.WebServer = testCase.webServer
			target := types.AuditTarget{Framework: "magento2", ProjectRoot: projectRoot, TargetPath: projectRoot, Mode: types.AuditTargetProject}

			var dockerCalls [][]string
			var capturedURL string
			runtime, err := cmd.NewAuditProfilerRuntimeForTest(cmd.AuditRunnerRequest{
				ProjectRoot:             projectRoot,
				Definition:              definition,
				Target:                  target,
				Config:                  config,
				ProfilerRuntimeRequired: true,
			}, cmd.AuditProfilerRuntimeDependenciesForTest{
				RunDocker: func(_ context.Context, arguments ...string) ([]byte, error) {
					copied := append([]string(nil), arguments...)
					dockerCalls = append(dockerCalls, copied)
					if reflect.DeepEqual(arguments, []string{"exec", "audit-shop-php-1", "cat", "/var/www/html/var/log/profiler.csv"}) {
						return []byte("type,timer\nfoo,1\n"), nil
					}
					return nil, nil
				},
				HTTPGet: func(_ context.Context, targetURL string) (int, error) {
					capturedURL = targetURL
					return 200, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			request := audit.ProfilerRequest{
				ProjectRoot: projectRoot,
				ProjectID:   "project-a",
				SessionID:   "session-a",
				RunID:       "run-0001",
				URL:         "https://audit-shop.test/catalogsearch/result/?q=$(touch%20/tmp/pwned)&product_list_limit=48",
				Target:      target,
			}

			if err := runtime.Activate(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			configDir := filepath.Join(projectRoot, testCase.configRelativeDir)
			entries, err := os.ReadDir(configDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].IsDir() || !strings.HasPrefix(entries[0].Name(), "govard-audit-profiler-") || !strings.HasSuffix(entries[0].Name(), ".conf") {
				t.Fatalf("temporary profiler configs = %#v, want one uniquely named .conf", entries)
			}
			configPath := filepath.Join(configDir, entries[0].Name())
			content, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), testCase.configNeedle) {
				t.Fatalf("temporary profiler config = %q, want %q", content, testCase.configNeedle)
			}

			if err := runtime.Capture(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if capturedURL != request.URL {
				t.Fatalf("captured URL = %q, want exact %q", capturedURL, request.URL)
			}
			csv, err := runtime.Collect(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if string(csv) != "type,timer\nfoo,1\n" {
				t.Fatalf("collected CSV = %q", csv)
			}
			if err := runtime.Restore(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(configPath); !os.IsNotExist(err) {
				t.Fatalf("temporary profiler config still exists after restore: %v", err)
			}
			env, err := os.ReadFile(envPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(env) != "<?php return ['sentinel' => true];\n" {
				t.Fatalf("app/etc/env.php was changed: %q", env)
			}

			wantCalls := [][]string{
				{"exec", "audit-shop-php-1", "rm", "-f", "/var/www/html/var/log/profiler.csv"},
				testCase.reload,
				{"exec", "audit-shop-php-1", "cat", "/var/www/html/var/log/profiler.csv"},
				testCase.reload,
				{"exec", "audit-shop-php-1", "rm", "-f", "/var/www/html/var/log/profiler.csv"},
			}
			if !reflect.DeepEqual(dockerCalls, wantCalls) {
				t.Fatalf("Docker calls = %#v, want %#v", dockerCalls, wantCalls)
			}
			for _, arguments := range dockerCalls {
				if strings.Contains(strings.Join(arguments, " "), request.URL) {
					t.Fatalf("user URL was interpolated into Docker command: %#v", arguments)
				}
			}
		})
	}
}

func TestAuditProfilerMagento2RejectsStandaloneBeforeMutation(t *testing.T) {
	definition, ok := frameworks.Get("magento2")
	if !ok {
		t.Fatal("Magento 2 definition is not registered")
	}
	root := t.TempDir()
	config := &engine.Config{ProjectName: "audit-shop", Framework: "magento2"}
	config.Stack.Services.WebServer = "nginx"
	var calls int
	_, err := cmd.NewAuditProfilerRuntimeForTest(cmd.AuditRunnerRequest{
		ProjectRoot: root,
		Definition:  definition,
		Target:      types.AuditTarget{Framework: "magento2", TargetPath: root, Mode: types.AuditTargetStandalone},
		Config:      config,
	}, cmd.AuditProfilerRuntimeDependenciesForTest{
		RunDocker: func(context.Context, ...string) ([]byte, error) {
			calls++
			return nil, nil
		},
		HTTPGet: func(context.Context, string) (int, error) {
			calls++
			return 200, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "standalone") {
		t.Fatalf("error = %v, want a clear standalone rejection", err)
	}
	if calls != 0 {
		t.Fatalf("external calls = %d, want none before standalone rejection", calls)
	}
}

func TestAuditProfilerMagento2RejectsUnsupportedWebServerBeforeMutation(t *testing.T) {
	definition, ok := frameworks.Get("magento2")
	if !ok {
		t.Fatal("Magento 2 definition is not registered")
	}
	root := t.TempDir()
	config := &engine.Config{ProjectName: "audit-shop", Framework: "magento2"}
	config.Stack.Services.WebServer = "caddy"
	_, err := cmd.NewAuditProfilerRuntimeForTest(cmd.AuditRunnerRequest{
		ProjectRoot: root,
		Definition:  definition,
		Target:      types.AuditTarget{Framework: "magento2", ProjectRoot: root, TargetPath: root, Mode: types.AuditTargetProject},
		Config:      config,
	}, cmd.AuditProfilerRuntimeDependenciesForTest{})
	if err == nil || !strings.Contains(err.Error(), "web server") {
		t.Fatalf("error = %v, want unsupported web server rejection", err)
	}
}
