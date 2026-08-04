package magento2

import (
	"fmt"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	engineremote "govard/internal/engine/remote"
)

// remoteAdminProbeScript is a PHP one-liner reading app/etc/env.php's
// backend.frontName, falling back to conventions.DefaultAdminPath.
const remoteAdminProbeScript = `$c=@include "app/etc/env.php"; if(!is_array($c)){fwrite(STDERR,"env.php not found"); exit(2);} echo (string)($c["backend"]["frontName"] ?? "` + conventions.DefaultAdminPath + `");`

// DetectRemoteAdminPath SSHs to remoteName and probes its app/etc/env.php
// for the configured admin frontName, falling back to
// conventions.DefaultAdminPath if the probe fails or returns nothing. This
// is the single source of truth for both internal/cmd/open_targets.go
// (govard open admin) and internal/desktop/remotes.go (desktop remote
// admin resolution), which previously each carried their own byte-identical
// copy of this probe.
func DetectRemoteAdminPath(remoteName string, remoteCfg engine.RemoteConfig) (string, error) {
	remoteCommand := "php -r " + engine.ShellQuote(remoteAdminProbeScript)
	if path := strings.TrimSpace(remoteCfg.Path); path != "" {
		remoteCommand = "cd " + engineremote.QuoteRemotePath(path) + " && " + remoteCommand
	}

	probeCmd := engineremote.BuildSSHExecCommand(remoteName, remoteCfg, true, remoteCommand)
	output, err := probeCmd.CombinedOutput()
	if err != nil {
		return conventions.DefaultAdminPath, fmt.Errorf("probe failed: %w", err)
	}

	value := strings.Trim(strings.TrimSpace(string(output)), "/")
	if value == "" {
		value = conventions.DefaultAdminPath
	}
	return value, nil
}
