package tests

import (
	"errors"
	"strings"
	"testing"

	"govard/internal/deploy"
)

// A recipe's sandbox block is the framework's default, not a statement about a
// project. One project runs PostgreSQL and another runs MySQL; without a
// project-level override the first cannot be rehearsed at all.
func TestSandboxSettingsReplaceTheRecipeDefault(t *testing.T) {
	base := deploy.SandboxRequirements{
		Packages:   []string{"default-mysql-client"},
		Extensions: []string{"intl", "mysql"},
		Services:   []string{"mariadb", "redis-server"},
	}
	settings := map[string]any{
		"sandbox_extensions": []any{"intl", "pgsql"},
		"sandbox_services":   []string{"postgresql", "redis-server"},
	}

	resolved := deploy.SandboxRequirementsWithSettings(base, settings)

	if got := strings.Join(resolved.Services, ","); got != "postgresql,redis-server" {
		t.Errorf("services = %q, want the project's list", got)
	}
	if got := strings.Join(resolved.Extensions, ","); got != "intl,pgsql" {
		t.Errorf("extensions = %q, want the project's list", got)
	}
	// A key the project did not set keeps the recipe's value.
	if got := strings.Join(resolved.Packages, ","); got != "default-mysql-client" {
		t.Errorf("packages = %q, want the recipe's list", got)
	}
}

// Unset is not the same as empty: a project that writes an empty list means
// "none", and that has to survive the merge.
func TestSandboxSettingsDistinguishUnsetFromEmpty(t *testing.T) {
	base := deploy.SandboxRequirements{Services: []string{"mariadb"}}

	cleared := deploy.SandboxRequirementsWithSettings(base, map[string]any{"sandbox_services": []any{}})
	if len(cleared.Services) != 0 {
		t.Fatalf("an explicit empty list must clear the recipe's services, got %v", cleared.Services)
	}

	untouched := deploy.SandboxRequirementsWithSettings(base, map[string]any{"sandbox_packages": []string{"unzip"}})
	if strings.Join(untouched.Services, ",") != "mariadb" {
		t.Fatalf("setting one list must not clear another, got %v", untouched.Services)
	}
}

// The engine declares the keys, so `deploy plan` and `sandbox up` agree on what
// a project is allowed to write.
func TestSandboxSettingsAreDeclaredByTheEngine(t *testing.T) {
	recipe := deploy.DefaultRecipe()

	valid := map[string]any{
		"sandbox_packages":   []string{"default-mysql-client"},
		"sandbox_extensions": []string{"mysqli"},
		"sandbox_services":   []string{"postgresql"},
		"sandbox_tools":      []string{"wp-cli"},
	}
	if err := deploy.ValidateSettings(recipe, valid); err != nil {
		t.Fatalf("the engine must declare the sandbox settings: %v", err)
	}
	if err := deploy.ValidateSettings(recipe, map[string]any{"sandbox_servcies": []string{"postgresql"}}); err == nil {
		t.Fatal("a misspelled sandbox setting must be refused")
	}
}

// A package word lands in a Dockerfile RUN line, so it has to be escaped rather
// than merely wrapped: a word with an apostrophe used to close the quote and put
// the rest of the value into the line.
func TestSandboxDockerfileEscapesAQuoteInAPackageWord(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{
		Profile:      deploy.SandboxProfilePHP,
		PHP:          "8.4",
		Requirements: deploy.SandboxRequirements{Packages: []string{"it's"}},
	})
	if err != nil {
		t.Fatalf("render the Dockerfile: %v", err)
	}
	if strings.Contains(dockerfile, "'it's'") {
		t.Fatalf("the package word was wrapped without escaping:\n%s", dockerfile)
	}
}

// A newline cannot be quoted away inside a Dockerfile RUN line, so it is refused
// as a configuration error while the project is still being read.
func TestSandboxPackagesRefuseANewline(t *testing.T) {
	err := deploy.ValidateSettings(deploy.DefaultRecipe(), map[string]any{
		"sandbox_packages": []string{"a\nRUN id"},
	})
	if !errors.Is(err, deploy.ErrInvalidConfiguration) {
		t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
	}
	if !strings.Contains(err.Error(), "sandbox_packages") {
		t.Fatalf("the refusal must name the setting, got %q", err.Error())
	}
}
