package bootstrap

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 is intentional here: Magento 1 uses salted MD5 for admin passwords
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"govard/internal/conventions"
	"os/exec"
	"time"
)

// GenerateMagento1CryptKey returns a random 32-character hex string for use
// as an encryption key. Exported (not a same-package-only helper) because
// OpenMage's and Magento1's bootstrap code, both now living in their own
// internal/frameworks/<name> packages, generate a local.xml crypt key and
// need to call this cross-package.
func GenerateMagento1CryptKey() (string, error) {
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
