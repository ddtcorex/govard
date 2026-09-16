package deploy

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// SeedDB names the origin database to snapshot. The password travels in a
// MYSQL_PWD env entry prepended to the client argv, never as --password= or
// -p (both are visible in argv) — same rule as COMPOSER_AUTH, one notch down
// (documented at mysqlPasswordEnv): mysqldump accepts no password on stdin.
type SeedDB struct {
	Container string
	User      string
	Password  string
	Name      string
	Engine    string
}

// SeedSource is what the seed reads: the origin must be running (fail-loud
// otherwise), and paths are host-absolute.
type SeedSource struct {
	OriginRunning bool
	DB            SeedDB
	MediaSource   string
	EnvSource     string
}

// SeedSpec is the resolved snapshot plan: per-container client argv without
// secrets, media endpoints, and the env-file rewrite mapping. The caller
// executes it through the container runtime; argv never names a secret.
type SeedSpec struct {
	// DBDumpContainer is the origin database container; DBDumpArgs is the
	// mysqldump client argv run inside it (password arrives via MYSQL_PWD).
	DBDumpContainer string
	DBDumpArgs      []string
	// DBImportArgs is the mysql client argv run inside the sandbox container.
	DBImportArgs []string
	DBPassword   string
	MediaSource  string
	MediaTarget  string
	EnvSource    string
	EnvMapping   map[string]string
}

// SeedEnvRewriter rewrites one env file's content for the sandbox (base_url,
// local hosts). Frameworks register implementations; the core only dispatches.
type SeedEnvRewriter func(content []byte, mapping map[string]string) ([]byte, error)

// ResolveSeedSpec validates the source and resolves the snapshot plan. A
// stopped origin is a refusal, not an empty sandbox: silent emptiness is the
// failure this gate exists to prevent.
func ResolveSeedSpec(source SeedSource) (SeedSpec, error) {
	if !source.OriginRunning {
		return SeedSpec{}, fmt.Errorf("the origin project is not running; start it with `govard env up` first, or pass --no-seed for an empty sandbox")
	}
	if strings.TrimSpace(source.DB.Container) == "" || strings.TrimSpace(source.DB.Name) == "" {
		return SeedSpec{}, fmt.Errorf("cannot seed without the origin database container and name")
	}
	return SeedSpec{
		DBDumpContainer: source.DB.Container,
		DBDumpArgs:      []string{"mysqldump", "-u", source.DB.User, "--single-transaction", "--skip-lock-tables", source.DB.Name},
		DBImportArgs:    []string{"mysql", "-u", source.DB.User, source.DB.Name},
		DBPassword:      source.DB.Password,
		MediaSource:     source.MediaSource,
		EnvSource:       source.EnvSource,
		EnvMapping:      map[string]string{},
	}, nil
}

var definerRe = regexp.MustCompile(`(?i)/\*!50013 DEFINER=` + "`[^`]+`@`[^`]+`" + ` SQL SECURITY DEFINER \*/`)

// StripDefiner removes foreign DEFINER clauses from a dump: they name users
// that do not exist in the sandbox, and creating views with them needs SUPER.
// Found live: a dev8-staging definer broke a restore with "Access denied; you
// need SUPER".
func StripDefiner(sql string) string {
	return definerRe.ReplaceAllString(sql, "")
}

// seedDBPolls bounds how long the seed waits for the sandbox database to
// answer: a fresh MariaDB needs seconds after the container reports SSH.
const seedDBPolls = 30

