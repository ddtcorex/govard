//go:build !windows

package desktop

import "syscall"

// diskFreeBytes reports the free bytes available on the filesystem holding
// path. The desktop dump preflight refuses before writing a byte when the
// estimate does not fit.
func diskFreeBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
