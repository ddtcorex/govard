package try

// Test wrappers, per the repository test convention: behaviour that lives
// unexported in a package but must be exercised from tests/ is exposed through a
// small ForTest function next to the code it wraps, never by moving the test
// into the package.

// TryETALineForTest exposes the wait estimate printed before a trial starts.
func TryETALineForTest(sample bool) string {
	return tryETALine(sample)
}

// TryHostsHintForTest exposes the hosts-file command printed when the trial
// domain does not resolve.
func TryHostsHintForTest(domain string) string {
	return tryHostsHint(domain)
}

// NearestExistingDirForTest exposes the ancestor walk used by the disk
// preflight.
func NearestExistingDirForTest(path string) (string, error) {
	return nearestExistingDir(path)
}
