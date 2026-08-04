package tests

import (
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type releaseNFPMConfig struct {
	NFPMS []releaseNFPM `yaml:"nfpms"`
}

type releaseNFPM struct {
	ID               string   `yaml:"id"`
	PackageName      string   `yaml:"package_name"`
	IDs              []string `yaml:"ids"`
	FileNameTemplate string   `yaml:"file_name_template"`
	Dependencies     []string `yaml:"dependencies"`
	Contents         []struct {
		Source      string `yaml:"src"`
		Destination string `yaml:"dst"`
	} `yaml:"contents"`
}

func TestLinuxPackageLayout(t *testing.T) {
	data, err := os.ReadFile("../.goreleaser.yml")
	if err != nil {
		t.Fatalf("read GoReleaser config: %v", err)
	}

	var config releaseNFPMConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse GoReleaser config: %v", err)
	}

	cli := findReleaseNFPM(t, config.NFPMS, "govard-cli")
	if cli.PackageName != "govard" {
		t.Errorf("CLI package name = %q, want govard", cli.PackageName)
	}
	if !slices.Equal(cli.IDs, []string{"govard"}) {
		t.Errorf("CLI build IDs = %v, want [govard]", cli.IDs)
	}
	if !strings.HasPrefix(cli.FileNameTemplate, "{{ .ProjectName }}_") {
		t.Errorf("CLI file name template = %q, want govard artifact prefix", cli.FileNameTemplate)
	}
	if slices.Contains(cli.Dependencies, "libwebkit2gtk-4.1-0") {
		t.Errorf("CLI dependencies unexpectedly include WebKitGTK: %v", cli.Dependencies)
	}
	if len(cli.Contents) != 0 {
		t.Errorf("CLI package unexpectedly contains Desktop assets: %v", cli.Contents)
	}

	desktop := findReleaseNFPM(t, config.NFPMS, "govard-desktop")
	if desktop.PackageName != "govard-desktop" {
		t.Errorf("Desktop package name = %q, want govard-desktop", desktop.PackageName)
	}
	if !slices.Equal(desktop.IDs, []string{"govard-desktop"}) {
		t.Errorf("Desktop build IDs = %v, want [govard-desktop]", desktop.IDs)
	}
	if !strings.HasPrefix(desktop.FileNameTemplate, "govard-desktop_") {
		t.Errorf("Desktop file name template = %q, want govard-desktop artifact prefix", desktop.FileNameTemplate)
	}
	for _, dependency := range []string{"govard", "libwebkit2gtk-4.1-0", "libgtk-3-0", "libnss3-tools"} {
		if !slices.Contains(desktop.Dependencies, dependency) {
			t.Errorf("Desktop dependencies = %v, want %q", desktop.Dependencies, dependency)
		}
	}
	if !releaseNFPMHasContent(desktop, "./packaging/linux/govard.desktop", "/usr/share/applications/govard.desktop") {
		t.Error("Desktop package is missing the application launcher")
	}
	if !releaseNFPMHasContent(desktop, "./packaging/icons/govard.svg", "/usr/share/icons/hicolor/scalable/apps/govard.svg") {
		t.Error("Desktop package is missing the scalable application icon")
	}
}

func findReleaseNFPM(t *testing.T, packages []releaseNFPM, id string) releaseNFPM {
	t.Helper()
	for _, pkg := range packages {
		if pkg.ID == id {
			return pkg
		}
	}
	t.Fatalf("GoReleaser package %q not found", id)
	return releaseNFPM{}
}

func releaseNFPMHasContent(pkg releaseNFPM, source, destination string) bool {
	for _, content := range pkg.Contents {
		if content.Source == source && content.Destination == destination {
			return true
		}
	}
	return false
}
