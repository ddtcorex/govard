package tests

import (
	"testing"

	"govard/internal/cmd"
	"govard/internal/updater"
)

func TestVersionChannelNoticeForTestStableIsSilent(t *testing.T) {
	if got := cmd.VersionChannelNoticeForTest(updater.ChannelStable); got != "" {
		t.Fatalf("VersionChannelNoticeForTest(stable) = %q, want empty", got)
	}
}

func TestVersionChannelNoticeForTestBetaAnnouncesChannel(t *testing.T) {
	got := cmd.VersionChannelNoticeForTest(updater.ChannelBeta)
	want := "Update channel: beta"
	if got != want {
		t.Fatalf("VersionChannelNoticeForTest(beta) = %q, want %q", got, want)
	}
}
