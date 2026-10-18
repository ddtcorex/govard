package tests

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"govard/internal/desktop"
)

// The engine wraps the stderr writer (quietStderr) while stdout keeps the
// value we pass, so os/exec sees two distinct writers and runs two copier
// goroutines on them. The delete buffer must survive concurrent writes from
// both — run with -race.
func TestDesktopPkgLockedDeleteBufferConcurrentWrites(t *testing.T) {
	buf, read := desktop.NewLockedDeleteBufferForTest(64 * 1024)
	// Two interface values over the same buffer: the plain one and a
	// wrapper, the way a stderr wrapper changes what exec.Compare sees.
	writers := []io.Writer{buf, struct{ io.Writer }{buf}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := writers[i%2]
			for j := 0; j < 200; j++ {
				fmt.Fprintf(w, "Warning: worker %d line %d\n", i, j)
			}
		}(i)
	}
	wg.Wait()
	if out := read(); !strings.Contains(out, "Warning: worker") {
		t.Fatalf("expected interleaved warnings, got %q", out[:min(120, len(out))])
	}
}

// Delete output is fully buffered now, so it stays capped: the tail (where
// the cause lives) survives, the head is dropped.
func TestDesktopPkgLockedDeleteBufferKeepsTail(t *testing.T) {
	buf, read := desktop.NewLockedDeleteBufferForTest(1024)
	if _, err := buf.Write([]byte("start-" + strings.Repeat("x", 2000))); err != nil {
		t.Fatal(err)
	}
	if _, err := buf.Write([]byte("-end")); err != nil {
		t.Fatal(err)
	}
	out := read()
	if len(out) > 1024 {
		t.Fatalf("buffer exceeded its bound: %d bytes", len(out))
	}
	if !strings.HasSuffix(out, "-end") {
		t.Fatalf("expected the tail to survive, got suffix %q", out[max(0, len(out)-20):])
	}
	if strings.Contains(out, "start-") {
		t.Fatalf("expected the head to be dropped, got prefix %q", out[:min(40, len(out))])
	}
}
