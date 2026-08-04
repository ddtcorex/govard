package tests

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"govard/internal/frameworks/wordpress"
)

// This catches a regression where the WordPress remote probe lived in generic
// engine code and its framework-specific wp-config.php contract was lost.
func TestDecodeWordPressEnvironmentPayload(t *testing.T) {
	raw, err := json.Marshal(map[string]string{
		"host":     "wordpress-db:3307",
		"username": "wordpress",
		"password": "secret",
		"dbname":   "wordpress",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	environment, err := wordpress.DecodeEnvironmentPayloadForTest(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if environment.DB.Host != "wordpress-db" || environment.DB.Port != 3307 {
		t.Fatalf("expected wordpress-db:3307, got %s:%d", environment.DB.Host, environment.DB.Port)
	}
	if environment.DB.Username != "wordpress" || environment.DB.Password != "secret" || environment.DB.Database != "wordpress" {
		t.Fatalf("expected decoded WordPress credentials, got %+v", environment.DB)
	}
}

func TestDecodeWordPressEnvironmentPayloadRejectsMissingCredentials(t *testing.T) {
	raw, err := json.Marshal(map[string]string{"host": "wordpress-db"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, err := wordpress.DecodeEnvironmentPayloadForTest(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("expected missing DB_USER and DB_NAME to be rejected")
	}
}
