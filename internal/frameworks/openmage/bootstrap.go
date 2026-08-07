package openmage

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"
	"govard/internal/frameworks/magento1"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

type OpenMageBootstrap struct {
	Options bootstrap.Options
}

func NewOpenMageBootstrap(opts bootstrap.Options) *OpenMageBootstrap {
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

	version := o.Options.Version
	packageName := "openmage/magento-lts"
	if version != "" {
		packageName = fmt.Sprintf("openmage/magento-lts:%s", version)
	}

	createInStage := func(stageDir string) error {
		return bootstrap.RunComposerProjectCommand(projectDir, nil, "create-project", packageName, stageDir, "--no-interaction")
	}
	runnerCommand := "composer create-project " + packageName + " \"$GOVARD_STAGE_DIR\" --no-interaction"
	if err := bootstrap.RunStagedCreateProject(projectDir, o.Options.Runner, createInStage, runnerCommand, conventions.DefaultWorkDir); err != nil {
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
		if err := o.runCLIInstall(projectDir); err != nil {
			pterm.Warning.Printf("OpenMage CLI installer unavailable, falling back to local.xml scaffold: %v\n", err)
			pterm.Info.Println("Creating local.xml configuration...")
			if err := o.createLocalXml(projectDir); err != nil {
				pterm.Warning.Printf("Failed to create local.xml: %v\n", err)
			}
		}
	}

	varPath := filepath.Join(projectDir, "var")
	_ = os.MkdirAll(varPath, conventions.PublicDirPerm)
	_ = os.MkdirAll(filepath.Join(varPath, "cache"), conventions.PublicDirPerm)
	_ = os.MkdirAll(filepath.Join(varPath, "session"), conventions.PublicDirPerm)
	_ = os.MkdirAll(filepath.Join(varPath, "log"), conventions.PublicDirPerm)

	mediaPath := filepath.Join(projectDir, "media")
	_ = os.MkdirAll(mediaPath, conventions.PublicDirPerm)

	pterm.Success.Println("OpenMage installation completed")
	return nil
}

// runCLIInstall runs OpenMage's non-interactive install.php CLI installer,
// which creates the DB schema, writes app/etc/local.xml, and seeds the
// default admin user (admin/Admin12345678$) in a single step - unlike PostClone's
// CreateAdmin, this is safe to run against a fresh, empty database because
// install.php creates the schema itself before seeding the admin user.
func (o *OpenMageBootstrap) runCLIInstall(projectDir string) error {
	installScript := filepath.Join(projectDir, "install.php")
	if _, err := os.Stat(installScript); os.IsNotExist(err) {
		return fmt.Errorf("install.php not found in project root")
	}

	dbHost, dbUser, dbPass, dbName := o.resolveDBConfig()
	if err := bootstrap.WaitForMySQLDatabase(projectDir, o.Options.Runner, dbHost, dbUser, dbPass, dbName); err != nil {
		return fmt.Errorf("database not reachable: %w", err)
	}

	siteURL := "http://localhost/"
	useSecure := "no"
	if domain := strings.TrimSpace(o.Options.Domain); domain != "" {
		siteURL = "https://" + domain + "/"
		useSecure = "yes"
	}

	args := []string{
		"--license_agreement_accepted", "yes",
		"--locale", "en_US",
		"--timezone", "UTC",
		"--default_currency", "USD",
		"--db_host", dbHost,
		"--db_name", dbName,
		"--db_user", dbUser,
		"--db_pass", dbPass,
		"--url", siteURL,
		"--use_rewrites", "yes",
		"--use_secure", useSecure,
		"--secure_base_url", siteURL,
		"--use_secure_admin", useSecure,
		"--skip_url_validation", "yes",
		"--admin_firstname", "Admin",
		"--admin_lastname", "User",
		"--admin_email", conventions.AdminEmailForDomain(o.Options.Domain),
		"--admin_username", conventions.DefaultAdminUser,
		"--admin_password", conventions.DefaultAdminPassword,
	}

	pterm.Info.Println("Running OpenMage CLI installer...")
	if err := bootstrap.RunPHPProjectScript(projectDir, o.Options.Runner, installScript, args...); err != nil {
		return fmt.Errorf("install.php failed: %w", err)
	}

	pterm.Success.Println("OpenMage installed via CLI installer (schema + admin user created)")
	return nil
}

// resolveDBConfig returns the DB connection details to use for OpenMage,
// falling back to the framework's conventional local Docker credentials
// when Options wasn't populated with resolved container credentials.
func (o *OpenMageBootstrap) resolveDBConfig() (host, user, pass, name string) {
	host = strings.TrimSpace(o.Options.DBHost)
	if host == "" {
		host = conventions.DefaultDBHost
	}
	user = strings.TrimSpace(o.Options.DBUser)
	if user == "" {
		user = DefaultDBUser
	}
	pass = o.Options.DBPass
	if pass == "" {
		pass = DefaultDBPass
	}
	name = strings.TrimSpace(o.Options.DBName)
	if name == "" {
		name = DefaultDBName
	}
	return host, user, pass, name
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

	localXmlPath := filepath.Join(projectDir, "app", "etc", "local.xml")
	if _, err := os.Stat(localXmlPath); os.IsNotExist(err) {
		if err := o.createLocalXml(projectDir); err != nil {
			pterm.Warning.Printf("Failed to create local.xml: %v\n", err)
		}
	}

	varPath := filepath.Join(projectDir, "var")
	_ = os.MkdirAll(varPath, conventions.PublicDirPerm)
	_ = os.MkdirAll(filepath.Join(varPath, "cache"), conventions.PublicDirPerm)
	_ = os.MkdirAll(filepath.Join(varPath, "session"), conventions.PublicDirPerm)

	if err := o.CreateAdmin(projectDir); err != nil {
		pterm.Warning.Printf("Failed to create admin user: %v\n", err)
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

// CreateAdmin seeds the default admin user (admin/Admin12345678$) via direct SQL,
// matching Magento1Bootstrap.CreateAdmin since OpenMage shares Magento 1's
// admin_user schema. Only safe once the DB schema exists - used by PostClone
// (schema comes from the imported dump). Fresh install seeds its own admin
// user via install.php in runCLIInstall instead.
func (o *OpenMageBootstrap) CreateAdmin(projectDir string) error {
	adminEmail := conventions.AdminEmailForDomain(o.Options.Domain)
	containerName := fmt.Sprintf("%s%s", o.Options.ProjectName, conventions.DBSuffix)

	pterm.Info.Println("Creating OpenMage admin user...")
	return magento1.RunAdminUserSQL(containerName, o.Options.DBUser, o.Options.DBPass, o.Options.DBName, strings.TrimSpace(o.Options.TablePrefix), adminEmail)
}

func (o *OpenMageBootstrap) createLocalXml(projectDir string) error {
	cryptKey, err := magento1.GenerateCryptKey()
	if err != nil {
		return fmt.Errorf("failed to generate crypt key: %w", err)
	}

	tablePrefix := strings.TrimSpace(o.Options.TablePrefix)
	dbHost, dbUser, dbPass, dbName := o.resolveDBConfig()
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
                    <frontName><![CDATA[%s]]></frontName>
                </args>
            </adminhtml>
        </routers>
    </admin>
</config>
`, cryptKey, tablePrefix, dbHost, dbUser, dbPass, dbName, conventions.DefaultAdminPath)

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

func (o *OpenMageBootstrap) runComposerCommand(projectDir string, args ...string) error {
	return bootstrap.RunComposerProjectCommand(projectDir, o.Options.Runner, args...)
}
