// Package blueprints exposes the merged blueprints filesystem: the
// remainder of internal/blueprints/files/** that was not relocated, unioned
// with each framework package's own embedded blueprint sub-filesystem
// (registered via RegisterFrameworkMount from that package's init()).
package blueprints

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// files is the remainder of internal/blueprints/files/** - everything that
// was NOT relocated next to a framework package (shared includes/, proxy.yml,
// the generic support/ tree minus the 9 relocated nginx templates).
//
//go:embed all:files
var files embed.FS

// fallback is files, rooted at "files" so paths match the historical
// contract (e.g. "proxy.yml", "includes/base.yml",
// "support/nginx/templates/default.conf").
var fallback fs.FS

func init() {
	var err error
	fallback, err = fs.Sub(files, "files")
	if err != nil {
		panic(err)
	}
}

// FrameworkMount describes how one framework package's embedded blueprint
// assets are grafted into the merged blueprints tree.
type FrameworkMount struct {
	// Framework is the canonical framework name (e.g. "magento2"). Also
	// used as the directory-mount prefix when HasDir is true.
	Framework string
	// FS is the framework package's embedded blueprint sub-filesystem,
	// already fs.Sub'd so paths are relative to the framework's own
	// blueprint root (e.g. "services.yml", "varnish/default.vcl",
	// "magento2.conf" - not "blueprint/services.yml").
	FS fs.FS
	// HasDir grafts the whole of FS as a directory at "<Framework>/" in the
	// merged tree (e.g. "magento2/services.yml", "magento2/varnish/default.vcl").
	// Frameworks that only contribute an nginx template and have no other
	// assets (cakephp, drupal, wordpress today) leave this false.
	HasDir bool
	// NginxTemplate is the file name within FS holding the framework's
	// nginx vhost template (e.g. "magento2.conf"), or "" if the framework
	// has none. When set it is grafted as a single file at
	// "support/nginx/templates/<NginxTemplate>".
	NginxTemplate string
}

// mounts accumulates every framework's RegisterFrameworkMount call. It is
// read directly by Open/ReadDir/Stat (never snapshotted), so registrations
// performed by framework package init() functions - which run after this
// package's own init(), since Go initializes imported packages before their
// importers - are always visible by the time any blueprint is rendered.
var mounts []FrameworkMount

// RegisterFrameworkMount is called from a framework package's init() to
// graft its embedded blueprint sub-filesystem into the merged blueprints.FS
// tree. Not safe for concurrent use; intended usage is exclusively from
// package init() functions, mirroring frameworks.Registry.Register.
func RegisterFrameworkMount(m FrameworkMount) {
	mounts = append(mounts, m)
}

// ResetMountsForTest clears the package-level mounts slice and returns a
// restore callback, since mounts is process-global and otherwise
// accumulates real framework registrations across tests (from every
// framework package's init()) as well as leaking between test functions
// within the same test binary. Callers should invoke the returned func via
// defer or t.Cleanup to restore the prior state.
func ResetMountsForTest() func() {
	previous := mounts
	mounts = nil
	return func() {
		mounts = previous
	}
}

// dirMountForPath returns the mount that owns name because name is either
// exactly the mount's directory root or a path beneath it, plus name's
// path relative to that mount's FS root.
//
// A mount's own NginxTemplate file is deliberately excluded here even
// though it physically lives in the same embedded blueprint/ directory as
// the mount's other assets: that file is grafted ONLY at
// "support/nginx/templates/<NginxTemplate>" (via fileMount), matching the
// pre-migration contract where e.g. "magento2/magento2.conf" never existed
// as a path. Without this exclusion the nginx template would appear twice
// in the merged tree - once via this dir mount, once via the file mount -
// which fs.WalkDir would then visit twice.
func dirMountForPath(name string) (FrameworkMount, string, bool) {
	for _, m := range mounts {
		if !m.HasDir {
			continue
		}
		if name == m.Framework {
			return m, ".", true
		}
		if rel, ok := strings.CutPrefix(name, m.Framework+"/"); ok {
			if m.NginxTemplate != "" && rel == m.NginxTemplate {
				continue
			}
			return m, rel, true
		}
	}
	return FrameworkMount{}, "", false
}

