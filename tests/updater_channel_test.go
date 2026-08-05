package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/updater"
)

func TestGetChannelDefaultsToStable(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	if got := updater.GetChannel(); got != updater.ChannelStable {
		t.Fatalf("GetChannel() = %q, want %q", got, updater.ChannelStable)
	}
}

func TestSetChannelPersistsAndRoundTrips(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	if err := updater.SetChannel("beta"); err != nil {
		t.Fatalf("SetChannel(beta) error = %v", err)
	}
	if got := updater.GetChannel(); got != updater.ChannelBeta {
		t.Fatalf("GetChannel() = %q, want %q", got, updater.ChannelBeta)
	}

	if err := updater.SetChannel("stable"); err != nil {
		t.Fatalf("SetChannel(stable) error = %v", err)
	}
	if got := updater.GetChannel(); got != updater.ChannelStable {
		t.Fatalf("GetChannel() = %q, want %q", got, updater.ChannelStable)
	}
}

func TestSetChannelRejectsInvalidValue(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	if err := updater.SetChannel("nightly"); err == nil {
		t.Fatal("SetChannel(nightly) expected error, got nil")
	}
}

func TestGetChannelIgnoresCorruptFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOVARD_HOME_DIR", home)

	if err := os.WriteFile(filepath.Join(home, "update-channel.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt channel file: %v", err)
	}

	if got := updater.GetChannel(); got != updater.ChannelStable {
		t.Fatalf("GetChannel() = %q, want %q for corrupt file", got, updater.ChannelStable)
	}
}
