//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks/magento2"
)

func TestBuildMagentoCommandsBasic(t *testing.T) {
	config := engine.Config{
		ProjectName: "magento-test",
		Framework:   "magento2",
		Domain:      "magento-test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-test", config)

	if len(commands) == 0 {
		t.Fatal("Expected at least one Magento command")
	}

	hasDBConfig := false
	for _, cmd := range commands {
		if strings.Contains(cmd.Desc, "Database") {
			hasDBConfig = true
			break
		}
	}

	if !hasDBConfig {
		t.Error("Expected database configuration command")
	}
}

func TestBuildMagentoCommandsWithRedis(t *testing.T) {
	config := engine.Config{
		ProjectName: "magento-redis",
		Framework:   "magento2",
		Domain:      "magento-redis.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "redis",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-redis", config)

	hasRedisCache := false
	hasRedisSession := false

	for _, cmd := range commands {
		if strings.Contains(cmd.Desc, "Redis Cache") {
			hasRedisCache = true
		}
		if strings.Contains(cmd.Desc, "Redis Sessions") {
			hasRedisSession = true
		}
	}

	if !hasRedisCache {
		t.Error("Expected Redis cache configuration command")
	}
	if !hasRedisSession {
		t.Error("Expected Redis sessions configuration command")
	}
}

func TestBuildMagentoCommandsWithValkey(t *testing.T) {
	config := engine.Config{
		ProjectName: "magento-valkey",
		Framework:   "magento2",
		Domain:      "magento-valkey.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "valkey",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-valkey", config)

	hasCacheConfig := false
	for _, cmd := range commands {
		if strings.Contains(cmd.Desc, "Cache") {
			hasCacheConfig = true
			break
		}
	}

	if !hasCacheConfig {
		t.Error("Expected cache configuration command for Valkey")
	}
}

func TestBuildMagentoCommandsWithVarnish(t *testing.T) {
	config := engine.Config{
		ProjectName: "magento-varnish",
		Framework:   "magento2",
		Domain:      "magento-varnish.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Features: engine.Features{
				Varnish: true,
			},
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-varnish", config)

	hasVarnishConfig := false
	for _, cmd := range commands {
		if strings.Contains(cmd.Desc, "Varnish") {
			hasVarnishConfig = true
			break
		}
	}

	if !hasVarnishConfig {
		t.Error("Expected Varnish configuration command")
	}
}

func TestBuildMagentoCommandsWithElasticsearch(t *testing.T) {
	config := engine.Config{
		ProjectName: "magento-es",
		Framework:   "magento2",
		Domain:      "magento-es.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "elasticsearch",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-es", config)

	hasSearchConfig := false
	for _, cmd := range commands {
		if strings.Contains(cmd.Desc, "Setting Search Engine") {
			hasSearchConfig = true
			break
		}
	}

	if !hasSearchConfig {
		t.Error("Expected Search Engine configuration command")
	}
}

func TestBuildMagentoCommandsWithOpenSearch(t *testing.T) {
	config := engine.Config{
		ProjectName:      "magento-os",
		Framework:        "magento2",
		Domain:           "magento-os.test",
		FrameworkVersion: "2.4.8",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "opensearch",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-os", config)

	hasSearchConfig := false
	for _, cmd := range commands {
		if strings.Contains(cmd.Desc, "Setting Search Engine") {
			hasSearchConfig = true
			break
		}
	}

	if !hasSearchConfig {
		t.Error("Expected Search Engine configuration command")
	}
}

func TestBuildMagentoCommandsAllFeatures(t *testing.T) {
	config := engine.Config{
		ProjectName:      "magento-full",
		Framework:        "magento2",
		Domain:           "magento-full.test",
		FrameworkVersion: "2.4.8",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Features: engine.Features{
				Varnish: true,
			},
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "opensearch",
				Cache:     "redis",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-full", config)

	requiredCommands := map[string]bool{
		"Database":              false,
		"Redis":                 false,
		"Varnish":               false,
		"Setting Search Engine": false,
		"Developer":             false,
		"reCAPTCHA":             false,
		"2FA":                   false,
	}

	for _, cmd := range commands {
		for keyword := range requiredCommands {
			if strings.Contains(cmd.Desc, keyword) {
				requiredCommands[keyword] = true
			}
		}
	}

	for keyword, found := range requiredCommands {
		if !found {
			t.Errorf("Missing command for: %s", keyword)
		}
	}
}

func TestBuildMagentoCommandsBaseURL(t *testing.T) {
	config := engine.Config{
		ProjectName: "magento-url",
		Framework:   "magento2",
		Domain:      "magento-url.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-url", config)

	hasBaseURL := false
	for _, cmd := range commands {
		if strings.Contains(cmd.Desc, "Base URL") || strings.Contains(cmd.Desc, "URLs") {
			hasBaseURL = true
			break
		}
	}

	if !hasBaseURL {
		t.Error("Expected base URL configuration command")
	}
}

func TestBuildMagentoCommandsNoDomain(t *testing.T) {
	config := engine.Config{
		ProjectName: "magento-no-domain",
		Framework:   "magento2",
		Domain:      "",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			WebServer:  "nginx",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-no-domain", config)

	if len(commands) == 0 {
		t.Fatal("Expected commands even without domain")
	}
}

func TestBuildMagentoCommandsContainerName(t *testing.T) {
	config := engine.Config{
		ProjectName: "test-project",
		Framework:   "magento2",
		Domain:      "test.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("test-project", config)

	for _, cmd := range commands {
		found := false
		for _, arg := range cmd.Args {
			if strings.Contains(arg, "test-project-php-1") {
				found = true
				break
			}
		}
		if found {
			return
		}
	}

	t.Error("Commands should reference correct container name")
}

func TestBuildMagentoCommandsUser(t *testing.T) {
	config := engine.Config{
		ProjectName: "magento-user",
		Framework:   "magento2",
		Domain:      "magento-user.test",
		Stack: engine.Stack{
			PHPVersion: "8.3",
			Services: engine.Services{
				WebServer: "nginx",
				Search:    "none",
				Cache:     "none",
				Queue:     "none",
			},
		},
	}

	commands := magento2.MagentoConfigCommandsForTest("magento-user", config)

	hasUserFlag := false
	for _, cmd := range commands {
		for _, arg := range cmd.Args {
			if arg == "-u" || arg == "magento" {
				hasUserFlag = true
				break
			}
		}
		if hasUserFlag {
			break
		}
	}

	if !hasUserFlag {
		t.Error("Commands should run as magento user")
	}
}