// fileMount returns the mount that grafts a single nginx template file at
// exactly name ("support/nginx/templates/<NginxTemplate>"), if any.
func fileMount(name string) (FrameworkMount, bool) {
	const nginxDir = "support/nginx/templates/"
	if !strings.HasPrefix(name, nginxDir) {
		return FrameworkMount{}, false
	}
	base := strings.TrimPrefix(name, nginxDir)
	if strings.Contains(base, "/") {
		return FrameworkMount{}, false
	}
	for _, m := range mounts {
		if m.NginxTemplate != "" && m.NginxTemplate == base {
			return m, true
		}
	}
	return FrameworkMount{}, false
}

// fileMountChildrenOf returns the synthetic file-mount DirEntry values that
// belong directly under dir (e.g. dir="support/nginx/templates" yields one
// entry per registered NginxTemplate).
func fileMountChildrenOf(dir string) ([]fs.DirEntry, error) {
	const nginxDir = "support/nginx/templates"
	if dir != nginxDir {
		return nil, nil
	}
	var out []fs.DirEntry
	for _, m := range mounts {
		if m.NginxTemplate == "" {
			continue
		}
		info, err := fs.Stat(m.FS, m.NginxTemplate)
		if err != nil {
			return nil, err
		}
		out = append(out, fs.FileInfoToDirEntry(info))
	}
	return out, nil
}

// dirMountChildrenOf returns the synthetic directory-mount DirEntry values
// that belong directly under dir (dir="." yields "django", "magento2", ...).
func dirMountChildrenOf(dir string) []fs.DirEntry {
	if dir != "." {
		return nil
	}
	var out []fs.DirEntry
	for _, m := range mounts {
		if !m.HasDir {
			continue
		}
		out = append(out, mountDirEntry{name: m.Framework})
	}
	return out
}

// mountDirEntry is a synthetic fs.DirEntry representing a framework's
// directory mount (e.g. "magento2") that has no backing entry in fallback.
type mountDirEntry struct{ name string }

func (e mountDirEntry) Name() string               { return e.name }
func (e mountDirEntry) IsDir() bool                { return true }
func (e mountDirEntry) Type() fs.FileMode          { return fs.ModeDir }
func (e mountDirEntry) Info() (fs.FileInfo, error) { return mountDirInfo(e), nil }

type mountDirInfo struct{ name string }

