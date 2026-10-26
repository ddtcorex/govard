//go:build windows

package engine

import "errors"

// FileOwnerUID has no uid model on Windows; callers treat the error as
// "ownership unknown" and skip uid-based checks.
func FileOwnerUID(string) (int, error) {
	return 0, errors.New("file ownership by uid is not available on windows")
}

// FileOwnership has no uid model on Windows.
func FileOwnership(string) (int, int, error) {
	return 0, 0, errors.New("file ownership by uid is not available on windows")
}
