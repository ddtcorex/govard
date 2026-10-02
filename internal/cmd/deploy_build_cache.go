package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/deploy"
)

// The sandbox artifact cache. A rehearsal rebuilds the same revision with the
// same lock file over and over (69 to 79 s each time on a storefront), and the
// target is a throwaway container, so reusing the artifact there is safe. It is
// deliberately limited to the sandbox remote: a build for a real remote always
// runs, because a stale artifact reaching production is the failure this cache
// must never be able to cause.

const (
	// buildCacheKeep is how many entries a store leaves behind: the most
	// recently used ones.
	buildCacheKeep = 3
	// buildCacheTreeName holds the artifact inside an entry; the marker beside
	// it says the copy finished.
	buildCacheTreeName     = "tree"
	buildCacheCompleteName = ".complete"
	buildCacheTempPrefix   = ".tmp-"
	// buildCacheTempMaxAge is when a leftover temporary directory (an
	// interrupted store) is swept by the next prune.
	buildCacheTempMaxAge = time.Hour
)

// buildCacheKey is everything that decides what an artifact contains. Any change
// to one of these is a different artifact, so it is a different entry.
type buildCacheKey struct {
	Revision           string
	ComposerLockSHA256 string
	PHP                string
	Mode               string
	GovardVersion      string
	// InputsSHA256 covers what the project asks the build to do beyond the
	// commit: the effective deploy settings, the hooks and the recipe's task
	// commands. A rehearsal tunes these in an untracked .govard.yml.
	InputsSHA256 string
	// Binary identifies the govard executable that builds, because a development
	// build keeps one version string across edits.
	Binary string
}

// ID is the entry name: the first 16 hex characters of a hash over the fields,
// each prefixed by its length so a field boundary cannot be shifted.
func (k buildCacheKey) ID() string {
	hash := sha256.New()
	for _, field := range []string{k.Revision, k.ComposerLockSHA256, k.PHP, k.Mode, k.GovardVersion, k.InputsSHA256, k.Binary} {
		fmt.Fprintf(hash, "%d:%s;", len(field), field)
	}
	return hex.EncodeToString(hash.Sum(nil))[:16]
}

// sandboxBuildCacheDir is where the entries live: under the sandbox state
// directory, so `sandbox down --purge` removes them with everything else
// derived.
func sandboxBuildCacheDir(projectRoot string) string {
	return filepath.Join(deploy.SandboxStateDir(projectRoot), "build-cache")
}

// buildCacheApplies reports whether a build uses the cache: only for the
// sandbox remote, and not when --no-cache asks for a rebuild.
func buildCacheApplies(remote string, noCache bool) bool {
	return !noCache && strings.EqualFold(strings.TrimSpace(remote), deploy.SandboxRemoteName)
}

// copyBuildTree is the copy a store and a restore use. A variable so a test can
// make it fail midway.
var copyBuildTree = linkTree

// lookupBuildCache returns the artifact directory of a complete entry.
func lookupBuildCache(dir string, key buildCacheKey) (string, bool) {
	entry := filepath.Join(dir, key.ID())
	if _, err := os.Stat(filepath.Join(entry, buildCacheCompleteName)); err != nil {
		return "", false
	}
	tree := filepath.Join(entry, buildCacheTreeName)
	if _, err := os.Stat(filepath.Join(tree, deploy.ArtifactManifestName)); err != nil {
		return "", false
	}
	// A hit makes the entry the most recently used, so a prune keeps it.
	now := time.Now()
	_ = os.Chtimes(entry, now, now)
	return tree, true
}

