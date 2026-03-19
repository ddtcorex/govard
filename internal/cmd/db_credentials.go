package cmd

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

type dbCredentials struct {
	Host     string
	Port     int
	Username string
	Password string
	Database string
}

func defaultDBCredentials() dbCredentials {
	return dbCredentials{
		Port:     3306,
		Username: "magento",
		Password: "magento",
		Database: "magento",
	}
}

func (credentials dbCredentials) withDefaults() dbCredentials {
	result := credentials
	if strings.TrimSpace(result.Username) == "" {
		result.Username = "magento"
	}
	if strings.TrimSpace(result.Database) == "" {
		result.Database = "magento"
	}
	if strings.TrimSpace(result.Host) != "" && result.Port <= 0 {
		result.Port = 3306
	}
	if result.Port < 0 {
		result.Port = 0
	}
	return result
}

func resolveRemoteDBCredentials(config engine.Config, remoteName string, remoteCfg engine.RemoteConfig) (dbCredentials, error) {
	fallback := defaultDBCredentials()
	switch strings.TrimSpace(config.Framework) {
	case "magento2":
		metadata, err := remote.ProbeMagento2Environment(remoteName, remoteCfg)
		if err != nil {
			return fallback, err
		}

		return dbCredentials{
			Host:     metadata.DB.Host,
			Port:     metadata.DB.Port,
			Username: metadata.DB.Username,
			Password: metadata.DB.Password,
			Database: metadata.DB.Database,
		}.withDefaults(), nil
	case "symfony", "laravel", "drupal", "wordpress", "shopware", "cakephp":
		metadata, err := remote.ProbeDotenvEnvironment(remoteName, remoteCfg)
		if err != nil {
			return fallback, err
		}
		return dbCredentials{
			Host:     metadata.DB.Host,
			Port:     metadata.DB.Port,
			Username: metadata.DB.Username,
			Password: metadata.DB.Password,
			Database: metadata.DB.Database,
		}.withDefaults(), nil
	default:
		return fallback, nil
	}
}

func resolveLocalDBCredentials(containerName string) dbCredentials {
	credentials := defaultDBCredentials()
	inspectCommand := exec.Command("docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", containerName)
	output, err := inspectCommand.Output()
	if err != nil {
		return credentials
	}

	envMap := parseEnvMap(string(output))
	if user := strings.TrimSpace(envMap["MYSQL_USER"]); user != "" {
		credentials.Username = user
	}
	if password := envMap["MYSQL_PASSWORD"]; password != "" {
		credentials.Password = password
	}
	if database := strings.TrimSpace(envMap["MYSQL_DATABASE"]); database != "" {
		credentials.Database = database
	}

	return credentials.withDefaults()
}

func parseEnvMap(raw string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result[strings.TrimSpace(parts[0])] = parts[1]
	}
	return result
}

func buildRemoteMySQLDumpCommandString(credentials dbCredentials, full bool) string {
	credentials = credentials.withDefaults()

	args := []string{"mysqldump", "--max-allowed-packet=512M", "--no-tablespaces"}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		args = append(args, "-h"+shellQuote(host))
	}
	if credentials.Port > 0 {
		args = append(args, "-P"+strconv.Itoa(credentials.Port))
	}
	args = append(args, "-u"+shellQuote(credentials.Username))

	if full {
		args = append(args, "--routines", "--events", "--triggers")
	}
	args = append(args, shellQuote(credentials.Database))

	return mysqlPasswordExportPrefix(credentials.Password) + strings.Join(args, " ")
}

func buildRemoteMySQLConnectCommandString(credentials dbCredentials) string {
	credentials = credentials.withDefaults()

	args := []string{"mysql"}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		args = append(args, "-h"+shellQuote(host))
	}
	if credentials.Port > 0 {
		args = append(args, "-P"+strconv.Itoa(credentials.Port))
	}
	args = append(args, "-u"+shellQuote(credentials.Username), shellQuote(credentials.Database))

	return mysqlPasswordExportPrefix(credentials.Password) + strings.Join(args, " ")
}

func buildRemoteMySQLImportCommandString(credentials dbCredentials) string {
	credentials = credentials.withDefaults()

	args := []string{"mysql", "--max-allowed-packet=512M"}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		args = append(args, "-h"+shellQuote(host))
	}
	if credentials.Port > 0 {
		args = append(args, "-P"+strconv.Itoa(credentials.Port))
	}
	args = append(args, "-u"+shellQuote(credentials.Username), shellQuote(credentials.Database), "-f")

	return mysqlPasswordExportPrefix(credentials.Password) + strings.Join(args, " ")
}

