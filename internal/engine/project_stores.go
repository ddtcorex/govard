package engine

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// projectStoreIDPattern is the shape of a per-project store directory name
// ("project-" + 16 hex): only an exact id match is ever removed.
var projectStoreIDPattern = regexp.MustCompile(`^project-[0-9a-f]{16}$`)

var (
	projectStoreMu        sync.Mutex
	projectStoreResolvers []func(root string) []string
)

// RegisterProjectStoreResolver lets a package that engine cannot import (the
// verify and audit stores key their directories by a project id derived from
// the project path) report the store directories a project owns under the
// Govard home. Each resolver returns candidate paths for one project root.
func RegisterProjectStoreResolver(resolver func(root string) []string) {
	projectStoreMu.Lock()
	defer projectStoreMu.Unlock()
	projectStoreResolvers = append(projectStoreResolvers, resolver)
}

// collectProjectStoreDirs keeps only real directories (never symlinks) with an
// exact project id name directly inside the Govard home.
func collectProjectStoreDirs(root string) []string {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	projectStoreMu.Lock()
	resolvers := append([]func(string) []string{}, projectStoreResolvers...)
	projectStoreMu.Unlock()

	seen := map[string]bool{}
	var dirs []string
	for _, resolve := range resolvers {
		for _, candidate := range resolve(root) {
			candidate = filepath.Clean(candidate)
			if seen[candidate] || !projectStoreIDPattern.MatchString(filepath.Base(candidate)) || !withinGovardHome(candidate) {
				continue
			}
			if info, err := os.Lstat(candidate); err != nil || !info.IsDir() {
				continue
			}
			seen[candidate] = true
			dirs = append(dirs, candidate)
		}
	}
	return dirs
}

// quietMissingResources drops docker's "No resource found" notes, which only
// say that there was nothing to remove and are not a failure worth an error line.
type quietMissingResources struct {
	out io.Writer
}

func (q quietMissingResources) Write(p []byte) (int, error) {
	var kept [][]byte
	for _, line := range bytes.SplitAfter(p, []byte("\n")) {
		if bytes.Contains(bytes.ToLower(line), []byte("no resource found")) {
			continue
		}
		kept = append(kept, line)
	}
	if _, err := q.out.Write(bytes.Join(kept, nil)); err != nil {
		return 0, err
	}
	return len(p), nil
}

func quietStderr(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}
	return quietMissingResources{out: w}
}
