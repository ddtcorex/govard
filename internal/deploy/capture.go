package deploy

import "fmt"

// captureLimit is how much of one command's own output the engine keeps.
//
// The captured copy is not a log: it exists to put the *reason* a command failed
// into the error message and the release record. Unbounded, it is a liability,
// because some real commands are enormous — a storefront's static content deploy
// failing inside a theme loop printed the same exception 27,333 times, and a
// 170 MiB captured stderr became a 170 MiB release record. The record is written
// to the target as a shell command, so the deploy then failed to record its own
// outcome ("argument list too long"), lost the record `status`, `--resume` and
// `rollback` read, and printed the whole payload back at the operator.
//
// The bound is on what is kept, never on what is shown: `--verbose` still streams
// every byte as it arrives.
const captureLimit = 256 * 1024

// boundedBuffer is the bounded capture behind Result. It keeps the last
// captureLimit bytes — the tail, because a failure's cause is at the end of its
// output — and counts what it dropped.
type boundedBuffer struct {
	limit   int
	buf     []byte
	dropped int
}

func newBoundedBuffer(limit int) *boundedBuffer {
	if limit <= 0 {
		limit = captureLimit
	}
	return &boundedBuffer{limit: limit, buf: make([]byte, 0, limit)}
}

// Write implements io.Writer, and always reports the input as consumed: the
// command must not be stopped or slowed because the engine decided to keep only
// part of its output.
func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	if n >= b.limit {
		// One write larger than the whole budget: the tail of *this* write is
		// what survives, and nothing that came before it does.
		b.dropped += len(b.buf) + n - b.limit
		b.buf = append(b.buf[:0], p[n-b.limit:]...)
		return n, nil
	}
	if overflow := len(b.buf) + n - b.limit; overflow > 0 {
		// Drop the oldest bytes, not the newest: the newest are the ones that
		// explain the failure.
		copy(b.buf, b.buf[overflow:])
		b.buf = b.buf[:len(b.buf)-overflow]
		b.dropped += overflow
	}
	b.buf = append(b.buf, p...)
	return n, nil
}

// String returns the kept tail, introduced by a marker that says how much was
// dropped. The marker only appears when something was, so a command whose output
// fits is reported exactly as it wrote it.
func (b *boundedBuffer) String() string {
	if b.dropped == 0 {
		return string(b.buf)
	}
	return fmt.Sprintf("… %s dropped, keeping the last %s …\n%s",
		humanBytes(b.dropped), humanBytes(len(b.buf)), b.buf)
}

// Len is the number of kept bytes.
func (b *boundedBuffer) Len() int { return len(b.buf) }

// humanBytes formats a byte count for a message a human reads.
func humanBytes(count int) string {
	switch {
	case count >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(count)/(1<<20))
	case count >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(count)/(1<<10))
	default:
		return fmt.Sprintf("%d B", count)
	}
}
