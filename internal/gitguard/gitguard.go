// Package gitguard keeps directories of generated or sensitive material out of
// version control. It is a leaf package so both the engine and the deploy
// packages can use it without an import cycle.
package gitguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// guardContent ignores everything in the directory, the guard file included,
// so the directory stages nothing whatever the project's own rules say.
const guardContent = "*\n"

// EnsureDir creates dir (with perm) and makes sure a `.gitignore` inside it
// ignores everything. These directories belong to govard (snapshots,
// diagnostics, the sandbox state) and hold material that must never be staged,
// so the guard fails closed:
//
//   - no `.gitignore` yet: one containing `*` is created;
//   - a regular `.gitignore` that already ends in an ignore-all rule is left as
//     it is;
//   - a regular `.gitignore` that does not (partial rules, or a trailing `!*`)
//     keeps its content and gets `*` appended, because trusting it would leave
//     the directory stageable;
//   - a `.gitignore` that is a symlink or not a regular file is refused: it
//     could point anywhere, and nothing is written through it.
//
// A directory that is itself a symlink is refused too, since the content and
// the guard would land wherever it points.
func EnsureDir(dir string, perm os.FileMode) error {
	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("create the directory %s: %w", dir, err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect the directory %s: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to use %s: the directory is a symlink", dir)
	}
	guard := filepath.Join(dir, ".gitignore")
	existing, err := os.Lstat(guard)
	switch {
	case err == nil:
		if !existing.Mode().IsRegular() {
			return fmt.Errorf("refusing to trust %s: it is not a regular file (symlink or special file)", guard)
		}
		return ensureIgnoreAll(guard)
	case os.IsNotExist(err):
		return createGuard(guard)
	default:
		return fmt.Errorf("inspect %s: %w", guard, err)
	}
}

func createGuard(guard string) error {
	file, err := os.OpenFile(guard, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			// Created between the check and now: judge what is there.
			return ensureIgnoreAll(guard)
		}
		return fmt.Errorf("write %s: %w", guard, err)
	}
	if _, err := file.WriteString(guardContent); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", guard, err)
	}
	return file.Close()
}

// ensureIgnoreAll appends the ignore-all rule to a regular file that does not
// already end in one.
func ensureIgnoreAll(guard string) error {
	data, err := os.ReadFile(guard)
	if err != nil {
		return fmt.Errorf("read %s: %w", guard, err)
	}
	if endsIgnoringEverything(string(data)) {
		return nil
	}
	file, err := os.OpenFile(guard, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("update %s: %w", guard, err)
	}
	addition := guardContent
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		addition = "\n" + addition
	}
	if _, err := file.WriteString(addition); err != nil {
		_ = file.Close()
		return fmt.Errorf("update %s: %w", guard, err)
	}
	return file.Close()
}

// endsIgnoringEverything reports whether the last rule that matches every entry
// is an ignore, not an un-ignore (`!*`).
func endsIgnoringEverything(content string) bool {
	ignoring := false
	for _, line := range strings.Split(content, "\n") {
		switch strings.TrimSpace(line) {
		case "*", "/*":
			ignoring = true
		case "!*", "!/*":
			ignoring = false
		}
	}
	return ignoring
}
