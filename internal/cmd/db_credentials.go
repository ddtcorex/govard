package cmd

import (
	_ "embed"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/remote"
)

type dbCredentials struct {
	Host        string
	Port        int
	Username    string
	Password    string
	Database    string
	TablePrefix string
}

func defaultDBCredentialsForFramework(framework string) dbCredentials {
	switch strings.TrimSpace(framework) {
	case conventions.FrameworkSymfony:
		return dbCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultSymfonyDBUser,
			Password: conventions.DefaultSymfonyDBPass,
			Database: conventions.DefaultSymfonyDBName,
		}
	case conventions.FrameworkLaravel:
		return dbCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultLaravelDBUser,
			Password: conventions.DefaultLaravelDBPass,
			Database: conventions.DefaultLaravelDBName,
		}
	case conventions.FrameworkWordPress:
		return dbCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultWordPressDBUser,
			Password: conventions.DefaultWordPressDBPass,
			Database: conventions.DefaultWordPressDBName,
		}
	case conventions.FrameworkPrestaShop:
		return dbCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultPrestaShopDBUser,
			Password: conventions.DefaultPrestaShopDBPass,
			Database: conventions.DefaultPrestaShopDBName,
		}
	case conventions.FrameworkMagento2, conventions.FrameworkMagento1, conventions.FrameworkOpenMage:
		return dbCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultMagentoDBUser,
			Password: conventions.DefaultMagentoDBPass,
			Database: conventions.DefaultMagentoDBName,
		}
	case conventions.FrameworkMageOS:
		return dbCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultMageOSDBUser,
			Password: conventions.DefaultMageOSDBPass,
			Database: conventions.DefaultMageOSDBName,
		}
	default:
		return dbCredentials{
			Port:     conventions.MySQLPort,
			Username: conventions.DefaultDBUser,
			Password: conventions.DefaultDBPass,
			Database: conventions.DefaultDBName,
		}
	}
}

func (credentials dbCredentials) withDefaults() dbCredentials {
	result := credentials
	if strings.TrimSpace(result.Username) == "" {
		result.Username = conventions.DefaultMagentoDBUser
	}
	if strings.TrimSpace(result.Database) == "" {
		result.Database = conventions.DefaultMagentoDBName
	}
	if strings.TrimSpace(result.Host) != "" && result.Port <= 0 {
		result.Port = conventions.MySQLPort
	}
	if result.Port < 0 {
		result.Port = 0
	}
	result.TablePrefix = engine.SafeTablePrefix(result.TablePrefix)
	return result
}

func resolveRemoteDBCredentials(config engine.Config, remoteName string, remoteCfg engine.RemoteConfig) (dbCredentials, error) {
	fallback := defaultDBCredentialsForFramework(config.Framework)
	fallback.TablePrefix = engine.NormalizeTablePrefix(config.TablePrefix)
	switch strings.TrimSpace(config.Framework) {
	case conventions.FrameworkMagento2, conventions.FrameworkMageOS:
		metadata, err := remote.ProbeMagento2Environment(remoteName, remoteCfg)
		if err != nil {
			return fallback, err
		}

		return dbCredentials{
			Host:        metadata.DB.Host,
			Port:        metadata.DB.Port,
			Username:    metadata.DB.Username,
			Password:    metadata.DB.Password,
			Database:    metadata.DB.Database,
			TablePrefix: firstNonEmpty(metadata.DB.TablePrefix, config.TablePrefix),
		}.withDefaults(), nil
	case conventions.FrameworkMagento1, conventions.FrameworkOpenMage:
		metadata, err := remote.ProbeMagento1Environment(remoteName, remoteCfg)
		if err != nil {
			return fallback, err
		}
		return dbCredentials{
			Host:        metadata.DB.Host,
			Port:        metadata.DB.Port,
			Username:    metadata.DB.Username,
			Password:    metadata.DB.Password,
			Database:    metadata.DB.Database,
			TablePrefix: firstNonEmpty(metadata.DB.TablePrefix, config.TablePrefix),
		}.withDefaults(), nil
	case conventions.FrameworkPrestaShop:
		metadata, err := remote.ProbePrestaShopEnvironment(remoteName, remoteCfg)
		if err != nil {
			return fallback, err
		}
		return dbCredentials{
			Host:        metadata.DB.Host,
			Port:        metadata.DB.Port,
			Username:    metadata.DB.Username,
			Password:    metadata.DB.Password,
			Database:    metadata.DB.Database,
			TablePrefix: firstNonEmpty(metadata.DB.TablePrefix, config.TablePrefix),
		}.withDefaults(), nil
	case "wordpress":
		metadata, err := remote.ProbeWordPressEnvironment(remoteName, remoteCfg)
		if err != nil {
			// Fallback to Dotenv for Bedrock-style WordPress sites
			metadataDotenv, errDotenv := remote.ProbeDotenvEnvironment(remoteName, remoteCfg)
			if errDotenv == nil {
				return dbCredentials{
					Host:     metadataDotenv.DB.Host,
					Port:     metadataDotenv.DB.Port,
					Username: metadataDotenv.DB.Username,
					Password: metadataDotenv.DB.Password,
					Database: metadataDotenv.DB.Database,
				}.withDefaults(), nil
			}
			return fallback, err
		}
		return dbCredentials{
			Host:     metadata.DB.Host,
			Port:     metadata.DB.Port,
			Username: metadata.DB.Username,
			Password: metadata.DB.Password,
			Database: metadata.DB.Database,
		}.withDefaults(), nil
	case "symfony", "laravel", "drupal", "shopware", "cakephp":
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
	case "custom":
		if remoteCfg.DBName != "" {
			fallback.Database = remoteCfg.DBName
		}
		if remoteCfg.DBUser != "" {
			fallback.Username = remoteCfg.DBUser
		}
		if remoteCfg.DBPass != "" {
			fallback.Password = remoteCfg.DBPass
		}
		if remoteCfg.DBPort > 0 {
			fallback.Port = remoteCfg.DBPort
		}
		return fallback, nil
	default:
		if remoteCfg.DBName != "" {
			fallback.Database = remoteCfg.DBName
		}
		if remoteCfg.DBUser != "" {
			fallback.Username = remoteCfg.DBUser
		}
		if remoteCfg.DBPass != "" {
			fallback.Password = remoteCfg.DBPass
		}
		if remoteCfg.DBPort > 0 {
			fallback.Port = remoteCfg.DBPort
		}
		return fallback, nil
	}
}

