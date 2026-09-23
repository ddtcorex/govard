package deploy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"govard/internal/engine"
)

// SeedDB names the origin database to snapshot. The password travels in a
// MYSQL_PWD environment entry the runtime passes through by name, never as
// --password= or -p (both are visible in argv) and never as a NAME=value argv
// entry either — same rule as COMPOSER_AUTH, one notch down (documented at
// mysqlPasswordEnv): mysqldump accepts no password on stdin.
type SeedDB struct {
	Container string
	User      string
	Password  string
	Name      string
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
// secrets, media endpoints, and the env-file rewrite target. The caller
// executes it through the container runtime; argv never names a secret.
type SeedSpec struct {
	// DBDumpContainer is the origin database container. DBDumpFlags is the
	// client argv without the binary: minimal MariaDB images ship
	// mariadb-dump without the mysqldump symlink (and MySQL images only have
	// mysqldump), so DBDumpCandidates is probed in order at seed time.
	DBDumpContainer  string
	DBDumpFlags      []string
	DBDumpCandidates []string
	// DBImportArgs is the mysql client argv run inside the sandbox container.
	DBImportArgs []string
	DBPassword   string
	MediaSource  string
	MediaTarget  string
	EnvSource    string
}

// SeedEnvRewriter rewrites one env file's content for the sandbox. It is an
// alias, not a copy: the canonical type lives in engine beside the registry,
// and deploy only carries values (deploy must not sprout a parallel type that
// drifts from the registered one).
type SeedEnvRewriter = engine.SandboxSeedRewriter

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
		DBDumpContainer:  source.DB.Container,
		DBDumpFlags:      []string{"-u", source.DB.User, "--single-transaction", "--skip-lock-tables", source.DB.Name},
		DBDumpCandidates: []string{"mariadb-dump", "mysqldump"},
		DBImportArgs:     []string{"mysql", "-u", source.DB.User, source.DB.Name},
		DBPassword:       source.DB.Password,
		MediaSource:      source.MediaSource,
		EnvSource:        source.EnvSource,
	}, nil
}

// resolveDumpBinary picks the dump client the origin container actually has.
// `command -v` prints the resolved path; the answer must name the candidate
// asked for, otherwise a canned answer for one query would satisfy another.
func resolveDumpBinary(ctx context.Context, runtime SandboxRuntime, container string, candidates []string) (string, error) {
	for _, candidate := range candidates {
		out, err := runtime.Exec(ctx, container, nil, "sh", "-c", "command -v "+candidate)
		if err != nil {
			continue
		}
		if path := strings.TrimSpace(out); path != "" && strings.Contains(path, candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no dump client (%s) in %s", strings.Join(candidates, ", "), container)
}

// ResolveDumpBinaryForTest exposes resolveDumpBinary to the tests/ package.
func ResolveDumpBinaryForTest(ctx context.Context, runtime SandboxRuntime, container string) (string, error) {
	return resolveDumpBinary(ctx, runtime, container, []string{"mariadb-dump", "mysqldump"})
}

var definerRe = regexp.MustCompile(`(?i)/\*!50013 DEFINER=` + "`[^`]+`@`[^`]+`" + ` SQL SECURITY DEFINER \*/`)

// StripDefiner removes foreign DEFINER clauses from a dump: they name users
// that do not exist in the sandbox, and creating views with them needs SUPER.
// Found live: a dev8-staging definer broke a restore with "Access denied; you
// need SUPER".
func StripDefiner(sql string) string {
	return definerRe.ReplaceAllString(sql, "")
}

// seedStripLineLimit bounds one line of a dump before the stripper refuses it.
// mysqldump's extended INSERT is bounded by net_buffer_length (about 1 MiB per
// the MySQL manual), so 64 MiB is far past anything a real dump produces: it is
// a refusal, not a working size. A cap rather than an unbounded buffer is the
// difference between a loud error and the allocator dying.
const seedStripLineLimit = 64 << 20

// StripDefinerStream rewrites a dump as it flows, so a multi-gigabyte database
// never has to be resident. One line is held at a time — a DEFINER clause never
// spans a line, so per-line rewriting is what the whole-string version does,
// with bounded memory.
func StripDefinerStream(source io.Reader, target io.Writer) error {
	return stripDefinerStream(source, target, seedStripLineLimit)
}

// StripDefinerStreamForTest runs the streaming stripper with a small line limit,
// so the refusal is exercised without a 64 MiB fixture.
func StripDefinerStreamForTest(source io.Reader, target io.Writer, lineLimit int) error {
	return stripDefinerStream(source, target, lineLimit)
}

func stripDefinerStream(source io.Reader, target io.Writer, lineLimit int) error {
	// The initial buffer must not be larger than the limit, or a line between
	// the two sizes would be accepted without the cap ever being consulted.
	initial := 64 << 10
	if lineLimit < initial {
		initial = lineLimit
	}
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, initial), lineLimit)
	writer := bufio.NewWriterSize(target, 64<<10)
	for scanner.Scan() {
		line := scanner.Bytes()
		// ReplaceAll copies the unmatched remainder even when nothing matches,
		// which for a line-oriented dump means allocating every line again.
		// Asking first keeps the pass-through free: a dump is millions of lines
		// and a handful of DEFINER clauses.
		if definerRe.Match(line) {
			line = definerRe.ReplaceAll(line, nil)
		}
		if _, err := writer.Write(line); err != nil {
			return err
		}
		// Scanner drops the delimiter; the dump is line-oriented, so putting it
		// back keeps the stream byte-for-byte equivalent to the string version.
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("the dump has a line longer than %d bytes: %w", lineLimit, err)
		}
		return err
	}
	return writer.Flush()
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

