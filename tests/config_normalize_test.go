package tests

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"govard/internal/engine"
)

func TestNormalizeConfigDefaultsMagento2(t *testing.T) {
	config := engine.Config{
		Recipe: "magento2",
	}

	engine.NormalizeConfig(&config)

	if config.Stack.DBType != "mariadb" {
		t.Fatalf("Expected DBType mariadb, got %s", config.Stack.DBType)
	}
	if config.Stack.DBVersion != "11.4" {
		t.Fatalf("Expected DBVersion 11.4, got %s", config.Stack.DBVersion)
	}
	if config.Stack.PHPVersion != "8.4" {
		t.Fatalf("Expected PHPVersion 8.4, got %s", config.Stack.PHPVersion)
	}
	if config.Stack.NodeVersion != "24" {
		t.Fatalf("Expected NodeVersion 24, got %s", config.Stack.NodeVersion)
	}

	if config.Stack.Services.WebServer != "nginx" {
		t.Fatalf("Expected WebServer nginx, got %s", config.Stack.Services.WebServer)
	}
	if config.Stack.Services.Cache != "valkey" {
		t.Fatalf("Expected Cache valkey, got %s", config.Stack.Services.Cache)
	}
	if config.Stack.Services.Search != "opensearch" {
		t.Fatalf("Expected Search opensearch, got %s", config.Stack.Services.Search)
	}
	if config.Stack.Services.Queue != "none" {
		t.Fatalf("Expected Queue none, got %s", config.Stack.Services.Queue)
	}

	if config.Stack.CacheVersion != "8.0.0" {
		t.Fatalf("Expected CacheVersion 8.0.0, got %s", config.Stack.CacheVersion)
	}
	if config.Stack.SearchVersion != "2.19.0" {
		t.Fatalf("Expected SearchVersion 2.19.0, got %s", config.Stack.SearchVersion)
	}
	if config.Stack.QueueVersion != "" {
		t.Fatalf("Expected QueueVersion empty, got %s", config.Stack.QueueVersion)
	}
}

func TestNormalizeConfigQueueDefaults(t *testing.T) {
	config := engine.Config{
		Recipe: "magento2",
		Stack: engine.Stack{
			Services: engine.Services{
				Queue: "rabbitmq",
			},
		},
	}

	engine.NormalizeConfig(&config)

	if config.Stack.QueueVersion != "3.13.7" {
		t.Fatalf("Expected QueueVersion 3.13.7, got %s", config.Stack.QueueVersion)
	}
}

func TestNormalizeConfigVersionAwareDefaultsMagento2(t *testing.T) {
	config := engine.Config{
		Recipe:           "magento2",
		FrameworkVersion: "2.4.7-p3",
	}

	engine.NormalizeConfig(&config)

	if config.Stack.PHPVersion != "8.3" {
		t.Fatalf("Expected PHPVersion 8.3, got %s", config.Stack.PHPVersion)
	}
	if config.Stack.DBType != "mariadb" {
		t.Fatalf("Expected DBType mariadb, got %s", config.Stack.DBType)
	}
	if config.Stack.DBVersion != "10.6" {
		t.Fatalf("Expected DBVersion 10.6, got %s", config.Stack.DBVersion)
	}
	if config.Stack.Services.Cache != "redis" {
		t.Fatalf("Expected Cache redis, got %s", config.Stack.Services.Cache)
	}
	if config.Stack.CacheVersion != "7.2" {
		t.Fatalf("Expected CacheVersion 7.2, got %s", config.Stack.CacheVersion)
	}
	if config.Stack.Services.Search != "opensearch" {
		t.Fatalf("Expected Search opensearch, got %s", config.Stack.Services.Search)
	}
	if config.Stack.SearchVersion != "2.12.0" {
		t.Fatalf("Expected SearchVersion 2.12.0, got %s", config.Stack.SearchVersion)
	}
	if config.Stack.Services.Queue != "rabbitmq" {
		t.Fatalf("Expected Queue rabbitmq, got %s", config.Stack.Services.Queue)
	}
	if config.Stack.QueueVersion != "3.13" {
		t.Fatalf("Expected QueueVersion 3.13, got %s", config.Stack.QueueVersion)
	}
	if config.Stack.WebRoot != "/pub" {
		t.Fatalf("Expected WebRoot /pub, got %s", config.Stack.WebRoot)
	}
}

