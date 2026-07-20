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