// mysqlPasswordEnv carries a password to a mysql client that accepts no password
// on stdin. It returns an *environment* entry, never an argv one: the runtime is
// asked to pass the name through (`docker exec -e MYSQL_PWD`), so the value
// lives in the runtime process's environment — readable by its owner and root,
// not by every local user running `ps`. Inside the container the variable is
// still visible in the process list while the command runs, which is
// acknowledged and bounded: the alternative (.my.cnf) leaves a secret file
// behind, and argv squirrels (--password=) are refused outright. Every client
// that accepts its input on stdin (the SQL itself) uses stdin instead.
func mysqlPasswordEnv(password string) []string {
	if password == "" {
		return nil
	}
	return []string{"MYSQL_PWD=" + password}
}

// streamDatabaseDump moves the origin's dump into the sandbox without ever
// holding it. The dump writes into a pipe, the stripper rewrites one line at a
// time into a second pipe, and the import reads that: the only dump bytes
// resident are one line. A dump and an import that both run to completion keep
// each other honest through the pipes' back-pressure — a stalled import blocks
// the dump instead of filling a buffer.
//
// A dump that dies midway leaves a partially imported database, and that is
// reported rather than swallowed: the import can still exit 0 after receiving a
// truncated stream.
func streamDatabaseDump(ctx context.Context, runtime SandboxRuntime, from, to string, env []string, dumpArgs, importArgs []string) error {
	stripStdin, dumpStdout := io.Pipe()
	importStdin, stripStdout := io.Pipe()

	dumpDone := make(chan error, 1)
	go func() {
		err := runtime.ExecStream(ctx, from, env, nil, dumpStdout, dumpArgs...)
		// Closing with the dump's error is what tells the stripper, and through
		// it the import, that the stream is incomplete.
		_ = dumpStdout.CloseWithError(err)
		dumpDone <- err
	}()

	stripDone := make(chan error, 1)
	go func() {
		err := StripDefinerStream(stripStdin, stripStdout)
		// Stop a dump that is still writing: the stripper is its only reader.
		_ = stripStdin.CloseWithError(err)
		_ = stripStdout.CloseWithError(err)
		stripDone <- err
	}()

	importErr := runtime.ExecStream(ctx, to, env, importStdin, nil, importArgs...)
	// The import stops reading when its client exits; release the reader so the
	// stripper cannot wait forever for a consumer that is gone.
	_ = importStdin.CloseWithError(importErr)

	dumpErr := <-dumpDone
	stripErr := <-stripDone

	if dumpErr != nil {
		return fmt.Errorf("dump the origin database (the sandbox database is partial, so re-run `govard sandbox up`): %w", dumpErr)
	}
	if importErr != nil {
		return fmt.Errorf("import the snapshot (the sandbox database is partial, so re-run `govard sandbox up`): %w", importErr)
	}
	if stripErr != nil {
		return fmt.Errorf("rewrite the snapshot for the sandbox: %w", stripErr)
	}
	return nil
}

