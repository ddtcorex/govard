package tests

import (
	"testing"

	cmdpkg "govard/internal/cmd"
)

func TestCmdDesktopBinaryArgsIncludesBackgroundFlag(t *testing.T) {
	args := cmdpkg.DesktopBinaryArgsForTest(true)
	if len(args) != 1 || args[0] != "--background" {
		t.Fatalf("expected [--background], got %v", args)
	}
}

func TestCmdDesktopBinaryArgsEmptyWhenBackgroundDisabled(t *testing.T) {
	args := cmdpkg.DesktopBinaryArgsForTest(false)
	if len(args) != 0 {
		t.Fatalf("expected empty args, got %v", args)
	}
}
