package magento2

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"govard/internal/engine"
	remote "govard/internal/engine/remote"
)

var magentoVersionPattern = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:-p\d+)?`)

type Magento2Environment struct {
	DB       remote.RemoteDatabaseMetadata
	CryptKey string
}

func ProbeMagento2Environment(remoteName string, remoteCfg engine.RemoteConfig) (Magento2Environment, error) {
	remoteCommand := remote.BuildProjectRemoteCommand(remoteCfg.Path, `php -r `+engine.ShellQuote(magentoDBProbePHP))
	encoded, err := remote.RunRemoteCapture(remoteName, remoteCfg, remoteCommand)
	if err != nil {
		return Magento2Environment{}, err
	}
	return decodeMagento2EnvironmentPayload(encoded)
}

func NormalizeMagentoVersion(raw string) string {
	return normalizeMagentoVersion(raw)
}

func DecodeMagento2EnvironmentPayloadForTest(encoded string) (Magento2Environment, error) {
	return decodeMagento2EnvironmentPayload(encoded)
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
		Host        string `json:"host"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		DBName      string `json:"dbname"`
		TablePrefix string `json:"table_prefix"`
		CryptKey    string `json:"crypt_key"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return Magento2Environment{}, fmt.Errorf("parse remote probe payload: %w", err)
	}

	host, port := remote.ParseDatabaseHostPort(payload.Host)
	username := strings.TrimSpace(payload.Username)
	database := strings.TrimSpace(payload.DBName)
	if username == "" || database == "" {
		return Magento2Environment{}, fmt.Errorf("remote env.php is missing db username or dbname")
	}

	return Magento2Environment{
		DB: remote.RemoteDatabaseMetadata{
			Host:        host,
			Port:        port,
			Username:    username,
			Password:    payload.Password,
			Database:    database,
			TablePrefix: engine.SafeTablePrefix(payload.TablePrefix),
		},
		CryptKey: strings.TrimSpace(payload.CryptKey),
	}, nil
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

const magentoDBProbePHP = `$c=@include "app/etc/env.php"; if(!is_array($c)){fwrite(STDERR,"env.php not found"); exit(2);} $d=$c["db"]["connection"]["default"] ?? null; if(!is_array($d)){fwrite(STDERR,"db.default missing"); exit(3);} $r=["host"=>$d["host"] ?? "", "username"=>$d["username"] ?? "", "password"=>$d["password"] ?? "", "dbname"=>$d["dbname"] ?? "", "table_prefix"=>($c["db"]["table_prefix"] ?? ""), "crypt_key"=>($c["crypt"]["key"] ?? "")]; echo base64_encode(json_encode($r));`