// storeBuildCache copies a finished build into the cache. The copy goes into a
// temporary sibling, the completion marker is written last, and the directory is
// renamed into place: a reader sees either no entry or a whole one, and two
// stores of the same key cannot corrupt each other.
func storeBuildCache(dir string, key buildCacheKey, built string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create the artifact cache %s: %w", dir, err)
	}
	final := filepath.Join(dir, key.ID())
	if _, err := os.Stat(filepath.Join(final, buildCacheCompleteName)); err == nil {
		return nil
	}
	temp, err := os.MkdirTemp(dir, buildCacheTempPrefix+key.ID()+"-")
	if err != nil {
		return fmt.Errorf("create a temporary cache entry: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(temp) }
	if err := copyBuildTree(built, filepath.Join(temp, buildCacheTreeName)); err != nil {
		cleanup()
		return fmt.Errorf("copy the artifact into the cache: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temp, buildCacheCompleteName), nil, 0o644); err != nil {
		cleanup()
		return fmt.Errorf("mark the cache entry complete: %w", err)
	}
	if err := os.Rename(temp, final); err != nil {
		cleanup()
		// Another store won the race: its entry is complete, ours is redundant.
		if _, statErr := os.Stat(filepath.Join(final, buildCacheCompleteName)); statErr == nil {
			return nil
		}
		return fmt.Errorf("publish the cache entry: %w", err)
	}
	pruneBuildCache(dir)
	return nil
}

// pruneBuildCache keeps the buildCacheKeep most recently used entries and sweeps
// temporary directories an interrupted store left behind.
func pruneBuildCache(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type item struct {
		name string
		mod  time.Time
	}
	var complete []item
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil || !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), buildCacheTempPrefix) {
			if time.Since(info.ModTime()) > buildCacheTempMaxAge {
				_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
			}
			continue
		}
		complete = append(complete, item{entry.Name(), info.ModTime()})
	}
	sort.Slice(complete, func(i, j int) bool { return complete[i].mod.After(complete[j].mod) })
	for i := buildCacheKeep; i < len(complete); i++ {
		_ = os.RemoveAll(filepath.Join(dir, complete[i].name))
	}
}

// restoreFromCache puts a cached artifact into the output directory with the
// same rules a build has: a non-empty output is refused without --force.
func restoreFromCache(tree, output string, force bool) error {
	entries, err := os.ReadDir(output)
	if err == nil && len(entries) > 0 {
		if !force {
			return fmt.Errorf("output directory %s is not empty; pass --force to replace its contents", output)
		}
		if err := os.RemoveAll(output); err != nil {
			return fmt.Errorf("clear output directory %s: %w", output, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect output directory %s: %w", output, err)
	}
	return copyBuildTree(tree, output)
}

// linkTree reproduces a directory tree, hard-linking regular files and falling
// back to a copy where a link is impossible (another filesystem). Symlinks stay
// symlinks and modes are kept. Nothing in the deploy flow writes into an artifact
// after it is built, which is what makes sharing the data blocks safe.
func linkTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			if err := os.Link(path, target); err == nil {
				return nil
			}
			return copyRegularFile(path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("cannot cache %s: unsupported file type %s", path, info.Mode().Type())
		}
	})
}

func copyRegularFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// sandboxBuildCacheKey derives the key of the build about to run, or reports
// that there is none (a revision git cannot resolve is left for the build to
// report in its own words).
func sandboxBuildCacheKey(ctx context.Context, workDir string, recipe deploy.Recipe, hooks []deploy.Hook, options deploy.Options, runnerName string) (buildCacheKey, bool) {
	revision := strings.TrimSpace(options.Revision)
	if revision == "" {
		revision = strings.TrimSpace(options.Tag)
	}
	if revision == "" {
		revision = "HEAD"
	}
	resolved, err := buildCacheGit(ctx, workDir, "rev-parse", "--verify", "--quiet", revision+"^{commit}")
	if err != nil || strings.TrimSpace(resolved) == "" {
		return buildCacheKey{}, false
	}
	sha := strings.TrimSpace(resolved)
	// A project with no composer.lock at that revision hashes the empty input:
	// the absence is part of what the artifact is.
	lock, _ := buildCacheGit(ctx, workDir, "show", sha+":composer.lock")
	lockHash := sha256.Sum256([]byte(lock))
	runner := strings.ToLower(strings.TrimSpace(runnerName))
	if runner == "" {
		runner = "host"
	}
	php, _ := options.Settings["php_version"].(string)
	return buildCacheKey{
		Revision:           sha,
		ComposerLockSHA256: hex.EncodeToString(lockHash[:]),
		PHP:                strings.TrimSpace(php),
		Mode:               deploy.BuildArtifact + "/" + runner,
		GovardVersion:      Version,
		InputsSHA256:       buildInputsDigest(recipe, hooks, options),
		Binary:             executableIdentity(),
	}, true
}

