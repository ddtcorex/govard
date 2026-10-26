//go:build !windows

package engine

import (
	"fmt"
	"os"
	"syscall"
)

// FileOwnerUID returns the uid owning path (the link itself for a symlink).
func FileOwnerUID(path string) (int, error) {
	uid, _, err := FileOwnership(path)
	return uid, err
}

// FileOwnership returns the uid and gid owning path, never following a
// symlink.
func FileOwnership(path string) (int, int, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("no ownership information for %s", path)
	}
	return int(stat.Uid), int(stat.Gid), nil
}
