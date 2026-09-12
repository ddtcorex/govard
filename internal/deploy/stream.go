package deploy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// linePrefix is what a running command's own output is indented with, so raw lines
// read as belonging to the task above them rather than as timeline entries.
const linePrefix = "  │ "

// heartbeatInterval is how often a running step says it is still running. Ten
// seconds is short enough to tell a slow step from a hung one and long enough not to
// bury the timeline.
const heartbeatInterval = 10 * time.Second

// heartbeatEvery is heartbeatInterval, replaceable in tests: asserting a heartbeat
// with the real interval would cost ten seconds per test.
var heartbeatEvery = heartbeatInterval

// SetHeartbeatForTest shortens the heartbeat for the duration of a test and returns
// the function that restores it.
func SetHeartbeatForTest(interval time.Duration) func() {
	previous := heartbeatEvery
	heartbeatEvery = interval
	return func() { heartbeatEvery = previous }
}

// streamTo writes a command's stream to the bounded capture the Result carries
// and, when a live writer was requested, to the operator watching it. A nil live
// writer is the captured behaviour exactly, which is what every caller that does
// not ask for streaming keeps.
//
// The bound is on the capture only: the operator watching a stream sees all of
// it, because that is what asking to watch a running command means.
func streamTo(capture *boundedBuffer, live io.Writer) io.Writer {
	if live == nil {
		return capture
	}
	return io.MultiWriter(capture, live)
}

// liveWriter is what a running command's output goes through when the operator asked
// to watch it: every line is nested under the task it belongs to, and the heartbeat
// writes through the same lock so it can never land inside a command's half-written
// line.
type liveWriter struct {
	out    io.Writer
	prefix string

	mu sync.Mutex
	// atLineStart is true between lines, where the prefix belongs. A command that
	// prints without a trailing newline keeps writing the same line.
	atLineStart bool
}

func newLiveWriter(out io.Writer, prefix string) *liveWriter {
	return &liveWriter{out: out, prefix: prefix, atLineStart: true}
}

// Write implements io.Writer. It returns the number of input bytes it consumed, as
// the contract requires, not the number it wrote.
func (w *liveWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	written := 0
	for len(p) > 0 {
		if w.atLineStart {
			if _, err := io.WriteString(w.out, w.prefix); err != nil {
				return written, err
			}
			w.atLineStart = false
		}
		chunk := p
		if index := bytes.IndexByte(p, '\n'); index >= 0 {
			chunk = p[:index+1]
		}
		if _, err := w.out.Write(chunk); err != nil {
			return written, err
		}
		written += len(chunk)
		if chunk[len(chunk)-1] == '\n' {
			w.atLineStart = true
		}
		p = p[len(chunk):]
	}
	return written, nil
}

// WriteLine writes one whole line that belongs to the step rather than to the
// command (the heartbeat), taking the same lock for the same reason.
func (w *liveWriter) WriteLine(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_, _ = io.WriteString(w.out, line)
	w.atLineStart = true
}

// isTerminal reports whether a writer is a terminal. It is the thin OS-level half of
// the progress decision: the branching lives in rsyncProgressArgs, which takes the
// answer as a value and is therefore testable without a terminal.
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// SetTerminalForTest exposes the terminal probe. It exists so a test can assert what
// the step context records without pretending a buffer is a terminal.
func SetTerminalForTest(w io.Writer) bool {
	return isTerminal(w)
}

// rsyncProgressArgs returns the flags that make rsync report progress while it runs.
//
// Two conditions, both necessary: without a terminal rsync cannot redraw its progress
// line, so every update becomes another line in a log (which is why CI and a
// redirected run never see it), and without `--verbose` the output is not streamed
// anywhere to be watched in.
func rsyncProgressArgs(streaming, terminal bool) []string {
	if streaming && terminal {
		return []string{"--info=progress2"}
	}
	return nil
}

// progressArgs is rsyncProgressArgs for one step: the run decides whether it is
// streaming, the terminal is recorded on the step context when the run starts.
func progressArgs(sc *StepContext) []string {
	return rsyncProgressArgs(sc.Opts.Verbose && !sc.Opts.JSON, sc.Terminal)
}

// startHeartbeat reports that a step is still running until the returned stop
// function is called.
//
// It covers every step, not only the chatty ones: a silent `composer install` and a
// stalled rsync look identical from the timeline until one of them finishes, and the
// whole point is that an operator can tell the difference without waiting for the
// timeout.
func startHeartbeat(step Step, writer *liveWriter) func() {
	if heartbeatEvery <= 0 || writer == nil {
		return func() {}
	}
	started := time.Now()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				writer.WriteLine(fmt.Sprintf("  … %s still running (%s)", step.ID, time.Since(started).Round(time.Second)))
			}
		}
	}()

	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}