// waitSandboxDB polls the sandbox database until it answers. It runs as the
// container's root over the unix socket, which Debian MariaDB grants without a
// password — no credential exists yet at this point by definition.
func waitSandboxDB(ctx context.Context, runtime SandboxRuntime, container string) error {
	var err error
	for i := 0; i < seedDBPolls; i++ {
		var out string
		out, err = runtime.Exec(ctx, container, nil, "mysqladmin", "ping")
		if err == nil && strings.Contains(out, "alive") {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("the sandbox database in %s never answered: %v", container, err)
}

// mysqlPasswordEnv smuggles a password to a mysql client that accepts no
// password on stdin. It is visible in the container's process list while the
// command runs — acknowledged and bounded: the alternative (.my.cnf) leaves a
// secret file behind, and argv squirrels (--password=) are refused outright.
// Every client that accepts its input on stdin (the SQL itself) uses stdin.
func mysqlPasswordEnv(password string) []string {
	if password == "" {
		return nil
	}
	return []string{"env", "MYSQL_PWD=" + password}
}

// runSandboxSeed snapshots the origin into a running sandbox container:
// create the app user/database, dump (origin) → strip → import (sandbox),
// stream one media tree, rewrite one env file. Every step is fail-loud;
// nothing is skipped silently.
func runSandboxSeed(ctx context.Context, runtime SandboxRuntime, out io.Writer, sandbox string, request SandboxRequest) error {
	spec, err := ResolveSeedSpec(SeedSource{
		OriginRunning: request.SeedOriginRunning,
		DB: SeedDB{
			Container: request.SeedDBContainer,
			User:      request.SeedDBUser,
			Password:  request.SeedDBPassword,
			Name:      request.SeedDBName,
		},
		MediaSource: request.SeedMediaSource,
		EnvSource:   request.SeedEnvSource,
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "waiting for the sandbox database")
	if err := waitSandboxDB(ctx, runtime, sandbox); err != nil {
		return err
	}

	create := fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'; CREATE DATABASE IF NOT EXISTS `%s`; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'; FLUSH PRIVILEGES;",
		escapeIdent(request.SeedDBUser), escapeLiteral(request.SeedDBPassword), escapeIdent(request.SeedDBName), escapeIdent(request.SeedDBName), escapeIdent(request.SeedDBUser))
	if _, err := runtime.Exec(ctx, sandbox, []byte(create), "mysql"); err != nil {
		return fmt.Errorf("create the sandbox database user: %w", err)
	}

	fmt.Fprintf(out, "dumping the origin database %s\n", request.SeedDBName)
	dumpArgs := append(mysqlPasswordEnv(request.SeedDBPassword), spec.DBDumpArgs...)
	dump, err := runtime.Exec(ctx, spec.DBDumpContainer, nil, dumpArgs...)
	if err != nil {
		return fmt.Errorf("dump the origin database: %w", err)
	}

	fmt.Fprintln(out, "importing the snapshot into the sandbox")
	importArgs := append(mysqlPasswordEnv(request.SeedDBPassword), spec.DBImportArgs...)
	if _, err := runtime.Exec(ctx, sandbox, []byte(StripDefiner(dump)), importArgs...); err != nil {
		return fmt.Errorf("import the snapshot: %w", err)
	}

	if request.SeedMediaSource != "" && request.SeedMediaTarget != "" {
		fmt.Fprintf(out, "copying media %s\n", request.SeedMediaSource)
		// The tar stream passes through this process: no new interface method,
		// no shell on either side. Very large media trees buffer in memory;
		// streaming across two Exec calls is the follow-up when one hurts.
		media, err := runtime.Exec(ctx, request.SeedAppContainer, nil, "tar", "-C", request.SeedMediaSource, "-cf", "-", ".")
		if err != nil {
			return fmt.Errorf("read the origin media: %w", err)
		}
		if _, err := runtime.Exec(ctx, sandbox, []byte(media), "mkdir", "-p", request.SeedMediaTarget); err != nil {
			return fmt.Errorf("prepare the sandbox media target: %w", err)
		}
		if _, err := runtime.Exec(ctx, sandbox, []byte(media), "tar", "-C", request.SeedMediaTarget, "-xf", "-"); err != nil {
			return fmt.Errorf("write the sandbox media: %w", err)
		}
	}

	if request.EnvRewriter != nil && request.SeedEnvSource != "" && request.SeedEnvTarget != "" {
		fmt.Fprintf(out, "rewriting %s for the sandbox\n", request.SeedEnvSource)
		raw, err := runtime.Exec(ctx, request.SeedAppContainer, nil, "cat", request.SeedEnvSource)
		if err != nil {
			return fmt.Errorf("read the origin env file: %w", err)
		}
		rewritten, err := request.EnvRewriter([]byte(raw), request.SeedEnvMapping)
		if err != nil {
			return fmt.Errorf("rewrite the env file: %w", err)
		}
		if _, err := runtime.Exec(ctx, sandbox, rewritten, "tee", request.SeedEnvTarget); err != nil {
			return fmt.Errorf("write the sandbox env file: %w", err)
		}
	}
	return nil
}

// escapeIdent quotes a MySQL identifier; escapeLiteral quotes a string
// literal. Both double the quote they use, which is the whole grammar.
func escapeIdent(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}

// escapeLiteral quotes a MySQL string literal.
func escapeLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
