package wordpress

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

// Environment holds credentials extracted remotely from this framework's
// wp-config.php.
type Environment struct {
	DB DatabaseInfo
}

// DatabaseInfo is the WordPress wp-config.php connection shape. It intentionally
// has no table-prefix field: WordPress remote imports do not inherit a local
// configuration prefix.
type DatabaseInfo struct {
	Host     string
	Port     int
	Username string
	Password string
	Database string
}

// ProbeEnvironment SSHs to the remote project and extracts WordPress DB
// credentials from wp-config.php. Deploy-layout targets keep the app under a
// served directory (sandbox `public_html`, deploy `current`) instead of the
// remote path root, so each candidate is tried in order before giving up.
func ProbeEnvironment(remoteName string, remoteCfg engine.RemoteConfig) (Environment, error) {
	return remote.TryProbeCandidatePaths(remoteCfg.Path, func(path string) (Environment, error) {
		cfg := remoteCfg
		cfg.Path = path
		remoteCommand := remote.BuildProjectRemoteCommand(path, `php -r `+engine.ShellQuote(dbProbePHP))
		encoded, err := remote.RunRemoteCapture(remoteName, cfg, remoteCommand)
		if err != nil {
			return Environment{}, err
		}
		env, decodeErr := decodeEnvironmentPayload(encoded)
		if decodeErr != nil {
			return Environment{}, fmt.Errorf("%w at %s: %v", remote.ErrProbeFilesNotFound, path, decodeErr)
		}
		return env, nil
	})
}

// DecodeEnvironmentPayloadForTest makes the remote payload boundary testable
// without an SSH server.
func DecodeEnvironmentPayloadForTest(encoded string) (Environment, error) {
	return decodeEnvironmentPayload(encoded)
}

func decodeEnvironmentPayload(encoded string) (Environment, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return Environment{}, fmt.Errorf("remote probe returned empty payload")
	}

	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return Environment{}, fmt.Errorf("decode remote probe payload: %w", err)
	}

	var payload struct {
		Host     string `json:"host"`
		Username string `json:"username"`
		Password string `json:"password"`
		DBName   string `json:"dbname"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return Environment{}, fmt.Errorf("parse remote probe payload: %w", err)
	}

	host, port := remote.ParseDatabaseHostPort(payload.Host)
	username := strings.TrimSpace(payload.Username)
	database := strings.TrimSpace(payload.DBName)
	if username == "" || database == "" {
		return Environment{}, fmt.Errorf("remote wp-config.php is missing DB_USER or DB_NAME")
	}

	return Environment{DB: DatabaseInfo{
		Host:     host,
		Port:     port,
		Username: username,
		Password: payload.Password,
		Database: database,
	}}, nil
}

const dbProbePHP = `
$dbname = ""; $dbuser = ""; $dbpass = ""; $dbhost = "";
$content = @file_get_contents("wp-config.php");
if ($content) {
    if (preg_match("/define\s*\(\s*['\"]DB_NAME['\"]\s*,\s*['\"]([^'\"]+)['\"]\s*\)/", $content, $m)) $dbname = $m[1];
    if (preg_match("/define\s*\(\s*['\"]DB_USER['\"]\s*,\s*['\"]([^'\"]+)['\"]\s*\)/", $content, $m)) $dbuser = $m[1];
    if (preg_match("/define\s*\(\s*['\"]DB_PASSWORD['\"]\s*,\s*['\"]([^'\"]+)['\"]\s*\)/", $content, $m)) $dbpass = $m[1];
    if (preg_match("/define\s*\(\s*['\"]DB_HOST['\"]\s*,\s*['\"]([^'\"]+)['\"]\s*\)/", $content, $m)) $dbhost = $m[1];
    if (!$dbname || !$dbuser) {
        define('SHORTINIT', true);
        @include "wp-config.php";
        if (defined('DB_NAME')) $dbname = DB_NAME;
        if (defined('DB_USER')) $dbuser = DB_USER;
        if (defined('DB_PASSWORD')) $dbpass = DB_PASSWORD;
        if (defined('DB_HOST')) $dbhost = DB_HOST;
    }
}
$r = ["host" => (string)$dbhost, "username" => (string)$dbuser, "password" => (string)$dbpass, "dbname" => (string)$dbname];
echo base64_encode(json_encode($r));
`
