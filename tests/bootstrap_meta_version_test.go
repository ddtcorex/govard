package tests

import (
	"bytes"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"

	"github.com/spf13/cobra"
)

// `govard init --framework-version 2.4.6` writes the version into .govard.yml,
// and a later `bootstrap --fresh` without the flag used to ignore it and install
// the latest release, which cannot resolve on the PHP the config also chose.
// The flag still wins; the config is the default; neither means latest.
func TestResolveBootstrapMetaVersion(t *testing.T) {
	cases := []struct {
		name        string
		framework   string
		fresh       bool
		flag        string
		config      string
		wantVersion string
		wantSource  string
		wantErr     string
		wantIgnored string
	}{
		{name: "flag wins over config", framework: "magento2", fresh: true, flag: "2.4.7", config: "2.4.6", wantVersion: "2.4.7", wantSource: cmd.BootstrapVersionSourceFlag},
		{name: "config is the default for fresh", framework: "magento2", fresh: true, config: "2.4.6", wantVersion: "2.4.6", wantSource: cmd.BootstrapVersionSourceConfig},
		{name: "config is trimmed", framework: "magento2", fresh: true, config: " 2.4.6 ", wantVersion: "2.4.6", wantSource: cmd.BootstrapVersionSourceConfig},
		{name: "neither means latest", framework: "magento2", fresh: true, wantVersion: "", wantSource: ""},
		{name: "config default applies to any framework with a meta version", framework: "laravel", fresh: true, config: "11", wantVersion: "11", wantSource: cmd.BootstrapVersionSourceConfig},
		{name: "not fresh leaves the config alone", framework: "magento2", fresh: false, config: "2.4.6", wantVersion: "", wantSource: ""},
		{name: "not fresh still honours the flag", framework: "magento2", fresh: false, flag: "2.4.7", config: "2.4.6", wantVersion: "2.4.7", wantSource: cmd.BootstrapVersionSourceFlag},
		{name: "invalid flag is the flag validation error", framework: "magento2", fresh: true, flag: "abc", config: "2.4.6", wantErr: "invalid --framework-version value \"abc\""},
		{name: "non-numeric config is ignored, not an error", framework: "magento2", fresh: true, config: "latest", wantIgnored: "latest"},
		{name: "numeric config below the minimum still names the config", framework: "magento2", fresh: true, config: "1.9.0", wantErr: "invalid --framework-version value \"1.9.0\""},
		{name: "caret constraint is ignored", framework: "laravel", fresh: true, config: "^11.31", wantIgnored: "^11.31"},
		{name: "tilde constraint is ignored", framework: "magento2", fresh: true, config: "~2.4.7", wantIgnored: "~2.4.7"},
		{name: "wildcard constraint is ignored", framework: "magento2", fresh: true, config: "2.4.*", wantIgnored: "2.4.*"},
		{name: "ignored config does not hide a valid flag", framework: "magento2", fresh: true, flag: "2.4.7", config: "~2.4.7", wantVersion: "2.4.7", wantSource: cmd.BootstrapVersionSourceFlag},
		{name: "invalid flag still fails with a constraint", framework: "laravel", fresh: true, flag: "^11.31", wantErr: "invalid --framework-version value \"^11.31\""},
		{name: "config below the framework minimum is refused", framework: "magento2", fresh: true, config: "1.9.0", wantErr: "2.0.0+"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			version, source, ignored, err := cmd.ResolveBootstrapMetaVersionForTest(testCase.framework, testCase.flag, testCase.config, testCase.fresh)
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, testCase.wantErr)
				}
				if testCase.flag == "" && !strings.Contains(err.Error(), ".govard.yml") {
					t.Errorf("a bad config value must say it came from .govard.yml, got %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if testCase.wantIgnored != "" && (!strings.Contains(ignored, testCase.wantIgnored) || !strings.Contains(ignored, "ignored")) {
				t.Errorf("ignored = %q, want it to name %q and say it was ignored", ignored, testCase.wantIgnored)
			}
			if testCase.wantIgnored == "" && ignored != "" {
				t.Errorf("unexpected ignored note %q", ignored)
			}
			if version != testCase.wantVersion || source != testCase.wantSource {
				t.Fatalf("got (%q, %q), want (%q, %q)", version, source, testCase.wantVersion, testCase.wantSource)
			}
		})
	}
}

