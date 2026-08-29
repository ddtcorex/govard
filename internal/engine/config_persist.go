package engine

import (
	"os"
	"strings"
)

// PrepareConfigForWrite removes runtime-derived values so persisted config stays portable.
func PrepareConfigForWrite(config Config) Config {
	writable := config

	defaultXdebugSession := "PHPSTORM"
	if profileResult, err := ResolveRuntimeProfile(writable.Framework, writable.FrameworkVersion); err == nil {
		candidate := strings.TrimSpace(profileResult.Profile.XdebugSession)
		if candidate != "" {
			defaultXdebugSession = candidate
		}
	}
	if strings.EqualFold(strings.TrimSpace(writable.Stack.XdebugSession), strings.TrimSpace(defaultXdebugSession)) {
		writable.Stack.XdebugSession = ""
	}
	if strings.EqualFold(strings.TrimSpace(writable.Stack.WebServer), strings.TrimSpace(writable.Stack.Services.WebServer)) {
		writable.Stack.WebServer = ""
	}

	// Strip web server version for the server not in use.
	// Hybrid mode uses both nginx and apache, so both versions are kept.
	activeWebServer := strings.ToLower(strings.TrimSpace(writable.Stack.Services.WebServer))
	if activeWebServer != "" && activeWebServer != "hybrid" {
		if activeWebServer != "nginx" {
			writable.Stack.NginxVersion = ""
		}
		if activeWebServer != "apache" {
			writable.Stack.ApacheVersion = ""
		}
	}

	// Strip "none" service values to empty string.
	// NormalizeConfig now treats absent (empty) = "none", so we no longer need to
	// write "none" explicitly to YAML. Omitting the field keeps the file minimal.
	if writable.Stack.Services.Cache == "none" || writable.Stack.Services.Cache == "" {
		writable.Stack.Services.Cache = ""
		writable.Stack.CacheVersion = ""
	}
	if writable.Stack.Services.Search == "none" || writable.Stack.Services.Search == "" {
		writable.Stack.Services.Search = ""
		writable.Stack.SearchVersion = ""
	}
	if writable.Stack.Services.Queue == "none" || writable.Stack.Services.Queue == "" {
		writable.Stack.Services.Queue = ""
		writable.Stack.QueueVersion = ""
	}
	if writable.Stack.Services.DB == "none" || writable.Stack.Services.DB == "" {
		writable.Stack.Services.DB = ""
		writable.Stack.DBVersion = ""
	}

	// Double-ensure redundant fields are zeroed for serialization (though they are yaml:"-")
	writable.Stack.Features.Cache = false
	writable.Stack.Features.Search = false
	writable.Stack.Features.Queue = false

	if writable.Stack.UserID <= 0 {
		writable.Stack.UserID = 0
	} else {
		uid := os.Getuid()
		if uid >= 0 && writable.Stack.UserID == uid {
			writable.Stack.UserID = 0
		}
	}

	if writable.Stack.GroupID <= 0 {
		writable.Stack.GroupID = 0
	} else {
		gid := os.Getgid()
		if gid >= 0 && writable.Stack.GroupID == gid {
			writable.Stack.GroupID = 0
		}
	}

	for name, remote := range writable.Remotes {
		defaultMethod := NormalizeRemoteAuthMethod("")
		if NormalizeRemoteAuthMethod(remote.Auth.Method) == defaultMethod {
			remote.Auth.Method = ""
		}
		// Strip capabilities block if no capability is disabled (all-true = default behavior).
		// Storing capabilities: {files: true, media: true, db: true} is redundant noise.
		if remote.Capabilities != nil {
			caps := remote.Capabilities
			anyDisabled := (caps.Files != nil && !*caps.Files) ||
				(caps.Media != nil && !*caps.Media) ||
				(caps.DB != nil && !*caps.DB)
			if !anyDisabled {
				remote.Capabilities = nil
			}
		}
		writable.Remotes[name] = remote
	}

	if slicesEqual(writable.Stack.ChownDirList, GetDefaultChownDirList(writable.Framework)) {
		writable.Stack.ChownDirList = nil
	}

	// Strip ComposerVersion when it matches the auto-derived default.
	// This keeps .govard.yml minimal: only non-default values are written.
	if profileResult, err := ResolveRuntimeProfile(writable.Framework, writable.FrameworkVersion); err == nil {
		defaultComposer := profileResult.Profile.ComposerVersion
		if defaultComposer == "" {
			defaultComposer = "latest"
		}
		if writable.Stack.ComposerVersion == defaultComposer {
			writable.Stack.ComposerVersion = ""
		}
	}

	// Do not persist a default audit.lint.provider for frameworks that do
	// not implement audit lint. This prevents bootstrap/init from writing
	// audit.lint.provider: govard for Symfony/Laravel where audit run would
	// otherwise promise a gate that immediately fails with "no framework can
	// resolve audit target".
	if !FrameworkSupportsAuditLint(writable.Framework) {
		writable.Audit.Lint.Provider = ""
		if len(writable.Audit.Lint.ExternalProviders) == 0 {
			writable.Audit.Lint.ExternalProviders = nil
		}
		if writable.Audit.Lint.Provider == "" && writable.Audit.Lint.ExternalProviders == nil {
			writable.Audit = AuditConfig{}
		}
	}

	return writable
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
