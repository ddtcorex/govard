package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
	_ "govard/internal/frameworks"
)

// The WordPress seed copied the database only, so siteurl and home kept the
// origin URL and the first deploy check was refused with a redirect.
func TestWordPressSandboxSeedRewritesSiteURLAndHome(t *testing.T) {
	definition, ok := engine.SandboxSeedFor("wordpress")
	if !ok {
		t.Fatal("wordpress must register a sandbox seed definition")
	}
	if definition.DBRewrite == nil {
		t.Fatal("the wordpress seed must rewrite siteurl and home after the import")
	}
	statements := definition.DBRewrite(nil, "http://127.0.0.1:32768/")
	joined := strings.Join(statements, "\n")
	for _, want := range []string{"siteurl", "home", "http://127.0.0.1:32768/", "option_value"} {
		if !strings.Contains(joined, want) {
			t.Errorf("rewrite SQL must mention %q: %s", want, joined)
		}
	}
}

func TestWordPressSandboxSeedQuotesTheURL(t *testing.T) {
	definition, _ := engine.SandboxSeedFor("wordpress")
	if definition.DBRewrite == nil {
		t.Skip("covered by the registration test")
	}
	joined := strings.Join(definition.DBRewrite(nil, "http://x/'; DROP TABLE a; --"), "\n")
	if strings.Contains(joined, "'http://x/'; DROP") {
		t.Fatalf("the URL must be quoted, not spliced: %s", joined)
	}
}
