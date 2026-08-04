package remote

import (
	"net"
	"strconv"
	"strings"

	"govard/internal/conventions"
)

// RemoteDatabaseMetadata holds database connection details extracted from a
// remote probe. It is a transport DTO: framework packages determine how to
// extract it while generic orchestration consumes only its common fields.
type RemoteDatabaseMetadata struct {
	Host        string
	Port        int
	Username    string
	Password    string
	Database    string
	TablePrefix string
	// Private is framework-owned opaque metadata carried alongside the generic
	// connection details. Core transport forwards it without interpreting it.
	Private map[string]string
}

// ParseDatabaseHostPort splits a raw host[:port] string into separate host and
// port values, applying conventions.DefaultDBHost/MySQLPort as fallbacks.
func ParseDatabaseHostPort(raw string) (string, int) {
	hostRaw := strings.TrimSpace(raw)
	if hostRaw == "" {
		return conventions.DefaultDBHost, conventions.MySQLPort
	}

	hostRaw = strings.TrimPrefix(hostRaw, "tcp://")
	if hostRaw == "" {
		return conventions.DefaultDBHost, conventions.MySQLPort
	}

	if host, port, err := net.SplitHostPort(hostRaw); err == nil {
		if parsed, parseErr := strconv.Atoi(port); parseErr == nil && parsed > 0 {
			if strings.TrimSpace(host) == "" {
				host = conventions.DefaultDBHost
			}
			return host, parsed
		}
	}

	if strings.Count(hostRaw, ":") == 1 {
		parts := strings.SplitN(hostRaw, ":", 2)
		portText := strings.TrimSpace(parts[1])
		if parsed, err := strconv.Atoi(portText); err == nil && parsed > 0 {
			host := strings.TrimSpace(parts[0])
			if host == "" {
				host = conventions.DefaultDBHost
			}
			return host, parsed
		}
	}

	return hostRaw, conventions.MySQLPort
}

// BuildProjectRemoteCommand prefixes body with a `cd <projectPath> &&` if
// projectPath is non-empty, so a probe runs from the project's root directory.
func BuildProjectRemoteCommand(projectPath string, body string) string {
	trimmedPath := strings.TrimSpace(projectPath)
	if trimmedPath == "" {
		return body
	}
	return "cd " + QuoteRemotePath(trimmedPath) + " && " + body
}