func buildLocalDBConnectCommand(containerName string, credentials dbCredentials) *exec.Cmd {
	credentials = credentials.withDefaults()
	args := []string{"exec", "-it"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args, containerName, "sh", "-lc", buildLocalMySQLClientCommandScript(credentials, false))
	return exec.Command("docker", args...)
}

func buildLocalDBImportCommand(containerName string, credentials dbCredentials) *exec.Cmd {
	credentials = credentials.withDefaults()
	args := []string{"exec", "-i"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args, containerName, "sh", "-lc", buildLocalMySQLClientCommandScript(credentials, true))
	return exec.Command("docker", args...)
}

func buildLocalDBDumpCommand(containerName string, credentials dbCredentials, full bool) *exec.Cmd {
	credentials = credentials.withDefaults()
	args := []string{"exec", "-i"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args, containerName)
	args = append(args, buildMySQLDumpCommandArgsWithCredentials(credentials, full)...)
	return exec.Command("docker", args...)
}

func buildMySQLDumpCommandArgsWithCredentials(credentials dbCredentials, full bool) []string {
	credentials = credentials.withDefaults()
	args := []string{"mysqldump", "--max-allowed-packet=512M", "--no-tablespaces", "-u", credentials.Username}
	if full {
		args = append(args, "--routines", "--events", "--triggers")
	}
	args = append(args, credentials.Database)
	return args
}

func mysqlPasswordExportPrefix(password string) string {
	if strings.TrimSpace(password) == "" {
		return ""
	}
	return "export MYSQL_PWD=" + shellQuote(password) + "; "
}

func buildLocalMySQLClientCommandScript(credentials dbCredentials, force bool) string {
	credentials = credentials.withDefaults()

	query := "exec \"$DB_CLI\" --max-allowed-packet=512M -u " + shellQuote(credentials.Username) + " " + shellQuote(credentials.Database)
	if force {
		query += " -f"
		query = "{ echo \"SET FOREIGN_KEY_CHECKS=0; SET UNIQUE_CHECKS=0; SET AUTOCOMMIT=0;\"; cat; echo \"COMMIT; SET FOREIGN_KEY_CHECKS=1; SET UNIQUE_CHECKS=1; SET AUTOCOMMIT=1;\"; } | " + query
	}

	return strings.Join([]string{
		`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else echo "mysql client not found (mysql/mariadb)" >&2; exit 127; fi`,
		query,
	}, " && ")
}

func formatRemoteDBProbeWarning(remoteName string, err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("Could not auto-detect DB credentials for '%s' from remote metadata (.env/env.php) (%v). Falling back to default credentials.", remoteName, err)
}

func BuildRemoteMySQLDumpCommandForTest(host string, port int, username string, password string, database string, full bool) string {
	return buildRemoteMySQLDumpCommandString(dbCredentials{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Database: database,
	}, full)
}

func BuildLocalDBImportCommandForTest(containerName string, username string, password string, database string) []string {
	command := buildLocalDBImportCommand(containerName, dbCredentials{
		Username: username,
		Password: password,
		Database: database,
	})
	return command.Args
}

func ParseEnvMapForTest(raw string) map[string]string {
	return parseEnvMap(raw)
}

func buildLocalDBQueryCommand(containerName string, credentials dbCredentials, query string) *exec.Cmd {
	credentials = credentials.withDefaults()
	args := []string{"exec", "-i"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args, containerName, "sh", "-lc", buildLocalMySQLQueryCommandScript(credentials, query))
	return exec.Command("docker", args...)
}

func buildLocalMySQLQueryCommandScript(credentials dbCredentials, query string) string {
	credentials = credentials.withDefaults()

	escapedQuery := strings.ReplaceAll(query, "'", "'\"'\"'")
	queryCmd := "exec \"$DB_CLI\" -u " + shellQuote(credentials.Username) + " -e '" + escapedQuery + "'" + " " + shellQuote(credentials.Database)

	return strings.Join([]string{
		`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else echo "mysql client not found (mysql/mariadb)" >&2; exit 127; fi`,
		queryCmd,
	}, " && ")
}

func buildRemoteMySQLQueryCommandString(credentials dbCredentials, query string) string {
	credentials = credentials.withDefaults()

	args := []string{"mysql"}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		args = append(args, "-h"+shellQuote(host))
	}
	if credentials.Port > 0 {
		args = append(args, "-P"+strconv.Itoa(credentials.Port))
	}
	args = append(args, "-u"+shellQuote(credentials.Username), "-e", shellQuote(query))

	return mysqlPasswordExportPrefix(credentials.Password) + strings.Join(args, " ")
}