func resolveLocalDBCredentials(config engine.Config, containerName string) dbCredentials {
	credentials := defaultDBCredentialsForFramework(config.Framework)
	credentials.TablePrefix = engine.NormalizeTablePrefix(config.TablePrefix)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func DefaultDBCredentialsForFrameworkForTest(framework string) dbCredentials {
	return defaultDBCredentialsForFramework(framework)
}

func buildRemoteMySQLDumpCommandString(credentials dbCredentials, noNoise bool, noPII bool, framework string, compress bool) string {
	credentials = credentials.withDefaults()

	dbCliDetect := conventions.MySQLDumpBinDetect

	// Common options
	commonArgs := []string{"\"$DUMP_BIN\"", "--max-allowed-packet=" + conventions.MySQLMaxAllowedPacket, "--force", "--single-transaction", "--no-tablespaces"}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		commonArgs = append(commonArgs, "-h"+engine.ShellQuote(host))
	}
	if credentials.Port > 0 {
		commonArgs = append(commonArgs, "-P"+strconv.Itoa(credentials.Port))
	}
	commonArgs = append(commonArgs, "-u"+engine.ShellQuote(credentials.Username))

	// Pass 1: Metadata (no data, routines, triggers)
	metadataArgs := append([]string{}, commonArgs...)
	metadataArgs = append(metadataArgs, "--no-data", "--routines", "--triggers")
	metadataArgs = append(metadataArgs, engine.ShellQuote(credentials.Database))

	// Pass 2: Data (no create info, skip triggers, exclude noise/PII)
	dataArgs := append([]string{}, commonArgs...)
	dataArgs = append(dataArgs, "--no-create-info", "--skip-triggers")
	ignoreArgs := buildIgnoredTableArgs(credentials.Database, credentials.TablePrefix, noNoise, noPII, framework)
	dataArgs = append(dataArgs, ignoreArgs...)
	dataArgs = append(dataArgs, engine.ShellQuote(credentials.Database))

	// Combine passes
	dumpCmd := fmt.Sprintf("{ %s; %s; }", strings.Join(metadataArgs, " "), strings.Join(dataArgs, " "))
	if compress {
		dumpCmd += " | gzip -c"
	}

	return dbCliDetect + " && " + mysqlPasswordExportPrefix(credentials.Password) + dumpCmd
}

func buildRemoteMySQLConnectCommandString(credentials dbCredentials) string {
	credentials = credentials.withDefaults()

	args := []string{"mysql"}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		args = append(args, "-h"+engine.ShellQuote(host))
	}
	if credentials.Port > 0 {
		args = append(args, "-P"+strconv.Itoa(credentials.Port))
	}
	args = append(args, "-u"+engine.ShellQuote(credentials.Username), engine.ShellQuote(credentials.Database))

	return mysqlPasswordExportPrefix(credentials.Password) + strings.Join(args, " ")
}