// buildInputsDigest hashes what the build is told to do. Maps marshal with
// sorted keys, so equal inputs always give the same digest.
func buildInputsDigest(recipe deploy.Recipe, hooks []deploy.Hook, options deploy.Options) string {
	type task struct {
		ID, Command, RunOn string
		Optional           bool
		HasCore            bool
	}
	tasks := make([]task, 0, len(recipe.Tasks))
	for _, t := range recipe.Tasks {
		tasks = append(tasks, task{ID: t.ID, Command: t.Command, RunOn: string(t.RunOn), Optional: t.Optional, HasCore: t.Core != nil})
	}
	raw, err := json.Marshal(struct {
		Recipe   string
		Tasks    []task
		Hooks    []deploy.Hook
		Settings map[string]any
	}{recipe.ID, tasks, hooks, options.Settings})
	if err != nil {
		// Unhashable inputs must not share an entry with anything else.
		return fmt.Sprintf("unhashable-%d", time.Now().UnixNano())
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// executableIdentity is the running binary's path, size and modification time.
func executableIdentity() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return path
	}
	return fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())
}

// BuildInputsDigestForTest exposes buildInputsDigest to the tests/ package.
func BuildInputsDigestForTest(recipe deploy.Recipe, hooks []deploy.Hook, options deploy.Options) string {
	return buildInputsDigest(recipe, hooks, options)
}

func buildCacheGit(ctx context.Context, workDir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", err
	}
	return string(output), nil
}

// readCachedManifest reads the manifest a restored artifact carries.
func readCachedManifest(output string) (*deploy.ArtifactManifest, error) {
	raw, err := os.ReadFile(filepath.Join(output, deploy.ArtifactManifestName))
	if err != nil {
		return nil, fmt.Errorf("read the cached artifact manifest: %w", err)
	}
	var manifest deploy.ArtifactManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse the cached artifact manifest: %w", err)
	}
	return &manifest, nil
}

// retargetManifestRevision makes a restored artifact's manifest carry the
// revision text of the build that asked for it. The deploy compares that text
// with the revision it is given, and the cache is keyed by the resolved commit, so
// a hit can come from a build that spelled the same commit another way. The
// manifest shares its data blocks with the cached copy, so it is replaced by a new
// file rather than written in place.
func retargetManifestRevision(output, revision string) (*deploy.ArtifactManifest, error) {
	manifest, err := readCachedManifest(output)
	if err != nil {
		return nil, err
	}
	if manifest.Revision == revision {
		return manifest, nil
	}
	manifest.Revision = revision
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode the cached artifact manifest: %w", err)
	}
	path := filepath.Join(output, deploy.ArtifactManifestName)
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("replace the cached artifact manifest: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return nil, fmt.Errorf("write the artifact manifest: %w", err)
	}
	return manifest, nil
}

// BuildCacheKeyForTest exposes the cache key to the tests/ package.
type BuildCacheKeyForTest = buildCacheKey

// BuildCacheKeepForTest exposes how many entries the cache keeps.
const BuildCacheKeepForTest = buildCacheKeep

// BuildCacheIDForTest exposes the entry name of a key.
func BuildCacheIDForTest(key buildCacheKey) string { return key.ID() }

// BuildCacheLookupForTest exposes lookupBuildCache.
func BuildCacheLookupForTest(dir string, key buildCacheKey) (string, bool) {
	return lookupBuildCache(dir, key)
}

// BuildCacheStoreForTest exposes storeBuildCache.
func BuildCacheStoreForTest(dir string, key buildCacheKey, built string) error {
	return storeBuildCache(dir, key, built)
}

// BuildCacheRestoreForTest exposes restoreFromCache.
func BuildCacheRestoreForTest(tree, output string, force bool) error {
	return restoreFromCache(tree, output, force)
}

// BuildCacheSetCopyForTest replaces the tree copy for one test and returns the
// function that puts the real one back.
func BuildCacheSetCopyForTest(fn func(src, dst string) error) func() {
	original := copyBuildTree
	copyBuildTree = fn
	return func() { copyBuildTree = original }
}
