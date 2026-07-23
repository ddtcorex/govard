package bootstrap

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 is intentional here: Magento 1 uses salted MD5 for admin passwords
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/conventions"

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
	return RunMagento1AdminUserSQL(containerName, m.Options.DBUser, m.Options.DBPass, m.Options.DBName, strings.TrimSpace(m.Options.TablePrefix), adminEmail)
}

// createLocalXml generates app/etc/local.xml with a random 32-hex crypt key and
// the default local Warden database credentials.
func (m *Magento1Bootstrap) createLocalXml(projectDir string) error {
	cryptKey, err := generateMagento1CryptKey()
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

// generateMagento1CryptKey returns a random 32-character hex string for use as an encryption key.
func generateMagento1CryptKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RunMagento1AdminUserSQL inserts/updates the admin user in the local DB using a salted MD5 hash.
// This matches the approach in warden-custom-commands bootstrap.cmd for maximum M1 compatibility.
func RunMagento1AdminUserSQL(containerName string, dbUser string, dbPassword string, dbName string, dbPrefix string, adminEmail string) error {
	// Salted MD5: md5(default admin user + default admin password) + ":" + default admin user.
	passHash := Md5SaltedHash(conventions.DefaultAdminUser, conventions.DefaultAdminPassword)
	saltedPass := passHash + ":" + conventions.DefaultAdminUser

	insertSQL := fmt.Sprintf(`
INSERT INTO %sadmin_user(username, firstname, lastname, email, password, created, lognum, reload_acl_flag, is_active, extra, rp_token, rp_token_created_at)
VALUES (%q, "Admin", "User", %q, %q, NOW(), 0, 0, 1, NULL, NULL, NOW())
ON DUPLICATE KEY UPDATE password = %q, is_active = 1;

-- Ensure Administrators group exists
INSERT IGNORE INTO %sadmin_role (parent_id, tree_level, sort_order, role_type, user_id, role_name)
VALUES (0, 1, 1, 'G', 0, 'Administrators');

-- Ensure full permissions
INSERT IGNORE INTO %sadmin_rule (role_id, resource_id, privileges, assert_id, role_type, permission)
SELECT role_id, 'all', NULL, 0, 'G', 'allow' FROM %sadmin_role WHERE role_type = 'G' AND role_name = 'Administrators' LIMIT 1;

-- Assign user to Administrators
INSERT INTO %sadmin_role (parent_id, tree_level, sort_order, role_type, user_id, role_name)
SELECT role_id, 2, 0, 'U', (SELECT user_id FROM %sadmin_user WHERE username = %q LIMIT 1), %q
FROM %sadmin_role WHERE role_type = 'G' AND role_name = 'Administrators' LIMIT 1
		ON DUPLICATE KEY UPDATE parent_id = VALUES(parent_id);
		`,
		dbPrefix, conventions.DefaultAdminUser, adminEmail, saltedPass, saltedPass,
		dbPrefix, dbPrefix, dbPrefix, dbPrefix, dbPrefix, conventions.DefaultAdminUser, conventions.DefaultAdminUser, dbPrefix)

	return RunSQLViaDockerExec(containerName, dbUser, dbPassword, dbName, insertSQL)
}

// RunSQLViaDockerExec executes a SQL statement via docker exec on the given DB container.
// This is framework-agnostic (no Magento-specific logic in the body) and is reused by
// other frameworks' bootstrap code that need to run a one-off SQL statement against the
// project's local DB container.
func RunSQLViaDockerExec(containerName string, dbUser string, dbPassword string, dbName string, sql string) error {
	script := fmt.Sprintf(
		`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else exit 1; fi && echo %s | "$DB_CLI" -u %s %s -f`,
		conventions.ShellQuote(sql), conventions.ShellQuote(dbUser), conventions.ShellQuote(dbName),
	)

	args := []string{"exec", "-i"}
	if dbPassword != "" {
		args = append(args, "-e", "MYSQL_PWD="+dbPassword)
	}
	args = append(args, containerName, "sh", "-lc", script)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SQL exec failed: %w: %s", err, out)
	}
	return nil
}

// Md5SaltedHash returns the MD5 hash of (salt + password) as a hex string.
// This matches Magento 1's salted password hashing: md5(salt . password).
//
// MD5 is required here, not a choice: Magento 1's own auth code only ever
// verifies this exact format, so using a stronger algorithm would produce a
// hash Magento 1 itself cannot log in with. This only ever hashes the local
// dev bootstrap's default admin credentials (see conventions.DefaultAdminUser
// / DefaultAdminPassword), never a real user secret.
func Md5SaltedHash(salt, password string) string {
	h := md5.New() //nolint:gosec
	fmt.Fprint(h, salt+password)
	return hex.EncodeToString(h.Sum(nil))
}