func buildRemoteMySQLImportCommandString(credentials dbCredentials) string {
	credentials = credentials.withDefaults()

	args := []string{"mysql", "--max-allowed-packet=" + conventions.MySQLMaxAllowedPacket}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		args = append(args, "-h"+engine.ShellQuote(host))
	}
	if credentials.Port > 0 {
		args = append(args, "-P"+strconv.Itoa(credentials.Port))
	}
	args = append(args, "-u"+engine.ShellQuote(credentials.Username), engine.ShellQuote(credentials.Database), "-f")

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

func buildLocalDBDumpCommand(containerName string, credentials dbCredentials, noNoise bool, noPII bool, framework string) *exec.Cmd {
	credentials = credentials.withDefaults()
	args := []string{"exec", "-i"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args, containerName, "sh", "-lc", buildLocalMySQLDumpCommandScript(credentials, noNoise, noPII, framework))
	return exec.Command("docker", args...)
}

func buildLocalMySQLDumpCommandScript(credentials dbCredentials, noNoise bool, noPII bool, framework string) string {
	credentials = credentials.withDefaults()

	dbCliDetect := conventions.MySQLDumpBinDetect

	// Common options
	commonArgs := []string{"\"$DUMP_BIN\"", "--max-allowed-packet=" + conventions.MySQLMaxAllowedPacket, "--force", "--single-transaction", "--no-tablespaces", "-h" + conventions.DefaultDBHost, "-u" + engine.ShellQuote(credentials.Username)}

	// Pass 1: Metadata
	metadataArgs := append([]string{}, commonArgs...)
	metadataArgs = append(metadataArgs, "--no-data", "--routines", "--triggers")
	metadataArgs = append(metadataArgs, engine.ShellQuote(credentials.Database))

	// Pass 2: Data
	dataArgs := append([]string{}, commonArgs...)
	dataArgs = append(dataArgs, "--no-create-info", "--skip-triggers")
	ignoreArgs := buildIgnoredTableArgs(credentials.Database, credentials.TablePrefix, noNoise, noPII, framework)
	dataArgs = append(dataArgs, ignoreArgs...)
	dataArgs = append(dataArgs, engine.ShellQuote(credentials.Database))
	dumpCmd := fmt.Sprintf("{ %s; %s; }", strings.Join(metadataArgs, " "), strings.Join(dataArgs, " "))
	return dbCliDetect + " && " + dumpCmd
}

// buildIgnoredTableArgs returns docker exec --ignore-table flags for the given credentials and filter flags.
func buildIgnoredTableArgs(dbName string, dbPrefix string, noNoise bool, noPII bool, framework string) []string {
	dbPrefix = engine.SafeTablePrefix(dbPrefix)
	tables := getIgnoredTableList(noNoise, noPII, framework)
	if len(tables) == 0 {
		return nil
	}

	args := make([]string, 0, len(tables))
	for _, t := range tables {
		args = append(args, "--ignore-table="+dbName+"."+dbPrefix+t)
	}
	return args
}

func getIgnoredTableList(noNoise bool, noPII bool, framework string) []string {
	return engine.GetFrameworkIgnoredTables(framework, noNoise, noPII)
}

func mysqlPasswordExportPrefix(password string) string {
	if strings.TrimSpace(password) == "" {
		return ""
	}
	return "export MYSQL_PWD=" + engine.ShellQuote(password) + "; "
}

func buildLocalMySQLClientCommandScript(credentials dbCredentials, force bool) string {
	credentials = credentials.withDefaults()

	query := "exec \"$DB_CLI\" --max-allowed-packet=512M -u " + engine.ShellQuote(credentials.Username) + " " + engine.ShellQuote(credentials.Database)
	if force {
		query += " -f"
		query = "{ echo \"SET FOREIGN_KEY_CHECKS=0; SET UNIQUE_CHECKS=0; SET AUTOCOMMIT=0;\"; cat; echo \"COMMIT; SET FOREIGN_KEY_CHECKS=1; SET UNIQUE_CHECKS=1; SET AUTOCOMMIT=1;\"; } | " + query
	}

	return strings.Join([]string{
		conventions.MySQLClientBinDetect,
		query,
	}, " && ")
}

func formatRemoteDBProbeWarning(remoteName string, err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("Could not auto-detect DB credentials for '%s' from remote metadata (.env/env.php) (%v). Falling back to default credentials.", remoteName, err)
}

func BuildRemoteMySQLDumpCommandForTest(host string, port int, username string, password string, database string, compress bool) string {
	return buildRemoteMySQLDumpCommandString(dbCredentials{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Database: database,
	}, false, false, "magento2", compress)
}

