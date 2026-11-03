//go:build windows

package try

import "math"

// freeBytes cannot measure free space portably on Windows in this batch.
// Windows is not supported (see docs/more/windows.md), so the preflight is a
// no-op there and the trial fails later, on its own terms.
func freeBytes(string) (uint64, error) {
	return math.MaxUint64, nil
}
