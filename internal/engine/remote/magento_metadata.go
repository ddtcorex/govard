package remote

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"govard/internal/engine"
)

var magentoVersionPattern = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:-p\d+)?`)

type MagentoDBInfo struct {
	Host     string
	Port     int
	Username string
	Password string
	Database string
}

type Magento2Environment struct {
	DB       MagentoDBInfo
	CryptKey string
}

func ProbeMagento2Environment(remoteName string, remoteCfg engine.RemoteConfig) (Magento2Environment, error) {
	remoteCommand := buildMagentoRemoteCommand(remoteCfg.Path, `php -r `+shellQuoteRemote(magentoDBProbePHP))
	encoded, err := runRemoteCapture(remoteName, remoteCfg, remoteCommand)
	if err != nil {
		return Magento2Environment{}, err
	}
	return decodeMagento2EnvironmentPayload(encoded)
}

func DetectMagento2Version(remoteName string, remoteCfg engine.RemoteConfig) (string, error) {
	remoteCommand := buildMagentoRemoteCommand(remoteCfg.Path, `php -r `+shellQuoteRemote(magentoVersionProbePHP))
	output, err := runRemoteCapture(remoteName, remoteCfg, remoteCommand)
	if err != nil {
		return "", err
	}
	version := normalizeMagentoVersion(strings.TrimSpace(output))
	if version == "" {
		return "", fmt.Errorf("remote composer.json does not contain a Magento package version")
	}
	return version, nil
}

func ParseMagentoDBHostPort(raw string) (string, int) {
	hostRaw := strings.TrimSpace(raw)
	if hostRaw == "" {
		return "db", 3306
	}

	hostRaw = strings.TrimPrefix(hostRaw, "tcp://")
	if hostRaw == "" {
		return "db", 3306
	}

	if host, port, err := net.SplitHostPort(hostRaw); err == nil {
		if parsed, parseErr := strconv.Atoi(port); parseErr == nil && parsed > 0 {
			if strings.TrimSpace(host) == "" {
				host = "db"
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
				host = "db"
			}
			return host, parsed
		}
	}

	return hostRaw, 3306
}

func NormalizeMagentoVersion(raw string) string {
	return normalizeMagentoVersion(raw)
}

func decodeMagento2EnvironmentPayload(encoded string) (Magento2Environment, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return Magento2Environment{}, fmt.Errorf("remote probe returned empty payload")
	}

	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return Magento2Environment{}, fmt.Errorf("decode remote probe payload: %w", err)
	}

	var payload struct {
		Host     string `json:"host"`
		Username string `json:"username"`
		Password string `json:"password"`
		DBName   string `json:"dbname"`
		CryptKey string `json:"crypt_key"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return Magento2Environment{}, fmt.Errorf("parse remote probe payload: %w", err)
	}

	host, port := ParseMagentoDBHostPort(payload.Host)
	username := strings.TrimSpace(payload.Username)
	database := strings.TrimSpace(payload.DBName)
	if username == "" || database == "" {
		return Magento2Environment{}, fmt.Errorf("remote env.php is missing db username or dbname")
	}

	return Magento2Environment{
		DB: MagentoDBInfo{
			Host:     host,
			Port:     port,
			Username: username,
			Password: payload.Password,
			Database: database,
		},
		CryptKey: strings.TrimSpace(payload.CryptKey),
	}, nil
}

func buildMagentoRemoteCommand(projectPath string, body string) string {
	trimmedPath := strings.TrimSpace(projectPath)
	if trimmedPath == "" {
		return body
	}
	return "cd " + shellQuoteRemote(trimmedPath) + " && " + body
}

func shellQuoteRemote(raw string) string {
	if raw == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(raw, "'", `'"'"'`) + "'"
}

func normalizeMagentoVersion(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return ""
	}

	for _, separator := range []string{"||", "|", ",", " "} {
		parts := strings.Split(cleaned, separator)
		if len(parts) <= 1 {
			continue
		}
		candidate := normalizeMagentoVersion(parts[0])
		if candidate != "" {
			return candidate
		}
	}

	cleaned = strings.TrimLeft(cleaned, "^~>=< ")
	cleaned = strings.TrimPrefix(cleaned, "v")
	cleaned = strings.TrimSpace(cleaned)
	if strings.ContainsAny(cleaned, "xX*") {
		return cleaned
	}
	if match := magentoVersionPattern.FindString(cleaned); match != "" {
		return match
	}
	return cleaned
}

const magentoDBProbePHP = `$c=@include "app/etc/env.php"; if(!is_array($c)){fwrite(STDERR,"env.php not found"); exit(2);} $d=$c["db"]["connection"]["default"] ?? null; if(!is_array($d)){fwrite(STDERR,"db.default missing"); exit(3);} $r=["host"=>$d["host"] ?? "", "username"=>$d["username"] ?? "", "password"=>$d["password"] ?? "", "dbname"=>$d["dbname"] ?? "", "crypt_key"=>($c["crypt"]["key"] ?? "")]; echo base64_encode(json_encode($r));`

const magentoVersionProbePHP = `$c=@json_decode(@file_get_contents("composer.json"), true); if(!is_array($c)){fwrite(STDERR,"composer.json missing"); exit(2);} $r=$c["require"] ?? []; $v=""; if(isset($r["magento/product-community-edition"])){$v=$r["magento/product-community-edition"]; } elseif(isset($r["magento/product-enterprise-edition"])){$v=$r["magento/product-enterprise-edition"]; } echo (string)$v;`
