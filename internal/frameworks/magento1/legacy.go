package magento1

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"govard/internal/conventions"
	"govard/internal/engine/bootstrap"
)

// GenerateCryptKey creates the random 32-character key required by Magento
// 1-family app/etc/local.xml files. OpenMage inherits this behavior.
func GenerateCryptKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RunAdminUserSQL creates the Magento 1-family default admin user. OpenMage
// reuses the schema and calls this parent-family implementation.
func RunAdminUserSQL(containerName, dbUser, dbPassword, dbName, dbPrefix, adminEmail string) error {
	insertSQL := fmt.Sprintf(`
-- Magento 1/OpenMage requires the legacy MD5:salt password representation.
SET @govard_admin_password := CONCAT(MD5(CONCAT(%q, %q)), ':', %q);

INSERT INTO %sadmin_user(username, firstname, lastname, email, password, created, lognum, reload_acl_flag, is_active, extra, rp_token, rp_token_created_at)
VALUES (%q, "Admin", "User", %q, @govard_admin_password, NOW(), 0, 0, 1, NULL, NULL, NOW())
ON DUPLICATE KEY UPDATE password = @govard_admin_password, is_active = 1;

INSERT IGNORE INTO %sadmin_role (parent_id, tree_level, sort_order, role_type, user_id, role_name)
VALUES (0, 1, 1, 'G', 0, 'Administrators');

INSERT IGNORE INTO %sadmin_rule (role_id, resource_id, privileges, assert_id, role_type, permission)
SELECT role_id, 'all', NULL, 0, 'G', 'allow' FROM %sadmin_role WHERE role_type = 'G' AND role_name = 'Administrators' LIMIT 1;

INSERT INTO %sadmin_role (parent_id, tree_level, sort_order, role_type, user_id, role_name)
SELECT role_id, 2, 0, 'U', (SELECT user_id FROM %sadmin_user WHERE username = %q LIMIT 1), %q
FROM %sadmin_role WHERE role_type = 'G' AND role_name = 'Administrators' LIMIT 1
	ON DUPLICATE KEY UPDATE parent_id = VALUES(parent_id);
		`,
		conventions.DefaultAdminUser, conventions.DefaultAdminPassword, conventions.DefaultAdminUser,
		dbPrefix, conventions.DefaultAdminUser, adminEmail,
		dbPrefix, dbPrefix, dbPrefix, dbPrefix, dbPrefix, conventions.DefaultAdminUser, conventions.DefaultAdminUser, dbPrefix)

	return bootstrap.RunSQLViaDockerExec(containerName, dbUser, dbPassword, dbName, insertSQL)
}
