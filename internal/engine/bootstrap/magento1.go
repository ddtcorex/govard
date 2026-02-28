package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pterm/pterm"
)

type Magento1Bootstrap struct {
	Options Options
}

func NewMagento1Bootstrap(opts Options) *Magento1Bootstrap {
	return &Magento1Bootstrap{Options: opts}
}

func (m *Magento1Bootstrap) Name() string {
	return "magento1"
}

func (m *Magento1Bootstrap) SupportsFreshInstall() bool {
	return false
}

func (m *Magento1Bootstrap) SupportsClone() bool {
	return true
}

func (m *Magento1Bootstrap) FreshCommands() []string {
	return []string{}
}

func (m *Magento1Bootstrap) CreateProject(projectDir string) error {
	return fmt.Errorf("fresh install not supported for Magento 1, use --clone instead")
}

func (m *Magento1Bootstrap) Install(projectDir string) error {
	pterm.Info.Println("Setting up Magento 1...")
	pterm.Success.Println("Magento 1 setup completed")
	return nil
}

func (m *Magento1Bootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring Magento 1 environment...")

	localXmlPath := filepath.Join(projectDir, "app", "etc", "local.xml")
	if _, err := os.Stat(localXmlPath); err == nil {
		pterm.Info.Println("Found local.xml configuration")
	}

	pterm.Success.Println("Magento 1 configured successfully")
	return nil
}

func (m *Magento1Bootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned Magento 1 project...")

	varPath := filepath.Join(projectDir, "var")
	_ = os.MkdirAll(varPath, 0777)

	cachePath := filepath.Join(varPath, "cache")
	_ = os.MkdirAll(cachePath, 0777)

	sessionPath := filepath.Join(varPath, "session")
	_ = os.MkdirAll(sessionPath, 0777)

	pterm.Success.Println("Post-clone setup completed")
	return nil
}
