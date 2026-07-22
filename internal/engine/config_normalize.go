package engine

import (
	"os"
	"strings"
)

func NormalizeConfig(config *Config, root string) {
	if config == nil {
		return
	}

	normalizeBlueprintRegistryConfig(&config.BlueprintRegistry)
	config.StoreDomains = normalizeStoreDomainMappings(config.StoreDomains)

	config.Framework = strings.ToLower(strings.TrimSpace(config.Framework))
	if config.Framework == "magento" {
		config.Framework = "magento2"
	}
	config.TablePrefix = NormalizeTablePrefix(config.TablePrefix)
	if config.TablePrefix == "" && root != "" {
		config.TablePrefix = DetectMagentoTablePrefix(root, config.Framework)
	}

	fwConfig, ok := GetFrameworkConfig(config.Framework)
	profileResult, profileErr := ResolveRuntimeProfile(config.Framework, config.FrameworkVersion)
	profileAvailable := profileErr == nil
	profile := profileResult.Profile

	if config.Stack.WebRoot == "" || (config.Stack.WebRoot == "/" && root != "") {
		detected := DetectWebRoot(root, config.Framework)
		if detected != "" {
			config.Stack.WebRoot = detected
		} else if config.Stack.WebRoot == "" {
			if profileAvailable && profile.WebRoot != "" {
				config.Stack.WebRoot = profile.WebRoot
			} else if ok && fwConfig.NGINXPUBLIC != "" {
				config.Stack.WebRoot = fwConfig.NGINXPUBLIC
			}
		}
	}

	// If db is not declared, treat as "none" (e.g. frontend-only or external DB projects).
	// Callers must explicitly declare the DB service they want.
	if config.Stack.Services.DB == "" {
		config.Stack.Services.DB = "none"
	}

	config.Stack.Services.DB = strings.ToLower(config.Stack.Services.DB)

	if config.Stack.DBVersion == "" {
		if config.Stack.Services.DB == "none" {
			config.Stack.DBVersion = ""
		} else if profileAvailable &&
			strings.EqualFold(config.Stack.Services.DB, profile.DB) &&
			profile.DBVersion != "" {
			config.Stack.DBVersion = profile.DBVersion
		} else if config.Stack.Services.DB == "mysql" && ok && fwConfig.DefaultMySQLVer != "" {
			config.Stack.DBVersion = fwConfig.DefaultMySQLVer
		} else if config.Stack.Services.DB == "mysql" {
			config.Stack.DBVersion = "8.4"
		} else if ok && fwConfig.DefaultDBVer != "" {
			config.Stack.DBVersion = fwConfig.DefaultDBVer
		} else {
			config.Stack.DBVersion = "10.6"
		}
	}

	if config.Stack.PHPVersion == "" {
		if profileAvailable && profile.PHPVersion != "" {
			config.Stack.PHPVersion = profile.PHPVersion
		} else if ok && fwConfig.DefaultPHP != "" {
			config.Stack.PHPVersion = fwConfig.DefaultPHP
		}
		// If DefaultPHP is empty (e.g., custom framework with no PHP),
		// leave PHPVersion empty to indicate no PHP container is needed
	}

	if config.Stack.PythonVersion == "" {
		if ok && fwConfig.DefaultPythonVer != "" {
			config.Stack.PythonVersion = fwConfig.DefaultPythonVer
		}
		// If DefaultPythonVer is empty (e.g., a non-Python framework), leave
		// PythonVersion empty - no Python container is needed.
	}

	if config.Stack.NodeVersion == "" {
		if profileAvailable && profile.NodeVersion != "" {
			config.Stack.NodeVersion = profile.NodeVersion
		} else if ok && fwConfig.DefaultNodeVer != "" {
			config.Stack.NodeVersion = fwConfig.DefaultNodeVer
		} else {
			config.Stack.NodeVersion = "24"
		}
	}

	if config.Stack.XdebugSession == "" {
		if profileAvailable && profile.XdebugSession != "" {
			config.Stack.XdebugSession = profile.XdebugSession
		} else {
			config.Stack.XdebugSession = "PHPSTORM"
		}
	}

	if config.Stack.WebRoot == "" {
		if profileAvailable && profile.WebRoot != "" {
			config.Stack.WebRoot = profile.WebRoot
		} else if ok && fwConfig.NGINXPUBLIC != "" {
			config.Stack.WebRoot = fwConfig.NGINXPUBLIC
		}
	}

	if config.Stack.NginxVersion == "" {
		if profileAvailable && profile.NginxVersion != "" {
			config.Stack.NginxVersion = profile.NginxVersion
		} else if ok && fwConfig.DefaultNginxVer != "" {
			config.Stack.NginxVersion = fwConfig.DefaultNginxVer
		} else {
			config.Stack.NginxVersion = "1.28"
		}
	}

	if config.Stack.ComposerVersion == "" {
		if profileAvailable && profile.ComposerVersion != "" {
			config.Stack.ComposerVersion = profile.ComposerVersion
		} else if ok && fwConfig.DefaultComposerVer != "" {
			config.Stack.ComposerVersion = fwConfig.DefaultComposerVer
		}
		// If DefaultComposerVer is empty (e.g., custom framework with no PHP),
		// leave ComposerVersion empty
	}

	// Safety override: Composer 2.3+ requires PHP >= 7.2.5.
	// Force Composer 2.2 LTS when running on older PHP, even if "latest" was requested.
	if config.Stack.ComposerVersion == "latest" &&
		config.Stack.PHPVersion != "" &&
		!IsNumericDotVersionAtLeast(config.Stack.PHPVersion, "7.2.5") {
		config.Stack.ComposerVersion = "2.2"
	}

	if config.Stack.ApacheVersion == "" {
		if profileAvailable && profile.ApacheVersion != "" {
			config.Stack.ApacheVersion = profile.ApacheVersion
		} else if ok && fwConfig.DefaultApacheVer != "" {
			config.Stack.ApacheVersion = fwConfig.DefaultApacheVer
		} else {
			config.Stack.ApacheVersion = "2.4"
		}
	}

	if config.Stack.Services.WebServer == "" {
		if config.Stack.WebServer != "" {
			config.Stack.Services.WebServer = config.Stack.WebServer
		} else if profileAvailable && profile.WebServer != "" {
			config.Stack.Services.WebServer = profile.WebServer
		} else if ok && fwConfig.DefaultWebServer != "" {
			config.Stack.Services.WebServer = fwConfig.DefaultWebServer
		} else {
			config.Stack.Services.WebServer = "nginx"
		}
	}

	config.Stack.Services.WebServer = strings.ToLower(config.Stack.Services.WebServer)

	// If search/cache/queue are not declared, treat as "none".
	// Callers must explicitly set the service they want — we do not auto-fill from profile.
	if config.Stack.Services.Search == "" {
		config.Stack.Services.Search = "none"
	}
	config.Stack.Services.Search = strings.ToLower(config.Stack.Services.Search)

	if config.Stack.Services.Cache == "" {
		config.Stack.Services.Cache = "none"
	}
	config.Stack.Services.Cache = strings.ToLower(config.Stack.Services.Cache)

	if config.Stack.Services.Queue == "" {
		config.Stack.Services.Queue = "none"
	}
	config.Stack.Services.Queue = strings.ToLower(config.Stack.Services.Queue)

	// Sync Features and Services (Service Presence as Master)
	// 1. If service string is missing or "none", ensure feature is false.
	// 2. If feature is true but service is "none", service wins (it's disabled).

	config.Stack.Features.Cache = config.Stack.Services.Cache != "" && config.Stack.Services.Cache != "none"
	config.Stack.Features.Search = config.Stack.Services.Search != "" && config.Stack.Services.Search != "none"
	config.Stack.Features.Queue = config.Stack.Services.Queue != "" && config.Stack.Services.Queue != "none"
	config.Stack.WebServer = config.Stack.Services.WebServer

	if config.Stack.Services.Cache == "none" {
		config.Stack.CacheVersion = ""
	} else if config.Stack.CacheVersion == "" &&
		profileAvailable &&
		strings.EqualFold(config.Stack.Services.Cache, profile.Cache) &&
		profile.CacheVersion != "" {
		config.Stack.CacheVersion = profile.CacheVersion
	} else if config.Stack.CacheVersion == "" {
		if config.Stack.Services.Cache == "valkey" {
			config.Stack.CacheVersion = "9.0"
		} else if ok && fwConfig.DefaultCacheVer != "" && strings.EqualFold(config.Stack.Services.Cache, fwConfig.DefaultCache) {
			config.Stack.CacheVersion = fwConfig.DefaultCacheVer
		} else {
			config.Stack.CacheVersion = "7.4"
		}
	}

	if config.Stack.Services.Search == "none" {
		config.Stack.SearchVersion = ""
	} else if config.Stack.SearchVersion == "" &&
		profileAvailable &&
		strings.EqualFold(config.Stack.Services.Search, profile.Search) &&
		profile.SearchVersion != "" {
		config.Stack.SearchVersion = profile.SearchVersion
	} else if config.Stack.SearchVersion == "" && ok && fwConfig.DefaultSearchVer != "" {
		config.Stack.SearchVersion = fwConfig.DefaultSearchVer
	} else if config.Stack.SearchVersion == "" {
		if config.Stack.Services.Search == "elasticsearch" {
			config.Stack.SearchVersion = "8.15"
		} else {
			config.Stack.SearchVersion = "3.0"
		}
	}

	if !config.Stack.Features.Varnish {
		config.Stack.VarnishVersion = ""
	} else if config.Stack.VarnishVersion == "" &&
		profileAvailable &&
		profile.VarnishVersion != "" {
		config.Stack.VarnishVersion = profile.VarnishVersion
	} else if config.Stack.VarnishVersion == "" && ok && fwConfig.DefaultVarnishVer != "" {
		config.Stack.VarnishVersion = fwConfig.DefaultVarnishVer
	} else if config.Stack.VarnishVersion == "" {
		config.Stack.VarnishVersion = "8.0"
	}

	if config.Stack.Services.Queue == "none" {
		config.Stack.QueueVersion = ""
	} else if config.Stack.QueueVersion == "" &&
		profileAvailable &&
		strings.EqualFold(config.Stack.Services.Queue, profile.Queue) &&
		profile.QueueVersion != "" {
		config.Stack.QueueVersion = profile.QueueVersion
	} else if config.Stack.QueueVersion == "" && ok && fwConfig.DefaultQueueVer != "" {
		config.Stack.QueueVersion = fwConfig.DefaultQueueVer
	} else if config.Stack.QueueVersion == "" {
		config.Stack.QueueVersion = "4.2"
	}

	if config.Stack.UserID == 0 {
		uid := os.Getuid()
		if uid < 0 {
			uid = 1000
		}
		config.Stack.UserID = uid
	}
	if config.Stack.GroupID == 0 {
		gid := os.Getgid()
		if gid < 0 {
			gid = 1000
		}
		config.Stack.GroupID = gid
	}

	if len(config.Stack.ChownDirList) == 0 {
		config.Stack.ChownDirList = GetDefaultChownDirList(config.Framework)
	}

	if config.Stack.WebRoot != "" && !strings.HasPrefix(config.Stack.WebRoot, "/") {
		config.Stack.WebRoot = "/" + config.Stack.WebRoot
	}

	if config.Remotes != nil {
		for name, remote := range config.Remotes {
			if remote.Port == 0 {
				remote.Port = 22
			}
			remote.Auth.Method = NormalizeRemoteAuthMethod(remote.Auth.Method)
			remote.Auth.KeyPath = strings.TrimSpace(remote.Auth.KeyPath)
			remote.Auth.KnownHostsFile = strings.TrimSpace(remote.Auth.KnownHostsFile)
			remote.Paths.Media = strings.TrimSpace(remote.Paths.Media)
			if remote.Auth.KnownHostsFile != "" {
				remote.Auth.StrictHostKey = true
			}
			config.Remotes[name] = remote
		}
	}
}
