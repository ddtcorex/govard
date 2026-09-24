package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
)

func TestDesktopSaveLogsToFileWritesSelectedPathForTest(t *testing.T) {
	desktop.ResetStateForTest()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "captured.log")
	var capturedTitle string
	var capturedDefaultFilename string

	restoreChooser := desktop.SetChooseSaveFileForDesktopForTest(
		func(_ desktop.Platform, opts desktop.SaveFileOptions) (string, error) {
			capturedTitle = opts.Title
			capturedDefaultFilename = opts.DefaultFilename
			return outputPath, nil
		},
	)
	defer restoreChooser()

	app := desktop.NewApp()
	message, err := app.Logs.SaveLogsToFile("line one", "../My logs?.log")
	if err != nil {
		t.Fatalf("SaveLogsToFile failed: %v", err)
	}

	if !strings.Contains(message, outputPath) {
		t.Fatalf("expected output path in message, got %q", message)
	}

	if capturedTitle != "Save Logs" {
		t.Fatalf("expected save dialog title, got %q", capturedTitle)
	}

	if strings.TrimSpace(capturedDefaultFilename) == "" {
		t.Fatalf("expected default filename to be provided")
	}
	if strings.Contains(capturedDefaultFilename, "/") || strings.Contains(capturedDefaultFilename, "\\") {
		t.Fatalf("expected sanitized default filename, got %q", capturedDefaultFilename)
	}
	if !strings.HasSuffix(strings.ToLower(capturedDefaultFilename), ".log") {
		t.Fatalf("expected .log default filename, got %q", capturedDefaultFilename)
	}

	data, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("read exported logs: %v", readErr)
	}
	if string(data) != "line one\n" {
		t.Fatalf("unexpected exported content: %q", string(data))
	}
}

func TestDesktopSaveLogsToFileCancelReturnsFriendlyMessageForTest(t *testing.T) {
	desktop.ResetStateForTest()

	restoreChooser := desktop.SetChooseSaveFileForDesktopForTest(
		func(_ desktop.Platform, _ desktop.SaveFileOptions) (string, error) {
			return "", nil
		},
	)
	defer restoreChooser()

	app := desktop.NewApp()
	message, err := app.Logs.SaveLogsToFile("line one", "env.log")
	if err != nil {
		t.Fatalf("expected cancel to be non-error, got %v", err)
	}

	if !strings.Contains(strings.ToLower(message), "cancelled") {
		t.Fatalf("expected cancelled message, got %q", message)
	}
}

func TestDesktopSaveLogsToFileRejectsEmptyContentForTest(t *testing.T) {
	desktop.ResetStateForTest()

	app := desktop.NewApp()
	_, err := app.Logs.SaveLogsToFile(" \n\t", "empty.log")
	if err == nil {
		t.Fatalf("expected empty content to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "no logs content") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDesktopSaveLogsToFilePropagatesWriteFailureForTest(t *testing.T) {
	desktop.ResetStateForTest()

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "captured.log")

	restoreChooser := desktop.SetChooseSaveFileForDesktopForTest(
		func(_ desktop.Platform, _ desktop.SaveFileOptions) (string, error) {
			return outputPath, nil
		},
	)
	defer restoreChooser()

	restoreWrite := desktop.SetWriteLogFileForDesktopForTest(
		func(_ string, _ []byte, _ os.FileMode) error {
			return fmt.Errorf("disk full")
		},
	)
	defer restoreWrite()

	app := desktop.NewApp()
	_, err := app.Logs.SaveLogsToFile("line one", "env.log")
	if err == nil {
		t.Fatalf("expected write error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "write logs file") {
		t.Fatalf("unexpected write error: %v", err)
	}
}
