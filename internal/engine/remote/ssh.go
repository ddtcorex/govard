package remote

import (
	"strconv"
	"strings"

	"govard/internal/engine"
)

func BuildSSHArgs(remoteName string, remoteCfg engine.RemoteConfig, forwardAgent bool, interactive bool) []string {
	args := []string{
		"-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=10",
	}
	// One connection per target for the whole run: a deploy runs a command per
	// step, and without this every one of them pays a full handshake and
	// authentication (see multiplexArgs).
	args = append(args, multiplexArgs()...)
	if !interactive {
		args = append(args, "-o", "BatchMode=yes")
	} else {
		args = append(args, "-t")
	}
	if remoteCfg.Auth.StrictHostKey {
		args = append(args, "-o", "StrictHostKeyChecking=yes")
		if remoteCfg.Auth.KnownHostsFile != "" {
			args = append(args, "-o", "UserKnownHostsFile="+remoteCfg.Auth.KnownHostsFile)
		}
	} else {
		args = append(args, "-o", "StrictHostKeyChecking=no")
		args = append(args, "-o", "UserKnownHostsFile=/dev/null")
	}
	if remoteCfg.Port > 0 {
		args = append(args, "-p", strconv.Itoa(remoteCfg.Port))
	}
	if forwardAgent {
		args = append(args, "-A")
	}
	if keyPath, _ := ResolveSSHKeyPath(remoteName, remoteCfg); keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	return args
}

func QuoteRemotePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return "$HOME/" + engine.ShellQuote(path[2:])
	}
	return engine.ShellQuote(path)
}
