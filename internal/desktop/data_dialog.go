package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FilePick is one native-dialog file choice handed back to the data tab: the
// dialog-picked path plus whether it already exists, so the island can show
// its overwrite-confirm before calling dump or export. The island never
// invents paths: every destination or source it sends backend-side came from
// one of these picks (controller binding decisions 1-2).
type FilePick struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// PickDumpDestination opens the native save dialog for a database dump and
// returns the picked path. An empty pick with no error means the user
// cancelled, like PickProjectDirectory.
func (s *DBService) PickDumpDestination(project string) (res FilePick, err error) {
	defer RecoverPanic(&err, "PickDumpDestination")
	return pickSaveDestination(s.platform, SaveFileOptions{
		Title:           "Save Database Dump",
		DefaultDir:      resolveLogExportDefaultDirectory(),
		DefaultFilename: dumpDefaultFilename(project),
	})
}

// PickImportSource opens the native open-file dialog for a database dump to
// import. An empty path with no error means the user cancelled.
func (s *DBService) PickImportSource() (res string, err error) {
	defer RecoverPanic(&err, "PickImportSource")
	path, err := chooseOpenFileForDesktop(s.platform, "Select Database Dump", resolveLogExportDefaultDirectory())
	if err != nil {
		return "", fmt.Errorf("open file dialog: %w", err)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	return filepath.Clean(path), nil
}

// GetDBWriteConfirmToken returns the per-project token the data tab confirm
// dialog passes back to RunDBQuery for a write statement. It guards against
// accidental clicks, not against malice: the desktop already holds the
// credentials, and the format stays Go-owned so the island never assembles
// the token itself.
func (s *DBService) GetDBWriteConfirmToken(project string) (res string, err error) {
	defer RecoverPanic(&err, "GetDBWriteConfirmToken")
	if normalizeDBProject(project) == "" {
		return "", fmt.Errorf("project is required")
	}
	return dbWriteConfirmToken(project), nil
}

// PickExportDestination opens the native save dialog for a snapshot export
// and returns the picked path. An empty pick with no error means the user
// cancelled.
func (s *SnapshotService) PickExportDestination(name string) (res FilePick, err error) {
	defer RecoverPanic(&err, "PickExportDestination")
	cleanName, err := validateSnapshotName(name)
	if err != nil {
		return FilePick{}, err
	}
	return pickSaveDestination(s.platform, SaveFileOptions{
		Title:           "Export Snapshot",
		DefaultDir:      resolveLogExportDefaultDirectory(),
		DefaultFilename: snapshotExportDefaultFilename(cleanName),
	})
}

// pickSaveDestination runs one save dialog and reports whether the picked
// path already holds a file. Only a real file counts: a directory destination
// is left for the dump/export call to refuse, like before.
func pickSaveDestination(p Platform, opts SaveFileOptions) (FilePick, error) {
	path, err := chooseSaveFileForDesktop(p, opts)
	if err != nil {
		return FilePick{}, fmt.Errorf("open save dialog: %w", err)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return FilePick{}, nil
	}
	clean := filepath.Clean(path)
	exists := false
	if info, err := os.Stat(clean); err == nil && !info.IsDir() {
		exists = true
	}
	return FilePick{Path: clean, Exists: exists}, nil
}

var dataDialogFilenameSanitizePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sanitizeDataDialogFilename keeps a dialog default filename to safe
// characters, falling back when nothing usable remains.
func sanitizeDataDialogFilename(name string, fallback string) string {
	cleaned := dataDialogFilenameSanitizePattern.ReplaceAllString(
		filepath.Base(strings.TrimSpace(name)), "-",
	)
	cleaned = strings.Trim(cleaned, "-.")
	if cleaned == "" {
		return fallback
	}
	return cleaned
}

// dumpDefaultFilename suggests a dump name for a project. The user can rename
// it in the dialog; the default only has to be predictable.
func dumpDefaultFilename(project string) string {
	return sanitizeDataDialogFilename(strings.TrimSpace(project), "database") + "-dump.sql"
}

// snapshotExportDefaultFilename suggests an export name for a snapshot,
// matching the CLI's <name>.tar.gz default.
func snapshotExportDefaultFilename(name string) string {
	base := sanitizeDataDialogFilename(name, "snapshot")
	if strings.HasSuffix(strings.ToLower(base), ".tar.gz") {
		return base
	}
	return base + ".tar.gz"
}

var defaultChooseOpenFileForDesktop = func(p Platform, title, defaultDir string) (string, error) {
	return p.ChooseOpenFile(title, defaultDir)
}

var chooseOpenFileForDesktop = defaultChooseOpenFileForDesktop
