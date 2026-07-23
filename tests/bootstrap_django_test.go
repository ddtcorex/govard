package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
)

func TestDjangoBootstrapCapabilities(t *testing.T) {
	b := bootstrap.NewDjangoBootstrap(bootstrap.Options{})
	if b.Name() != "django" {
		t.Errorf("Name() = %q, want %q", b.Name(), "django")
	}
	if !b.SupportsFreshInstall() {
		t.Error("expected SupportsFreshInstall() to be true")
	}
	if !b.SupportsClone() {
		t.Error("expected SupportsClone() to be true")
	}
}

func TestDjangoBootstrapFreshCommandsNotEmpty(t *testing.T) {
	b := bootstrap.NewDjangoBootstrap(bootstrap.Options{})
	if cmds := b.FreshCommands(); len(cmds) == 0 {
		t.Error("expected FreshCommands() to be non-empty now that fresh-install is supported")
	}
}

func TestDjangoBootstrapInstallUsesContainerExecRunner(t *testing.T) {
	var gotContainer, gotScript string
	restore := bootstrap.SetDjangoContainerExecRunnerForTest(func(containerName, script string) error {
		gotContainer = containerName
		gotScript = script
		return nil
	})
	defer restore()

	b := bootstrap.NewDjangoBootstrap(bootstrap.Options{ProjectName: "sample-project"})
	if err := b.Install(t.TempDir()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if gotContainer != "sample-project-web-1" {
		t.Errorf("containerName = %q, want %q", gotContainer, "sample-project-web-1")
	}
	if gotScript != "pip install --no-cache-dir -r requirements.txt && python manage.py migrate" {
		t.Errorf("script = %q", gotScript)
	}
}

func TestWriteDjangoRequirementsPinnedVersion(t *testing.T) {
	projectDir := t.TempDir()
	if err := bootstrap.WriteDjangoRequirementsForTest(projectDir, "5.1"); err != nil {
		t.Fatalf("WriteDjangoRequirementsForTest() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectDir, "requirements.txt"))
	if err != nil {
		t.Fatalf("read requirements.txt: %v", err)
	}
	want := "Django==5.1\npsycopg2-binary\n"
	if string(content) != want {
		t.Errorf("requirements.txt = %q, want %q", string(content), want)
	}
}

func TestWriteDjangoRequirementsUnpinnedVersion(t *testing.T) {
	projectDir := t.TempDir()
	if err := bootstrap.WriteDjangoRequirementsForTest(projectDir, ""); err != nil {
		t.Fatalf("WriteDjangoRequirementsForTest() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectDir, "requirements.txt"))
	if err != nil {
		t.Fatalf("read requirements.txt: %v", err)
	}
	want := "Django\npsycopg2-binary\n"
	if string(content) != want {
		t.Errorf("requirements.txt = %q, want %q", string(content), want)
	}
}

func TestPatchDjangoSettingsForPostgresRewritesDatabasesBlock(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.py")
	original := `"""
Django settings.
"""

from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent

# Database
# https://docs.djangoproject.com/en/5.1/ref/settings/#databases

DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.sqlite3',
        'NAME': BASE_DIR / 'db.sqlite3',
    }
}
`
	if err := os.WriteFile(settingsPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture settings.py: %v", err)
	}

	if err := bootstrap.PatchDjangoSettingsForPostgresForTest(settingsPath); err != nil {
		t.Fatalf("PatchDjangoSettingsForPostgresForTest() error = %v", err)
	}

	patched, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read patched settings.py: %v", err)
	}
	content := string(patched)

	if !strings.HasPrefix(content, "\"\"\"") {
		t.Errorf("expected docstring to remain at the start of the file, got:\n%s", content)
	}
	if !strings.Contains(content, "import os\n") {
		t.Errorf("expected `import os` to be present in file, got:\n%s", content)
	}
	// Verify import os comes AFTER the docstring, not before it
	docstringClose := strings.Index(content, "\"\"\"")
	if docstringClose == -1 {
		t.Fatalf("expected docstring with closing triple quotes, got:\n%s", content)
	}
	// Find the end of the closing triple quotes
	docstringEnd := docstringClose + 3
	importOsPos := strings.Index(content, "import os")
	if importOsPos < docstringEnd {
		t.Errorf("expected `import os` to come after the docstring, but it appears before. Content:\n%s", content)
	}
	if !strings.Contains(content, "from pathlib import Path") {
		t.Errorf("expected pathlib import to remain untouched, got:\n%s", content)
	}
	if !strings.Contains(content, "'ENGINE': 'django.db.backends.postgresql'") {
		t.Errorf("expected postgres engine, got:\n%s", content)
	}
	if !strings.Contains(content, "os.environ.get('POSTGRES_HOST', 'db')") {
		t.Errorf("expected POSTGRES_HOST env lookup with 'db' default, got:\n%s", content)
	}
	if strings.Contains(content, "django.db.backends.sqlite3") {
		t.Errorf("expected sqlite engine to be replaced, got:\n%s", content)
	}
}

// TestPatchDjangoSettingsForPostgresInsertsImportOsWithoutPathlibAnchor
// covers the case where the sqlite DATABASES block this function matches on
// is present, but the `from pathlib import Path` line it used to anchor the
// `import os` insertion on is not (e.g. a future Django template change).
// The insertion must not silently no-op in that case.
func TestPatchDjangoSettingsForPostgresInsertsImportOsWithoutPathlibAnchor(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.py")
	original := `"""
Django settings.
"""

BASE_DIR = get_base_dir()

DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.sqlite3',
        'NAME': BASE_DIR / 'db.sqlite3',
    }
}
`
	if err := os.WriteFile(settingsPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture settings.py: %v", err)
	}

	if err := bootstrap.PatchDjangoSettingsForPostgresForTest(settingsPath); err != nil {
		t.Fatalf("PatchDjangoSettingsForPostgresForTest() error = %v", err)
	}

	patched, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read patched settings.py: %v", err)
	}
	content := string(patched)

	if !strings.HasPrefix(content, "\"\"\"") {
		t.Errorf("expected docstring to remain at the start of the file, got:\n%s", content)
	}
	if !strings.Contains(content, "import os\n") {
		t.Errorf("expected `import os` to be present in file, got:\n%s", content)
	}
	// Verify import os comes AFTER the docstring, not before it
	docstringClose := strings.Index(content, "\"\"\"")
	if docstringClose == -1 {
		t.Fatalf("expected docstring with closing triple quotes, got:\n%s", content)
	}
	// Find the end of the closing triple quotes
	docstringEnd := docstringClose + 3
	importOsPos := strings.Index(content, "import os")
	if importOsPos < docstringEnd {
		t.Errorf("expected `import os` to come after the docstring, but it appears before. Content:\n%s", content)
	}
	if !strings.Contains(content, "'ENGINE': 'django.db.backends.postgresql'") {
		t.Errorf("expected postgres engine, got:\n%s", content)
	}
}

// TestPatchDjangoSettingsForPostgresPreservesDocstringAsModuleDocstring
// verifies that when a module-level docstring is present (triple-quoted string
// as the first statement), import os is inserted AFTER the docstring, not before,
// so the docstring remains a true module docstring (recognized by Python as __doc__)
// and is not demoted to a dead expression statement.
func TestPatchDjangoSettingsForPostgresPreservesDocstringAsModuleDocstring(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.py")
	// Realistic fixture matching Django 5.1's django-admin startproject output
	original := `"""
Django settings for config project.

Generated by 'django-admin startproject' using Django 5.1.

For more information on this file, see
https://docs.djangoproject.com/en/5.1/topics/settings/

For the full list of settings and their values, see
https://docs.djangoproject.com/en/5.1/ref/settings/
"""

from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent

SECRET_KEY = 'django-insecure-test-key'
DEBUG = True
ALLOWED_HOSTS = []
INSTALLED_APPS = [
    'django.contrib.admin',
    'django.contrib.auth',
]
DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.sqlite3',
        'NAME': BASE_DIR / 'db.sqlite3',
    }
}
`
	if err := os.WriteFile(settingsPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture settings.py: %v", err)
	}

	if err := bootstrap.PatchDjangoSettingsForPostgresForTest(settingsPath); err != nil {
		t.Fatalf("PatchDjangoSettingsForPostgresForTest() error = %v", err)
	}

	patched, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read patched settings.py: %v", err)
	}
	content := string(patched)

	// Docstring must remain at the very start
	if !strings.HasPrefix(content, "\"\"\"") {
		t.Errorf("docstring must remain at the start of file, got:\n%s", content)
	}

	// import os must be present
	if !strings.Contains(content, "import os") {
		t.Errorf("expected `import os` to be present, got:\n%s", content)
	}

	// import os must appear AFTER the closing """ of the docstring
	// Find the position of the closing triple quotes (second occurrence of """)
	firstQuote := strings.Index(content, "\"\"\"")
	if firstQuote == -1 {
		t.Fatalf("expected closing \"\"\" in docstring, got:\n%s", content)
	}
	secondQuote := strings.Index(content[firstQuote+3:], "\"\"\"")
	if secondQuote == -1 {
		t.Fatalf("expected closing \"\"\" in docstring, got:\n%s", content)
	}
	docstringEnd := firstQuote + 3 + secondQuote + 3
	importOsPos := strings.Index(content, "import os")
	if importOsPos < docstringEnd {
		t.Errorf("import os (at pos %d) must come after docstring close (at pos %d), got:\n%s", importOsPos, docstringEnd, content)
	}

	// The docstring content itself must be untouched
	if !strings.Contains(content, "Django settings for config project") {
		t.Errorf("expected docstring content to be preserved, got:\n%s", content)
	}

	// Database block must be rewritten to PostgreSQL
	if !strings.Contains(content, "'ENGINE': 'django.db.backends.postgresql'") {
		t.Errorf("expected PostgreSQL engine, got:\n%s", content)
	}
	if !strings.Contains(content, "os.environ.get('POSTGRES_DB'") {
		t.Errorf("expected POSTGRES_DB env lookup, got:\n%s", content)
	}
	if strings.Contains(content, "django.db.backends.sqlite3") {
		t.Errorf("expected sqlite3 to be removed, got:\n%s", content)
	}
}

func TestPatchDjangoSettingsForPostgresErrorsWhenBlockMissing(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.py")
	if err := os.WriteFile(settingsPath, []byte("# no databases block here\n"), 0o644); err != nil {
		t.Fatalf("write fixture settings.py: %v", err)
	}

	err := bootstrap.PatchDjangoSettingsForPostgresForTest(settingsPath)
	if err == nil {
		t.Fatal("expected error when default sqlite DATABASES block is not found")
	}
}

func TestPatchDjangoSettingsForDomainRewritesAllowedHosts(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.py")
	original := "DEBUG = True\n\nALLOWED_HOSTS = []\n\n\n# Application definition\n"
	if err := os.WriteFile(settingsPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture settings.py: %v", err)
	}

	if err := bootstrap.PatchDjangoSettingsForDomainForTest(settingsPath, "django.test"); err != nil {
		t.Fatalf("PatchDjangoSettingsForDomainForTest() error = %v", err)
	}

	patched, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read patched settings.py: %v", err)
	}
	content := string(patched)

	if !strings.Contains(content, "ALLOWED_HOSTS = ['django.test', 'localhost', '127.0.0.1']") {
		t.Errorf("expected ALLOWED_HOSTS to include the project domain, got:\n%s", content)
	}
	if !strings.Contains(content, "CSRF_TRUSTED_ORIGINS = ['https://django.test']") {
		t.Errorf("expected CSRF_TRUSTED_ORIGINS for the project domain, got:\n%s", content)
	}
	if strings.Contains(content, "ALLOWED_HOSTS = []") {
		t.Errorf("expected default empty ALLOWED_HOSTS to be replaced, got:\n%s", content)
	}
}

func TestPatchDjangoSettingsForDomainErrorsWhenDomainEmpty(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.py")
	if err := os.WriteFile(settingsPath, []byte("ALLOWED_HOSTS = []\n"), 0o644); err != nil {
		t.Fatalf("write fixture settings.py: %v", err)
	}

	if err := bootstrap.PatchDjangoSettingsForDomainForTest(settingsPath, ""); err == nil {
		t.Fatal("expected error when domain is empty")
	}
}

func TestPatchDjangoSettingsForDomainErrorsWhenLineMissing(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.py")
	if err := os.WriteFile(settingsPath, []byte("# no allowed hosts line here\n"), 0o644); err != nil {
		t.Fatalf("write fixture settings.py: %v", err)
	}

	if err := bootstrap.PatchDjangoSettingsForDomainForTest(settingsPath, "django.test"); err == nil {
		t.Fatal("expected error when default ALLOWED_HOSTS line is not found")
	}
}
