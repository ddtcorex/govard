package tests

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"govard/internal/blueprints"
)

var errBrokenBlueprintMount = errors.New("broken blueprint mount")

type brokenBlueprintMountFS struct{}

func (brokenBlueprintMountFS) Open(string) (fs.File, error) {
	return nil, errBrokenBlueprintMount
}

func TestUnionFSWalkDirEnumeratesMergedTreeExactlyOnce(t *testing.T) {
	t.Cleanup(blueprints.ResetMountsForTest())
	blueprints.RegisterFrameworkMount(blueprints.FrameworkMount{
		Framework:     "testfw",
		FS:            fstest.MapFS{"services.yml": &fstest.MapFile{Data: []byte("a: 1")}, "testfw.conf": &fstest.MapFile{Data: []byte("conf")}},
		HasDir:        true,
		NginxTemplate: "testfw.conf",
	})

	var visited []string
	err := fs.WalkDir(blueprints.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			visited = append(visited, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}

	foundServicesYML := false
	foundNginxConf := false
	nginxConfCount := 0
	for _, p := range visited {
		if p == "testfw/services.yml" {
			foundServicesYML = true
		}
		if p == "support/nginx/templates/testfw.conf" {
			foundNginxConf = true
			nginxConfCount++
		}
		if p == "testfw/testfw.conf" {
			t.Errorf("nginx template leaked into dir-mount listing at %q - should only appear under support/nginx/templates/", p)
		}
	}
	if !foundServicesYML {
		t.Errorf("expected testfw/services.yml in walk, got %v", visited)
	}
	if !foundNginxConf {
		t.Errorf("expected support/nginx/templates/testfw.conf in walk, got %v", visited)
	}
	if nginxConfCount != 1 {
		t.Errorf("expected testfw.conf visited exactly once, got %d times", nginxConfCount)
	}
}

func TestUnionFSStatDirMountRootReportsVirtualName(t *testing.T) {
	t.Cleanup(blueprints.ResetMountsForTest())
	blueprints.RegisterFrameworkMount(blueprints.FrameworkMount{
		Framework: "testfw",
		FS:        fstest.MapFS{"services.yml": &fstest.MapFile{Data: []byte("a: 1")}},
		HasDir:    true,
	})

	info, err := blueprints.Stat("testfw")
	if err != nil {
		t.Fatalf("Stat(testfw): %v", err)
	}
	if info.Name() != "testfw" {
		t.Errorf("Stat(testfw).Name() = %q, want %q", info.Name(), "testfw")
	}
	if !info.IsDir() {
		t.Errorf("Stat(testfw).IsDir() = false, want true")
	}
}

func TestUnionFSFallbackPathsStillWork(t *testing.T) {
	t.Cleanup(blueprints.ResetMountsForTest())
	// proxy.yml is a real, never-relocated file under internal/blueprints/files/.
	data, err := fs.ReadFile(blueprints.FS, "proxy.yml")
	if err != nil {
		t.Fatalf("ReadFile(proxy.yml): %v", err)
	}
	if len(data) == 0 {
		t.Errorf("proxy.yml read via union FS is empty")
	}
}

func TestUnionFSReportsBrokenFrameworkTemplateMount(t *testing.T) {
	t.Cleanup(blueprints.ResetMountsForTest())
	blueprints.RegisterFrameworkMount(blueprints.FrameworkMount{
		Framework:     "broken",
		FS:            brokenBlueprintMountFS{},
		NginxTemplate: "broken.conf",
	})

	if _, err := fs.ReadDir(blueprints.FS, "support/nginx/templates"); !errors.Is(err, errBrokenBlueprintMount) {
		t.Fatalf("ReadDir() error = %v, want broken mount error", err)
	}
}

func TestUnionFSConformance(t *testing.T) {
	t.Cleanup(blueprints.ResetMountsForTest())
	blueprints.RegisterFrameworkMount(blueprints.FrameworkMount{
		Framework:     "testfw",
		FS:            fstest.MapFS{"services.yml": &fstest.MapFile{Data: []byte("a: 1")}, "testfw.conf": &fstest.MapFile{Data: []byte("conf")}},
		HasDir:        true,
		NginxTemplate: "testfw.conf",
	})
	if err := fstest.TestFS(blueprints.FS, "proxy.yml", "testfw/services.yml", "support/nginx/templates/testfw.conf", "support/nginx/templates/default.conf"); err != nil {
		t.Fatalf("fstest.TestFS: %v", err)
	}
}