func (i mountDirInfo) Name() string       { return i.name }
func (i mountDirInfo) Size() int64        { return 0 }
func (i mountDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (i mountDirInfo) ModTime() time.Time { return time.Time{} }
func (i mountDirInfo) IsDir() bool        { return true }
func (i mountDirInfo) Sys() any           { return nil }

// unionFS merges fallback with every registered framework mount. It
// implements fs.FS, fs.ReadDirFS and fs.StatFS so fs.WalkDir enumerates the
// merged tree without ever needing to Open() a directory itself.
type unionFS struct{}

// FS is the merged blueprints filesystem: fallback (internal/blueprints/files
// minus relocated assets) unioned with every framework's registered mount.
var FS fs.FS = unionFS{}

// WithSourceOverrides layers blueprint files from a source checkout over base.
// The shared assets live under internal/blueprints/files while framework-owned
// assets live beside their owning framework package. Missing checkout files
// always fall back to base, which keeps partial checkouts and installed builds
// usable.
func WithSourceOverrides(base fs.FS, checkoutRoot string) fs.FS {
	return sourceOverlayFS{base: base, checkoutRoot: filepath.Clean(checkoutRoot)}
}

type sourceOverlayFS struct {
	base         fs.FS
	checkoutRoot string
}

func (s sourceOverlayFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	baseInfo, baseErr := fs.Stat(s.base, name)
	overridePath, hasOverride := s.pathFor(name)
	overrideInfo, overrideErr := s.statOverride(overridePath, hasOverride)
	if overrideErr != nil {
		return nil, overrideErr
	}

	if overrideInfo != nil && !overrideInfo.IsDir() {
		return os.Open(overridePath)
	}
	if (overrideInfo != nil && overrideInfo.IsDir()) || (baseErr == nil && baseInfo.IsDir()) {
		entries, err := s.ReadDir(name)
		if err != nil {
			return nil, err
		}
		info := baseInfo
		if baseErr != nil {
			info = overrideInfo
		}
		return &mergedDirFile{info: info, entries: entries}, nil
	}
	if baseErr != nil {
		return nil, baseErr
	}
	return s.base.Open(name)
}

func (s sourceOverlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	entriesByName := map[string]fs.DirEntry{}
	baseEntries, baseErr := fs.ReadDir(s.base, name)
	if baseErr != nil && !errors.Is(baseErr, fs.ErrNotExist) {
		return nil, baseErr
	}
	for _, entry := range baseEntries {
		entriesByName[entry.Name()] = entry
	}

	overridePath, hasOverride := s.pathFor(name)
	if hasOverride {
		overrideEntries, err := os.ReadDir(overridePath)
		if err == nil {
			for _, entry := range overrideEntries {
				if mount, relative, mounted := dirMountForPath(name); mounted && relative == "." && entry.Name() == mount.NginxTemplate {
					continue
				}
				entriesByName[entry.Name()] = entry
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}

	// Framework nginx templates are mounted virtually at this shared path,
	// while their disk overrides live beside their framework package.
	if name == "support/nginx/templates" {
		for _, mount := range mounts {
			if mount.NginxTemplate == "" {
				continue
			}
			overrideTemplate := filepath.Join(s.checkoutRoot, "internal", "frameworks", mount.Framework, "blueprint", mount.NginxTemplate)
			if info, err := os.Stat(overrideTemplate); err == nil && !info.IsDir() {
				entriesByName[mount.NginxTemplate] = fs.FileInfoToDirEntry(info)
			}
		}
	}

	if len(entriesByName) == 0 && baseErr != nil {
		return nil, baseErr
	}
	entries := make([]fs.DirEntry, 0, len(entriesByName))
	for _, entry := range entriesByName {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (s sourceOverlayFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	overridePath, hasOverride := s.pathFor(name)
	overrideInfo, err := s.statOverride(overridePath, hasOverride)
	if err != nil {
		return nil, err
	}
	if overrideInfo != nil && !overrideInfo.IsDir() {
		return overrideInfo, nil
	}
	if info, baseErr := fs.Stat(s.base, name); baseErr == nil {
		return info, nil
	} else if overrideInfo != nil {
		return overrideInfo, nil
	} else {
		return nil, baseErr
	}
}

func (s sourceOverlayFS) pathFor(name string) (string, bool) {
	if mount, ok := fileMount(name); ok {
		return filepath.Join(s.checkoutRoot, "internal", "frameworks", mount.Framework, "blueprint", mount.NginxTemplate), true
	}
	if mount, relative, ok := dirMountForPath(name); ok {
		if relative == "." {
			return filepath.Join(s.checkoutRoot, "internal", "frameworks", mount.Framework, "blueprint"), true
		}
		return filepath.Join(s.checkoutRoot, "internal", "frameworks", mount.Framework, "blueprint", filepath.FromSlash(relative)), true
	}
	return filepath.Join(s.checkoutRoot, "internal", "blueprints", "files", filepath.FromSlash(name)), true
}

func (s sourceOverlayFS) statOverride(overridePath string, present bool) (fs.FileInfo, error) {
	if !present {
		return nil, nil
	}
	info, err := os.Stat(overridePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (unionFS) Open(name string) (fs.File, error) {
	// fs.FS.Open's contract (io/fs doc) requires rejecting invalid names
	// via fs.ValidPath rather than silently normalizing them (e.g.
	// "django/." or "support//nginx" must error, not be treated as
	// "django" / "support/nginx").
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	if m, ok := fileMount(name); ok {
		return m.FS.Open(m.NginxTemplate)
	}

	// An exact dir-mount root (e.g. Open("magento2")) must NOT delegate
	// straight to the framework's embedded sub-FS: that would (a) leak the
	// NginxTemplate file, which belongs only under
	// "support/nginx/templates/", and (b) report the wrong directory Name()
	// (fs.Sub roots keep the original embed pattern's base name, e.g.
	// "blueprint", not the virtual mount name "magento2"). Route it through
	// the same merged-directory construction used for "." and
	// "support/nginx/templates" below.
	if _, rel, ok := dirMountForPath(name); ok && rel == "." {
		return openMergedDir(name)
	}
	if m, rel, ok := dirMountForPath(name); ok {
		return m.FS.Open(rel)
	}

	// Directories that merge fallback entries with synthetic mount
	// entries (root "." and "support/nginx/templates") need a synthetic
	// directory file so a plain Open() + type-assert to fs.ReadDirFile
	// still behaves, even though our own ReadDir method (below) is what
	// fs.WalkDir actually calls.
	fileEntries, err := fileMountChildrenOf(name)
	if err != nil {
		return nil, err
	}
	extra := append(dirMountChildrenOf(name), fileEntries...)
	if len(extra) > 0 {
		return openMergedDir(name)
	}

	return fallback.Open(name)
}

// openMergedDir builds the fs.File for a directory whose listing is
// synthesized rather than a direct passthrough to a single backing fs.FS:
// root ".", "support/nginx/templates", and every dir-mount's own root
// (e.g. "magento2", which must exclude its NginxTemplate entry and report
// its virtual name rather than the embedded FS's internal root name).
func openMergedDir(name string) (fs.File, error) {
	entries, err := readDirMerged(name)
	if err != nil {
		return nil, err
	}
	info, err := Stat(name)
	if err != nil {
		return nil, err
	}
	return &mergedDirFile{info: info, entries: entries}, nil
}

func (unionFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	return readDirMerged(name)
}

func readDirMerged(name string) ([]fs.DirEntry, error) {
	// name is exactly a dir-mount root, or somewhere inside one (e.g.
	// "magento2" or "magento2/varnish"): the whole listing comes from that
	// framework's own FS - fallback has nothing at these paths (the
	// directory was relocated out of internal/blueprints/files entirely),
	// and no other framework's mount can nest inside it.
	if m, rel, ok := dirMountForPath(name); ok {
		entries, err := fs.ReadDir(m.FS, rel)
		if err != nil {
			return nil, err
		}
		if rel == "." && m.NginxTemplate != "" {
			// Filter out the nginx template: it is grafted only at
			// "support/nginx/templates/<NginxTemplate>", not here (see
			// dirMountForPath).
			filtered := entries[:0:0]
			for _, e := range entries {
				if e.Name() != m.NginxTemplate {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
		return entries, nil
	}

	var base []fs.DirEntry
	if entries, err := fs.ReadDir(fallback, name); err == nil {
		base = entries
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	fileEntries, err := fileMountChildrenOf(name)
	if err != nil {
		return nil, err
	}
	extra := append(dirMountChildrenOf(name), fileEntries...)
	if len(base) == 0 && len(extra) == 0 {
		// Neither side has this directory: surface a real not-exist error
		// (matches the fallback's own error rather than silently
		// returning an empty, "successful" listing).
		if _, err := fs.ReadDir(fallback, name); err != nil {
			return nil, err
		}
	}

	merged := append(append([]fs.DirEntry{}, base...), extra...)
	sort.Slice(merged, func(i, j int) bool { return merged[i].Name() < merged[j].Name() })
	return merged, nil
}

// Stat returns file info for name in the merged blueprints tree.
func Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if m, ok := fileMount(name); ok {
		return fs.Stat(m.FS, m.NginxTemplate)
	}
	// An exact dir-mount root reports its virtual name (e.g. "magento2"),
	// not the embedded sub-FS's own root name (e.g. "blueprint") - see
	// openMergedDir's doc comment for why.
	if m, rel, ok := dirMountForPath(name); ok {
		if rel == "." {
			return mountDirInfo{m.Framework}, nil
		}
		return fs.Stat(m.FS, rel)
	}
	if name == "." {
		return fallbackRootInfo{}, nil
	}
	fileEntries, err := fileMountChildrenOf(name)
	if err != nil {
		return nil, err
	}
	extra := append(dirMountChildrenOf(name), fileEntries...)
	if len(extra) > 0 {
		return mountDirInfo{path.Base(name)}, nil
	}
	return fs.Stat(fallback, name)
}

func (unionFS) Stat(name string) (fs.FileInfo, error) { return Stat(name) }

type fallbackRootInfo struct{}

func (fallbackRootInfo) Name() string       { return "." }
func (fallbackRootInfo) Size() int64        { return 0 }
func (fallbackRootInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (fallbackRootInfo) ModTime() time.Time { return time.Time{} }
func (fallbackRootInfo) IsDir() bool        { return true }
func (fallbackRootInfo) Sys() any           { return nil }

// mergedDirFile is the fs.File returned by Open() for a directory whose
// listing is a merge of fallback entries and synthetic mount entries (root
// "." and "support/nginx/templates"). It only needs to support Stat and
// ReadDir - callers that want file content always Open() a leaf path, which
// routes to a real backing fs.FS above and never reaches this type.
type mergedDirFile struct {
	info    fs.FileInfo
	entries []fs.DirEntry
	off     int
}

func (f *mergedDirFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *mergedDirFile) Read([]byte) (int, error)   { return 0, fs.ErrInvalid }
func (f *mergedDirFile) Close() error               { return nil }
func (f *mergedDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		rest := f.entries[f.off:]
		f.off = len(f.entries)
		return rest, nil
	}
	if f.off >= len(f.entries) {
		return nil, io.EOF
	}
	end := f.off + n
	if end > len(f.entries) {
		end = len(f.entries)
	}
	out := f.entries[f.off:end]
	f.off = end
	return out, nil
}
