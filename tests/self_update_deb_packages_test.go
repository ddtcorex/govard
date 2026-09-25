package tests

import (
	"reflect"
	"testing"

	"govard/internal/cmd"
)

const debChecksums = `aaa  govard_1.77.0-beta.1_linux_amd64.deb
bbb  govard-desktop_1.77.0-beta.1_linux_amd64.deb
`

func TestSelfUpdateDebPackagesToInstall(t *testing.T) {
	cases := []struct {
		name             string
		tag              string
		desktopInstalled bool
		want             []string
	}{
		{"CLI only", "v1.77.0-beta.1", false, []string{"govard_1.77.0-beta.1_linux_amd64.deb"}},
		{"desktop installed: CLI first, then desktop", "v1.77.0-beta.1", true, []string{
			"govard_1.77.0-beta.1_linux_amd64.deb",
			"govard-desktop_1.77.0-beta.1_linux_amd64.deb",
		}},
		{"tag without v prefix", "1.77.0-beta.1", true, []string{
			"govard_1.77.0-beta.1_linux_amd64.deb",
			"govard-desktop_1.77.0-beta.1_linux_amd64.deb",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, skipped, err := cmd.DebPackagesToInstallForTest(tc.tag, "amd64", tc.desktopInstalled, true, debChecksums)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("assets = %#v, want %#v", got, tc.want)
			}
			if len(skipped) != 0 {
				t.Fatalf("skipped = %#v, want none", skipped)
			}
		})
	}
}

func TestSelfUpdateDebPlanSkipsDesktopWithoutChecksum(t *testing.T) {
	cliOnly := "aaa  govard_1.77.0-beta.1_linux_amd64.deb\n"
	got, skipped, err := cmd.DebPackagesToInstallForTest("v1.77.0-beta.1", "amd64", true, true, cliOnly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"govard_1.77.0-beta.1_linux_amd64.deb"}) {
		t.Fatalf("assets = %#v, want the CLI package only", got)
	}
	if !reflect.DeepEqual(skipped, []string{"govard-desktop_1.77.0-beta.1_linux_amd64.deb"}) {
		t.Fatalf("skipped = %#v, want the desktop package named", skipped)
	}
}

func TestSelfUpdateDebPlanRejectsEmptyTag(t *testing.T) {
	if _, _, err := cmd.DebPackagesToInstallForTest("", "amd64", true, true, debChecksums); err == nil {
		t.Fatal("expected an error for an empty release tag")
	}
}

// Ubuntu 22.04 and Debian 12 have no WebKitGTK 6.0. Installing the new desktop
// package there leaves it unconfigurable, and the apt-get -f fallback would
// then remove govard-desktop entirely, taking the user's working old desktop
// with it. The desktop package is skipped until its runtime is present.
func TestSelfUpdateDebPlanSkipsDesktopWithoutWebKitGTK6(t *testing.T) {
	got, skipped, err := cmd.DebPackagesToInstallForTest("v1.77.0-beta.1", "amd64", true, false, debChecksums)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"govard_1.77.0-beta.1_linux_amd64.deb"}) {
		t.Fatalf("assets = %#v, want the CLI package only", got)
	}
	if !reflect.DeepEqual(skipped, []string{"govard-desktop_1.77.0-beta.1_linux_amd64.deb"}) {
		t.Fatalf("skipped = %#v, want the desktop package named", skipped)
	}
}
