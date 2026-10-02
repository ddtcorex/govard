package deploy

import "fmt"

// SandboxDBVolumeLabel marks the named volume that keeps a sandbox's database
// across `sandbox down`. Its value is the container name (the project slug plus
// a hash of the checkout path), so `down --purge` finds this checkout's volume
// without knowing its name and never another checkout's. The series the data directory was
// written by is stamped under sandboxDBLabel, the same key the container uses.
const SandboxDBVolumeLabel = "govard.sandbox.db-volume"

// SandboxDBDataDir is where the database server keeps its data in the image.
const SandboxDBDataDir = "/var/lib/mysql"

// SandboxDBVolumeName is the volume that holds one sandbox's database, named
// after its container so a volume has exactly one owner: two checkouts of the
// same project (or two names the slug folds together) each get their own, and
// two servers never open one data directory. It does not depend on the database
// series: a changed series is refused by CheckSandboxDBVolumeSeries rather than
// silently getting a second volume.
func SandboxDBVolumeName(container string) string {
	return container + "-db"
}

// CheckSandboxDBVolumeSeries refuses to open a data directory written by one
// database series with another. An empty series means the base distribution's
// own server, which is a series of its own for this purpose.
func CheckSandboxDBVolumeSeries(volumeSeries, requested string) error {
	if volumeSeries == requested {
		return nil
	}
	describe := func(series string) string {
		if series == "" {
			return "the distribution default"
		}
		return series
	}
	return fmt.Errorf("the sandbox database volume holds data written by %s, which %s cannot open: "+
		"run `govard sandbox down --purge` to discard it, or pass --db %s to keep it",
		describe(volumeSeries), describe(requested), keepSeriesFlag(volumeSeries))
}

func keepSeriesFlag(series string) string {
	if series == "" {
		return "default"
	}
	return series
}
