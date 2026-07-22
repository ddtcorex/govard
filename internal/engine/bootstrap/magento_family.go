package bootstrap

import "fmt"

// magento2FamilyBootstrap adapts Magento2FreshCommands/MageOSFreshCommands
// (plain functions, unlike every other framework's FrameworkBootstrap
// struct) to the FrameworkBootstrap interface, so the registry's Bootstrap
// factory field can be populated for magento2/mageos too. Only
// FreshCommands is real: Magento 2 and Mage-OS's actual fresh-install
// orchestration (admin user creation, reindexing, sample data, etc.) lives
// in internal/cmd/bootstrap_fresh_install.go's runBootstrapFreshInstall,
// not this generic interface, so the other lifecycle methods report that
// explicitly instead of silently no-op'ing.
type magento2FamilyBootstrap struct {
	options       Options
	name          string
	freshCommands func(Options) []string
}

func NewMagento2Bootstrap(opts Options) *magento2FamilyBootstrap {
	return &magento2FamilyBootstrap{options: opts, name: "magento2", freshCommands: Magento2FreshCommands}
}

func NewMageOSBootstrap(opts Options) *magento2FamilyBootstrap {
	return &magento2FamilyBootstrap{options: opts, name: "mageos", freshCommands: MageOSFreshCommands}
}

func (m *magento2FamilyBootstrap) Name() string {
	return m.name
}

func (m *magento2FamilyBootstrap) SupportsFreshInstall() bool {
	return true
}

func (m *magento2FamilyBootstrap) SupportsClone() bool {
	return true
}

func (m *magento2FamilyBootstrap) FreshCommands() []string {
	return m.freshCommands(m.options)
}

func (m *magento2FamilyBootstrap) unsupported(step string) error {
	return fmt.Errorf("%s %s is orchestrated by 'govard bootstrap --fresh' (internal/cmd/bootstrap_fresh_install.go), not bootstrap.FrameworkBootstrap", m.name, step)
}

func (m *magento2FamilyBootstrap) CreateProject(projectDir string) error {
	return m.unsupported("project creation")
}

func (m *magento2FamilyBootstrap) Install(projectDir string) error {
	return m.unsupported("installation")
}

func (m *magento2FamilyBootstrap) Configure(projectDir string) error {
	return m.unsupported("configuration")
}

func (m *magento2FamilyBootstrap) PostClone(projectDir string) error {
	return m.unsupported("post-clone setup")
}
