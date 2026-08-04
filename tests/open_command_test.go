package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/magento2"
)

func TestOpenAdminURL(t *testing.T) {
	cfg := engine.Config{Domain: "shop.test"}
	url := cmd.OpenAdminURLForTest(cfg)
	if url != "https://shop.test/admin" {
		t.Fatalf("unexpected url: %s", url)
	}
}

func TestOpenAdminURLEmdash(t *testing.T) {
	cfg := engine.Config{Domain: "emdash.test", Framework: "emdash"}
	url := cmd.OpenAdminURLForTest(cfg)
	if url != "https://emdash.test/_emdash/admin" {
		t.Fatalf("unexpected url: %s", url)
	}
}

func TestDetectLocalAdminURLEmdash(t *testing.T) {
	cfg := engine.Config{Domain: "emdash.test", Framework: "emdash"}
	url := cmd.DetectLocalAdminURLForTest(cfg)
	if url != "https://emdash.test/_emdash/admin" {
		t.Fatalf("unexpected local admin url: %s", url)
	}
}

func TestResolveOpenEnvironmentForTest(t *testing.T) {
	cfg := engine.Config{
		Remotes: map[string]engine.RemoteConfig{
			"dev": {},
			"stg": {},
		},
	}

	t.Run("defaults to local when unset", func(t *testing.T) {
		environment, isRemote, err := cmd.ResolveOpenEnvironmentForTest(cfg, "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if isRemote {
			t.Fatal("expected local mode")
		}
		if environment != "local" {
			t.Fatalf("expected local environment, got %q", environment)
		}
	})

	t.Run("supports local explicit value", func(t *testing.T) {
		environment, isRemote, err := cmd.ResolveOpenEnvironmentForTest(cfg, "local")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if isRemote {
			t.Fatal("expected local mode")
		}
		if environment != "local" {
			t.Fatalf("expected local environment, got %q", environment)
		}
	})

	t.Run("resolves remote by name", func(t *testing.T) {
		environment, isRemote, err := cmd.ResolveOpenEnvironmentForTest(cfg, "dev")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !isRemote {
			t.Fatal("expected remote mode")
		}
		if environment != "dev" {
			t.Fatalf("expected dev remote, got %q", environment)
		}
	})

	t.Run("resolves remote by environment alias", func(t *testing.T) {
		environment, isRemote, err := cmd.ResolveOpenEnvironmentForTest(cfg, "staging")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !isRemote {
			t.Fatal("expected remote mode")
		}
		if environment != "stg" {
			t.Fatalf("expected stg remote, got %q", environment)
		}
	})

	t.Run("fails for unknown remote", func(t *testing.T) {
		_, _, err := cmd.ResolveOpenEnvironmentForTest(cfg, "missing")
		if err == nil {
			t.Fatal("expected unknown remote environment to fail")
		}
	})
}

func TestBuildRemoteAdminURLForTest(t *testing.T) {
	url := cmd.BuildRemoteAdminURLForTest(engine.RemoteConfig{Host: "dev.example.com"}, "backend_xyz")
	if url != "https://dev.example.com/backend_xyz" {
		t.Fatalf("unexpected remote admin url: %s", url)
	}
}

func TestFrameworkRemoteAdminPathDispatch(t *testing.T) {
	emdashPath, err := frameworks.ResolveRemoteAdminPath("emdash", "staging", engine.RemoteConfig{})
	if err != nil {
		t.Fatalf("resolve Emdash admin path: %v", err)
	}
	if emdashPath != "_emdash/admin" {
		t.Fatalf("expected Emdash path, got %q", emdashPath)
	}

	defaultPath, err := frameworks.ResolveRemoteAdminPath("laravel", "staging", engine.RemoteConfig{})
	if err != nil {
		t.Fatalf("resolve Laravel admin path: %v", err)
	}
	if defaultPath != "admin" {
		t.Fatalf("expected generic admin path, got %q", defaultPath)
	}

	magento, ok := frameworks.Get("magento2")
	if !ok || magento.ResolveRemoteAdminPath == nil {
		t.Fatal("expected Magento 2 to own remote admin-path probing")
	}
	mageOS, ok := frameworks.Get("mageos")
	if !ok || mageOS.ResolveRemoteAdminPath == nil {
		t.Fatal("expected Mage-OS to inherit remote admin-path probing")
	}
}

