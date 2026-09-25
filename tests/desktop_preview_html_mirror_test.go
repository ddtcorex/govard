package tests

import (
	"strings"
	"testing"
)

// preview.html is a hand copy of index.html plus the preview bootstrap. The
// behaviour tests run against preview.html, so an index.html edit that is not
// mirrored makes them test stale markup.
func appMarkup(t *testing.T, rel string) string {
	t.Helper()
	src := readRepoFile(t, rel)
	end := strings.Index(src, "<!-- End #app Root Container -->")
	if end < 0 {
		t.Fatalf("%s has no End #app Root Container marker", rel)
	}
	var kept []string
	for _, line := range strings.Split(src[:end], "\n") {
		if strings.Contains(line, "preview") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func TestPreviewHTMLMirrorsIndexHTML(t *testing.T) {
	index := appMarkup(t, "desktop/frontend/index.html")
	preview := appMarkup(t, "desktop/frontend/preview.html")
	if index == preview {
		return
	}
	a, b := strings.Split(index, "\n"), strings.Split(preview, "\n")
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			t.Fatalf("preview.html diverges from index.html at app line %d:\n index:   %q\n preview: %q", i+1, a[i], b[i])
		}
	}
	t.Fatalf("preview.html and index.html differ in length (%d vs %d app lines)", len(b), len(a))
}
