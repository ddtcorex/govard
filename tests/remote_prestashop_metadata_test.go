package tests

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"govard/internal/engine/remote"
)

func TestDecodePrestaShopEnvironmentPayload(t *testing.T) {
	payload := map[string]string{
		"host":         "db:3306",
		"username":     "prestashop",
		"password":     "secret",
		"dbname":       "prestashop",
		"table_prefix": "ps_",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)

	env, err := remote.DecodePrestaShopEnvironmentPayloadForTest(encoded)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if env.DB.Host != "db" {
		t.Fatalf("expected host 'db', got %q", env.DB.Host)
	}
	if env.DB.Port != 3306 {
		t.Fatalf("expected port 3306, got %d", env.DB.Port)
	}
	if env.DB.Username != "prestashop" {
		t.Fatalf("expected username 'prestashop', got %q", env.DB.Username)
	}
	if env.DB.Password != "secret" {
		t.Fatalf("expected password 'secret', got %q", env.DB.Password)
	}
	if env.DB.Database != "prestashop" {
		t.Fatalf("expected database 'prestashop', got %q", env.DB.Database)
	}
	if env.DB.TablePrefix != "ps_" {
		t.Fatalf("expected table prefix 'ps_', got %q", env.DB.TablePrefix)
	}
}

func TestDecodePrestaShopEnvironmentPayloadIncludesSecrets(t *testing.T) {
	payload := map[string]string{
		"host":           "db:3306",
		"username":       "prestashop",
		"password":       "secret",
		"dbname":         "prestashop",
		"table_prefix":   "ps_",
		"secret":         "remote-secret",
		"cookie_key":     "remote-cookie-key",
		"cookie_iv":      "remote-cookie-iv",
		"new_cookie_key": "remote-new-cookie-key",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)

	env, err := remote.DecodePrestaShopEnvironmentPayloadForTest(encoded)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if env.Secrets.Secret != "remote-secret" {
		t.Fatalf("expected secret 'remote-secret', got %q", env.Secrets.Secret)
	}
	if env.Secrets.CookieKey != "remote-cookie-key" {
		t.Fatalf("expected cookie_key 'remote-cookie-key', got %q", env.Secrets.CookieKey)
	}
	if env.Secrets.CookieIV != "remote-cookie-iv" {
		t.Fatalf("expected cookie_iv 'remote-cookie-iv', got %q", env.Secrets.CookieIV)
	}
	if env.Secrets.NewCookieKey != "remote-new-cookie-key" {
		t.Fatalf("expected new_cookie_key 'remote-new-cookie-key', got %q", env.Secrets.NewCookieKey)
	}
}

func TestDecodePrestaShopEnvironmentPayloadSecretsOptional(t *testing.T) {
	payload := map[string]string{
		"host":     "db",
		"username": "prestashop",
		"dbname":   "prestashop",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)

	env, err := remote.DecodePrestaShopEnvironmentPayloadForTest(encoded)
	if err != nil {
		t.Fatalf("expected no error when secrets are absent, got: %v", err)
	}
	if env.Secrets.Secret != "" || env.Secrets.CookieKey != "" || env.Secrets.CookieIV != "" || env.Secrets.NewCookieKey != "" {
		t.Fatalf("expected empty secrets when absent from payload, got: %+v", env.Secrets)
	}
}

func TestDecodePrestaShopEnvironmentPayloadMissingRequiredFields(t *testing.T) {
	payload := map[string]string{"host": "db"}
	raw, _ := json.Marshal(payload)
	encoded := base64.StdEncoding.EncodeToString(raw)

	if _, err := remote.DecodePrestaShopEnvironmentPayloadForTest(encoded); err == nil {
		t.Fatal("expected error when username/dbname are missing")
	}
}

func TestDecodePrestaShopEnvironmentPayloadEmpty(t *testing.T) {
	if _, err := remote.DecodePrestaShopEnvironmentPayloadForTest(""); err == nil {
		t.Fatal("expected error for empty payload")
	}
}
