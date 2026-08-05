package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
)

// Update channels recognized by Govard's self-update tooling.
const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"

	updateChannelFileName = "update-channel.json"
)

type updateChannelState struct {
	Channel string `json:"channel"`
}

// GetChannel returns the persisted update channel for this machine, defaulting
// to ChannelStable when unset, unreadable, or invalid. Shared by the CLI and
// Govard Desktop so both stay on the same channel.
func GetChannel() string {
	data, err := os.ReadFile(channelFilePath())
	if err != nil {
		return ChannelStable
	}

	var state updateChannelState
	if err := json.Unmarshal(data, &state); err != nil {
		return ChannelStable
	}
	if state.Channel == ChannelBeta {
		return ChannelBeta
	}
	return ChannelStable
}

// SetChannel validates and persists the update channel.
func SetChannel(channel string) error {
	normalized := strings.ToLower(strings.TrimSpace(channel))
	if normalized != ChannelStable && normalized != ChannelBeta {
		return fmt.Errorf("invalid update channel %q: must be %q or %q", channel, ChannelStable, ChannelBeta)
	}

	path := channelFilePath()
	if err := os.MkdirAll(filepath.Dir(path), conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create govard home directory: %w", err)
	}

	data, err := json.Marshal(updateChannelState{Channel: normalized})
	if err != nil {
		return fmt.Errorf("encode update channel: %w", err)
	}
	if err := os.WriteFile(path, data, conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write update channel file: %w", err)
	}
	return nil
}

func channelFilePath() string {
	return filepath.Join(conventions.GetGovardHome(), updateChannelFileName)
}
