package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/django"
)

func TestDjangoBootstrapCapabilities(t *testing.T) {
	b := django.NewDjangoBootstrap(bootstrap.Options{})
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
	b := django.NewDjangoBootstrap(bootstrap.Options{})
	if cmds := b.FreshCommands(); len(cmds) == 0 {
		t.Error("expected FreshCommands() to be non-empty now that fresh-install is supported")
	}
}

func TestDjangoBootstrapInstallUsesContainerExecRunner(t *testing.T) {
	var gotContainer, gotScript string
	restore := django.SetDjangoContainerExecRunnerForTest(func(containerName, script string) error {
		gotContainer = containerName
		gotScript = script
		return nil
	})
	defer restore()

	b := django.NewDjangoBootstrap(bootstrap.Options{ProjectName: "sample-project"})
	if err := b.Install(t.TempDir()); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if gotContainer != "sample-project-web-1" {
		t.Errorf("containerName = %q, want %q", gotContainer, "sample-project-web-1")
	}
	wantScript := "pip install --no-cache-dir -r requirements.txt && python manage.py migrate; rc=$?; chown -R \"$(stat -c %u:%g .)\" . 2>/dev/null; exit $rc"
	if gotScript != wantScript {
		t.Errorf("script = %q, want %q", gotScript, wantScript)
	}
}

func TestWriteDjangoRequirementsPinnedVersion(t *testing.T) {
	projectDir := t.TempDir()
	if err := django.WriteDjangoRequirementsForTest(projectDir, "5.1"); err != nil {
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
	if err := django.WriteDjangoRequirementsForTest(projectDir, ""); err != nil {
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

	if err := django.PatchDjangoSettingsForPostgresForTest(settingsPath); err != nil {
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

	if err := django.PatchDjangoSettingsForPostgresForTest(settingsPath); err != nil {
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

	if err := django.PatchDjangoSettingsForPostgresForTest(settingsPath); err != nil {
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

	err := django.PatchDjangoSettingsForPostgresForTest(settingsPath)
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

	if err := django.PatchDjangoSettingsForDomainForTest(settingsPath, "django.test"); err != nil {
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

	if err := django.PatchDjangoSettingsForDomainForTest(settingsPath, ""); err == nil {
		t.Fatal("expected error when domain is empty")
	}
}

func TestPatchDjangoSettingsForDomainErrorsWhenLineMissing(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.py")
	if err := os.WriteFile(settingsPath, []byte("# no allowed hosts line here\n"), 0o644); err != nil {
		t.Fatalf("write fixture settings.py: %v", err)
	}

	if err := django.PatchDjangoSettingsForDomainForTest(settingsPath, "django.test"); err == nil {
		t.Fatal("expected error when default ALLOWED_HOSTS line is not found")
	}
}

func TestBootstrapPkgDjangoFreshInstallSupport(t *testing.T) {
	opts := bootstrap.Options{}
	djangoBootstrap := django.NewDjangoBootstrap(opts)

	if !djangoBootstrap.SupportsFreshInstall() {
		t.Error("expected Django to support fresh install")
	}
	if !djangoBootstrap.SupportsClone() {
		t.Error("expected Django to support clone")
	}
}

func TestDjangoCreateProjectWithRunnerStagesStartProject(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".govard.yml"), []byte("project_name: sample-project\n"), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}

	var capturedCommand string
	djangoBootstrap := django.NewDjangoBootstrap(bootstrap.Options{
		Version: "5.1",
		Domain:  "sample.test",
		Runner: func(command string) error {
			capturedCommand = command
			stageDir := extractStageHostDir(t, command)
			if err := os.WriteFile(filepath.Join(stageDir, "manage.py"), []byte("#!/usr/bin/env python\n"), 0o644); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(stageDir, "config"), 0o755); err != nil {
				return err
			}
			settingsContent := "from pathlib import Path\n\nBASE_DIR = Path(__file__).resolve().parent.parent\n\nALLOWED_HOSTS = []\n\nDATABASES = {\n    'default': {\n        'ENGINE': 'django.db.backends.sqlite3',\n        'NAME': BASE_DIR / 'db.sqlite3',\n    }\n}\n"
			return os.WriteFile(filepath.Join(stageDir, "config", "settings.py"), []byte(settingsContent), 0o644)
		},
	})

	if err := djangoBootstrap.CreateProject(projectDir); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	if !strings.Contains(capturedCommand, `pip install --no-cache-dir 'Django==5.1'`) {
		t.Fatalf("unexpected runner command: %s", capturedCommand)
	}
	if !strings.Contains(capturedCommand, `django-admin startproject config "$GOVARD_STAGE_DIR"`) {
		t.Fatalf("unexpected runner command: %s", capturedCommand)
	}
	if !strings.Contains(capturedCommand, "GOVARD_STAGE_DIR='/app/") {
		t.Fatalf("expected staged dir under PythonWorkDir (/app), got: %s", capturedCommand)
	}

	if _, err := os.Stat(filepath.Join(projectDir, "manage.py")); err != nil {
		t.Fatalf("expected staged manage.py to be copied into project dir: %v", err)
	}

	reqContent, err := os.ReadFile(filepath.Join(projectDir, "requirements.txt"))
	if err != nil {
		t.Fatalf("read requirements.txt: %v", err)
	}
	if string(reqContent) != "Django==5.1\npsycopg2-binary\n" {
		t.Fatalf("requirements.txt = %q", string(reqContent))
	}

	settingsContent, err := os.ReadFile(filepath.Join(projectDir, "config", "settings.py"))
	if err != nil {
		t.Fatalf("read settings.py: %v", err)
	}
	if !strings.Contains(string(settingsContent), "django.db.backends.postgresql") {
		t.Fatalf("expected settings.py to be patched for postgres, got:\n%s", string(settingsContent))
	}
	if !strings.Contains(string(settingsContent), "ALLOWED_HOSTS = ['sample.test', 'localhost', '127.0.0.1']") {
		t.Fatalf("expected settings.py ALLOWED_HOSTS to include the project domain, got:\n%s", string(settingsContent))
	}
	if !strings.Contains(string(settingsContent), "CSRF_TRUSTED_ORIGINS = ['https://sample.test']") {
		t.Fatalf("expected settings.py CSRF_TRUSTED_ORIGINS for the project domain, got:\n%s", string(settingsContent))
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".govard.yml")); err != nil {
		t.Fatalf("expected .govard.yml to be preserved: %v", err)
	}
}
