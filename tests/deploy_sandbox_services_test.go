package tests

import (
	"strings"
	"testing"

	"govard/internal/deploy"
)

// A service the distribution gives no init script used to be skipped in silence:
// the entrypoint started what it had a script for and said nothing about the
// rest, so a project asking for a database found out from a migration step much
// later. It now says so.
//
// The entrypoint is not run here — that needs a container — so the assertion is
// on the rendered script. The end-to-end half (PostgreSQL actually answering)
// is rehearsal evidence, not a unit test.
func TestSandboxEntrypointReportsAServiceItCannotStart(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfileFull,
		PHP:          "8.2",
		Requirements: deploy.SandboxRequirements{Services: []string{"postgresql", "redis-server"}},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(dockerfile, "no init script for the service") {
		t.Errorf("the entrypoint must name a service it could not start:\n%s", dockerfile)
	}
	if !strings.Contains(dockerfile, "1>&2") && !strings.Contains(dockerfile, ">&2") {
		t.Errorf("the report must go to stderr, where it is not mixed into the application's output:\n%s", dockerfile)
	}
}

// Debian bookworm ships PostgreSQL through systemd only, so the image has to
// write the init script the entrypoint looks for.
func TestSandboxWritesTheInitScriptForAServiceThatHasNone(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfileFull,
		PHP:          "8.2",
		Requirements: deploy.SandboxRequirements{Services: []string{"postgresql"}},
	})
	if err != nil {
		t.Fatalf("render with postgresql: %v", err)
	}
	for _, want := range []string{"/etc/init.d/postgresql", "pg_ctlcluster"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("the rendered image does not write %q:\n%s", want, dockerfile)
		}
	}
	// The script is written by `printf` from single-quoted words, so nothing in
	// it is expanded while the image is built.
	if !strings.Contains(dockerfile, `'version="$(ls /etc/postgresql | head -n1)"'`) {
		t.Errorf("the init script must be written verbatim:\n%s", dockerfile)
	}
}

// A service named without its package is a service that never starts, so the
// name carries the package.
func TestSandboxInstallsThePackageAServiceNeeds(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfileFull,
		PHP:          "8.2",
		Requirements: deploy.SandboxRequirements{Services: []string{"postgresql"}},
	})
	if err != nil {
		t.Fatalf("render with postgresql: %v", err)
	}
	if !strings.Contains(dockerfile, "'postgresql'") {
		t.Errorf("the image does not install the server: %s", firstLineContaining(dockerfile, "apt-get install"))
	}
}

// The default requirements must not grow an init script: a profile that provides
// its own services renders no write, which keeps every existing image unchanged.
func TestSandboxWithoutTableServicesWritesNoInitScript(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfileFull,
		PHP:          "8.2",
		Requirements: deploy.SandboxRequirements{Services: []string{"mariadb", "redis-server"}},
	})
	if err != nil {
		t.Fatalf("render the default services: %v", err)
	}
	if strings.Contains(dockerfile, "pg_ctlcluster") {
		t.Errorf("an unrequested service must not be written into the image:\n%s", dockerfile)
	}
}

// A service the profile cannot start stays renderable on purpose: a recipe
// declares what the framework needs, and the profile decides what is provided —
// `--profile php` on a Magento project is a documented, weaker rehearsal rather
// than a configuration error. What changed is that it is no longer silent.
func TestSandboxRendersAServiceTheProfileCannotStart(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfilePHP,
		PHP:          "8.2",
		Requirements: deploy.SandboxRequirements{Services: []string{"mariadb", "redis-server"}},
	})
	if err != nil {
		t.Fatalf("the php profile must still render a recipe's database requirement: %v", err)
	}
	if !strings.Contains(dockerfile, `GOVARD_SANDBOX_SERVICES="govard-sandbox-web mariadb redis-server"`) {
		t.Errorf("the services must reach the container even when the profile cannot start them:\n%s", dockerfile)
	}
}

// firstLineContaining returns the first line that mentions want, for a failure
// message that shows what the image actually installs.
func firstLineContaining(text, want string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	return "(no line mentions " + want + ")"
}

// sandboxServicesEnv returns the ENV line that tells the entrypoint what to
// start, so a test asserts on the list rather than on a substring of the image.
func sandboxServicesEnv(t *testing.T, dockerfile string) string {
	t.Helper()
	for _, line := range strings.Split(dockerfile, "\n") {
		if strings.Contains(line, "GOVARD_SANDBOX_SERVICES=") {
			return line
		}
	}
	t.Fatalf("the rendered image carries no GOVARD_SANDBOX_SERVICES line:\n%s", dockerfile)
	return ""
}

// The `full` profile installs a database server and a cache. The entrypoint
// starts only what it is named, so naming nothing but the web tier is how a
// sandbox came up with both packages installed and neither server running:
// `sandbox up` then waited out its database timeout and failed, while sshd
// answered the whole time.
//
// Found live: an image rendered for a project whose framework declares no
// sandbox services (the default recipe) carried
// GOVARD_SANDBOX_SERVICES="govard-sandbox-web" in a full-profile build.
func TestSandboxFullProfileStartsTheDatabaseAndCacheItInstalls(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile: deploy.SandboxProfileFull,
		PHP:     "8.2",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	env := sandboxServicesEnv(t, dockerfile)
	for _, want := range []string{"mariadb", "redis-server"} {
		if !strings.Contains(env, want) {
			t.Errorf("the full profile installs %s but does not start it: %s", want, env)
		}
	}
}

// A project that sets `sandbox_services` itself, including to an empty list,
// has said what to start; the profile adds nothing on top.
func TestSandboxExplicitServiceListIsNotExtendedByTheProfile(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfileFull,
		PHP:          "8.2",
		Requirements: deploy.SandboxRequirements{ServicesExplicit: true},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	env := sandboxServicesEnv(t, dockerfile)
	for _, unwanted := range []string{"mariadb", "redis-server"} {
		if strings.Contains(env, unwanted) {
			t.Errorf("an explicit service list was extended with %s: %s", unwanted, env)
		}
	}
}

// The settings layer is what knows a list was set by the project rather than
// defaulted by the recipe.
func TestSandboxSettingsMarkAnExplicitServiceList(t *testing.T) {
	withKey := deploy.SandboxRequirementsWithSettings(deploy.SandboxRequirements{}, map[string]any{"sandbox_services": []any{}})
	if !withKey.ServicesExplicit {
		t.Error("sandbox_services present as an empty list must count as explicit")
	}
	withoutKey := deploy.SandboxRequirementsWithSettings(deploy.SandboxRequirements{}, map[string]any{})
	if withoutKey.ServicesExplicit {
		t.Error("no sandbox_services key must not count as explicit")
	}
}
