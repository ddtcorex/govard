package deploy

import "fmt"

// SandboxDBVolumeLabel marks the named volume that keeps a sandbox's database
// across `sandbox down`. Its value is the project name, so `down --purge` can
// find the volume without knowing its name. The series the data directory was
// written by is stamped under sandboxDBLabel, the same key the container uses.
const SandboxDBVolumeLabel = "govard.sandbox.db-volume"

// SandboxDBDataDir is where the database server keeps its data in the image.
const SandboxDBDataDir = "/var/lib/mysql"

// SandboxDBVolumeName is the volume that holds a project's sandbox database.
// It does not depend on the database series: a changed series is refused by
// CheckSandboxDBVolumeSeries rather than silently getting a second volume.
func SandboxDBVolumeName(project string) string {
	return "govard-sandbox-" + sandboxSlug(project) + "-db"
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
