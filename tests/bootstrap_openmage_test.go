package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/openmage"
)

func TestBootstrapPkgOpenMageFreshCommands(t *testing.T) {
	opts := bootstrap.Options{}
	openmageBootstrap := openmage.NewOpenMageBootstrap(opts)
	cmds := openmageBootstrap.FreshCommands()

	if len(cmds) == 0 {
		t.Fatal("expected commands for OpenMage, got none")
	}

	if !containsSubstring(cmds[0], "openmage/magento-lts") {
		t.Errorf("expected command to contain 'openmage/magento-lts', got %q", cmds[0])
	}
}

func TestBootstrapPkgOpenMagePostCloneWritesTablePrefix(t *testing.T) {
	projectDir := t.TempDir()
	openmageBootstrap := openmage.NewOpenMageBootstrap(bootstrap.Options{TablePrefix: "demo_"})

	if err := openmageBootstrap.PostClone(projectDir); err != nil {
		t.Fatalf("PostClone() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectDir, "app", "etc", "local.xml"))
	if err != nil {
		t.Fatalf("read local.xml: %v", err)
	}
	if !strings.Contains(string(content), "<table_prefix><![CDATA[demo_]]></table_prefix>") {
		t.Fatalf("expected local.xml table prefix, got:\n%s", string(content))
	}
}
