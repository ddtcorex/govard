package desktop

import (
	"fmt"
	"govard/internal/conventions"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultLogExportFilename = "govard-logs.log"

var logFilenameSanitizePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

var defaultChooseSaveFileForDesktop = func(p Platform, opts SaveFileOptions) (string, error) {
	return p.ChooseSaveFile(opts)
}

var chooseSaveFileForDesktop = defaultChooseSaveFileForDesktop

var defaultWriteLogFileForDesktop = func(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

var writeLogFileForDesktop = defaultWriteLogFileForDesktop

// LogService.SaveLogsToFile writes the frontend's log buffer through the
// platform save dialog. The free function below keeps its own signature so the
// contract stays testable without a service.
func (s *LogService) SaveLogsToFile(content string, suggestedName string) (res string, err error) {
	defer RecoverPanic(&err, "SaveLogsToFile")
	return saveLogsToFile(s.platform, content, suggestedName)
}

func saveLogsToFile(p Platform, content string, suggestedName string) (string, error) {
	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" {
		return "", fmt.Errorf("no logs content to save")
	}

	savePath, err := chooseSaveFileForDesktop(p, SaveFileOptions{
		Title:           "Save Logs",
		DefaultDir:      resolveLogExportDefaultDirectory(),
		DefaultFilename: sanitizeLogExportFilename(suggestedName),
	})
	if err != nil {
		return "", fmt.Errorf("open save dialog: %w", err)
	}

	savePath = strings.TrimSpace(savePath)
	if savePath == "" {
		return "Log export cancelled.", nil
	}

	cleanPath := filepath.Clean(savePath)
	output := content
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	if err := writeLogFileForDesktop(cleanPath, []byte(output), conventions.DefaultFilePerm); err != nil {
		return "", fmt.Errorf("write logs file: %w", err)
	}

	return fmt.Sprintf("Logs saved to %s", cleanPath), nil
}

func sanitizeLogExportFilename(name string) string {
	baseName := filepath.Base(strings.TrimSpace(name))
	if baseName == "." || baseName == string(filepath.Separator) || baseName == "" {
		return defaultLogExportFilename
	}

	sanitized := logFilenameSanitizePattern.ReplaceAllString(baseName, "-")
	sanitized = strings.Trim(sanitized, "-.")
	if sanitized == "" {
		sanitized = defaultLogExportFilename
	}

	if filepath.Ext(sanitized) == "" {
		sanitized += ".log"
	}

	return sanitized
}

func resolveLogExportDefaultDirectory() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	downloadsDir := filepath.Join(homeDir, "Downloads")
	if info, statErr := os.Stat(downloadsDir); statErr == nil && info.IsDir() {
		return downloadsDir
	}

	if info, statErr := os.Stat(homeDir); statErr == nil && info.IsDir() {
		return homeDir
	}

	return ""
}
