package tests

import (
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
)

func TestGenerateMagento1CryptKeyReturnsRandom32CharHex(t *testing.T) {
	key1, err := bootstrap.GenerateMagento1CryptKey()
	if err != nil {
		t.Fatalf("GenerateMagento1CryptKey() error = %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("expected a 32-character hex string, got %d chars: %q", len(key1), key1)
	}
	for _, r := range key1 {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("expected hex characters only, got %q", key1)
		}
	}

	key2, err := bootstrap.GenerateMagento1CryptKey()
	if err != nil {
		t.Fatalf("GenerateMagento1CryptKey() error = %v", err)
	}
	if key1 == key2 {
		t.Fatalf("expected two calls to produce different random keys, got the same value twice: %q", key1)
	}
}
