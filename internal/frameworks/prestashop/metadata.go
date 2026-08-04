package prestashop

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

// Environment holds database credentials and encryption secrets extracted
// remotely from this framework's app/config/parameters.php.
type Environment struct {
	DB      remote.RemoteDatabaseMetadata
	Secrets Secrets
}

// Secrets holds the encryption-related parameters.php keys. They are carried
// over (rather than regenerated) when fabricating a local parameters.php so
// module data encrypted under the remote's keys stays decryptable locally.
type Secrets struct {
	Secret       string
	CookieKey    string
	CookieIV     string
	NewCookieKey string
}

func (s Secrets) metadata() map[string]string {
	return map[string]string{
		"prestashop.secret":         s.Secret,
		"prestashop.cookie_key":     s.CookieKey,
		"prestashop.cookie_iv":      s.CookieIV,
		"prestashop.new_cookie_key": s.NewCookieKey,
	}
}

// ProbeEnvironment SSHs to the remote environment and includes this
// framework's app/config/parameters.php via PHP to extract connection
// credentials and encryption material.
func ProbeEnvironment(remoteName string, remoteCfg engine.RemoteConfig) (Environment, error) {
	remoteCommand := remote.BuildProjectRemoteCommand(remoteCfg.Path, `php -r `+engine.ShellQuote(parametersProbePHP))
	encoded, err := remote.RunRemoteCapture(remoteName, remoteCfg, remoteCommand)
	if err != nil {
		return Environment{}, err
	}
	return decodeEnvironmentPayload(encoded)
}

// DecodePrestaShopEnvironmentPayloadForTest makes the remote payload boundary
// testable without an SSH server.
func DecodePrestaShopEnvironmentPayloadForTest(encoded string) (Environment, error) {
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
		Host         string `json:"host"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		DBName       string `json:"dbname"`
		TablePrefix  string `json:"table_prefix"`
		Secret       string `json:"secret"`
		CookieKey    string `json:"cookie_key"`
		CookieIV     string `json:"cookie_iv"`
		NewCookieKey string `json:"new_cookie_key"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return Environment{}, fmt.Errorf("parse remote probe payload: %w", err)
	}

	host, port := remote.ParseDatabaseHostPort(payload.Host)
	username := strings.TrimSpace(payload.Username)
	database := strings.TrimSpace(payload.DBName)
	if username == "" || database == "" {
		return Environment{}, fmt.Errorf("remote parameters.php is missing database_user or database_name")
	}

	return Environment{
		DB: remote.RemoteDatabaseMetadata{
			Host:        host,
			Port:        port,
			Username:    username,
			Password:    payload.Password,
			Database:    database,
			TablePrefix: engine.SafeTablePrefix(payload.TablePrefix),
		},
		Secrets: Secrets{
			Secret:       strings.TrimSpace(payload.Secret),
			CookieKey:    strings.TrimSpace(payload.CookieKey),
			CookieIV:     strings.TrimSpace(payload.CookieIV),
			NewCookieKey: strings.TrimSpace(payload.NewCookieKey),
		},
	}, nil
}

// parametersProbePHP includes app/config/parameters.php directly (it is
// guaranteed-valid PHP, since PrestaShop's own kernel includes it at boot) and
// reads its database and encryption keys from the returned array.
const parametersProbePHP = `
$dbhost=""; $dbport=""; $dbuser=""; $dbpass=""; $dbname=""; $dbprefix="";
$secret=""; $cookieKey=""; $cookieIV=""; $newCookieKey="";
$f = "app/config/parameters.php";
if (@is_file($f)) {
    $config = include $f;
    if (is_array($config) && isset($config['parameters']) && is_array($config['parameters'])) {
        $p = $config['parameters'];
        $dbhost = isset($p['database_host']) ? (string)$p['database_host'] : "";
        $dbport = isset($p['database_port']) ? (string)$p['database_port'] : "";
        $dbuser = isset($p['database_user']) ? (string)$p['database_user'] : "";
        $dbpass = isset($p['database_password']) ? (string)$p['database_password'] : "";
        $dbname = isset($p['database_name']) ? (string)$p['database_name'] : "";
        $dbprefix = isset($p['database_prefix']) ? (string)$p['database_prefix'] : "";
        $secret = isset($p['secret']) ? (string)$p['secret'] : "";
        $cookieKey = isset($p['cookie_key']) ? (string)$p['cookie_key'] : "";
        $cookieIV = isset($p['cookie_iv']) ? (string)$p['cookie_iv'] : "";
        $newCookieKey = isset($p['new_cookie_key']) ? (string)$p['new_cookie_key'] : "";
    }
}
$host = $dbhost;
if ($dbport !== "") { $host = $dbhost . ":" . $dbport; }
$r = ["host"=>$host, "username"=>$dbuser, "password"=>$dbpass, "dbname"=>$dbname, "table_prefix"=>$dbprefix, "secret"=>$secret, "cookie_key"=>$cookieKey, "cookie_iv"=>$cookieIV, "new_cookie_key"=>$newCookieKey];
echo base64_encode(json_encode($r));`
