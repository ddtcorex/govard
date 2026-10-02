package tests

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// `govard deploy check` leaves nothing behind on the target, but it is not
// read-only: the `mv -T` probe creates a `.dep` scratch directory (and, on a
// fresh host, the missing parents of the deploy path) and removes them again.
// A page that calls the command read-only or side-effect free promises more
// than the code does, and it is the wording a reader acts on, so no sentence
// that names the command may use it. Issue #505 tracks the same rule for the
// two sibling repositories.
//
// The scan reads every Markdown page of the docs (English and Vietnamese) and
// the README, one sentence at a time, so a page that says "the checklist is
// read-only" in one sentence and describes `deploy check` in the next is fine.
var deployCheckAbsolutePhrases = []string{
	"read-only", "read only", "readonly",
	"changes nothing", "change nothing", "creates nothing", "writes nothing",
	"no side effect", "side-effect-free", "side-effect free",
	// Vietnamese mirrors.
	"chỉ-đọc", "chỉ đọc", "không thay đổi gì", "không tạo gì", "không ghi gì",
}

var sentenceBoundary = regexp.MustCompile(`[.!?]\s+`)

// markdownUnits splits a page into units that cannot share a sentence: a
// paragraph, a table row, a list item, a heading or a code line each stand
// alone, with the soft line breaks inside a paragraph folded to spaces.
func markdownUnits(page string) []string {
	var units []string
	var current []string
	flush := func() {
		if len(current) > 0 {
			units = append(units, strings.Join(current, " "))
			current = nil
		}
	}
	for _, line := range strings.Split(page, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			flush()
		case strings.HasPrefix(trimmed, "|"), strings.HasPrefix(trimmed, "#"),
			strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, "- "),
			strings.HasPrefix(trimmed, "* "):
			flush()
			current = append(current, trimmed)
			if strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "```") {
				flush()
			}
		default:
			current = append(current, trimmed)
		}
	}
	flush()
	return units
}

// deployCheckOverclaims returns every sentence of page that names `deploy
// check` and also uses one of the absolute phrases.
func deployCheckOverclaims(page string) []string {
	var found []string
	for _, unit := range markdownUnits(page) {
		for _, sentence := range sentenceBoundary.Split(unit, -1) {
			lower := strings.ToLower(sentence)
			if !strings.Contains(lower, "deploy check") {
				continue
			}
			for _, phrase := range deployCheckAbsolutePhrases {
				if strings.Contains(lower, phrase) {
					found = append(found, strings.TrimSpace(sentence))
					break
				}
			}
		}
	}
	return found
}

func TestDeployCheckOverclaimDetector(t *testing.T) {
	for name, tc := range map[string]struct {
		page string
		want int
	}{
		"read-only claim":           {"`govard deploy check` is read-only.", 1},
		"changes nothing":           {"`govard deploy check` connects over ssh but changes nothing.", 1},
		"vietnamese":                {"`deploy check` chỉ đọc, không ghi gì.", 1},
		"folded paragraph":          {"The checklist stays\nread-only, so `deploy check`\nis left out.", 1},
		"split across sentences":    {"The checklist is read-only. `deploy check` leaves nothing behind.", 0},
		"table row is its own unit": {"| a | read-only |\n| `deploy check` | ssh |", 0},
		"the accurate wording":      {"`deploy check` leaves nothing behind on the target, local or remote.", 0},
	} {
		if got := len(deployCheckOverclaims(tc.page)); got != tc.want {
			t.Errorf("%s: %d overclaiming sentences, want %d", name, got, tc.want)
		}
	}
}

func TestDocsDoNotClaimDeployCheckIsReadOnly(t *testing.T) {
	root := ".."
	var pages []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "superpowers", ".superpowers":
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasSuffix(path, ".md") && (strings.HasPrefix(rel, "docs"+string(os.PathSeparator)) || rel == "README.md") {
			pages = append(pages, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
	if len(pages) < 20 {
		t.Fatalf("found only %d pages, the scan is not reading the docs", len(pages))
	}
	for _, rel := range pages {
		for _, sentence := range deployCheckOverclaims(readRepoFile(t, rel)) {
			t.Errorf("%s calls `deploy check` read-only or side-effect free: %q\ndescribe it by what it leaves behind (nothing on the target) and mention the `.dep` scratch directory its mv -T probe creates and removes", rel, sentence)
		}
	}
}
