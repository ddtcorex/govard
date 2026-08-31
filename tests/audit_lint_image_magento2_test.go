package tests

import (
	"io/fs"
	"reflect"
	"regexp"
	"strings"
	"testing"

	auditmagento "govard/docker/audit"
	"govard/internal/frameworks"
)

// The Magento lint image, its runner, its contract suite, and the Magento
// framework profile must all describe the same PHP policy. Drift is otherwise
// silent: a version the profile selects but the image does not support yields
// a well-formed report full of "unsupported" outcomes instead of an error.

func TestAuditLintImageMatchesMagentoAuditPHPPolicy(t *testing.T) {
	definition, ok := frameworks.Get("magento2")
	if !ok {
		t.Fatal("framework magento2 is not registered")
	}
	if definition.AuditLint == nil {
		t.Fatal("magento2 exposes no audit lint profile")
	}
	if !reflect.DeepEqual(auditmagento.PHPVersions(), definition.AuditLint.ProjectPHPVersions) {
		t.Fatalf("image PHP versions %#v do not match the Magento project policy %#v",
			auditmagento.PHPVersions(), definition.AuditLint.ProjectPHPVersions)
	}
	for _, standalone := range definition.AuditLint.StandalonePHPVersions {
		if !containsVersion(auditmagento.PHPVersions(), standalone) {
			t.Fatalf("standalone policy version %q is not provided by the image", standalone)
		}
	}
}

func TestAuditLintImageContextDeclaresOnePHPPolicy(t *testing.T) {
	definition, ok := frameworks.Get("magento2")
	if !ok {
		t.Fatal("framework magento2 is not registered")
	}
	wantSupported := auditmagento.PHPVersions()
	wantStandalone := definition.AuditLint.StandalonePHPVersions

	dockerfile := readContextFile(t, "Dockerfile")
	runner := readContextFile(t, "bin/glint")
	contract := readContextFile(t, "tests/contract_test.sh")

	for _, declaration := range []struct {
		name    string
		content string
		pattern string
		want    []string
	}{
		{"Dockerfile ARG GOVARD_GLINT_PHP_VERSIONS", dockerfile, `(?m)^ARG GOVARD_GLINT_PHP_VERSIONS="([^"]+)"`, wantSupported},
		{"runner SUPPORTED_PHP_VERSIONS", runner, `(?m)^SUPPORTED_PHP_VERSIONS="([^"]+)"`, wantSupported},
		{"runner STANDALONE_PHP_VERSIONS", runner, `(?m)^STANDALONE_PHP_VERSIONS="([^"]+)"`, wantStandalone},
		{"contract suite SUPPORTED_MATRIX", contract, `(?m)^SUPPORTED_MATRIX="([^"]+)"`, wantSupported},
	} {
		got := matchVersionList(t, declaration.name, declaration.content, declaration.pattern)
		if !reflect.DeepEqual(got, declaration.want) {
			t.Errorf("%s declares %#v, want %#v", declaration.name, got, declaration.want)
		}
	}

	// The image label must derive from the single Dockerfile declaration
	// rather than repeat the list.
	if !strings.Contains(dockerfile, `io.govard.audit.php-versions="${GOVARD_GLINT_PHP_VERSIONS}"`) {
		t.Error("the php-versions label is not derived from GOVARD_GLINT_PHP_VERSIONS")
	}
	// The policy has a floor and a ceiling. Versions just outside either end
	// must never drift in: 7.3 predates the oldest toolchain Govard ships, and
	// 8.6 does not exist yet, so neither can have a locked toolchain.
	for _, outside := range []string{"7.3", "8.6"} {
		if containsVersion(wantSupported, outside) {
			t.Errorf("PHP %s must not be part of the supported policy", outside)
		}
	}
	// Every supported version must ship a committed toolchain manifest, so a
	// version cannot be added to the policy without its locked analyzers.
	for _, supported := range wantSupported {
		for _, name := range []string{"composer.json", "composer.lock"} {
			path := "toolchains/php-" + supported + "/" + name
			if _, err := fs.ReadFile(auditmagento.ContextFS, path); err != nil {
				t.Errorf("supported version %s has no %s in the build context: %v", supported, name, err)
			}
		}
	}
}

func readContextFile(t *testing.T, path string) string {
	t.Helper()
	content, err := fs.ReadFile(auditmagento.ContextFS, path)
	if err != nil {
		t.Fatalf("read embedded context %q: %v", path, err)
	}
	return string(content)
}

func matchVersionList(t *testing.T, name, content, pattern string) []string {
	t.Helper()
	matches := regexp.MustCompile(pattern).FindStringSubmatch(content)
	if len(matches) != 2 {
		t.Fatalf("%s is not declared", name)
	}
	return strings.FieldsFunc(matches[1], func(character rune) bool {
		return character == ',' || character == ' '
	})
}

func containsVersion(versions []string, want string) bool {
	for _, version := range versions {
		if version == want {
			return true
		}
	}
	return false
}
