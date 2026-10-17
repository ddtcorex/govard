//go:build windows

package desktop

import "golang.org/x/sys/windows"

// diskFreeBytes reports the free bytes available on the filesystem holding
// path. The desktop dump preflight refuses before writing a byte when the
// estimate does not fit.
func diskFreeBytes(path string) (uint64, error) {
	wpath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(wpath, &free, nil, nil); err != nil {
		return 0, err
	}
	return free, nil
}
