package verify

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/engine"
)

const phase4Suffix = "-phase4.json"

// usableSnapshot reports whether a snapshot can restore what the destructive
// phase is about to destroy.
//
// P5-02 runs `env down -v`, which deletes the database volume, so a snapshot is
// only a valid restore target when it carries a restorable dump. Two traps make
// a directory-only or metadata-only check wrong:
//
//   - engine.ListSnapshots reports EVERY directory under the snapshot root as a
//     snapshot, defaulting CreatedAt to the zero time when metadata.yml is
//     missing or unparseable.
//   - `govard snapshot create` exits 0 and prints SUCCESS even when the dump
//     failed, leaving metadata with db:false plus a valid but EMPTY gzip stream.
//
// Requiring `db: true` AND a dump that decompresses to at least one byte closes
// both: a media-only snapshot cannot bring the database back, and neither can an
// empty stream. Without this the phase could wipe the volume, "restore" nothing
// and still report success.
func usableSnapshot(projectRoot, name string) bool {
	dir := filepath.Join(engine.SnapshotRoot(projectRoot), name)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	// ListSnapshots already parses metadata.yml, so reuse that instead of
	// re-implementing the YAML shape. It sizes every directory as it goes, which
	// is why the gate is the only caller that pays for it.
	snapshots, err := engine.ListSnapshots(projectRoot)
	if err != nil {
		return false
	}
	recordsDatabase := false
	for _, s := range snapshots {
		if strings.TrimSpace(s.Name) != name {
			continue
		}
		recordsDatabase = s.DB
		break
	}
	if !recordsDatabase {
		return false
	}
	return dumpHasContent(filepath.Join(dir, "db.sql.gz"))
}

// dumpHasContent reports whether a dump file is a gzip stream with content. Size
// alone is not enough: a failed dump leaves a valid, non-empty gzip whose
// decompressed payload is empty.
func dumpHasContent(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	defer zr.Close()
	buf := make([]byte, 512)
	n, _ := zr.Read(buf)
	return n > 0
}

// LatestSnapshotName returns the newest usable snapshot of the project. Entries
// with no metadata sort with a zero CreatedAt and are never preferred; a store
// with nothing usable is "no snapshot", not an error.
func LatestSnapshotName(projectRoot string) (string, bool) {
	snapshots, err := engine.ListSnapshots(projectRoot)
	if err != nil {
		return "", false
	}
	var best string
	var bestTime time.Time
	for _, s := range snapshots {
		name := strings.TrimSpace(s.Name)
		if name == "" || !usableSnapshot(projectRoot, name) {
			continue
		}
		if best == "" || s.CreatedAt.After(bestTime) {
			best, bestTime = name, s.CreatedAt
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// LatestSnapshotNameForTest exposes LatestSnapshotName for tests in /tests.
func LatestSnapshotNameForTest(projectRoot string) (string, bool) {
	return LatestSnapshotName(projectRoot)
}

// GateSatisfyingSnapshot reports the snapshot a phase-5 run may destroy data
// for, and whether one exists.
//
// An artifact satisfies the gate only when it is (a) not a plan run, (b) written
// for this project, (c) has a P4-08 that exited 0, and (d) names a snapshot that
// is still usable on disk. Files are scanned newest-first and a plan artifact is
// skipped rather than fatal, so `verify --phase 4 --plan` cannot poison the
// store. Artifacts written before project scoping carry no identity and are
// treated as unsatisfied. The name returned is the one P5-05 restores.
func GateSatisfyingSnapshot(opts VerifyOpts) (string, bool) {
	dir := ProjectRunsDir(opts.ProjectRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var candidates []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), phase4Suffix) {
			continue
		}
		candidates = append(candidates, e.Name())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(candidates)))

	wantProject := ProjectID(opts.ProjectRoot)
	for _, name := range candidates {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var res RunResult
		if err := json.Unmarshal(b, &res); err != nil {
			continue
		}
		if res.Mode == "plan" || res.ProjectID != wantProject {
			continue
		}
		for _, it := range res.Items {
			if it.ID != "P4-08" || it.ExitCode != 0 || len(it.Artifacts) == 0 {
				continue
			}
			artifact := strings.TrimSpace(it.Artifacts[0])
			if artifact == "" || !usableSnapshot(opts.ProjectRoot, artifact) {
				continue
			}
			return artifact, true
		}
	}
	return "", false
}
