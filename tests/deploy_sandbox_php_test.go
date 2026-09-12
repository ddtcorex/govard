package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

// A sandbox exists to rehearse a deploy against the target a project actually
// has. The base image carries one PHP series, and a project whose composer.lock
// needs another cannot even install its dependencies there — the failure lands
// in the middle of a deploy and looks like a project defect. `--php` closes that
// gap, and what follows is the contract it has to keep.

func TestSandboxPHPIsAClosedSet(t *testing.T) {
	for _, php := range []string{"", "  ", "8.4", " 8.4 ", "8.2", "7.4", "10.0"} {
		if _, err := deploy.ValidateSandboxPHP(php); err != nil {
			t.Errorf("ValidateSandboxPHP(%q) = %v, want no error", php, err)
		}
	}
	// The value is rendered into the image definition, so anything that is not a
	// `major.minor` series is refused rather than escaped and hoped for.
	for _, php := range []string{"8", "8.4.1", "php8.4", "latest", "8.4; rm -rf /", "8.4\nRUN whoami", "$(whoami)", "8..4", "v8.4"} {
		if _, err := deploy.ValidateSandboxPHP(php); err == nil {
			t.Errorf("ValidateSandboxPHP(%q) accepted a value that is not a PHP series", php)
		}
	}
}

func TestSandboxPHPSeriesIsNormalized(t *testing.T) {
	series, err := deploy.ValidateSandboxPHP("  8.4\t")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if series != "8.4" {
		t.Fatalf("series = %q, want the trimmed 8.4", series)
	}
	// The trim has to reach the image, or `--php " 8.4"` would pin the image and
	// declare 8.4 to the pipeline while rendering `php 8.4-cli`.
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxProfilePHP, "  8.4\t", deploy.SandboxRequirements{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(dockerfile, "'php8.4-cli'") {
		t.Fatalf("the trimmed series must reach the package list, got:\n%s", dockerfile)
	}
}

func TestSandboxDockerfileInstallsTheRequestedSeries(t *testing.T) {
	requirements := deploy.SandboxRequirements{
		Extensions: []string{"bcmath", "intl"},
		Services:   []string{"mariadb"},
	}
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxProfileFull, "8.4", requirements)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// The series has to come from somewhere Debian does not carry it, and the
	// repository has to be keyed: an unsigned source is rejected by apt.
	for _, want := range []string{
		"packages.sury.org/php/",
		"sury-php.list",
		"/usr/share/keyrings/sury-php.gpg",
		"signed-by=/usr/share/keyrings/sury-php.gpg",
		// The extensions follow the series, or the image would carry the base
		// distribution's PHP extensions next to the requested interpreter.
		"'php8.4-cli'",
		"'php8.4-bcmath'",
		"'php8.4-intl'",
		// Every recipe command, the sandbox remote's php_bin and the fixtures all
		// call the plain binary.
		"update-alternatives --install /usr/bin/php php /usr/bin/php8.4 100",
		// A build that silently produced the wrong interpreter would be worse
		// than one that fails.
		`php -r 'exit(strpos(PHP_VERSION, "8.4") === 0 ? 0 : 1);'`,
		"composer.phar",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("a php 8.4 image must contain %q, got:\n%s", want, dockerfile)
		}
	}

	// Debian's `composer` package depends on the distribution's php-cli, which
	// would drag the base series back in beside 8.4.
	if strings.Contains(dockerfile, "'composer'") {
		t.Error("a series image must not install Debian's composer package")
	}
	if strings.Contains(dockerfile, "'php-cli'") {
		t.Error("a series image must not install the unversioned php-cli package")
	}
	if strings.Contains(dockerfile, "mariadb-server") == false {
		t.Error("the full profile must keep providing the database it promises")
	}
}

func TestSandboxDockerfileWithoutPHPKeepsTheBaseImage(t *testing.T) {
	// Two lists or a half-migrated render would put the base series and the
	// requested one in the same image.
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxProfilePHP, "", deploy.SandboxRequirements{
		Extensions: []string{"intl"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"'php-cli'", "'composer'", "'php-intl'"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("without --php the image keeps the distribution packages, missing %q", want)
		}
	}
	for _, absent := range []string{"sury", "update-alternatives", "composer.phar"} {
		if strings.Contains(dockerfile, absent) {
			t.Errorf("without --php the image must not contain %q", absent)
		}
	}
}

