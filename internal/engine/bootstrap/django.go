package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/conventions"

	"github.com/pterm/pterm"
)

type DjangoBootstrap struct {
	Options Options
}

func NewDjangoBootstrap(opts Options) *DjangoBootstrap {
	return &DjangoBootstrap{Options: opts}
}

func (d *DjangoBootstrap) Name() string {
	return "django"
}

func (d *DjangoBootstrap) SupportsFreshInstall() bool {
	return true
}

func (d *DjangoBootstrap) SupportsClone() bool {
	return true
}

func (d *DjangoBootstrap) FreshCommands() []string {
	return []string{
		"pip install " + djangoPipSpec(d.Options.Version),
		"django-admin startproject config .",
		"python manage.py migrate",
	}
}

func (d *DjangoBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh Django project...")

	djangoSpec := djangoPipSpec(d.Options.Version)
	createInStage := func(stageDir string) error {
		return createDjangoProjectInStage(stageDir, djangoSpec)
	}
	runnerCommand := "pip install --no-cache-dir " + conventions.ShellQuote(djangoSpec) + ` && django-admin startproject config "$GOVARD_STAGE_DIR"`
	if err := runStagedCreateProject(projectDir, d.Options.Runner, createInStage, runnerCommand, conventions.PythonWorkDir); err != nil {
		return fmt.Errorf("failed to create Django project: %w", err)
	}

	if err := writeDjangoRequirements(projectDir, d.Options.Version); err != nil {
		return fmt.Errorf("failed to write requirements.txt: %w", err)
	}

	settingsPath := filepath.Join(projectDir, "config", "settings.py")
	if err := patchDjangoSettingsForPostgres(settingsPath); err != nil {
		pterm.Warning.Printf("Could not configure Django settings.py for Postgres, leaving default config: %v\n", err)
	}
	if err := patchDjangoSettingsForDomain(settingsPath, d.Options.Domain); err != nil {
		pterm.Warning.Printf("Could not configure Django ALLOWED_HOSTS for domain, leaving default config: %v\n", err)
	}

	pterm.Success.Println("Django project created successfully")
	return nil
}

