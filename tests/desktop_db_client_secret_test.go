package tests

import (
	"strings"
	"testing"

	"govard/internal/desktop"
)

// The external-client URL carries credentials by design (one-click open),
// so any error quoting that URL must have the secret scrubbed — raw and in
// the percent-encoded form the URL string carries.
func TestDesktopPkgScrubDBClientURLSecret(t *testing.T) {
	url := desktop.BuildDesktopDBClientURLForTest("mysql", "dbuser", "s3cr!t p@ss", "127.0.0.1", "3306", "shop")
	if !strings.Contains(url, "s3cr") && !strings.Contains(url, "%") {
		t.Fatalf("test setup wrong: expected credentials in URL, got %q", url)
	}
	quoted := "open failed for " + url + " (exit status 1)"
	scrubbed := desktop.ScrubDBClientURLSecretForTest(quoted, "dbuser", "s3cr!t p@ss")
	if strings.Contains(scrubbed, "s3cr") {
		t.Fatalf("scrubbed text still carries the password: %q", scrubbed)
	}
	// The whole userinfo token goes (usernames are redacted with it: a
	// partial credential is worse than a missing username); host, database
	// and message stay for debugging.
	for _, keep := range []string{"127.0.0.1", "shop", "open failed"} {
		if !strings.Contains(scrubbed, keep) {
			t.Fatalf("scrub removed non-secret text %q: %q", keep, scrubbed)
		}
	}
	if got := desktop.ScrubDBClientURLSecretForTest("plain", "user", ""); got != "plain" {
		t.Fatalf("empty password must leave text untouched, got %q", got)
	}
}

func TestDesktopPkgScrubDBClientURLSecretEdges(t *testing.T) {
	// A username needing percent-encoding must not defeat the scrub: the
	// raw "user:" prefix never appears in the URL, only the encoded form.
	url := desktop.BuildDesktopDBClientURLForTest("mysql", "ops team", "p@ss", "127.0.0.1", "3306", "shop")
	quoted := "open failed for " + url
	scrubbed := desktop.ScrubDBClientURLSecretForTest(quoted, "ops team", "p@ss")
	if strings.Contains(scrubbed, "p@ss") || strings.Contains(scrubbed, "%40ss") {
		t.Fatalf("encoded username left the password behind: %q", scrubbed)
	}

	// A short password must not turn the scrubber into a message mangler:
	// only the exact credential token is redacted, never bare substrings.
	quoted = "db cluster db is unreachable"
	if got := desktop.ScrubDBClientURLSecretForTest(quoted, "dbuser", "db"); got != quoted {
		t.Fatalf("short password mangled innocent text: %q", got)
	}
}
