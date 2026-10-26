package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// OwnedPath is one path under .govard with its current owner.
type OwnedPath struct {
	Path string
	UID  int
	GID  int
}

// ForeignOwnedGovardPaths lists every path under <root>/.govard (the
// directory itself included) not owned by uid. The walk uses Lstat and never
// follows a symlink, so a link pointing outside the project is reported as
// the link itself. A missing .govard yields no paths.
func ForeignOwnedGovardPaths(projectRoot string, uid int) ([]OwnedPath, error) {
	return foreignOwnedGovardPaths(projectRoot, uid, FileOwnership)
}

// ForeignOwnedGovardPathsForTest injects the ownership lookup, since tests
// cannot chown files to another user.
func ForeignOwnedGovardPathsForTest(projectRoot string, uid int, owner func(string) (int, int, error)) ([]OwnedPath, error) {
	return foreignOwnedGovardPaths(projectRoot, uid, owner)
}

func foreignOwnedGovardPaths(projectRoot string, uid int, owner func(string) (int, int, error)) ([]OwnedPath, error) {
	govardDir := filepath.Join(projectRoot, ".govard")
	if _, err := os.Lstat(govardDir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var paths []OwnedPath
	err := filepath.WalkDir(govardDir, func(path string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		fileUID, fileGID, err := owner(path)
		if err != nil {
			return err
		}
		if fileUID != uid {
			paths = append(paths, OwnedPath{Path: path, UID: fileUID, GID: fileGID})
		}
		return nil
	})
	return paths, err
}

func checkGovardOwnershipInRoot(root string) error {
	projectRoot := root
	if projectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil
		}
		projectRoot = wd
	}
	uid := os.Getuid()
	if uid < 0 {
		return nil // no uid model (Windows)
	}
	paths, err := ForeignOwnedGovardPaths(projectRoot, uid)
	if err != nil || len(paths) == 0 {
		return err
	}
	names := make([]string, 0, 5)
	for i, p := range paths {
		if i == 5 {
			break
		}
		rel, relErr := filepath.Rel(projectRoot, p.Path)
		if relErr != nil {
			rel = p.Path
		}
		names = append(names, fmt.Sprintf("%s (uid %d)", rel, p.UID))
	}
	return fmt.Errorf("%d paths under .govard are not owned by uid %d: %s", len(paths), uid, strings.Join(names, ", "))
}
