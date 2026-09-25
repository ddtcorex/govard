package tests

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"govard/internal/deploy"
)

// stallingTail is an import that stops reading near the end of the stream, as
// mysql does while it executes one large INSERT: by then the dump process has
// written everything and exited, and only the last pipe buffer is in flight.
type stallingTail struct {
	total, n int
	stall    time.Duration
	stalled  bool
}

func (w *stallingTail) Write(p []byte) (int, error) {
	w.n += len(p)
	if !w.stalled && w.n > w.total-40000 {
		w.stalled = true
		time.Sleep(w.stall)
	}
	return len(p), nil
}

// A streamed stdout must be delivered to the end however slowly the consumer
// reads it. The seed lost the tail of a Magento dump this way on a real
// rehearsal: `exec: WaitDelay expired before I/O complete`, reported as a timed
// out command, and the import failed on a statement cut in half.
func TestSandboxStreamDeliversTheWholeOutputToASlowConsumer(t *testing.T) {
	const total = 4000000
	sink := &stallingTail{total: total, stall: 1500 * time.Millisecond}
	run := deploy.NewSandboxCommandRunnerForTest("head")
	if _, err := run(context.Background(), deploy.SandboxCommand{
		Args:   []string{"-c", "4000000", "/dev/zero"},
		Stdout: sink,
	}); err != nil {
		t.Fatalf("a stream to a slow consumer failed: %v", err)
	}
	if sink.n != total {
		t.Fatalf("delivered %d of %d bytes", sink.n, total)
	}
}

// Delivering the whole stream must not cost the ability to stop: a cancelled
// run still ends promptly with the process killed.
func TestSandboxStreamStillStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	run := deploy.NewSandboxCommandRunnerForTest("sleep")
	start := time.Now()
	_, err := run(ctx, deploy.SandboxCommand{Args: []string{"30"}, Stdout: &stallingTail{total: 1 << 30}})
	if err == nil {
		t.Fatal("a cancelled stream must fail")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("a cancelled stream took %s to stop", elapsed)
	}
}

// A consumer that gives up closes its end with its error, and the stream must
// report that rather than wait: the seed's import closes the pipe this way when
// the client exits.
func TestSandboxStreamReportsAConsumerThatGaveUp(t *testing.T) {
	reader, writer := io.Pipe()
	_ = reader.CloseWithError(errors.New("import gave up"))
	run := deploy.NewSandboxCommandRunnerForTest("head")
	done := make(chan error, 1)
	go func() {
		_, err := run(context.Background(), deploy.SandboxCommand{Args: []string{"-c", "4000000", "/dev/zero"}, Stdout: writer})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a stream nobody read to the end must fail")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the stream hung on a consumer that had closed its end")
	}
}