func TestNormalizeConfigCacheAndSearchVersions(t *testing.T) {
	config := engine.Config{
		Recipe: "magento2",
		Stack: engine.Stack{
			Services: engine.Services{
				Cache:  "valkey",
				Search: "opensearch",
			},
		},
	}

	engine.NormalizeConfig(&config)

	if config.Stack.CacheVersion != "8.0.0" {
		t.Fatalf("Expected CacheVersion 8.0.0, got %s", config.Stack.CacheVersion)
	}
	if config.Stack.SearchVersion != "2.19.0" {
		t.Fatalf("Expected SearchVersion 2.19.0, got %s", config.Stack.SearchVersion)
	}
}

func TestPrepareConfigForWriteOmitsCurrentRuntimeUserIDs(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()
	if uid < 0 || gid < 0 {
		t.Skip("uid/gid not available on this platform")
	}

	config := engine.Config{
		Stack: engine.Stack{
			UserID:  uid,
			GroupID: gid,
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	if writable.Stack.UserID != 0 {
		t.Fatalf("expected UserID to be omitted, got %d", writable.Stack.UserID)
	}
	if writable.Stack.GroupID != 0 {
		t.Fatalf("expected GroupID to be omitted, got %d", writable.Stack.GroupID)
	}

	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "user_id:") {
		t.Fatalf("expected serialized config to omit user_id, got:\n%s", content)
	}
	if strings.Contains(content, "group_id:") {
		t.Fatalf("expected serialized config to omit group_id, got:\n%s", content)
	}
}

func TestPrepareConfigForWriteKeepsCustomUserIDs(t *testing.T) {
	uid := os.Getuid()
	gid := os.Getgid()

	customUID := uid + 111
	customGID := gid + 222
	if uid < 0 {
		customUID = 2001
	}
	if gid < 0 {
		customGID = 2002
	}

	config := engine.Config{
		Stack: engine.Stack{
			UserID:  customUID,
			GroupID: customGID,
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	if writable.Stack.UserID != customUID {
		t.Fatalf("expected custom UserID %d to persist, got %d", customUID, writable.Stack.UserID)
	}
	if writable.Stack.GroupID != customGID {
		t.Fatalf("expected custom GroupID %d to persist, got %d", customGID, writable.Stack.GroupID)
	}
}

func TestPrepareConfigForWriteKeepsRuntimeProfileDefaults(t *testing.T) {
	profileResult, err := engine.ResolveRuntimeProfile("magento2", "2.4.7-p3")
	if err != nil {
		t.Fatalf("resolve runtime profile: %v", err)
	}
	profile := profileResult.Profile

	config := engine.Config{
		ProjectName:      "demo",
		Recipe:           "magento2",
		FrameworkVersion: "2.4.7-p3",
		Domain:           "demo.test",
		Stack: engine.Stack{
			PHPVersion:    profile.PHPVersion,
			NodeVersion:   profile.NodeVersion,
			DBType:        profile.DBType,
			DBVersion:     profile.DBVersion,
			WebRoot:       profile.WebRoot,
			CacheVersion:  profile.CacheVersion,
			SearchVersion: profile.SearchVersion,
			QueueVersion:  profile.QueueVersion,
			XdebugSession: profile.XdebugSession,
			WebServer:     profile.WebServer,
			Services: engine.Services{
				WebServer: profile.WebServer,
				Search:    profile.Search,
				Cache:     profile.Cache,
				Queue:     profile.Queue,
			},
			Features: engine.Features{
				Xdebug:        true,
				Redis:         profile.Cache != "none",
				Elasticsearch: profile.Search != "none",
			},
		},
		Remotes: map[string]engine.RemoteConfig{},
		Hooks:   map[string][]engine.HookStep{},
	}

	writable := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)

	for _, key := range []string{
		"php_version:",
		"node_version:",
		"db_type:",
		"db_version:",
		"web_root:",
		"cache_version:",
		"search_version:",
		"queue_version:",
		"web_server:",
		"search:",
		"cache:",
		"queue:",
		"remotes:",
	} {
		if !strings.Contains(content, key) {
			t.Fatalf("expected %q to be present in serialized config, got:\n%s", key, content)
		}
	}
	if strings.Contains(content, "hooks:") {
		t.Fatalf("expected empty hooks to be omitted, got:\n%s", content)
	}
	if strings.Contains(content, "xdebug_session:") {
		t.Fatalf("expected default xdebug_session to be omitted, got:\n%s", content)
	}
	if strings.Contains(content, "\n  web_server:") {
		t.Fatalf("expected duplicated top-level web_server to be omitted, got:\n%s", content)
	}
	if !strings.Contains(content, "    web_server:") {
		t.Fatalf("expected services.web_server to remain serialized, got:\n%s", content)
	}
	if !strings.Contains(content, "xdebug: true") {
		t.Fatalf("expected xdebug feature to be serialized, got:\n%s", content)
	}
	if strings.Contains(content, "redis:") {
		t.Fatalf("expected derived feature redis to be omitted, got:\n%s", content)
	}
	if strings.Contains(content, "elasticsearch:") {
		t.Fatalf("expected derived feature elasticsearch to be omitted, got:\n%s", content)
	}
}

func TestPrepareConfigForWriteKeepsNonDefaultRuntimeOverrides(t *testing.T) {
	config := engine.Config{
		Recipe:           "magento2",
		FrameworkVersion: "2.4.7-p3",
		Stack: engine.Stack{
			PHPVersion: "8.2",
			Services: engine.Services{
				Cache: "valkey",
			},
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	if writable.Stack.PHPVersion != "8.2" {
		t.Fatalf("expected non-default php_version to remain, got %q", writable.Stack.PHPVersion)
	}
	if writable.Stack.Services.Cache != "valkey" {
		t.Fatalf("expected non-default cache service to remain, got %q", writable.Stack.Services.Cache)
	}
}

func TestPrepareConfigForWriteKeepsNonDefaultXdebugSession(t *testing.T) {
	config := engine.Config{
		Recipe:           "magento2",
		FrameworkVersion: "2.4.7-p3",
		Stack: engine.Stack{
			XdebugSession: "VSCODE",
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	if writable.Stack.XdebugSession != "VSCODE" {
		t.Fatalf("expected non-default xdebug_session to remain, got %q", writable.Stack.XdebugSession)
	}

	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "xdebug_session: VSCODE") {
		t.Fatalf("expected serialized non-default xdebug_session, got:\n%s", content)
	}
}

func TestPrepareConfigForWritePrunesDefaultRemoteAuthAndPaths(t *testing.T) {
	config := engine.Config{
		ProjectName: "demo",
		Recipe:      "magento2",
		Domain:      "demo.test",
		Remotes: map[string]engine.RemoteConfig{
			"dev": {
				Host:        "example.com",
				User:        "deploy",
				Port:        22,
				Path:        "/var/www/html",
				Environment: "dev",
				Capabilities: engine.RemoteCapabilities{
					Files:  true,
					Media:  true,
					DB:     true,
					Deploy: true,
				},
				Auth: engine.RemoteAuth{
					Method: engine.RemoteAuthMethodKeychain,
				},
				Paths: engine.RemotePaths{
					Media: "",
				},
			},
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "method: keychain") {
		t.Fatalf("expected default remote auth method to be omitted, got:\n%s", content)
	}
	if strings.Contains(content, "key_path:") {
		t.Fatalf("expected empty auth.key_path to be omitted, got:\n%s", content)
	}
	if strings.Contains(content, "strict_host_key:") {
		t.Fatalf("expected default strict_host_key to be omitted, got:\n%s", content)
	}
	if strings.Contains(content, "known_hosts_file:") {
		t.Fatalf("expected empty known_hosts_file to be omitted, got:\n%s", content)
	}
	if strings.Contains(content, "paths:") {
		t.Fatalf("expected empty paths block to be omitted, got:\n%s", content)
	}
}

func TestPrepareConfigForWriteKeepsNonDefaultRemoteAuthAndPaths(t *testing.T) {
	config := engine.Config{
		ProjectName: "demo",
		Recipe:      "magento2",
		Domain:      "demo.test",
		Remotes: map[string]engine.RemoteConfig{
			"staging": {
				Host:        "staging.example.com",
				User:        "deploy",
				Port:        22,
				Path:        "/srv/www/staging",
				Environment: "staging",
				Capabilities: engine.RemoteCapabilities{
					Files:  true,
					Media:  true,
					DB:     true,
					Deploy: true,
				},
				Auth: engine.RemoteAuth{
					Method:         engine.RemoteAuthMethodKeyfile,
					KeyPath:        "~/.ssh/id_ed25519",
					StrictHostKey:  true,
					KnownHostsFile: "~/.ssh/known_hosts",
				},
				Paths: engine.RemotePaths{
					Media: "/srv/media",
				},
			},
		},
	}

	writable := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)
	for _, key := range []string{
		"method: keyfile",
		"key_path: ~/.ssh/id_ed25519",
		"strict_host_key: true",
		"known_hosts_file: ~/.ssh/known_hosts",
		"paths:",
		"media: /srv/media",
	} {
		if !strings.Contains(content, key) {
			t.Fatalf("expected %q to be serialized, got:\n%s", key, content)
		}
	}
}

func TestPrepareConfigForWriteOmitsEmptyFrameworkVersion(t *testing.T) {
	config := engine.Config{
		ProjectName:      "demo",
		Recipe:           "magento2",
		FrameworkVersion: "",
		Domain:           "demo.test",
		Stack: engine.Stack{
			PHPVersion:  "8.3",
			NodeVersion: "24",
			DBType:      "mariadb",
			DBVersion:   "10.6",
			WebRoot:     "/pub",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "opensearch",
				Cache:     "redis",
				Queue:     "rabbitmq",
			},
			Features: engine.Features{
				Xdebug: true,
			},
		},
		Remotes: map[string]engine.RemoteConfig{},
		Hooks:   map[string][]engine.HookStep{},
	}

	writable := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "framework_version:") {
		t.Fatalf("expected empty framework_version to be omitted, got:\n%s", content)
	}
}

func TestPrepareConfigForWriteKeepsNonEmptyFrameworkVersion(t *testing.T) {
	config := engine.Config{
		ProjectName:      "demo",
		Recipe:           "magento2",
		FrameworkVersion: "2.4.7-p3",
		Domain:           "demo.test",
		Stack: engine.Stack{
			PHPVersion:  "8.3",
			NodeVersion: "24",
			DBType:      "mariadb",
			DBVersion:   "10.6",
			WebRoot:     "/pub",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "opensearch",
				Cache:     "redis",
				Queue:     "rabbitmq",
			},
			Features: engine.Features{
				Xdebug: true,
			},
		},
		Remotes: map[string]engine.RemoteConfig{},
		Hooks:   map[string][]engine.HookStep{},
	}

	writable := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writable)
	if err != nil {
		t.Fatalf("marshal writable config: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "framework_version: 2.4.7-p3") {
		t.Fatalf("expected non-empty framework_version to be serialized, got:\n%s", content)
	}
}
