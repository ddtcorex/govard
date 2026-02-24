package engine

import (
	"os"
	"strings"
)

func NormalizeConfig(config *Config) {
	if config == nil {
		return
	}

	normalizeBlueprintRegistryConfig(&config.BlueprintRegistry)

	fwConfig, ok := GetFrameworkConfig(config.Recipe)
	profileResult, profileErr := ResolveRuntimeProfile(config.Recipe, config.FrameworkVersion)
	profileAvailable := profileErr == nil
	profile := profileResult.Profile

	if config.Stack.DBType == "" {
		if profileAvailable && profile.DBType != "" {
			config.Stack.DBType = profile.DBType
		} else if ok && fwConfig.DefaultDB != "" {
			config.Stack.DBType = fwConfig.DefaultDB
		} else {
			config.Stack.DBType = "mariadb"
		}
	}

	if config.Stack.DBVersion == "" {
		if config.Stack.DBType == "none" {
			config.Stack.DBVersion = ""
		} else if profileAvailable &&
			strings.EqualFold(config.Stack.DBType, profile.DBType) &&
			profile.DBVersion != "" {
			config.Stack.DBVersion = profile.DBVersion
		} else if config.Stack.DBType == "mysql" && ok && fwConfig.DefaultMySQLVer != "" {
			config.Stack.DBVersion = fwConfig.DefaultMySQLVer
		} else if config.Stack.DBType == "mysql" {
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
		} else {
			config.Stack.PHPVersion = "8.4"
		}
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

	if config.Stack.Services.Search == "" {
		if profileAvailable && profile.Search != "" {
			config.Stack.Services.Search = profile.Search
		} else if ok && fwConfig.DefaultSearch != "" {
			config.Stack.Services.Search = fwConfig.DefaultSearch
		} else if config.Stack.Features.Elasticsearch {
			config.Stack.Services.Search = "opensearch"
		} else {
			config.Stack.Services.Search = "none"
		}
	}

	config.Stack.Services.Search = strings.ToLower(config.Stack.Services.Search)

	if config.Stack.Services.Cache == "" {
		if profileAvailable && profile.Cache != "" {
			config.Stack.Services.Cache = profile.Cache
		} else if ok && fwConfig.DefaultCache != "" {
			config.Stack.Services.Cache = fwConfig.DefaultCache
		} else if config.Stack.Features.Redis {
			config.Stack.Services.Cache = "redis"
		} else {
			config.Stack.Services.Cache = "none"
		}
	}

	config.Stack.Services.Cache = strings.ToLower(config.Stack.Services.Cache)

	if config.Stack.Services.Queue == "" {
		if profileAvailable && profile.Queue != "" {
			config.Stack.Services.Queue = profile.Queue
		} else if ok && fwConfig.DefaultQueue != "" {
			config.Stack.Services.Queue = fwConfig.DefaultQueue
		} else {
			config.Stack.Services.Queue = "none"
		}
	}
	config.Stack.Services.Queue = strings.ToLower(config.Stack.Services.Queue)

	config.Stack.Features.Redis = config.Stack.Services.Cache != "" && config.Stack.Services.Cache != "none"
	config.Stack.Features.Elasticsearch = config.Stack.Services.Search != "" && config.Stack.Services.Search != "none"
	config.Stack.WebServer = config.Stack.Services.WebServer

	if config.Stack.Services.Cache == "none" {
		config.Stack.CacheVersion = ""
	} else if config.Stack.CacheVersion == "" &&
		profileAvailable &&
		strings.EqualFold(config.Stack.Services.Cache, profile.Cache) &&
		profile.CacheVersion != "" {
		config.Stack.CacheVersion = profile.CacheVersion
	} else if config.Stack.CacheVersion == "" && ok && fwConfig.DefaultCacheVer != "" {
		config.Stack.CacheVersion = fwConfig.DefaultCacheVer
	} else if config.Stack.CacheVersion == "" {
		if config.Stack.Services.Cache == "valkey" {
			config.Stack.CacheVersion = "8.0.0"
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
			config.Stack.SearchVersion = "8.19.11"
		} else {
			config.Stack.SearchVersion = "3.4.0"
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
		config.Stack.VarnishVersion = "7.4"
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
		config.Stack.QueueVersion = "3.13.7"
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

	if config.Stack.WebRoot != "" && !strings.HasPrefix(config.Stack.WebRoot, "/") {
		config.Stack.WebRoot = "/" + config.Stack.WebRoot
	}

	if config.Remotes != nil {
		for name, remote := range config.Remotes {
			if remote.Port == 0 {
				remote.Port = 22
			}
			remote.Environment = NormalizeRemoteEnvironment(remote.Environment)
			remote.Capabilities = normalizeRemoteCapabilities(remote.Capabilities)
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
