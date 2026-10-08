package wordpress

import "strings"

// SandboxBaseURLStatements returns the SQL that points a seeded WordPress
// database at the sandbox's own web URL: the `siteurl` and `home` options, the
// same two values bootstrap rewrites ("WordPress site URLs updated").
//
// The table prefix is not known here (it lives in wp-config.php or .env, and the
// seed does not read either), so the options table is found in the imported
// schema itself: the shortest table name ending in `options` that has both
// `option_name` and `option_value` columns, which is the main site's table on a
// single site and on a multisite network. The name is checked against a plain
// identifier pattern before it is spliced into a prepared statement, and the URL
// travels through QUOTE() so no character in it can end the literal.
func SandboxBaseURLStatements(_ []byte, baseURL string) []string {
	url := "'" + strings.NewReplacer(`\`, `\\`, `'`, `''`).Replace(baseURL) + "'"
	return []string{strings.Join([]string{
		"SET @govard_options_table = (SELECT t.table_name FROM information_schema.tables t" +
			" JOIN information_schema.columns n ON n.table_schema = t.table_schema AND n.table_name = t.table_name AND n.column_name = 'option_name'" +
			" JOIN information_schema.columns v ON v.table_schema = t.table_schema AND v.table_name = t.table_name AND v.column_name = 'option_value'" +
			" WHERE t.table_schema = DATABASE() AND t.table_name REGEXP '^[A-Za-z0-9_]*options$'" +
			" ORDER BY CHAR_LENGTH(t.table_name), t.table_name LIMIT 1)",
		"SET @govard_rewrite = IF(@govard_options_table IS NULL, 'DO 0'," +
			" CONCAT('UPDATE `', @govard_options_table, '` SET option_value = ', QUOTE(" + url + "), ' WHERE option_name IN (''siteurl'', ''home'')'))",
		"PREPARE govard_rewrite FROM @govard_rewrite",
		"EXECUTE govard_rewrite",
		"DEALLOCATE PREPARE govard_rewrite",
	}, ";\n") + ";"}
}