func TestSandboxDockerfileIsParseable(t *testing.T) {
	// The renderer builds the file out of Go raw strings, where a `\n` written
	// for a newline stays two characters: the build then dies with "unknown
	// instruction: \nRUN" — a failure the unit suite must catch, because the
	// only other place it shows up is a `sandbox up` on a developer's machine.
	//
	// This is the structural invariant the Dockerfile parser enforces: every
	// line either opens an instruction or continues the one before it.
	instructions := map[string]bool{
		"FROM": true, "ARG": true, "ENV": true, "RUN": true, "USER": true,
		"WORKDIR": true, "COPY": true, "ADD": true, "EXPOSE": true,
		"CMD": true, "ENTRYPOINT": true, "VOLUME": true, "LABEL": true,
	}
	for _, profile := range []string{deploy.SandboxProfileBasic, deploy.SandboxProfilePHP, deploy.SandboxProfileFull} {
		for _, php := range []string{"", "8.4"} {
			dockerfile, err := deploy.SandboxDockerfile(profile, php, deploy.SandboxRequirements{
				Packages:   []string{"libxslt1-dev"},
				Extensions: []string{"intl"},
				Services:   []string{"mariadb"},
			})
			if err != nil {
				t.Fatalf("render %s/%q: %v", profile, php, err)
			}
			if strings.Contains(dockerfile, `\nRUN`) {
				t.Errorf("the %s/%q render carries a literal backslash-n before an instruction, so Docker rejects the file", profile, php)
			}
			continuing := false
			for number, line := range strings.Split(dockerfile, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				if continuing {
					continuing = strings.HasSuffix(line, "\\")
					continue
				}
				// A comment is always at instruction level, and is the one line
				// that is not required to name an instruction.
				if strings.HasPrefix(trimmed, "#") {
					continue
				}
				keyword, _, _ := strings.Cut(trimmed, " ")
				if !instructions[keyword] {
					t.Fatalf("the %s/%q render has an instruction Docker does not know on line %d: %q",
						profile, php, number+1, line)
				}
				continuing = strings.HasSuffix(line, "\\")
			}
		}
	}
}

func TestSandboxBasicProfileNeverGetsPHP(t *testing.T) {
	// `basic` is the profile that promises no PHP at all; a series must not turn
	// it into a PHP image, and the composer phar step must not run without an
	// interpreter to invoke it.
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxProfileBasic, "8.4", deploy.SandboxRequirements{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, absent := range []string{"'php8.4-cli'", "composer.phar", "/usr/bin/php"} {
		if strings.Contains(dockerfile, absent) {
			t.Errorf("the basic profile must not contain %q, got:\n%s", absent, dockerfile)
		}
	}
}

func TestSandboxImageTagTracksThePHPSeries(t *testing.T) {
	// The image tag is the hash of the rendered Dockerfile, so a series that did
	// not move the tag would silently reuse an image built for another PHP.
	tagFor := func(php string) string {
		t.Helper()
		tag, err := deploy.SandboxImageTag(deploy.SandboxSpec{
			Project: "sample-project",
			Profile: deploy.SandboxProfilePHP,
			PHP:     php,
		})
		if err != nil {
			t.Fatalf("tag for %q: %v", php, err)
		}
		return tag
	}
	base, eightThree, eightFour := tagFor(""), tagFor("8.3"), tagFor("8.4")
	if base == eightFour {
		t.Fatal("asking for 8.4 reused the base image's tag")
	}
	if eightThree == eightFour {
		t.Fatal("two PHP series share an image tag")
	}
	if again := tagFor("8.4"); again != eightFour {
		t.Fatalf("the tag is not deterministic for one series: %q vs %q", eightFour, again)
	}
}

func TestSandboxUpRefusesAnUnsupportedPHPBeforeTouchingTheRuntime(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()

	_, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		PHP:         "8.4; rm -rf /",
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	})
	if err == nil {
		t.Fatal("want an error for a value that is not a PHP series")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("an invalid series reached the container runtime: %v", fake.calls)
	}
}

func TestSandboxUpDeclaresThePHPItShipped(t *testing.T) {
	// The pipeline refuses to run when the target's PHP does not match the
	// series the connection declares, so an image pinned to 8.4 whose remote
	// still said nothing (or 8.2) would fail its very first check — the failure
	// the flag exists to avoid.
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfileFull,
		PHP:         "8.4",
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}

	remote, set, err := deploy.SandboxRemoteForTest(root, "sandbox")
	if err != nil {
		t.Fatalf("read the sandbox remote: %v", err)
	}
	if !set || remote.Deploy == nil {
		t.Fatal("up did not write a sandbox remote with deploy settings")
	}
	if got := remote.Deploy.Settings["php_version"]; got != "8.4" {
		t.Fatalf("php_version = %v, want the series the image was built with", got)
	}
	if got := remote.Deploy.Settings["php_bin"]; got != "php" {
		t.Fatalf("php_bin = %v, want the plain binary the image alternates to", got)
	}

	// The rendered Dockerfile on disk is what --recreate rebuilds from, so it
	// must be the same image the tag was computed for.
	content := mustReadFile(t, deploy.SandboxDockerfilePath(root))
	if !strings.Contains(string(content), "'php8.4-cli'") {
		t.Fatalf("the written Dockerfile does not install the requested series:\n%s", content)
	}
	state, err := deploy.SandboxStatus(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if state.PHP != "8.4" {
		t.Fatalf("status reports php %q, want it read back from the remote", state.PHP)
	}
}

func TestSandboxUpWithoutPHPDeclaresNoSeries(t *testing.T) {
	// A project that never asked for a series must not be gated on one: the
	// image keeps the distribution's PHP and the remote keeps quiet about it.
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"

	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}

	remote, _, err := deploy.SandboxRemoteForTest(root, "sandbox")
	if err != nil {
		t.Fatalf("read the sandbox remote: %v", err)
	}
	if _, ok := remote.Deploy.Settings["php_version"]; ok {
		t.Fatal("up declared a PHP series nobody asked for")
	}
	if got := remote.Deploy.Settings["php_bin"]; got != "php" {
		t.Fatalf("php_bin = %v, want the profile's own binary", got)
	}
}
