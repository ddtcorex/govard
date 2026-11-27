package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"govard/internal/conventions"
)

// RunSQLViaDockerExec executes a SQL statement via docker exec on the
// project's local DB container. Framework packages supply their own SQL.
func RunSQLViaDockerExec(containerName string, dbUser string, dbPassword string, dbName string, sql string) error {
	script := fmt.Sprintf(
		`if command -v mysql >/dev/null 2>&1; then DB_CLI=mysql; elif command -v mariadb >/dev/null 2>&1; then DB_CLI=mariadb; else exit 1; fi && echo %s | "$DB_CLI" -u %s %s -f`,
		conventions.ShellQuote(sql), conventions.ShellQuote(dbUser), conventions.ShellQuote(dbName),
	)

	args := []string{"exec", "-i"}
	if dbPassword != "" {
		// By name: docker takes the value from its own environment, so the
		// password is not in the argument list.
		args = append(args, "-e", "MYSQL_PWD")
	}
	args = append(args, containerName, "sh", "-lc", script)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	if dbPassword != "" {
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+dbPassword)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SQL exec failed: %w: %s", err, out)
	}
	return nil
}