func BuildRemoteMySQLDumpCommandWithPrefixForTest(database string, tablePrefix string, noNoise bool, noPII bool, framework string) string {
	return buildRemoteMySQLDumpCommandString(dbCredentials{
		Username:    conventions.DefaultMagentoDBUser,
		Password:    conventions.DefaultMagentoDBPass,
		Database:    database,
		TablePrefix: tablePrefix,
	}, noNoise, noPII, framework, false)
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

// BuildIgnoredTableArgsForTest exposes buildIgnoredTableArgs for tests.
func BuildIgnoredTableArgsForTest(dbName string, dbPrefix string, noNoise bool, noPII bool, framework string) []string {
	return buildIgnoredTableArgs(dbName, dbPrefix, noNoise, noPII, framework)
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

	queryCmd := "exec \"$DB_CLI\" -u " + engine.ShellQuote(credentials.Username) + " -e " + engine.ShellQuote(query) + " " + engine.ShellQuote(credentials.Database)

	return strings.Join([]string{
		conventions.MySQLClientBinDetect,
		queryCmd,
	}, " && ")
}

func buildRemoteMySQLQueryCommandString(credentials dbCredentials, query string) string {
	credentials = credentials.withDefaults()

	args := []string{"mysql"}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		args = append(args, "-h"+engine.ShellQuote(host))
	}
	if credentials.Port > 0 {
		args = append(args, "-P"+strconv.Itoa(credentials.Port))
	}
	args = append(args, "-u"+engine.ShellQuote(credentials.Username), "-e", engine.ShellQuote(query))

	return mysqlPasswordExportPrefix(credentials.Password) + strings.Join(args, " ")
}

func GetDatabaseSize(config engine.Config, remoteName string, remoteCfg engine.RemoteConfig, credentials dbCredentials, noNoise bool, noPII bool) (int64, error) {
	credentials = credentials.withDefaults()
	ignoredTables := getIgnoredTableList(noNoise, noPII, config.Framework)
	whereClause := fmt.Sprintf("WHERE table_schema = '%s'", strings.ReplaceAll(credentials.Database, "'", "''"))
	if len(ignoredTables) > 0 {
		quotedTables := make([]string, len(ignoredTables))
		for i, t := range ignoredTables {
			tableName := credentials.TablePrefix + t
			quotedTables[i] = "'" + strings.ReplaceAll(tableName, "'", "''") + "'"
		}
		whereClause += fmt.Sprintf(" AND table_name NOT IN (%s)", strings.Join(quotedTables, ","))
	}

	// query the total logical size (data_length is better for estimating dump size than avg_row_length)
	query := fmt.Sprintf("SELECT SUM(data_length) FROM information_schema.tables %s", whereClause)

	mysqlArgs := []string{"\"$DB_CLI\"", "-BN"}
	if host := strings.TrimSpace(credentials.Host); host != "" {
		mysqlArgs = append(mysqlArgs, "-h"+engine.ShellQuote(host))
	}
	if credentials.Port > 0 {
		mysqlArgs = append(mysqlArgs, "-P"+strconv.Itoa(credentials.Port))
	}
	mysqlArgs = append(mysqlArgs, "-u"+engine.ShellQuote(credentials.Username), "-e", engine.ShellQuote(query))

	dbCliDetect := conventions.MySQLClientBinDetect
	mysqlCmd := mysqlPasswordExportPrefix(credentials.Password) + strings.Join(mysqlArgs, " ")
	cmdStr := fmt.Sprintf("%s && %s", dbCliDetect, mysqlCmd)

	var output []byte
	var err error
	if remoteName == "local" {
		containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
		output, err = exec.Command("docker", "exec", containerName, "sh", "-c", cmdStr).CombinedOutput()
	} else {
		sshCmd := remote.BuildSSHExecCommand(remoteName, remoteCfg, true, cmdStr)
		output, err = sshCmd.CombinedOutput()
	}

	if err != nil {
		return 0, err
	}

	totalSizeStr := strings.TrimSpace(string(output))
	if totalSizeStr == "" || totalSizeStr == "NULL" {
		return 0, nil
	}

	var logicalSize int64
	_, _ = fmt.Sscanf(totalSizeStr, "%d", &logicalSize)

	// Since mysqldump generates a compact SQL text file while InnoDB stores data in 16KB pages
	// (often with significant internal overhead/fragmentation), the logical size is usually
	// an overestimate. We apply a 0.6 heuristic to bring it closer to actual dump results.
	targetSize := int64(float64(logicalSize) * 0.6)

	return targetSize, nil
}

func BuildLocalMySQLQueryCommandScriptForTest(username string, database string, query string) string {
	return buildLocalMySQLQueryCommandScript(dbCredentials{
		Username: username,
		Database: database,
	}, query)
}
