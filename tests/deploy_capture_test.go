package tests

import (
	"context"
	"os"
	"strings"
	"testing"

	"govard/internal/deploy"
)

// The engine keeps only part of a command's output, and the part it keeps is the
// tail. The bounded capture exists because a real project produced 170 MiB of
// captured stderr, which became a 170 MiB release record: the record is written to
// the target as a command, so the deploy failed to record its own outcome, and the
// failure message printed the whole payload back. These tests state the bound and
// state that the tail is what survives.

func TestCapturedOutputIsBoundedAndKeepsTheTail(t *testing.T) {
	// One mebibyte of output, then a marker line nothing else can produce.
	command := "yes abcdefghijklmnopqrstuvwxyz | head -c 1048576; echo THE-VERY-END"
	result, err := deploy.LocalRunner{}.Run(context.Background(), command, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	const limit = 256 * 1024
	if len(result.Stdout) > limit+1024 {
		t.Fatalf("captured %d bytes of a 1 MiB command; the capture must stay bounded", len(result.Stdout))
	}
	if len(result.Stdout) < limit/2 {
		t.Fatalf("captured only %d bytes; a bounded capture must still keep a useful tail", len(result.Stdout))
	}
	if !strings.Contains(result.Stdout, "dropped") {
		t.Fatalf("a truncated capture must say so, got %q", head(result.Stdout, 120))
	}
	if !strings.HasSuffix(result.Stdout, "THE-VERY-END\n") {
		t.Fatalf("the tail must survive: a failure's cause is at the end of its output, got %q", tail(result.Stdout, 80))
	}
}

func TestCapturedStderrIsBoundedToo(t *testing.T) {
	// The failure this bounds came through stderr, which is the stream that ends
	// up inside the error message and therefore inside the record.
	command := "yes stderr-noise | head -c 1048576 >&2; echo THE-LAST-WORD >&2"
	result, err := deploy.LocalRunner{}.Run(context.Background(), command, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Stderr) > 257*1024 {
		t.Fatalf("captured %d bytes of stderr; the capture must stay bounded", len(result.Stderr))
	}
	if !strings.HasSuffix(result.Stderr, "THE-LAST-WORD\n") {
		t.Fatalf("the end of stderr is what explains a failure, got %q", tail(result.Stderr, 80))
	}
}

func TestCapturedOutputIsExactWhenItFits(t *testing.T) {
	// The bound must never touch a command whose output fits: every parser in the
	// engine reads this copy, so a marker or a truncation would be a silent
	// corruption of perfectly ordinary output.
	command := `printf '{"ok":true}\n'`
	result, err := deploy.LocalRunner{}.Run(context.Background(), command, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Stdout != "{\"ok\":true}\n" {
		t.Fatalf("stdout = %q, want the command's own bytes", result.Stdout)
	}
	if strings.Contains(result.Stdout, "dropped") {
		t.Fatal("an untruncated capture must not carry a truncation marker")
	}
}

func TestALargeReleaseRecordIsWrittenWithoutTouchingArgv(t *testing.T) {
	// The record is written to the target by a command. A payload in the command
	// text is limited by the kernel's per-argument limit (128 KiB on Linux), and a
	// chatty deploy exceeded it — the record could not be written, so `status`,
	// `--resume` and `rollback` lost the release. The payload travels on standard
	// input instead.
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})

	record := deploy.NewReleaseForTest("9", "deadbeef", "main")
	record.Status = deploy.StatusFailed
	// Just past the kernel's 128 KiB per-argument limit, and inside the capture
	// bound, so the record is one the engine can both write and read back.
	record.Tasks = []deploy.StepRecord{{
		ID:     "build:assets",
		Status: deploy.StatusFailed,
		Error:  strings.Repeat("a noisy remote ", 13000),
	}}
	if err := deploy.WriteRelease(context.Background(), host, record); err != nil {
		t.Fatalf("write a large release record: %v", err)
	}

	info, err := os.Stat(host.ReleaseRecordPath("9"))
	if err != nil {
		t.Fatalf("stat record: %v", err)
	}
	if info.Size() < 128*1024 {
		t.Fatalf("record is %d bytes, want the whole payload written", info.Size())
	}
	read, err := deploy.ReadRelease(context.Background(), host, "9")
	if err != nil {
		t.Fatalf("read the large record back: %v", err)
	}
	if len(read.Tasks) != 1 || len(read.Tasks[0].Error) != len(record.Tasks[0].Error) {
		t.Fatalf("the record did not round trip: %d tasks, error of %d bytes",
			len(read.Tasks), len(read.Tasks[0].Error))
	}
}

func TestALargeHistoryEntryIsWrittenToo(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	record := deploy.NewReleaseForTest("1", "cafebabe", "main")
	record.Tasks = []deploy.StepRecord{{
		ID:     "build:vendors",
		Status: deploy.StatusFailed,
		Error:  strings.Repeat("composer noise ", 13000),
	}}
	if err := deploy.AppendHistory(context.Background(), host, record); err != nil {
		t.Fatalf("append a large history entry: %v", err)
	}
	content, err := os.ReadFile(host.DepPath() + "/history.jsonl")
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(content) < 128*1024 {
		t.Fatalf("history entry is %d bytes, want the whole payload", len(content))
	}
	if !strings.Contains(string(content), `"cafebabe"`) {
		t.Fatal("the history entry lost its revision")
	}
}

func TestCommandErrorCarriesTheBoundedOutput(t *testing.T) {
	// The error message embeds the captured stderr and ends up in the record, so
	// bounding the capture is what bounds the error too.
	command := "yes boom | head -c 524288 >&2; echo THE-REAL-CAUSE >&2; exit 3"
	_, err := deploy.LocalRunner{}.Run(context.Background(), command, deploy.RunOptions{})
	if err == nil {
		t.Fatal("want an error for a failing command")
	}
	if len(err.Error()) > 300*1024 {
		t.Fatalf("error message is %d bytes; the captured output must be bounded", len(err.Error()))
	}
	if !strings.Contains(err.Error(), "THE-REAL-CAUSE") {
		t.Fatalf("the error must keep the end of stderr, got %q", tail(err.Error(), 120))
	}
}

func head(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[:n]
}

func tail(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[len(value)-n:]
}
