package tests

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"govard/internal/engine/remote"
	"govard/internal/frameworks/magento1"
	"govard/internal/frameworks/magento2"
)

func TestParseRemoteDatabaseHostPort(t *testing.T) {
	testCases := []struct {
		name    string
		raw     string
		expectH string
		expectP int
	}{
		{name: "empty host", raw: "", expectH: "db", expectP: 3306},
		{name: "host only", raw: "database.internal", expectH: "database.internal", expectP: 3306},
		{name: "host and port", raw: "database.internal:3307", expectH: "database.internal", expectP: 3307},
		{name: "tcp prefix", raw: "tcp://db.example:3310", expectH: "db.example", expectP: 3310},
		{name: "ipv6 bracket host", raw: "[::1]:3309", expectH: "::1", expectP: 3309},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			host, port := remote.ParseDatabaseHostPort(testCase.raw)
			if host != testCase.expectH {
				t.Fatalf("host mismatch: got %q want %q", host, testCase.expectH)
			}
			if port != testCase.expectP {
				t.Fatalf("port mismatch: got %d want %d", port, testCase.expectP)
			}
		})
	}
}

func TestNormalizeMagentoVersion(t *testing.T) {
	testCases := []struct {
		name   string
		raw    string
		expect string
	}{
		{name: "caret constraint", raw: "^2.4.7-p1", expect: "2.4.7-p1"},
		{name: "tilde constraint", raw: "~2.4.6", expect: "2.4.6"},
		{name: "comparison constraint", raw: ">=2.4.8 <2.5", expect: "2.4.8"},
		{name: "pipe constraint", raw: "2.4.6-p3 || 2.4.7", expect: "2.4.6-p3"},
		{name: "wildcard", raw: "2.4.x", expect: "2.4.x"},
		{name: "empty", raw: "", expect: ""},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			actual := magento2.NormalizeMagentoVersion(testCase.raw)
			if actual != testCase.expect {
				t.Fatalf("version mismatch: got %q want %q", actual, testCase.expect)
			}
		})
	}
}

func TestDecodeMagento2EnvironmentPayloadIncludesTablePrefix(t *testing.T) {
	encoded := encodePayloadForTest(map[string]string{
		"host":         "db.example:3307",
		"username":     "mage",
		"password":     "secret",
		"dbname":       "magento",
		"table_prefix": "demo_",
	})

	metadata, err := magento2.DecodeMagento2EnvironmentPayloadForTest(encoded)
	if err != nil {
		t.Fatalf("DecodeMagento2EnvironmentPayloadForTest() error = %v", err)
	}
	if metadata.DB.TablePrefix != "demo_" {
		t.Fatalf("expected table prefix demo_, got %q", metadata.DB.TablePrefix)
	}
}

func TestDecodeMagento1EnvironmentPayloadIncludesTablePrefix(t *testing.T) {
	encoded := encodePayloadForTest(map[string]string{
		"host":         "db.example:3307",
		"username":     "mage",
		"password":     "secret",
		"dbname":       "magento",
		"table_prefix": "demo_",
	})

	metadata, err := magento1.DecodeMagento1EnvironmentPayloadForTest(encoded)
	if err != nil {
		t.Fatalf("DecodeMagento1EnvironmentPayloadForTest() error = %v", err)
	}
	if metadata.DB.TablePrefix != "demo_" {
		t.Fatalf("expected table prefix demo_, got %q", metadata.DB.TablePrefix)
	}
}

func encodePayloadForTest(payload map[string]string) string {
	data, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(data)
}
