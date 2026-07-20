package remote

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"govard/internal/engine"
)

// PrestaShopEnvironment holds DB credentials and encryption secrets extracted
// remotely from a PrestaShop app/config/parameters.php.
type PrestaShopEnvironment struct {
	DB      MagentoDBInfo
	Secrets PrestaShopSecrets
}

// PrestaShopSecrets holds the encryption-related parameters.php keys. These are
// carried over (rather than regenerated) when fabricating a local parameters.php
// after a clone, so that any module data encrypted under the remote's keys stays
// decryptable locally.
type PrestaShopSecrets struct {
	Secret       string
	CookieKey    string
	CookieIV     string
	NewCookieKey string
}

// ProbePrestaShopEnvironment SSHs to the remote environment and includes
// app/config/parameters.php via PHP to extract DB connection credentials.
func ProbePrestaShopEnvironment(remoteName string, remoteCfg engine.RemoteConfig) (PrestaShopEnvironment, error) {
	remoteCommand := buildMagentoRemoteCommand(remoteCfg.Path, `php -r `+engine.ShellQuote(prestashopParametersProbePHP))
	encoded, err := runRemoteCapture(remoteName, remoteCfg, remoteCommand)
	if err != nil {
		return PrestaShopEnvironment{}, err
	}
	return decodePrestaShopEnvironmentPayload(encoded)
}

func DecodePrestaShopEnvironmentPayloadForTest(encoded string) (PrestaShopEnvironment, error) {
	return decodePrestaShopEnvironmentPayload(encoded)
}

func decodePrestaShopEnvironmentPayload(encoded string) (PrestaShopEnvironment, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return PrestaShopEnvironment{}, fmt.Errorf("remote probe returned empty payload")
	}

	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return PrestaShopEnvironment{}, fmt.Errorf("decode remote probe payload: %w", err)
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
		return PrestaShopEnvironment{}, fmt.Errorf("parse remote probe payload: %w", err)
	}

	host, port := ParseMagentoDBHostPort(payload.Host)
	username := strings.TrimSpace(payload.Username)
	database := strings.TrimSpace(payload.DBName)
	if username == "" || database == "" {
		return PrestaShopEnvironment{}, fmt.Errorf("remote parameters.php is missing database_user or database_name")
	}

	return PrestaShopEnvironment{
		DB: MagentoDBInfo{
			Host:        host,
			Port:        port,
			Username:    username,
			Password:    payload.Password,
			Database:    database,
			TablePrefix: engine.SafeTablePrefix(payload.TablePrefix),
		},
		Secrets: PrestaShopSecrets{
			Secret:       strings.TrimSpace(payload.Secret),
			CookieKey:    strings.TrimSpace(payload.CookieKey),
			CookieIV:     strings.TrimSpace(payload.CookieIV),
			NewCookieKey: strings.TrimSpace(payload.NewCookieKey),
		},
	}, nil
}

// prestashopParametersProbePHP includes app/config/parameters.php directly (it's
// guaranteed-valid PHP, since PrestaShop's own kernel includes it at boot) and
// reads the database_* keys out of the returned array.
const prestashopParametersProbePHP = `
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
