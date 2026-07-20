package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestPMAConfigContentIncludesPrestaShop(t *testing.T) {
	content := engine.BuildPMAConfigContentForTest()
	if !strings.Contains(content, "'prestashop' => 'prestashop'") {
		t.Fatalf("expected PMA config dbMap to include prestashop, got:\n%s", content)
	}
}