// streamContainerTar moves one directory from the origin container into the
// sandbox through this process without holding the archive: a media tree of
// tens of gigabytes streams the same way the dump does.
func streamContainerTar(ctx context.Context, runtime SandboxRuntime, from, to, source, target string) error {
	importStdin, tarStdout := io.Pipe()

	writeDone := make(chan error, 1)
	go func() {
		err := runtime.ExecStream(ctx, from, nil, nil, tarStdout, "tar", "-C", source, "-cf", "-", ".")
		_ = tarStdout.CloseWithError(err)
		writeDone <- err
	}()

	readErr := runtime.ExecStream(ctx, to, nil, importStdin, nil, "tar", "-C", target, "-xf", "-")
	_ = importStdin.CloseWithError(readErr)

	writeErr := <-writeDone
	if writeErr != nil {
		return fmt.Errorf("read the origin media: %w", writeErr)
	}
	if readErr != nil {
		return fmt.Errorf("write the sandbox media: %w", readErr)
	}
	return nil
}

// runSandboxSeed snapshots the origin into a running sandbox container:
// create the app user/database, dump (origin) → strip → import (sandbox),
// stream one media tree, rewrite one env file. webPort is the sandbox's
// published HTTP port (0 when the profile serves no web tier): it defaults
// base_url, because docker chooses the port and no caller can know it upfront.
// Every step is fail-loud; nothing is skipped silently.
func runSandboxSeed(ctx context.Context, runtime SandboxRuntime, out io.Writer, sandbox string, webPort int, request SandboxRequest) error {
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
	dumpBin, err := resolveDumpBinary(ctx, runtime, spec.DBDumpContainer, spec.DBDumpCandidates)
	if err != nil {
		return err
	}
	dumpArgs := append([]string{dumpBin}, spec.DBDumpFlags...)
	passwordEnv := mysqlPasswordEnv(spec.DBPassword)

	fmt.Fprintln(out, "importing the snapshot into the sandbox")
	if err := streamDatabaseDump(ctx, runtime, spec.DBDumpContainer, sandbox, passwordEnv, dumpArgs, spec.DBImportArgs); err != nil {
		return err
	}

	if request.SeedMediaSource != "" && request.SeedMediaTarget != "" {
		fmt.Fprintf(out, "copying media %s\n", request.SeedMediaSource)
		// The tar stream passes through this process between two Exec calls, so
		// neither side needs a shell and no archive is held.
		if _, err := runtime.Exec(ctx, sandbox, nil, "mkdir", "-p", request.SeedMediaTarget); err != nil {
			return fmt.Errorf("prepare the sandbox media target: %w", err)
		}
		if err := streamContainerTar(ctx, runtime, request.SeedAppContainer, sandbox, request.SeedMediaSource, request.SeedMediaTarget); err != nil {
			return err
		}
	}

	if request.EnvRewriter != nil && request.SeedEnvSource != "" && request.SeedEnvTarget != "" {
		fmt.Fprintf(out, "rewriting %s for the sandbox\n", request.SeedEnvSource)
		raw, err := runtime.Exec(ctx, request.SeedAppContainer, nil, "cat", request.SeedEnvSource)
		if err != nil {
			return fmt.Errorf("read the origin env file: %w", err)
		}
		// base_url defaults to the sandbox web URL; explicit mapping wins.
		mapping := map[string]string{}
		if webPort > 0 {
			mapping["base_url"] = fmt.Sprintf("http://127.0.0.1:%d/", webPort)
		}
		for key, value := range request.SeedEnvMapping {
			mapping[key] = value
		}
		rewritten, skipped, err := request.EnvRewriter([]byte(raw), mapping)
		if err != nil {
			return fmt.Errorf("rewrite the env file: %w", err)
		}
		for _, key := range skipped {
			fmt.Fprintf(out, "note: env key %q not present, left as-is\n", key)
		}
		// The shared tree does not exist until the first deploy links it: the
		// seed is what creates the file's home.
		if _, err := runtime.Exec(ctx, sandbox, nil, "mkdir", "-p", path.Dir(request.SeedEnvTarget)); err != nil {
			return fmt.Errorf("prepare the sandbox env target: %w", err)
		}
		if _, err := runtime.Exec(ctx, sandbox, rewritten, "tee", request.SeedEnvTarget); err != nil {
			return fmt.Errorf("write the sandbox env file: %w", err)
		}
	}
	// Exec runs as container root, so everything the seed wrote is root-owned
	// while the pipeline runs as the deploy user: hand the tree over, or the
	// first deploy dies in deploy:check on a non-writable deploy path (found
	// live). Numeric IDs, never the name: the container's passwd is not
	// guaranteed to resolve them, and chown accepts both.
	if _, err := runtime.Exec(ctx, sandbox, nil, "chown", "-R",
		fmt.Sprintf("%d:%d", SandboxUserUID, SandboxUserGID), SandboxDefaultPaths().DeployPath); err != nil {
		return fmt.Errorf("hand the seeded tree to the deploy user: %w", err)
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
