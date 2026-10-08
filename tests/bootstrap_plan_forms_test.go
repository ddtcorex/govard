package tests

import (
	"bytes"
	"strings"
	"testing"

	"govard/internal/cmd"
)

// runBootstrapPlanForTest runs `govard bootstrap <args>` in a project of the
// given framework and returns what it printed.
func runBootstrapPlanForTest(t *testing.T, framework, extraConfig string, args ...string) string {
	t.Helper()
	resetBootstrapFlagsForRuntimeTest(t)
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	t.Setenv("HOME", tempDir)
	writeRuntimeConfig(t, tempDir, "project_name: sample-project\ndomain: sample.test\nframework: "+framework+"\n"+extraConfig)

	root := cmd.RootCommandForTest()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("govard %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

const planStagingRemote = "remotes:\n  staging:\n    host: staging.example.com\n    user: deploy\n    path: /srv/www/app\n"

func TestBootstrapPlanWithoutRemotePrintsAPlan(t *testing.T) {
	out := runBootstrapPlanForTest(t, "wordpress", "", "bootstrap", "--plan", "--no-up")
	if !strings.Contains(out, "Planned Actions") {
		t.Fatalf("a plan with no configured remote printed no plan:\n%q", out)
	}
	if !strings.Contains(out, "no remote environment") {
		t.Fatalf("the plan must say why the remote is unnamed:\n%s", out)
	}
}

func TestBootstrapPlanWithUnknownRemotePrintsAPlan(t *testing.T) {
	out := runBootstrapPlanForTest(t, "wordpress", "", "bootstrap", "-e", "qa", "--plan", "--no-up")
	if !strings.Contains(out, "Planned Actions") {
		t.Fatalf("a plan for an unconfigured remote printed no plan:\n%q", out)
	}
	if !strings.Contains(out, "qa") {
		t.Fatalf("the plan must name the requested remote:\n%s", out)
	}
}

func TestBootstrapClonePlanExcludesComeFromTheFramework(t *testing.T) {
	wp := runBootstrapPlanForTest(t, "wordpress", planStagingRemote, "bootstrap", "-e", "staging", "--clone", "--plan", "--no-up")
	for _, magentoOnly := range []string{"app/etc/env.php", "pub/static", "pub/media", "app/etc/local.xml"} {
		if strings.Contains(wp, magentoOnly) {
			t.Errorf("a WordPress clone plan lists the Magento-only exclude %q:\n%s", magentoOnly, wp)
		}
	}
	for _, want := range []string{"wp-config.php", "wp-content/uploads"} {
		if !strings.Contains(wp, want) {
			t.Errorf("a WordPress clone plan must protect %q:\n%s", want, wp)
		}
	}

	m2 := runBootstrapPlanForTest(t, "magento2", planStagingRemote, "bootstrap", "-e", "staging", "--clone", "--plan", "--no-up")
	for _, want := range []string{"app/etc/env.php", "pub/static", "pub/media"} {
		if !strings.Contains(m2, want) {
			t.Errorf("a Magento 2 clone plan lost the exclude %q:\n%s", want, m2)
		}
	}
}
