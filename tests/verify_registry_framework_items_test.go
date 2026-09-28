package tests

import (
	"context"
	"testing"

	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/verify"
)

func idsOf(items []verify.Item) map[string]bool {
	ids := map[string]bool{}
	for _, it := range items {
		ids[it.ID] = true
	}
	return ids
}

// The declaration is only reachable if the engine-definition projection carries
// it. Asserted over the real registry, so a framework that declares items and
// is not projected fails here rather than silently running nothing.
func TestVerifyToolItemDefinitionsAreProjected(t *testing.T) {
	declaring := 0
	for _, definition := range frameworks.All() {
		want := len(definition.VerifyToolItems)
		if got := len(engine.VerifyToolItems(definition.Name)); got != want {
			t.Fatalf("framework %q: engine has %d verify items, the definition declares %d",
				definition.Name, got, want)
		}
		if want > 0 {
			declaring++
		}
	}
	if declaring < 4 {
		t.Fatalf("only %d frameworks declare verify items; the dev-loop gap covers magento2, laravel, symfony and wordpress", declaring)
	}
}

func TestRegistryForComposesFrameworkItems(t *testing.T) {
	for _, tc := range []struct {
		framework string
		want      []string
	}{
		{"magento2", []string{"P5-MAG-01"}},
		{"laravel", []string{"P3-LAR-01", "P3-LAR-02", "P3-LAR-03", "P5-LAR-01"}},
		{"symfony", []string{"P3-SYM-01", "P3-SYM-02", "P3-SYM-03", "P5-SYM-01"}},
		{"wordpress", []string{"P3-WP-01", "P3-WP-02", "P3-WP-03", "P5-WP-01"}},
		{"drupal", nil},
	} {
		t.Run(tc.framework, func(t *testing.T) {
			cfg := engine.Config{Framework: tc.framework}
			got := idsOf(verify.RegistryFor(cfg))

			for _, id := range tc.want {
				if !got[id] {
					t.Errorf("%s: RegistryFor is missing %s", tc.framework, id)
				}
			}
			if len(got) != len(verify.Registry)+len(tc.want) {
				t.Errorf("%s: RegistryFor returned %d items, want %d",
					tc.framework, len(got), len(verify.Registry)+len(tc.want))
			}
		})
	}
}

// A declaration is data a framework owns, so its shape is checked directly —
// a Phase outside 1..5 would make the item invisible in every `--phase N` run
// and appear only in a bare `--phase 0`, which is the silent failure this
// guards against. Every registered framework is walked, not a hand-picked list:
// one that starts declaring items without being added here would go unchecked.
func TestFrameworkVerifyItemDeclarationsAreWellFormed(t *testing.T) {
	for _, definition := range frameworks.All() {
		for _, decl := range engine.VerifyToolItems(definition.Name) {
			if decl.ID == "" {
				t.Errorf("%s: a declaration has an empty id", definition.Name)
			}
			if decl.Title == "" {
				t.Errorf("%s/%s: empty title", definition.Name, decl.ID)
			}
			if decl.Tool == "" {
				t.Errorf("%s/%s: empty tool command", definition.Name, decl.ID)
			}
			if decl.Phase < 1 || decl.Phase > 5 {
				t.Errorf("%s/%s: phase %d is outside 1..5; the item would run only under --phase 0",
					definition.Name, decl.ID, decl.Phase)
			}
		}
	}
}

// The composed list is appended, never de-duplicated, so a declaration that
// repeats a static id — or another declaration's id — would run twice.
func TestRegistryForComposesFrameworkItemsWithoutCollisions(t *testing.T) {
	static := idsOf(verify.Registry)
	for _, definition := range frameworks.All() {
		cfg := engine.Config{Framework: definition.Name}
		declared := engine.VerifyToolItems(definition.Name)
		composed := verify.RegistryFor(cfg)

		if len(composed) != len(verify.Registry)+len(declared) {
			t.Errorf("%s: RegistryFor returned %d items, want %d (static %d + declared %d)",
				definition.Name, len(composed), len(verify.Registry)+len(declared),
				len(verify.Registry), len(declared))
		}

		seen := map[string]bool{}
		for _, it := range composed {
			if it.ID == "" {
				t.Errorf("%s: composed item with an empty id", definition.Name)
				continue
			}
			if seen[it.ID] {
				t.Errorf("%s: id %q appears twice in the composed registry", definition.Name, it.ID)
			}
			seen[it.ID] = true
		}
		for _, decl := range declared {
			if static[decl.ID] {
				t.Errorf("%s: declared id %q collides with a static registry item", definition.Name, decl.ID)
			}
		}
	}
}