// The plan names the version and where it came from, so an operator sees the
// config default before anything is installed.
func TestBootstrapFreshPlanSaysWhereTheVersionCameFrom(t *testing.T) {
	cases := []struct {
		name   string
		opts   cmd.BootstrapRuntimeOptions
		want   []string
		reject []string
	}{
		{name: "from config", opts: cmd.BootstrapRuntimeOptions{MetaVersion: "2.4.6", MetaVersionSource: cmd.BootstrapVersionSourceConfig}, want: []string{"2.4.6", "framework_version in .govard.yml"}},
		{name: "from flag", opts: cmd.BootstrapRuntimeOptions{MetaVersion: "2.4.7", MetaVersionSource: cmd.BootstrapVersionSourceFlag}, want: []string{"2.4.7", "--framework-version"}},
		{name: "constraint ignored", opts: cmd.BootstrapRuntimeOptions{MetaVersionIgnored: "framework_version \"^11.31\" in .govard.yml is not a plain numeric version, so it was ignored"}, want: []string{"latest", "^11.31", "ignored"}, reject: []string{"at version"}},
		{name: "latest", opts: cmd.BootstrapRuntimeOptions{}, want: []string{"latest"}, reject: []string{"at version"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			lines, err := cmd.BuildBootstrapFreshPlanForTest(engine.Config{ProjectName: "sample-project", Framework: "magento2"}, "magento2", testCase.opts)
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			joined := strings.Join(lines, "\n")
			for _, want := range testCase.want {
				if !strings.Contains(joined, want) {
					t.Errorf("plan must contain %q:\n%s", want, joined)
				}
			}
			for _, reject := range testCase.reject {
				if strings.Contains(joined, reject) {
					t.Errorf("plan must not contain %q:\n%s", reject, joined)
				}
			}
		})
	}
}

// End to end through the command: the reproduction from the field. The config
// says 2.4.6, the command line says nothing, and the installer must be handed
// 2.4.6 rather than "".
func TestBootstrapFreshUsesTheConfigFrameworkVersion(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{name: "config default", args: []string{"bootstrap", "--fresh", "--no-up", "-y"}, want: "2.4.6"},
		{name: "flag wins", args: []string{"bootstrap", "--fresh", "--no-up", "-y", "--framework-version", "2.4.7"}, want: "2.4.7"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resetBootstrapFlagsForRuntimeTest(t)
			tempDir := t.TempDir()
			chdirForTest(t, tempDir)
			writeRuntimeConfig(t, tempDir, "project_name: sample-project\ndomain: sample.test\nframework: magento2\nframework_version: 2.4.6\n")
			defer cmd.SetBootstrapEnsureInitForTest(func(*cobra.Command, string) error { return nil })()

			got := "<not called>"
			defer cmd.SetBootstrapFreshInstallForTest(func(_ *cobra.Command, _ engine.Config, opts cmd.BootstrapRuntimeOptions) error {
				got = opts.MetaVersion
				return nil
			})()

			root := cmd.RootCommandForTest()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(testCase.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("bootstrap: %v\n%s", err, out.String())
			}
			if got != testCase.want {
				t.Fatalf("the installer got version %q, want %q", got, testCase.want)
			}
		})
	}
}

// `--fresh --plan` prints the resolved version without installing anything.
func TestBootstrapFreshPlanCommandPrintsTheResolvedVersion(t *testing.T) {
	resetBootstrapFlagsForRuntimeTest(t)
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeRuntimeConfig(t, tempDir, "project_name: sample-project\ndomain: sample.test\nframework: magento2\nframework_version: 2.4.6\n")

	root := cmd.RootCommandForTest()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"bootstrap", "--fresh", "--plan", "--no-up"})
	if err := root.Execute(); err != nil {
		t.Fatalf("bootstrap --fresh --plan: %v\n%s", err, out.String())
	}
	for _, want := range []string{"2.4.6", "framework_version in .govard.yml"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the plan must contain %q:\n%s", want, out.String())
		}
	}
}
