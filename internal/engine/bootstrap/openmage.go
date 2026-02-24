package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pterm/pterm"
)

type OpenMageBootstrap struct {
	Options Options
}

func NewOpenMageBootstrap(opts Options) *OpenMageBootstrap {
	return &OpenMageBootstrap{Options: opts}
}

func (o *OpenMageBootstrap) Name() string {
	return "openmage"
}

func (o *OpenMageBootstrap) SupportsFreshInstall() bool {
	return true
}

func (o *OpenMageBootstrap) SupportsClone() bool {
	return true
}

func (o *OpenMageBootstrap) FreshCommands() []string {
	return []string{
		"composer create-project openmage/magento-lts .",
	}
}

func (o *OpenMageBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh OpenMage project...")

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read project directory: %w", err)
	}

	if len(entries) > 0 {
		pterm.Warning.Println("Project directory is not empty. Cleaning up...")
		for _, entry := range entries {
			if entry.Name() == ".govard" || entry.Name() == "govard.yml" {
				continue
			}
			path := filepath.Join(projectDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("failed to remove %s: %w", entry.Name(), err)
			}
		}
	}

	version := o.Options.Version
	packageName := "openmage/magento-lts"
	if version != "" {
		packageName = fmt.Sprintf("openmage/magento-lts:%s", version)
	}

	cmd := exec.Command("composer", "create-project", packageName, ".", "--no-interaction")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create OpenMage project: %w", err)
	}

	pterm.Success.Println("OpenMage project created successfully")
	return nil
}

func (o *OpenMageBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running OpenMage installation steps...")

	if err := o.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		pterm.Warning.Printf("Composer install warning: %v\n", err)
	}

	localXmlPath := filepath.Join(projectDir, "app", "etc", "local.xml")
	if _, err := os.Stat(localXmlPath); os.IsNotExist(err) {
		pterm.Info.Println("Creating local.xml configuration...")
		if err := o.createLocalXml(projectDir); err != nil {
			pterm.Warning.Printf("Failed to create local.xml: %v\n", err)
		}
	}

	varPath := filepath.Join(projectDir, "var")
	os.MkdirAll(varPath, 0777)
	os.MkdirAll(filepath.Join(varPath, "cache"), 0777)
	os.MkdirAll(filepath.Join(varPath, "session"), 0777)
	os.MkdirAll(filepath.Join(varPath, "log"), 0777)

	mediaPath := filepath.Join(projectDir, "media")
	os.MkdirAll(mediaPath, 0777)

	pterm.Success.Println("OpenMage installation completed")
	return nil
}

func (o *OpenMageBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring OpenMage environment...")

	localXmlPath := filepath.Join(projectDir, "app", "etc", "local.xml")
	if _, err := os.Stat(localXmlPath); os.IsNotExist(err) {
		if err := o.createLocalXml(projectDir); err != nil {
			return err
		}
	}

	pterm.Success.Println("OpenMage configured successfully")
	return nil
}

func (o *OpenMageBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned OpenMage project...")

	if err := o.runComposerCommand(projectDir, "install", "--no-interaction"); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	localXmlPath := filepath.Join(projectDir, "app", "etc", "local.xml")
	if _, err := os.Stat(localXmlPath); os.IsNotExist(err) {
		if err := o.createLocalXml(projectDir); err != nil {
			pterm.Warning.Printf("Failed to create local.xml: %v\n", err)
		}
	}

	varPath := filepath.Join(projectDir, "var")
	os.MkdirAll(varPath, 0777)
	os.MkdirAll(filepath.Join(varPath, "cache"), 0777)
	os.MkdirAll(filepath.Join(varPath, "session"), 0777)

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (o *OpenMageBootstrap) createLocalXml(projectDir string) error {
	localXmlContent := `<?xml version="1.0"?>
<config>
    <global>
        <install>
            <date><![CDATA[Wed, 01 Jan 2025 00:00:00 +0000]]></date>
        </install>
        <crypt>
            <key><![CDATA[openmage_local_development_key_12345]]></key>
        </crypt>
        <disable_local_modules>false</disable_local_modules>
        <resources>
            <db>
                <table_prefix><![CDATA[]]></table_prefix>
            </db>
            <default_setup>
                <connection>
                    <host><![CDATA[db]]></host>
                    <username><![CDATA[openmage]]></username>
                    <password><![CDATA[openmage]]></password>
                    <dbname><![CDATA[openmage]]></dbname>
                    <initStatements><![CDATA[SET NAMES utf8]]></initStatements>
                    <model><![CDATA[mysql4]]></model>
                    <type><![CDATA[pdo_mysql]]></type>
                    <pdoType><![CDATA[]]></pdoType>
                    <active>1</active>
                </connection>
            </default_setup>
        </resources>
        <session_save><![CDATA[files]]></session_save>
        <session_save_path><![CDATA[var/session]]></session_save_path>
    </global>
    <admin>
        <routers>
            <adminhtml>
                <args>
                    <frontName><![CDATA[admin]]></frontName>
                </args>
            </adminhtml>
        </routers>
    </admin>
</config>
`

	etcPath := filepath.Join(projectDir, "app", "etc")
	if err := os.MkdirAll(etcPath, 0755); err != nil {
		return fmt.Errorf("failed to create app/etc directory: %w", err)
	}

	localXmlPath := filepath.Join(etcPath, "local.xml")
	if err := os.WriteFile(localXmlPath, []byte(localXmlContent), 0644); err != nil {
		return fmt.Errorf("failed to write local.xml: %w", err)
	}

	pterm.Success.Println("Created local.xml with container database settings")
	return nil
}

func (o *OpenMageBootstrap) runComposerCommand(projectDir string, args ...string) error {
	cmd := exec.Command("composer", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
