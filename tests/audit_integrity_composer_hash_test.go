package tests

import (
	"strings"
	"testing"

	"govard/internal/audit"
)

// The expected values below were produced by Composer's own algorithm, not by
// this repository. To regenerate them (Docker is enough; no PHP needed on the
// host), write the manifest to /tmp/hashgen/composer.json and run:
//
//	docker run --rm -v /tmp/hashgen:/app -w /app composer:2 php -r '
//	  $c = json_decode(file_get_contents("/app/composer.json"), true);
//	  $keys = ["name","version","require","require-dev","conflict","replace",
//	           "provide","minimum-stability","prefer-stable","repositories","extra"];
//	  $out = [];
//	  foreach (array_intersect($keys, array_keys($c)) as $k) { $out[$k] = $c[$k]; }
//	  if (isset($c["config"]["platform"])) { $out["config"]["platform"] = $c["config"]["platform"]; }
//	  ksort($out);
//	  echo md5(json_encode($out)), "\n";'
//
// One of them is additionally cross-checked against a real `composer update`
// lock file (see TestComposerContentHashMatchesARealLock), because a
// reimplementation that agrees with a reimplementation proves nothing.
func TestComposerContentHashMatchesRealComposer(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		want     string
	}{
		{
			// Unicode, escaped slashes, an empty JSON object and a nested list.
			name: "encoding edges",
			manifest: `{
                "name": "acme/sample",
                "description": "golden fixture",
                "require": {"php": ">=8.1", "psr/log": "^3.0"},
                "require-dev": {"psr/container": "^2.0"},
                "config": {"platform": {"php": "8.1.0"}},
                "extra": {"acme": {"unicode": "café — naïve ✓", "path": "app/code/Acme/Demo", "empty": {}, "list": []}},
                "repositories": [{"type": "composer", "url": "https://repo.packagist.org"}],
                "minimum-stability": "stable"
            }`,
			want: "152210304525b237a1733442d5b19f60",
		},
		{
			name:     "minimal",
			manifest: `{"name":"a/b","require":{"php":">=8.1"}}`,
			want:     "b69ffcca18d652d1bf0757529d0b7827",
		},
		{
			// Keys Composer does not hash must not change the digest.
			name:     "irrelevant keys are excluded",
			manifest: `{"name":"a/b","description":"x","authors":[{"name":"Someone","email":"s@example.com"}],"keywords":["a","b"],"homepage":"https://example.com/"}`,
			want:     "7f686f5492b90e6f7c1f5878e6f3e259",
		},
		{
			name:     "numbers, booleans and null",
			manifest: `{"name":"a/b","extra":{"count":7,"ratio":0.5,"enabled":true,"disabled":false,"nothing":null,"big":12345678901234,"neg":-3}}`,
			want:     "9c11df42d09bdedc980739e928a81ccf",
		},
		{
			// Top-level keys are sorted before hashing, so the author's order
			// must not matter.
			name:     "scrambled key order",
			manifest: `{"repositories":[{"type":"vcs","url":"https://github.com/acme/demo.git"}],"require":{"psr/log":"^3.0"},"name":"acme/scrambled","prefer-stable":true,"extra":{"deep":{"a":{"b":{"c":[1,2,3]}}}},"minimum-stability":"dev"}`,
			want:     "dd9ae547a0d9b6bb705100ff5976be51",
		},
		{
			// PHP decodes `{}` to an empty array and re-encodes it as `[]`, so
			// an empty object is not byte-identical to itself.
			name:     "empty objects become empty arrays",
			manifest: `{"name":"a/b","require":{},"require-dev":{},"extra":{}}`,
			want:     "4242105192258cea0ebe6b9b326711a9",
		},
		{
			name:     "quotes, backslashes and control characters",
			manifest: `{"name":"a/b","extra":{"quote":"he said \"hi\"","backslash":"C:\\path\\to","slashes":"a/b/c","tab":"a\tb","newline":"a\nb"}}`,
			want:     "e47834bbdd3f2ad1ddc441a6a901c05f",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := audit.ComposerContentHashForTest([]byte(tc.manifest))
			if err != nil {
				t.Fatalf("hash: %v", err)
			}
			if got != tc.want {
				t.Fatalf("hash = %s, want %s (the manifest hashes differently than Composer would)", got, tc.want)
			}
		})
	}
}

func TestComposerContentHashMatchesARealLock(t *testing.T) {
	// The one golden that is not a reimplementation: this manifest was passed
	// to the real `composer update`, and the digest is what it wrote into
	// composer.lock.
	manifest := `{
    "name": "acme/sample",
    "description": "golden fixture",
    "type": "project",
    "license": "MIT",
    "require": {"php": ">=8.1", "psr/log": "^3.0"},
    "require-dev": {"psr/container": "^2.0"},
    "config": {"platform": {"php": "8.1.0"}},
    "extra": {"acme": {"unicode": "café — naïve ✓", "path": "app/code/Acme/Demo", "empty": {}, "list": []}},
    "repositories": [{"type": "composer", "url": "https://repo.packagist.org"}],
    "minimum-stability": "stable"
}`
	// `composer update` wrote exactly this into composer.lock.
	const fromRealComposer = "152210304525b237a1733442d5b19f60"

	got, err := audit.ComposerContentHashForTest([]byte(manifest))
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if got != fromRealComposer {
		t.Fatalf("hash = %s, want the %s real Composer wrote", got, fromRealComposer)
	}
}

func TestComposerContentHashRejectsBrokenJSON(t *testing.T) {
	if _, err := audit.ComposerContentHashForTest([]byte("{not json")); err == nil {
		t.Fatal("want an error for a manifest that is not JSON")
	}
}

func TestComposerIntegrityDetectsAStaleLock(t *testing.T) {
	findings := analyzeComposerFixture(t, "composer-stale-lock", "8.3")
	assertRule(t, findings, "COMPOSER_LOCK_CONTENT_HASH_MISMATCH")
	for _, finding := range findings {
		if finding.Rule != "COMPOSER_LOCK_CONTENT_HASH_MISMATCH" {
			continue
		}
		if !strings.Contains(finding.Message, "composer.json") {
			t.Fatalf("the finding must send the operator to composer.json, got %q", finding.Message)
		}
	}
}

func TestComposerIntegrityLeavesALockWithoutAContentHashAlone(t *testing.T) {
	// A lock written before Composer recorded the field has nothing to compare;
	// inventing a mismatch would fail projects that are in fact fine.
	findings := analyzeComposerFixture(t, "composer-lock-without-hash", "8.3")
	assertNoRule(t, findings, "COMPOSER_LOCK_CONTENT_HASH_MISMATCH")
}

func TestComposerIntegrityAcceptsALockThatMatchesItsManifest(t *testing.T) {
	// `composer-valid` carries the digest real Composer computes for its
	// manifest, so the new rule must stay silent on it.
	findings := analyzeComposerFixture(t, "composer-valid", "8.3")
	assertNoRule(t, findings, "COMPOSER_LOCK_CONTENT_HASH_MISMATCH")
}
