package tests

import (
	"reflect"
	"testing"

	"govard/internal/cmd"
)

func noFiles(string) bool { return false }

func TestSelfUpdateMissingSystemDependencies(t *testing.T) {
	cases := []struct {
		name             string
		hasCertutil      bool
		ldconfig         string
		fileExists       func(string) bool
		desktopInstalled bool
		want             []string
	}{
		{
			name:             "desktop installed with WebKitGTK 6.0 needs nothing",
			hasCertutil:      true,
			ldconfig:         "\tlibwebkitgtk-6.0.so.4 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libwebkitgtk-6.0.so.4\n",
			fileExists:       noFiles,
			desktopInstalled: true,
			want:             nil,
		},
		{
			name:             "desktop installed with only the old WebKitGTK 4.1 asks for 6.0",
			hasCertutil:      true,
			ldconfig:         "\tlibwebkit2gtk-4.1.so.0 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libwebkit2gtk-4.1.so.0\n",
			fileExists:       noFiles,
			desktopInstalled: true,
			want:             []string{"libwebkitgtk-6.0-4"},
		},
		{
			name:        "WebKitGTK 6.0 found by file when ldconfig misses it",
			hasCertutil: true,
			ldconfig:    "",
			fileExists: func(path string) bool {
				return path == "/usr/lib/x86_64-linux-gnu/libwebkitgtk-6.0.so.4"
			},
			desktopInstalled: true,
			want:             nil,
		},
		{
			name:             "CLI-only install never asks for WebKitGTK",
			hasCertutil:      true,
			ldconfig:         "",
			fileExists:       noFiles,
			desktopInstalled: false,
			want:             nil,
		},
		{
			name:             "missing certutil is reported for CLI-only installs too",
			hasCertutil:      false,
			ldconfig:         "",
			fileExists:       noFiles,
			desktopInstalled: false,
			want:             []string{"libnss3-tools"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cmd.MissingSystemDependenciesForTest(tc.hasCertutil, tc.ldconfig, tc.fileExists, tc.desktopInstalled)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("missing deps = %#v, want %#v", got, tc.want)
			}
		})
	}
}
