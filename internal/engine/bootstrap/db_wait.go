package bootstrap

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"govard/internal/conventions"
)

// WaitForMySQLDatabase polls a MySQL/MariaDB connection until it succeeds,
// retrying for up to 30 seconds. Used by frameworks whose fresh-install runs
// an installer against the DB before the DB container has finished starting.
// Exported (unlike the other staged_project.go helpers, this one has no
// unexported/exported pair) because WordPress's FrameworkBootstrap moved
// out of package bootstrap as part of the self-contained-framework-folder
// migration and is currently the only caller.
func WaitForMySQLDatabase(projectDir string, runner func(command string) error, host, user, pass, name string) error {
	code := strings.Join([]string{
		"mysqli_report(MYSQLI_REPORT_OFF);",
		"$db = mysqli_init();",
		"if (!$db) { exit(1); }",
		"if (!@mysqli_real_connect($db, " + strconv.Quote(host) + ", " + strconv.Quote(user) + ", " + strconv.Quote(pass) + ", " + strconv.Quote(name) + ", " + strconv.Itoa(conventions.MySQLPort) + ")) {",
		"    exit(1);",
		"}",
	}, "\n")

	var lastErr error
	for range 30 {
		if err := runPHPOneLiner(projectDir, runner, code); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(time.Second)
	}

	return fmt.Errorf("wait for database: %w", lastErr)
}
