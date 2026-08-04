package magento1

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

type Magento1Bootstrap struct {
	Options bootstrap.Options
}

func NewMagento1Bootstrap(opts bootstrap.Options) *Magento1Bootstrap {
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
	_ = os.MkdirAll(varPath, conventions.PublicDirPerm)
	_ = os.MkdirAll(filepath.Join(varPath, "cache"), conventions.PublicDirPerm)
	_ = os.MkdirAll(filepath.Join(varPath, "session"), conventions.PublicDirPerm)

	mediaPath := filepath.Join(projectDir, "media")
	_ = os.MkdirAll(mediaPath, conventions.PublicDirPerm)

	localXmlPath := filepath.Join(projectDir, "app", "etc", "local.xml")
	if _, err := os.Stat(localXmlPath); os.IsNotExist(err) {
		if err := m.createLocalXml(projectDir); err != nil {
			pterm.Warning.Printf("Failed to create local.xml: %v\n", err)
		}
	}

	if err := m.CreateAdmin(projectDir); err != nil {
		pterm.Warning.Printf("Failed to create admin user: %v\n", err)
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (m *Magento1Bootstrap) CreateAdmin(projectDir string) error {
	adminEmail := conventions.AdminEmailForDomain(m.Options.Domain)
	containerName := fmt.Sprintf("%s%s", m.Options.ProjectName, conventions.DBSuffix)

	pterm.Info.Println("Creating Magento 1 admin user...")
	return RunAdminUserSQL(containerName, m.Options.DBUser, m.Options.DBPass, m.Options.DBName, strings.TrimSpace(m.Options.TablePrefix), adminEmail)
}

// createLocalXml generates app/etc/local.xml with a random 32-hex crypt key and
// the default local Warden database credentials.
func (m *Magento1Bootstrap) createLocalXml(projectDir string) error {
	cryptKey, err := GenerateCryptKey()
	if err != nil {
		return fmt.Errorf("failed to generate crypt key: %w", err)
	}

	tablePrefix := strings.TrimSpace(m.Options.TablePrefix)
	localXmlContent := fmt.Sprintf(`<?xml version="1.0"?>
<config>
    <global>
        <install>
            <date><![CDATA[Wed, 01 Jan 2025 00:00:00 +0000]]></date>
        </install>
        <crypt>
            <key><![CDATA[%s]]></key>
        </crypt>
        <disable_local_modules>false</disable_local_modules>
        <resources>
            <db>
                <table_prefix><![CDATA[%s]]></table_prefix>
            </db>
            <default_setup>
                <connection>
	                    <host><![CDATA[%s]]></host>
	                    <username><![CDATA[%s]]></username>
	                    <password><![CDATA[%s]]></password>
	                    <dbname><![CDATA[%s]]></dbname>
                    <initStatements><![CDATA[SET NAMES utf8]]></initStatements>
                    <model><![CDATA[mysql4]]></model>
                    <type><![CDATA[pdo_mysql]]></type>
                    <pdoType></pdoType>
                    <active>1</active>
                </connection>
            </default_setup>
        </resources>
        <session_save><![CDATA[files]]></session_save>
        <session_save_path><![CDATA[var/session]]></session_save_path>
    </global>
    <default>
        <web>
            <secure>
                <offloader_header><![CDATA[HTTP_X_FORWARDED_PROTO]]></offloader_header>
            </secure>
        </web>
    </default>
    <admin>
        <routers>
            <adminhtml>
                <args>
	                    <frontName><![CDATA[%s]]></frontName>
                </args>
            </adminhtml>
        </routers>
    </admin>
</config>
`, cryptKey,
		tablePrefix,
		conventions.DefaultDBHost,
		conventions.DefaultMagentoDBUser,
		conventions.DefaultMagentoDBPass,
		conventions.DefaultMagentoDBName,
		conventions.DefaultAdminPath)

	etcPath := filepath.Join(projectDir, "app", "etc")
	if err := os.MkdirAll(etcPath, conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create app/etc directory: %w", err)
	}

	localXmlPath := filepath.Join(etcPath, "local.xml")
	if err := os.WriteFile(localXmlPath, []byte(localXmlContent), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write local.xml: %w", err)
	}

	pterm.Success.Println("Created local.xml with random crypt key")
	return nil
}
