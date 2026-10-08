package magento2

import (
	"os"
	"path/filepath"
)

// sampleMediaPackage is the Composer package whose map is `* -> pub/media`.
const sampleMediaPackage = "vendor/magento/sample-data-media"

// GuardComposerInstallMedia protects pub/media from the Magento composer
// installer. The sample-data-media package maps every top-level entry into
// pub/media, and the installer copies a directory INTO an existing one, so a
// reinstall (a clone bootstrap wipes vendor/ first) leaves
// pub/media/catalog/catalog/... next to the real media.
//
// It records how deep the pub/media/<name>/<name>/... chain already is for each
// directory and returns a function that, after the install, removes only the
// next level when it appeared in between and the package ships an entry for
// that name. Anything that was there before, and anything the package does not
// ship, is never touched. (A chain can legitimately exist already: the sample
// data itself ships downloadable/downloadable.)
func GuardComposerInstallMedia(projectDir string) func() {
	media := filepath.Join(projectDir, "pub", "media")
	before := map[string]int{}
	if entries, err := os.ReadDir(media); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				before[entry.Name()] = nestingDepth(media, entry.Name())
			}
		}
	}
	return func() {
		pkgEntries, err := os.ReadDir(filepath.Join(projectDir, filepath.FromSlash(sampleMediaPackage)))
		if err != nil {
			return
		}
		for _, entry := range pkgEntries {
			name := entry.Name()
			if !entry.IsDir() {
				continue
			}
			depth := before[name]
			if nestingDepth(media, name) <= depth {
				continue
			}
			parts := []string{media}
			for i := 0; i < depth+2; i++ {
				parts = append(parts, name)
			}
			_ = os.RemoveAll(filepath.Join(parts...))
		}
	}
}

// nestingDepth counts how many times name repeats below media/name.
func nestingDepth(media, name string) int {
	path := filepath.Join(media, name)
	depth := 0
	for {
		path = filepath.Join(path, name)
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			return depth
		}
		depth++
	}
}