// Phase 3 is where the framework dev-loop items live, and it runs without a
// gate, so the exact argv of each one can be read back from the fake runner.
func TestVerifyFrameworkItemArgv(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	t.Setenv("GOVARD_VERIFY_FAKE", "1")

	want := map[string]map[string]string{
		"laravel": {
			"P3-LAR-01": "fake: tool artisan --version",
			"P3-LAR-02": "fake: tool artisan migrate:status",
			"P3-LAR-03": "fake: tool artisan cache:clear",
		},
		"symfony": {
			"P3-SYM-01": "fake: tool symfony --version",
			"P3-SYM-02": "fake: tool symfony cache:clear",
			"P3-SYM-03": "fake: tool symfony debug:router",
		},
		"wordpress": {
			"P3-WP-01": "fake: tool wp core version",
			"P3-WP-02": "fake: tool wp option get siteurl",
			"P3-WP-03": "fake: tool wp cache flush",
		},
	}

	for framework, expectations := range want {
		t.Run(framework, func(t *testing.T) {
			t.Setenv("GOVARD_HOME_DIR", t.TempDir())
			t.Setenv("GOVARD_VERIFY_FAKE", "1")

			res, err := verify.RunPhase(context.Background(), engine.Config{Framework: framework}, 3, verify.VerifyOpts{})
			if err != nil {
				t.Fatalf("phase 3: %v", err)
			}
			seen := map[string]bool{}
			for _, it := range res.Items {
				expected, ok := expectations[it.ID]
				if !ok {
					continue
				}
				seen[it.ID] = true
				if it.EvidenceExcerpt != expected {
					t.Errorf("%s ran %q, want %q", it.ID, it.EvidenceExcerpt, expected)
				}
			}
			for id := range expectations {
				if !seen[id] {
					t.Errorf("%s missing from phase 3", id)
				}
			}
		})
	}
}

// Phase 5 is where a typo costs most — a wrong arg is a write on a restored
// database — and plan mode never runs the item, so each one is pinned by a real
// gated run against the fake runner. Gate fixtures live in verify_gate_test.go.
func TestVerifyFrameworkPhase5ItemArgv(t *testing.T) {
	for _, tc := range []struct{ framework, id, excerpt string }{
		{"magento2", "P5-MAG-01", "fake: tool magento setup:db:status"},
		{"laravel", "P5-LAR-01", "fake: tool artisan migrate:status"},
		{"symfony", "P5-SYM-01", "fake: tool symfony cache:clear"},
		{"wordpress", "P5-WP-01", "fake: tool wp db check"},
	} {
		t.Run(tc.framework, func(t *testing.T) {
			t.Setenv("GOVARD_HOME_DIR", t.TempDir())
			t.Setenv("GOVARD_VERIFY_FAKE", "1")
			root := t.TempDir()
			makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
			writePhase4(t, root, goodPhase4(root, "20260101-000000"))

			res, err := verify.RunPhase(context.Background(), engine.Config{Framework: tc.framework}, 5,
				verify.VerifyOpts{AllowDestructive: true, ProjectRoot: root})
			if err != nil {
				t.Fatalf("phase 5: %v", err)
			}
			for _, it := range res.Items {
				if it.ID != tc.id {
					continue
				}
				if it.EvidenceExcerpt != tc.excerpt {
					t.Fatalf("%s ran %q, want %q", tc.id, it.EvidenceExcerpt, tc.excerpt)
				}
				return
			}
			t.Fatalf("%s missing from phase 5", tc.id)
		})
	}
}

// A composed item's failure path is pinned out of process, in
// tests/integration/verify_command_test.go: execGovard treats the running
// executable as a test binary whenever its path contains ".test", so every
// in-process item is stubbed to exit 0 and GOVARD_VERIFY_BIN is ignored here.
