package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	return false
}

func (d *DjangoBootstrap) SupportsClone() bool {
	return true
}

func (d *DjangoBootstrap) FreshCommands() []string {
	return nil
}

func (d *DjangoBootstrap) CreateProject(projectDir string) error {
	return fmt.Errorf("fresh-install scaffolding is not supported for django yet - clone an existing project instead")
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

// installAndMigrate runs pip install + migrate inside the compose-managed
// "web" container, not the host - the container's Python/pip is the one
// that must match the project's requirements.txt, not whatever (if
// anything) is installed on the developer's machine. By the time
// PostClone runs, `env up` has already started containers (see
// internal/cmd/bootstrap.go's ordering), so the container is available.
func (d *DjangoBootstrap) installAndMigrate() error {
	containerName := d.Options.ProjectName + conventions.WebSuffix
	script := "pip install --no-cache-dir -r requirements.txt && python manage.py migrate"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "sh", "-lc", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("django pip install/migrate failed: %w", err)
	}
	return nil
}