func TestBuildSFTPURLForTest(t *testing.T) {
	url := cmd.BuildSFTPURLForTest(engine.RemoteConfig{
		Host: "dev.example.com",
		User: "deploy",
		Port: 2222,
		Path: "/srv/www/html",
	})
	if !strings.HasPrefix(url, "sftp://deploy@dev.example.com:2222/") {
		t.Fatalf("unexpected sftp url host segment: %s", url)
	}
	if !strings.Contains(url, "/srv/www/html") {
		t.Fatalf("unexpected sftp url path: %s", url)
	}
}

func TestResolveMagentoAdminURLForTest(t *testing.T) {
	baseURL := "https://shop.test"

	t.Run("uses env frontName by default", func(t *testing.T) {
		url := magento2.ResolveLocalAdminURL(baseURL, "backend_x", map[string]string{})
		if url != "https://shop.test/backend_x" {
			t.Fatalf("unexpected admin url from env frontName: %s", url)
		}
	})

	t.Run("uses custom_path from db when enabled", func(t *testing.T) {
		url := magento2.ResolveLocalAdminURL(baseURL, "backend_x", map[string]string{
			"admin/url/use_custom_path": "1",
			"admin/url/custom_path":     "super-secret-admin",
		})
		if url != "https://shop.test/super-secret-admin" {
			t.Fatalf("unexpected custom_path admin url: %s", url)
		}
	})

	t.Run("uses custom url from db when enabled", func(t *testing.T) {
		url := magento2.ResolveLocalAdminURL(baseURL, "backend_x", map[string]string{
			"admin/url/use_custom": "1",
			"admin/url/custom":     "https://admin.example.com/secure-panel",
		})
		if url != "https://admin.example.com/secure-panel" {
			t.Fatalf("unexpected custom admin url: %s", url)
		}
	})

	t.Run("falls back to admin when no data", func(t *testing.T) {
		url := magento2.ResolveLocalAdminURL(baseURL, "", map[string]string{})
		if url != "https://shop.test/admin" {
			t.Fatalf("unexpected fallback admin url: %s", url)
		}
	})
}

func TestResolveOpenDBEnvironmentForTest(t *testing.T) {
	cfg := engine.Config{
		Remotes: map[string]engine.RemoteConfig{
			"dev": {},
			"stg": {},
		},
	}

	t.Run("defaults to local when unset", func(t *testing.T) {
		environment, isRemote, err := cmd.ResolveOpenDBEnvironmentForTest(cfg, "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if isRemote {
			t.Fatal("expected local mode for default environment")
		}
		if environment != "local" {
			t.Fatalf("expected local environment, got %q", environment)
		}
	})

	t.Run("supports local override", func(t *testing.T) {
		environment, isRemote, err := cmd.ResolveOpenDBEnvironmentForTest(cfg, "local")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if isRemote {
			t.Fatal("expected local mode")
		}
		if environment != "local" {
			t.Fatalf("expected local environment, got %q", environment)
		}
	})

	t.Run("resolves remote by environment alias", func(t *testing.T) {
		environment, isRemote, err := cmd.ResolveOpenDBEnvironmentForTest(cfg, "staging")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !isRemote {
			t.Fatal("expected remote mode")
		}
		if environment != "stg" {
			t.Fatalf("expected stg remote name, got %q", environment)
		}
	})

	t.Run("fails on unknown remote environment", func(t *testing.T) {
		_, _, err := cmd.ResolveOpenDBEnvironmentForTest(cfg, "prod")
		if err == nil {
			t.Fatal("expected unknown environment to fail")
		}
	})

	t.Run("defaults to local when no remotes are configured", func(t *testing.T) {
		emptyCfg := engine.Config{}
		environment, isRemote, err := cmd.ResolveOpenDBEnvironmentForTest(emptyCfg, "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if isRemote {
			t.Fatal("expected local fallback")
		}
		if environment != "local" {
			t.Fatalf("expected local environment, got %q", environment)
		}
	})
}

func TestBuildOpenDBConnectionURLForTest(t *testing.T) {
	connectionURL := cmd.BuildOpenDBConnectionURLForTest("user@name", "pa:ss word", "shop db", 13306)
	if !strings.HasPrefix(connectionURL, "mysql://user%40name:pa%3Ass%20word@127.0.0.1:13306/") {
		t.Fatalf("expected encoded credentials in URL, got %q", connectionURL)
	}
	if !strings.HasSuffix(connectionURL, "/shop%20db") {
		t.Fatalf("expected encoded database name in URL, got %q", connectionURL)
	}
}