// createDjangoProjectInStage is the host-side fallback used only when
// Options.Runner is nil (no Docker runner configured, e.g. tests). It
// can't pin the Django version like the container path does - it uses
// whatever `django-admin` is already on the host's PATH.
func createDjangoProjectInStage(stageDir string, djangoSpec string) error {
	if _, err := exec.LookPath("django-admin"); err != nil {
		return fmt.Errorf("django-admin not found in PATH, cannot create Django project (wanted %s)", djangoSpec)
	}

	cmd := exec.Command("django-admin", "startproject", "config", stageDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (d *DjangoBootstrap) Install(projectDir string) error {
	return d.installAndMigrate()
}

func (d *DjangoBootstrap) Configure(projectDir string) error {
	pterm.Success.Println("Django configured successfully")
	return nil
}

func (d *DjangoBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned Django project...")
	if err := d.installAndMigrate(); err != nil {
		return err
	}
	pterm.Success.Println("Post-clone setup completed")
	return nil
}

// djangoContainerExecRunner execs a shell script inside the Django "web"
// container - overridable in tests via SetDjangoContainerExecRunnerForTest
// so Install()/PostClone() don't require a real Docker daemon.
var djangoContainerExecRunner = func(containerName string, script string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "sh", "-lc", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// installAndMigrate runs pip install + migrate inside the compose-managed
// "web" container, not the host - the container's Python/pip is the one
// that must match the project's requirements.txt, not whatever (if
// anything) is installed on the developer's machine. By the time
// PostClone runs, `env up` has already started containers (see
// internal/cmd/bootstrap.go's ordering), so the container is available.
func (d *DjangoBootstrap) installAndMigrate() error {
	containerName := d.Options.ProjectName + conventions.WebSuffix
	script := "pip install --no-cache-dir -r requirements.txt && python manage.py migrate"

	if err := djangoContainerExecRunner(containerName, script); err != nil {
		return fmt.Errorf("django pip install/migrate failed: %w", err)
	}
	return nil
}

const djangoDefaultSQLiteDatabases = `DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.sqlite3',
        'NAME': BASE_DIR / 'db.sqlite3',
    }
}`

const djangoPostgresDatabases = `DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.postgresql',
        'NAME': os.environ.get('POSTGRES_DB', 'django'),
        'USER': os.environ.get('POSTGRES_USER', 'django'),
        'PASSWORD': os.environ.get('POSTGRES_PASSWORD', 'django'),
        'HOST': os.environ.get('POSTGRES_HOST', 'db'),
        'PORT': os.environ.get('POSTGRES_PORT', '5432'),
    }
}`

// djangoPipSpec returns the pip requirement spec for Django: pinned to
// version when given (matching --framework-version), otherwise unpinned so
// pip resolves latest.
func djangoPipSpec(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "Django"
	}
	return "Django==" + version
}

// writeDjangoRequirements writes requirements.txt with Django pinned (or
// not) plus psycopg2-binary, which every fresh Django project needs to talk
// to the Postgres container Govard's compose file provisions.
func writeDjangoRequirements(projectDir string, version string) error {
	content := djangoPipSpec(version) + "\npsycopg2-binary\n"
	return os.WriteFile(filepath.Join(projectDir, "requirements.txt"), []byte(content), conventions.DefaultFilePerm)
}

// insertImportOsAfterDocstring inserts "import os\n" into content.
// If content starts with a module-level docstring (triple-quoted string),
// the import is inserted after the closing quotes to preserve the docstring
// as a true module docstring (recognized by Python as __doc__).
// Otherwise, it prepends to the very start.
func insertImportOsAfterDocstring(content string) string {
	trimmed := strings.TrimLeft(content, " \t\n\r")

	// Check if content starts with a triple-quoted docstring
	var quoteMarker string
	if strings.HasPrefix(trimmed, `"""`) {
		quoteMarker = `"""`
	} else if strings.HasPrefix(trimmed, "'''") {
		quoteMarker = "'''"
	} else {
		// No leading docstring; prepend to the very start
		return "import os\n" + content
	}

	// Find the closing triple quote (after the opening one)
	// Skip past the opening triple quote (3 chars)
	searchStart := len(quoteMarker)
	closeIdx := strings.Index(trimmed[searchStart:], quoteMarker)
	if closeIdx == -1 {
		// Malformed docstring; prepend as fallback
		return "import os\n" + content
	}

	// closeIdx is relative to trimmed[searchStart:], convert to absolute position in trimmed
	closeIdx += searchStart + len(quoteMarker)

	// Find how much leading whitespace was removed
	leadingWhitespaceLen := len(content) - len(trimmed)

	// Insert "import os\n" after the closing triple quotes
	insertPos := leadingWhitespaceLen + closeIdx
	return content[:insertPos] + "\nimport os" + content[insertPos:]
}

// patchDjangoSettingsForPostgres rewires settings.py's default sqlite
// DATABASES block to read the POSTGRES_* env vars that
// internal/blueprints/files/django/services.yml already injects into the
// web container, so `manage.py migrate` targets the real project database
// instead of a throwaway db.sqlite3. Returns an error (soft-fail, caller
// decides whether to warn) if Django's template changed and the expected
// block can't be found.
func patchDjangoSettingsForPostgres(settingsPath string) error {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings.py: %w", err)
	}
	content := string(data)

	if !strings.Contains(content, djangoDefaultSQLiteDatabases) {
		return fmt.Errorf("default sqlite DATABASES block not found in %s", settingsPath)
	}
	content = strings.Replace(content, djangoDefaultSQLiteDatabases, djangoPostgresDatabases, 1)

	if !strings.Contains(content, "import os") {
		content = insertImportOsAfterDocstring(content)
	}

	return os.WriteFile(settingsPath, []byte(content), conventions.DefaultFilePerm)
}

const djangoDefaultAllowedHosts = "ALLOWED_HOSTS = []"

// djangoAllowedHostsReplacement builds the ALLOWED_HOSTS/CSRF_TRUSTED_ORIGINS
// replacement text for a given domain. CSRF_TRUSTED_ORIGINS is required
// alongside ALLOWED_HOSTS (not just a convenience) because Django 4+ checks
// incoming POST requests' Origin header against it, which Govard's HTTPS
// proxy always sets to the project's domain.
func djangoAllowedHostsReplacement(domain string) string {
	return "ALLOWED_HOSTS = ['" + domain + "', 'localhost', '127.0.0.1']\n\nCSRF_TRUSTED_ORIGINS = ['https://" + domain + "']"
}

// patchDjangoSettingsForDomain wires settings.py's ALLOWED_HOSTS to the
// project's configured domain so Django accepts requests proxied through it
// instead of rejecting them with DisallowedHost - Django's default
// ALLOWED_HOSTS = [] only special-cases localhost/127.0.0.1/[::1], not
// arbitrary dev domains like "django.test". Also adds CSRF_TRUSTED_ORIGINS
// for the same domain over HTTPS. Returns an error (soft-fail, caller
// decides whether to warn) if domain is empty or Django's template changed
// and the expected line can't be found.
func patchDjangoSettingsForDomain(settingsPath string, domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return fmt.Errorf("no domain configured, leaving ALLOWED_HOSTS untouched")
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings.py: %w", err)
	}
	content := string(data)

	if !strings.Contains(content, djangoDefaultAllowedHosts) {
		return fmt.Errorf("default ALLOWED_HOSTS line not found in %s", settingsPath)
	}
	content = strings.Replace(content, djangoDefaultAllowedHosts, djangoAllowedHostsReplacement(domain), 1)

	return os.WriteFile(settingsPath, []byte(content), conventions.DefaultFilePerm)
}

// PatchDjangoSettingsForDomainForTest exposes patchDjangoSettingsForDomain for tests in /tests.
func PatchDjangoSettingsForDomainForTest(settingsPath string, domain string) error {
	return patchDjangoSettingsForDomain(settingsPath, domain)
}

// WriteDjangoRequirementsForTest exposes writeDjangoRequirements for tests in /tests.
func WriteDjangoRequirementsForTest(projectDir string, version string) error {
	return writeDjangoRequirements(projectDir, version)
}

// PatchDjangoSettingsForPostgresForTest exposes patchDjangoSettingsForPostgres for tests in /tests.
func PatchDjangoSettingsForPostgresForTest(settingsPath string) error {
	return patchDjangoSettingsForPostgres(settingsPath)
}

// SetDjangoContainerExecRunnerForTest overrides the docker-exec runner used
// by Install()/PostClone(), returning a restore function.
func SetDjangoContainerExecRunnerForTest(fn func(containerName string, script string) error) func() {
	previous := djangoContainerExecRunner
	if fn != nil {
		djangoContainerExecRunner = fn
	}
	return func() {
		djangoContainerExecRunner = previous
	}
}
