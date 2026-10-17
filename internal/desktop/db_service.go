package desktop

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"govard/internal/conventions"
)

// DBService exposes project database info and guarded ad-hoc queries to the
// desktop data tab. Queries default to read-only: only SELECT-style
// statements run unless the caller passes the explicit per-project write
// opt-in plus its confirm token. Connection secrets never appear in any
// returned struct: DBInfo carries no password field by construction, and
// query output passes through sanitizeUIText.
type DBService struct {
	serviceBase
}

// NewDBService builds a DBService with the default platform. (Wiring into
// App/boundServices plus bridge routes lands with the data tab UI task.)
func NewDBService() *DBService {
	return &DBService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// DBInfo is the masked connection fact sheet for one project database. It
// deliberately has no password field: HasPassword only reports whether one
// is configured, and DisplayURL renders it as ****.
type DBInfo struct {
	Project     string `json:"project"`
	Engine      string `json:"engine"`
	Host        string `json:"host"`
	Port        string `json:"port"`
	Database    string `json:"database"`
	User        string `json:"user"`
	HasPassword bool   `json:"hasPassword"`
	DisplayURL  string `json:"displayUrl"`
}

// DBQueryResult is one executed ad-hoc query: the classified first keyword,
// whether it ran under the read-only gate, and the sanitized output.
type DBQueryResult struct {
	Project   string `json:"project"`
	Statement string `json:"statement"`
	Output    string `json:"output"`
	ReadOnly  bool   `json:"readOnly"`
}

// DBQueryCall records one executor invocation for tests. It carries no
// password by construction.
type DBQueryCall struct {
	Container string
	Scheme    string
	Host      string
	Port      string
	Database  string
	User      string
	SQL       string
}

// dbConnection is the discovered live-database endpoint for a project. It is
// the only type that holds the password, and it never crosses into UI
// structs, errors, or logs: the executor receives it directly and passes the
// secret via container env (MYSQL_PWD/PGPASSWORD), never via shell text.
type dbConnection struct {
	project   string
	container string
	scheme    string
	user      string
	password  string
	host      string
	port      string
	database  string
}

// Seams below keep discovery and execution hermetic in tests; ResetStateForTest
// restores every one of them.

var inspectDBContainerEnvForDesktop = defaultInspectDBContainerEnvForDesktop
var publishedDBContainerPortForDesktop = defaultPublishedDBContainerPortForDesktop
var inspectDBContainerIPsForDesktop = defaultInspectDBContainerIPsForDesktop
var probeReachableDBHostForDesktop = defaultProbeReachableDBHostForDesktop
var runDBQueryForDesktop = defaultRunDBQueryForDesktop

func defaultInspectDBContainerEnvForDesktop(container string) (string, error) {
	out, err := exec.Command(
		"docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", container,
	).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func defaultPublishedDBContainerPortForDesktop(container string, internalPort string) (string, error) {
	out, err := exec.Command("docker", "port", container, internalPort).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func defaultInspectDBContainerIPsForDesktop(container string) (string, error) {
	out, err := exec.Command(
		"docker", "inspect", "-f",
		"{{range .NetworkSettings.Networks}}{{println .IPAddress}}{{end}}",
		container,
	).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func defaultProbeReachableDBHostForDesktop(candidates []string, port string) string {
	return chooseReachableContainerHost(candidates, port)
}

func defaultRunDBQueryForDesktop(ctx context.Context, target dbConnection, sql string) (string, error) {
	args, _, cleanup, err := buildDBQueryDockerArgs(target, sql)
	if err != nil {
		return "", err
	}
	defer cleanup()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return string(out), err
}

// buildDBQueryDockerArgs builds the docker exec argv for one ad-hoc query.
// Like the transfer path, the password travels via a 0600 --env-file, so it
// never appears in argv (visible via the process list): only the temp path
// does, and cleanup removes the file.
func buildDBQueryDockerArgs(target dbConnection, sql string) ([]string, string, func(), error) {
	var script string
	if target.scheme == "postgresql" {
		script = "exec psql -U " + shellQuoteForDB(target.user) +
			" -v ON_ERROR_STOP=1 -c " + shellQuoteForDB(sql) +
			" " + shellQuoteForDB(target.database)
	} else {
		script = "exec mysql -u " + shellQuoteForDB(target.user) +
			" -e " + shellQuoteForDB(sql) +
			" " + shellQuoteForDB(target.database)
	}
	return buildDBTransferDockerArgs(target.container, target.password, dbTransferEnvName(target.scheme), script)
}

// shellQuoteForDB single-quotes s for sh -lc. The password never reaches
// this helper: it travels via a 0600 --env-file, never via shell text.
func shellQuoteForDB(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// discoverDBConnection runs the credential-discovery path previously inline
// in openDBClient (docker inspect env for user/password/database, docker
// port plus container-IP probing for host/port, project config for the
// engine). openDBClient and DBService share it so discovery exists exactly
// once; only sampling failures fall back to defaults, never to secrets.
func discoverDBConnection(project string) (dbConnection, error) {
	normalizedProject := strings.TrimSpace(project)
	if normalizedProject == "" {
		selected, err := selectProject("")
		if err != nil {
			return dbConnection{}, err
		}
		normalizedProject = strings.TrimSpace(selected.name)
	}

	info, err := loadProjectInfo(normalizedProject)
	if err != nil {
		info = &projectInfo{name: normalizedProject}
	}

	containerProjectName := strings.TrimSpace(info.name)
	if containerProjectName == "" {
		containerProjectName = normalizedProject
	}
	containerName := containerProjectName + conventions.DBSuffix

	user := conventions.DefaultMagentoDBUser
	pass := conventions.DefaultMagentoDBPass
	db := conventions.DefaultMagentoDBName

	if envOut, err := inspectDBContainerEnvForDesktop(containerName); err == nil {
		for _, line := range strings.Split(envOut, "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
			if len(parts) != 2 {
				continue
			}
			switch parts[0] {
			case conventions.EnvMySQLUser, conventions.EnvPostgresUser:
				if parts[1] != "" {
					user = parts[1]
				}
			case conventions.EnvMySQLPassword, conventions.EnvPostgresPassword:
				if parts[1] != "" {
					pass = parts[1]
				}
			case conventions.EnvMySQLDatabase, conventions.EnvPostgresDatabase:
				if parts[1] != "" {
					db = parts[1]
				}
			}
		}
	}

	scheme := "mysql"
	internalPort := strconv.Itoa(conventions.MySQLPort)
	if info.configLoaded {
		dbType := strings.ToLower(info.config.Stack.Services.DB)
		if strings.Contains(dbType, conventions.ServicePostgreSQL) {
			scheme = "postgresql"
			internalPort = defaultDatabasePortForType(info.config.Stack.Services.DB)
		}
	}

	host := "127.0.0.1"
	port := internalPort
	if portOut, err := publishedDBContainerPortForDesktop(containerName, internalPort); err == nil {
		if mappedHost, mappedPort, ok := parseDockerPublishedPort(strings.TrimSpace(portOut)); ok {
			host = mappedHost
			port = mappedPort
		}
	}

	if host == "127.0.0.1" && port == internalPort {
		if ipOut, err := inspectDBContainerIPsForDesktop(containerName); err == nil {
			candidates := parseContainerIPAddresses(ipOut)
			if resolvedHost := probeReachableDBHostForDesktop(candidates, port); resolvedHost != "" {
				host = resolvedHost
			}
		}
	}

	return dbConnection{
		project:   containerProjectName,
		container: containerName,
		scheme:    scheme,
		user:      user,
		password:  pass,
		host:      host,
		port:      port,
		database:  db,
	}, nil
}

// GetDBInfo returns the masked connection fact sheet for a project. The
// password never appears: there is no field for it, and the display URL
// renders ****.
func (s *DBService) GetDBInfo(project string) (res DBInfo, err error) {
	defer RecoverPanic(&err, "GetDBInfo")
	conn, err := discoverDBConnection(project)
	if err != nil {
		return DBInfo{}, err
	}
	return DBInfo{
		Project:     sanitizeUIText(conn.project, 0),
		Engine:      sanitizeUIText(conn.scheme, 0),
		Host:        sanitizeUIText(conn.host, 0),
		Port:        sanitizeUIText(conn.port, 0),
		Database:    sanitizeUIText(conn.database, 0),
		User:        sanitizeUIText(conn.user, 0),
		HasPassword: strings.TrimSpace(conn.password) != "",
		DisplayURL:  maskedDBDisplayURL(conn),
	}, nil
}

// maskedDBDisplayURL renders the connection shape for UI surfaces with a
// literal **** password. It is built by hand instead of via net/url because
// URL encoding would mangle the mask into %2A%2A%2A%2A; every part is
// sanitized first, and each fact also ships in its own field.
func maskedDBDisplayURL(conn dbConnection) string {
	scheme := sanitizeUIText(conn.scheme, 0)
	user := sanitizeUIText(conn.user, 0)
	host := sanitizeUIText(conn.host, 0)
	port := sanitizeUIText(conn.port, 0)
	db := "/" + strings.TrimPrefix(sanitizeUIText(conn.database, 0), "/")
	if db == "/" {
		db = ""
	}
	at := ""
	if user != "" {
		at = user + ":****@"
	}
	return scheme + "://" + at + host + ":" + port + db
}

// RunDBQuery executes one ad-hoc SQL statement against the project database.
// Read-only statements run by default; anything else needs allowWrite plus
// the per-project confirm token from dbWriteConfirmToken, which the data tab
// confirm dialog passes back. Multi-statement input is always refused, and
// refusal happens before discovery so a refused statement touches nothing.
// Read-only means keyword-read-only: a SELECT that calls a known-destructive
// function (pg_terminate_backend, lo_*, dblink_exec) is a write and takes the
// same opt-in path.
func (s *DBService) RunDBQuery(project string, sql string, allowWrite bool, confirmToken string) (res DBQueryResult, err error) {
	defer RecoverPanic(&err, "RunDBQuery")
	keyword, readOnly, err := classifyDBStatement(sql)
	if err != nil {
		return DBQueryResult{}, err
	}
	if !readOnly && !allowWrite {
		return DBQueryResult{}, fmt.Errorf(
			"refused: %s statements need the explicit write opt-in for this project", keyword,
		)
	}
	conn, err := discoverDBConnection(project)
	if err != nil {
		return DBQueryResult{}, err
	}
	if !readOnly && normalizeDBConfirmToken(confirmToken) != dbWriteConfirmToken(conn.project) {
		return DBQueryResult{}, fmt.Errorf(
			"refused: invalid confirm token for a write query on %s",
			sanitizeUIText(conn.project, 0),
		)
	}
	out, err := runDBQueryForDesktop(s.lifecycleContext(), conn, strings.TrimSpace(sql))
	if err != nil {
		return DBQueryResult{}, err
	}
	return DBQueryResult{
		Project:   sanitizeUIText(conn.project, 0),
		Statement: keyword,
		Output:    sanitizeUIText(out, dbQueryOutputMax),
		ReadOnly:  readOnly,
	}, nil
}

// dbWriteConfirmToken is the per-project token the data tab confirm dialog
// passes back for a write statement. It guards against accidental clicks,
// not against malice: the desktop already holds the credentials.
func dbWriteConfirmToken(project string) string {
	return "confirm-write:" + normalizeDBProject(project)
}

// normalizeDBProject canonicalizes a project name for token purposes.
func normalizeDBProject(project string) string {
	return strings.ToLower(strings.TrimSpace(project))
}

// normalizeDBConfirmToken canonicalizes a caller-supplied confirm token the
// same way the generator canonicalizes the project, so a token the dialog
// shows with different casing or padding still verifies.
func normalizeDBConfirmToken(token string) string {
	return strings.ToLower(strings.TrimSpace(token))
}

// dbReadOnlyKeywords are the statement first keywords allowed without the
// write opt-in. Everything else (UPDATE, DELETE, DROP, INSERT, CREATE,
// ALTER, TRUNCATE, unknown words) takes the opt-in plus confirm-token path.
var dbReadOnlyKeywords = map[string]bool{
	"SELECT": true, "WITH": true, "SHOW": true,
	"DESCRIBE": true, "DESC": true, "EXPLAIN": true,
}

// dbQueryOutputMax caps ad-hoc query output kept for UI surfaces.
const dbQueryOutputMax = 20000

// classifyDBStatement returns the statement's first keyword (uppercased) and
// whether it may run under the read-only default. It rejects empty input,
// unparseable input, and multi-statement input: only exactly one statement
// is ever allowed, so a trailing SELECT cannot smuggle a second write.
// A first keyword on the allowlist is not enough on its own: the whole
// statement is additionally scanned for data-modifying words (data-modifying
// CTEs, SELECT ... INTO, SELECT ... FOR UPDATE, EXPLAIN ANALYZE) and for
// known-destructive function calls (pg_terminate_backend, lo_*, dblink_exec),
// so a read-looking opener cannot smuggle a write in its body. Read-only
// means keyword-read-only: function-call writes need the opt-in path.
func classifyDBStatement(sql string) (string, bool, error) {
	statements := splitDBStatements(sql)
	if len(statements) == 0 {
		return "", false, fmt.Errorf("query is empty")
	}
	if len(statements) > 1 {
		return "", false, fmt.Errorf("refused: multiple statements in one query are not allowed")
	}
	keyword := firstDBKeyword(statements[0])
	if keyword == "" {
		return "", false, fmt.Errorf("query is empty")
	}
	if !dbReadOnlyKeywords[keyword] {
		return keyword, false, nil
	}
	if hit := scanDBReadOnlyViolation(statements[0]); hit != "" {
		return keyword, false, nil
	}
	if hit := scanDBDeniedFunction(statements[0]); hit != "" {
		return keyword, false, nil
	}
	return keyword, true, nil
}

// dbReadOnlyDeniedWords are words that must not appear anywhere in a
// statement running under the read-only default, matched whole-word after
// string literals, comments, and quoted identifiers are stripped. They cover
// data-modifying CTE bodies (WITH ... DELETE/UPDATE/INSERT), table-creating
// SELECT ... INTO, locking reads (FOR UPDATE / FOR SHARE / LOCK IN SHARE
// MODE via the phrase table in scanDBReadOnlyViolation), executing
// EXPLAIN ANALYZE, and the DDL/DCL families that a read-looking opener
// must never smuggle.
var dbReadOnlyDeniedWords = map[string]bool{
	"DELETE": true, "UPDATE": true, "INSERT": true, "MERGE": true,
	"REPLACE": true, "UPSERT": true, "TRUNCATE": true, "DROP": true,
	"ALTER": true, "CREATE": true, "GRANT": true, "REVOKE": true,
	"LOAD": true, "COPY": true, "SET": true, "RESET": true,
	"DISCARD": true, "ANALYZE": true, "INTO": true, "VACUUM": true,
	"REINDEX": true, "CALL": true, "EXECUTE": true,
	// Set combinators stay out of the read-only default: each leg is a
	// SELECT on its own, but a stacked UNION is the classic injection
	// shape, so combined queries take the opt-in path like everything
	// else that is not a plain single-table read.
	"UNION": true, "INTERSECT": true, "EXCEPT": true,
	// Bare admin words, defense in depth: the first-keyword gate already
	// refuses them as openers, but a read-looking opener must never smuggle
	// them either. Read-only has no legitimate use for any of these.
	"LOCK": true, "FLUSH": true, "OPTIMIZE": true, "REPAIR": true,
	"KILL": true, "HANDLER": true, "INSTALL": true, "PURGE": true,
}

// dbReadOnlyDeniedPhrases are consecutive-word phrases refused in read-only
// mode, matched on the same stripped token stream. Locking reads take and
// hold row locks, so they take the opt-in path with the writes.
var dbReadOnlyDeniedPhrases = [][]string{
	{"FOR", "UPDATE"},
	{"FOR", "SHARE"},
	{"FOR", "KEY", "SHARE"},
	{"LOCK", "IN", "SHARE", "MODE"},
}

// dbDeniedReadOnlyFunctions are known-destructive functions that must never
// run under the read-only default, even when the statement opens with a
// read-only keyword (SELECT pg_terminate_backend(...)). Read-only here means
// keyword-read-only: the gate inspects the statement shape, not the engine's
// behavior, so function-call writes need the explicit write opt-in plus its
// confirm token, like every other write. Matching is case-insensitive,
// allows an optional schema qualification (pg_catalog.pg_terminate_backend)
// and whitespace before the call paren, and fails closed: any call-shaped
// occurrence takes the opt-in path.
var dbDeniedReadOnlyFunctions = []string{
	"pg_terminate_backend",
	"pg_cancel_backend",
	"lo_create",
	"lo_import",
	"lo_export",
	"lo_unlink",
	"dblink_exec",
}

// dbDeniedFunctionPatterns matches one call-shaped occurrence per denied
// function over the noise-stripped statement (literals, quoted identifiers,
// and comments blanked, so a name inside 'a pg_terminate_backend note' never
// trips while a real call always does).
var dbDeniedFunctionPatterns = buildDBDeniedFunctionPatterns()

func buildDBDeniedFunctionPatterns() []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(dbDeniedReadOnlyFunctions))
	for _, name := range dbDeniedReadOnlyFunctions {
		patterns = append(patterns, regexp.MustCompile(
			`(?i)(?:^|[^a-z0-9_$])(?:[a-z][a-z0-9_$]*\.)*`+name+`\s*\(`,
		))
	}
	return patterns
}

// scanDBDeniedFunction reports the first denied function called in the
// noise-stripped statement, or "" when the statement makes no such call.
func scanDBDeniedFunction(statement string) string {
	stripped := stripDBNoise(statement)
	for i, pattern := range dbDeniedFunctionPatterns {
		if pattern.MatchString(stripped) {
			return dbDeniedReadOnlyFunctions[i]
		}
	}
	return ""
}

// scanDBReadOnlyViolation reports the first denied word or phrase in
// statement, or "" when the statement is clean. Matching runs over the
// statement with string literals, quoted identifiers, and comments replaced
// by blanks, so a word inside 'a delete note' or a column named updated_at
// never trips it, while a real DELETE anywhere in the body always does.
func scanDBReadOnlyViolation(statement string) string {
	tokens := dbWordTokens(stripDBNoise(statement))
	for i, tok := range tokens {
		for _, phrase := range dbReadOnlyDeniedPhrases {
			if matchDBPhrase(tokens, i, phrase) {
				return strings.Join(phrase, " ")
			}
		}
		if dbReadOnlyDeniedWords[tok] {
			return tok
		}
	}
	return ""
}

// matchDBPhrase reports whether phrase matches tokens starting at pos.
func matchDBPhrase(tokens []string, pos int, phrase []string) bool {
	if pos+len(phrase) > len(tokens) {
		return false
	}
	for j, word := range phrase {
		if tokens[pos+j] != word {
			return false
		}
	}
	return true
}

// stripDBNoise replaces string literals, quoted identifiers, and comments
// with blanks, keeping token boundaries so words on either side never fuse.
// Runs of skipped text collapse to one blank.
func stripDBNoise(sql string) string {
	var builder strings.Builder
	builder.Grow(len(sql))
	single, double, backtick := false, false, false
	lineComment, blockComment := false, false
	escaped := false
	blanked := false

	blank := func() {
		if !blanked {
			builder.WriteByte(' ')
			blanked = true
		}
	}

	runes := []rune(sql)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		var next rune
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if lineComment {
			if r == '\n' {
				lineComment = false
				blanked = false
				builder.WriteRune(r)
			} else {
				blank()
			}
			continue
		}
		if blockComment {
			if r == '*' && next == '/' {
				blockComment = false
				i++
			}
			blank()
			continue
		}
		if single || double || backtick {
			blank()
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && !backtick {
				escaped = true
				continue
			}
			if single && r == '\'' {
				if next == '\'' {
					i++
					continue
				}
				single = false
			} else if double && r == '"' {
				if next == '"' {
					i++
					continue
				}
				double = false
			} else if backtick && r == '`' {
				backtick = false
			}
			continue
		}

		switch {
		case r == '\'':
			single = true
			blank()
		case r == '"':
			double = true
			blank()
		case r == '`':
			backtick = true
			blank()
		case r == '-' && next == '-':
			lineComment = true
			i++
			blank()
		case r == '#':
			lineComment = true
			blank()
		case r == '/' && next == '*':
			blockComment = true
			i++
			blank()
		default:
			blanked = false
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// dbWordTokens splits s into uppercased words. Word characters are letters,
// digits, underscore, and dollar, so updated_at stays one token and never
// matches UPDATE, while a bare UPDATE matches exactly.
func dbWordTokens(s string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, strings.ToUpper(current.String()))
			current.Reset()
		}
	}
	for _, r := range s {
		if isDBLetter(r) || (r >= '0' && r <= '9') || r == '_' || r == '$' {
			current.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

// splitDBStatements splits sql on semicolons outside string literals and
// comments, dropping empty fragments. A lone trailing semicolon therefore
// stays a single statement, while a second statement of any kind is seen.
func splitDBStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	single, double, backtick := false, false, false
	lineComment, blockComment := false, false
	escaped := false

	flush := func() {
		if trimmed := strings.TrimSpace(current.String()); trimmed != "" {
			statements = append(statements, trimmed)
		}
		current.Reset()
	}

	runes := []rune(sql)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		var next rune
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if lineComment {
			if r == '\n' {
				lineComment = false
				current.WriteRune(r)
			}
			continue
		}
		if blockComment {
			if r == '*' && next == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if single || double || backtick {
			current.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && !backtick {
				escaped = true
				continue
			}
			if single && r == '\'' {
				if next == '\'' {
					current.WriteRune(next)
					i++
					continue
				}
				single = false
			} else if double && r == '"' {
				if next == '"' {
					current.WriteRune(next)
					i++
					continue
				}
				double = false
			} else if backtick && r == '`' {
				backtick = false
			}
			continue
		}

		switch {
		case r == '\'':
			single = true
			current.WriteRune(r)
		case r == '"':
			double = true
			current.WriteRune(r)
		case r == '`':
			backtick = true
			current.WriteRune(r)
		case r == '-' && next == '-':
			lineComment = true
			i++
		case r == '#':
			lineComment = true
		case r == '/' && next == '*':
			blockComment = true
			i++
		case r == ';':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return statements
}

// firstDBKeyword reads the first letter-run after leading whitespace and
// SQL comments, uppercased. Anything that is not a plain leading keyword
// (empty input, comment-only input, a parenthesized expression) yields "",
// which callers treat as empty, and unknown keywords default to the write
// path, never to read-only.
func firstDBKeyword(statement string) string {
	runes := []rune(statement)
	i := 0
	for i < len(runes) {
		for i < len(runes) && isDBSpace(runes[i]) {
			i++
		}
		if i+1 < len(runes) && runes[i] == '-' && runes[i+1] == '-' {
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			continue
		}
		if i < len(runes) && runes[i] == '#' {
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(runes) && runes[i] == '/' && runes[i+1] == '*' {
			i += 2
			for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i += 2
			if i > len(runes) {
				i = len(runes)
			}
			continue
		}
		break
	}
	if i > len(runes) {
		i = len(runes)
	}
	start := i
	for i < len(runes) && isDBLetter(runes[i]) {
		i++
	}
	return strings.ToUpper(string(runes[start:i]))
}

func isDBSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
}

func isDBLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// ---- Database dump/import transfer ----

const (
	dbTransferOpKindDump   = "db-dump"
	dbTransferOpKindImport = "db-import"
)

// DBTransferTarget is the scrubbed transfer endpoint handed to the dump and
// import seams. It carries no password by construction, like DBQueryCall.
type DBTransferTarget struct {
	Container string
	Scheme    string
	Host      string
	Port      string
	Database  string
	User      string
}

// DBTransferProgress is the structured db:progress payload: byte counts for
// the bar plus a short human message for the toast. Every transfer event
// carries the op identity so late events from a superseded op drop stale.
type DBTransferProgress struct {
	OpID          string `json:"opId"`
	Generation    uint64 `json:"generation"`
	Project       string `json:"project"`
	Kind          string `json:"kind"`
	BytesWritten  int64  `json:"bytesWritten"`
	TotalEstimate int64  `json:"totalEstimate"`
	Percent       int    `json:"percent"`
	Message       string `json:"message"`
}

// DBTransferCompleted is the db:completed payload for a finished transfer.
type DBTransferCompleted struct {
	OpID    string `json:"opId"`
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// DBTransferFailed is the db:failed payload for a failed or cancelled transfer.
type DBTransferFailed struct {
	OpID    string `json:"opId"`
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

var dbTransferOpCounter int64

// NewDBServiceWithPlatform builds a DBService that emits transfer events on
// p. Tests pass a FakePlatform; production wiring arrives with the data tab.
func NewDBServiceWithPlatform(p Platform) *DBService {
	if p == nil {
		p = newDefaultPlatform()
	}
	return &DBService{serviceBase: serviceBase{platform: p}}
}

func (s *DBService) DumpDB(project string, destPath string) (res string, err error) {
	defer RecoverPanic(&err, "DumpDB")
	dest := filepath.Clean(strings.TrimSpace(destPath))
	if strings.TrimSpace(destPath) == "" {
		return "", fmt.Errorf("dump destination path is empty")
	}
	conn, err := discoverDBConnection(project)
	if err != nil {
		return "", err
	}
	// Preflight runs before anything is created or started: a refusal here
	// must leave no file behind and never start the dump process.
	estimate, err := estimateDBSizeForDesktop(s.lifecycleContext(), conn)
	if err != nil {
		return "", fmt.Errorf("refusing dump: cannot estimate database size: %w", err)
	}
	if estimate < 0 {
		estimate = 0
	}
	// The free-space probe runs against the nearest existing ancestor BEFORE
	// any directory is created, so a preflight refusal leaves nothing behind:
	// no files, no directories, no started process.
	free, err := freeDiskSpaceForDesktop(nearestExistingDir(filepath.Dir(dest)))
	if err != nil {
		return "", fmt.Errorf("refusing dump: cannot probe free disk space: %w", err)
	}
	if estimate > int64(free) {
		return "", fmt.Errorf(
			"insufficient disk space for dump of %s: estimated dump size %s exceeds free space %s at %s",
			sanitizeUIText(conn.project, 0),
			formatDBBytes(estimate),
			formatDBBytes(int64(free)),
			sanitizeUIText(filepath.Dir(dest), 0),
		)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("create dump directory: %w", err)
	}
	timeout := govardCommandTimeout([]string{"db", "dump"})
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), timeout)
	opID := fmt.Sprintf("db-dump-%d", atomic.AddInt64(&dbTransferOpCounter, 1))
	generation := atomic.AddUint64(&dbTransferGenCounter, 1)
	if ok, existing := RegisterDBTransferOp(opID, conn.project, dbTransferOpKindDump, cancel); !ok {
		cancel()
		return "", fmt.Errorf("dump already running: %s is already running for project %s",
			sanitizeUIText(existing, 0), sanitizeUIText(conn.project, 0))
	}
	noteDBTransferOp(opID, generation, conn.project, dbTransferOpKindDump)
	go s.runDBDump(ctx, opID, generation, conn, dest, estimate)
	return opID, nil
}

func (s *DBService) ImportDB(project string, srcPath string) (res string, err error) {
	defer RecoverPanic(&err, "ImportDB")
	if strings.TrimSpace(srcPath) == "" {
		return "", fmt.Errorf("import source path is empty")
	}
	src := filepath.Clean(strings.TrimSpace(srcPath))
	conn, err := discoverDBConnection(project)
	if err != nil {
		return "", err
	}
	// Header validation runs synchronously before the op is registered, so a
	// refused file never opens the database writer: nothing is written.
	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("open import file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf(
			"import refused: %s is a directory, not a database dump",
			sanitizeUIText(src, 0),
		)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf(
			"import refused: %s is empty, not a database dump; refusing to write anything to %s",
			sanitizeUIText(src, 0),
			sanitizeUIText(conn.project, 0),
		)
	}
	engine, ok, err := sniffDBDumpFile(src)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf(
			"import refused: %s is not a recognized database dump (missing mysqldump/pg_dump header); refusing to write anything to %s",
			sanitizeUIText(src, 0),
			sanitizeUIText(conn.project, 0),
		)
	}
	if engine != conn.scheme {
		return "", fmt.Errorf(
			"import refused: %s looks like a %s dump but project %s uses %s; refusing to write anything to the database",
			sanitizeUIText(src, 0),
			dbEngineDisplayName(engine),
			sanitizeUIText(conn.project, 0),
			dbEngineDisplayName(conn.scheme),
		)
	}
	timeout := govardCommandTimeout([]string{"db", "import"})
	ctx, cancel := context.WithTimeout(s.lifecycleContext(), timeout)
	opID := fmt.Sprintf("db-import-%d", atomic.AddInt64(&dbTransferOpCounter, 1))
	generation := atomic.AddUint64(&dbTransferGenCounter, 1)
	if ok, existing := RegisterDBTransferOp(opID, conn.project, dbTransferOpKindImport, cancel); !ok {
		cancel()
		return "", fmt.Errorf("import already running: %s is already running for project %s",
			sanitizeUIText(existing, 0), sanitizeUIText(conn.project, 0))
	}
	noteDBTransferOp(opID, generation, conn.project, dbTransferOpKindImport)
	go s.runDBImport(ctx, opID, generation, conn, src, info.Size())
	return opID, nil
}

// CancelDBOp cancels a running dump or import: the registry fires the op
// context (which kills the docker process started with it), the transfer
// identity is dropped so late events from the dying goroutine never reach
// the UI, and the dump runner removes its partial file.
func (s *DBService) CancelDBOp(opID string) bool {
	id := strings.TrimSpace(opID)
	if id == "" {
		return false
	}
	dbTransferGens.Lock()
	ident, ok := dbTransferGens.byOp[id]
	if ok {
		delete(dbTransferGens.byOp, id)
	}
	dbTransferGens.Unlock()
	cancelled := CancelOp(id)
	if cancelled {
		s.platform.Emit(EventDBFailed, DBTransferFailed{
			OpID:    sanitizeUIText(id, 0),
			Project: sanitizeUIText(ident.project, 0),
			Kind:    ident.kind,
			Message: sanitizeUIText(dbTransferKindLabel(ident.kind)+" cancelled by user", 0),
		})
	}
	return cancelled
}

// transferTarget scrubs the connection down to the password-free facts the
// transfer seams may record.
func (c dbConnection) transferTarget() DBTransferTarget {
	return DBTransferTarget{
		Container: c.container,
		Scheme:    c.scheme,
		Host:      c.host,
		Port:      c.port,
		Database:  c.database,
		User:      c.user,
	}
}

// Seams below keep transfer hermetic in tests; ResetStateForTest restores
// every one of them. Test setters receive the scrubbed DBTransferTarget, so
// the password never crosses into test code.

var estimateDBSizeForDesktop = defaultEstimateDBSizeForDesktop
var freeDiskSpaceForDesktop = defaultFreeDiskSpaceForDesktop
var openDBDumpReaderForDesktop = defaultOpenDBDumpReaderForDesktop
var openDBImportWriterForDesktop = defaultOpenDBImportWriterForDesktop
var createDBDumpDestForDesktop = defaultCreateDBDumpDestForDesktop

// nearestExistingDir walks dir up to the closest ancestor that exists, so the
// preflight free-space probe never creates directories itself.
func nearestExistingDir(dir string) string {
	current := filepath.Clean(dir)
	for {
		if info, err := os.Stat(current); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return current
		}
		current = parent
	}
}

// defaultCreateDBDumpDestForDesktop opens the dump destination (always a
// .partial sibling, renamed into place only on success) with an explicit
// 0600, not umask-reliant: open(2) masks the create mode with ~umask.
func defaultCreateDBDumpDestForDesktop(path string) (io.WriteCloser, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func defaultEstimateDBSizeForDesktop(ctx context.Context, conn dbConnection) (int64, error) {
	db := strings.TrimSpace(conn.database)
	var query string
	if conn.scheme == "postgresql" {
		query = "SELECT pg_database_size('" + strings.ReplaceAll(db, "'", "''") + "');"
	} else {
		query = "SELECT SUM(data_length) FROM information_schema.tables WHERE table_schema = '" +
			strings.ReplaceAll(db, "'", "''") + "'"
	}
	out, err := runDBQueryForDesktop(ctx, conn, query)
	if err != nil {
		return 0, err
	}
	// Query output may be tabular (header row plus value row): the last
	// integer-looking token is the size. An empty database reports NULL,
	// which estimates zero rather than failing the preflight.
	found := false
	var last int64
	for _, field := range strings.Fields(out) {
		if v, parseErr := strconv.ParseInt(strings.TrimSpace(field), 10, 64); parseErr == nil {
			last = v
			found = true
		}
	}
	if !found {
		if strings.Contains(strings.ToUpper(out), "NULL") {
			return 0, nil
		}
		return 0, fmt.Errorf("could not parse database size from %q", sanitizeUIText(out, 200))
	}
	// Mirror GetDatabaseSize's 0.6 logical-to-dump heuristic
	// (internal/cmd/db.go): InnoDB pages overstate the SQL text size.
	return int64(float64(last) * 0.6), nil
}

func defaultFreeDiskSpaceForDesktop(path string) (uint64, error) {
	return diskFreeBytes(path)
}

func defaultOpenDBDumpReaderForDesktop(ctx context.Context, conn dbConnection) (io.ReadCloser, func() error, error) {
	script := dbDumpScriptForConn(conn)
	envName := dbTransferEnvName(conn.scheme)
	args, _, cleanup, err := buildDBTransferDockerArgs(conn.container, conn.password, envName, script)
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, nil, err
	}
	wait := func() error {
		defer cleanup()
		if err := cmd.Wait(); err != nil {
			if tail := strings.TrimSpace(stderr.String()); tail != "" {
				return fmt.Errorf("%w: %s", err, sanitizeUIText(tail, 2000))
			}
			return err
		}
		return nil
	}
	return stdout, wait, nil
}

func defaultOpenDBImportWriterForDesktop(ctx context.Context, conn dbConnection) (io.WriteCloser, func() error, error) {
	script := dbImportScriptForConn(conn)
	envName := dbTransferEnvName(conn.scheme)
	args, _, cleanup, err := buildDBTransferDockerArgs(conn.container, conn.password, envName, script)
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, nil, err
	}
	wait := func() error {
		defer cleanup()
		if err := cmd.Wait(); err != nil {
			if tail := strings.TrimSpace(stderr.String()); tail != "" {
				return fmt.Errorf("%w: %s", err, sanitizeUIText(tail, 2000))
			}
			return err
		}
		return nil
	}
	return stdin, wait, nil
}

// formatDBBytes renders a byte count for preflight errors and progress
// messages, so both sides of a refusal carry size numbers.
func formatDBBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	value := float64(n)
	for _, unit := range []string{"B", "KB", "MB", "GB", "TB"} {
		if value < 1024 || unit == "TB" {
			if unit == "B" {
				return fmt.Sprintf("%d B", n)
			}
			return fmt.Sprintf("%.1f %s", value, unit)
		}
		value /= 1024
	}
	return fmt.Sprintf("%d B", n)
}

// sniffDBDumpHead reports the dump engine ("mysql" or "postgresql") when the
// head bytes carry a recognized dump header, transparently handling gzip:
// a .gz source keeps its magic bytes, so the check decompresses the sample
// before matching. Unknown content (wrong file, empty, truncated gzip)
// reports false and the import is refused before the writer opens.
func sniffDBDumpHead(head []byte) (string, bool) {
	sample := head
	if len(sample) >= 2 && sample[0] == 0x1f && sample[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(sample))
		if err != nil {
			return "", false
		}
		decoded, _ := io.ReadAll(io.LimitReader(reader, dbTransferSniffBytes))
		_ = reader.Close()
		if len(decoded) == 0 {
			return "", false
		}
		sample = decoded
	}
	lowered := strings.ToLower(string(sample))
	switch {
	case strings.Contains(lowered, "postgresql database dump"):
		return "postgresql", true
	case strings.Contains(lowered, "mariadb dump"):
		return "mysql", true
	case strings.Contains(lowered, "mysql dump"):
		return "mysql", true
	}
	return "", false
}

// buildDBTransferDockerArgs builds the docker exec argv for one transfer side.
// The password travels via a 0600 env file passed as --env-file, so it never
// appears in argv (visible via the process list): only the temp path does,
// and cleanup removes the file. An empty password needs no file.
func buildDBTransferDockerArgs(container string, password string, envName string, script string) ([]string, string, func(), error) {
	cleanup := func() {}
	args := []string{"exec", "-i"}
	envFile := ""
	if strings.TrimSpace(password) != "" {
		file, err := os.CreateTemp("", "govard-db-transfer-*.env")
		if err != nil {
			return nil, "", cleanup, fmt.Errorf("create transfer env file: %w", err)
		}
		envFile = file.Name()
		cleanup = func() { _ = os.Remove(envFile) }
		// A password holding a newline would break the env-file line shape;
		// database passwords never legitimately contain one, and embedding it
		// raw would leak bytes into a second variable.
		trimmed := strings.ReplaceAll(password, "\n", "")
		trimmed = strings.ReplaceAll(trimmed, "\r", "")
		if _, err := io.WriteString(file, envName+"="+trimmed+"\n"); err != nil {
			_ = file.Close()
			cleanup()
			return nil, "", cleanup, fmt.Errorf("write transfer env file: %w", err)
		}
		if err := file.Close(); err != nil {
			cleanup()
			return nil, "", cleanup, fmt.Errorf("close transfer env file: %w", err)
		}
		// Explicit 0600, not umask-reliant: open(2) masks the create mode with
		// ~umask, so a restrictive umask is fine but a permissive one must
		// not widen the secret file. (A SIGKILL-hard-killed transfer can still
		// leave the temp file behind; that failure mode is uncatchable and
		// accepted.)
		if err := os.Chmod(envFile, 0o600); err != nil {
			cleanup()
			return nil, "", cleanup, fmt.Errorf("chmod transfer env file: %w", err)
		}
		args = append(args, "--env-file", envFile)
	}
	args = append(args, container, "sh", "-lc", script)
	return args, envFile, cleanup, nil
}

// dbTransferSniffBytes bounds how much of a dump file the import header check
// reads: headers sit in the first kilobytes, the rest streams untouched.
const dbTransferSniffBytes = 8192

// dbTransferChunkSize is the streaming copy quantum between dump and file.
const dbTransferChunkSize = 32 * 1024

// dbImportSessionPrefix and dbImportSessionSuffix wrap a mysql import stream
// with the same session posture as RunImportFromReaderWithProgress
// (internal/cmd/db.go): foreign key checks off, one transaction. The same
// bytes reach the server, so CLI and desktop imports behave identically.
// PostgreSQL imports skip both: psql runs with ON_ERROR_STOP=1 instead.
const dbImportSessionPrefix = "SET FOREIGN_KEY_CHECKS=0; SET UNIQUE_CHECKS=0; SET AUTOCOMMIT=0; SET SQL_MODE='NO_AUTO_VALUE_ON_ZERO';\n"
const dbImportSessionSuffix = "\nCOMMIT; SET FOREIGN_KEY_CHECKS=1; SET UNIQUE_CHECKS=1; SET AUTOCOMMIT=1;\n"

// sniffDBDumpFile reads the head of path and reports its dump engine. It only
// peeks: the transfer reopens the file from the start afterwards.
func sniffDBDumpFile(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, fmt.Errorf("open import file: %w", err)
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, dbTransferSniffBytes)
	read, err := io.ReadFull(file, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", false, fmt.Errorf("read import header: %w", err)
	}
	engine, ok := sniffDBDumpHead(head[:read])
	return engine, ok, nil
}

// revalidateDBImportSourceFile re-sniffs the header and re-stats the size
// from an already-open import source. runDBImport calls it right after
// opening the file so bytes swapped in between the call-time validation and
// the op start are refused before the database writer ever opens: the
// returned size is the fresh stat, and the stream rewinds to the start.
func revalidateDBImportSourceFile(file *os.File, scheme string) (int64, string, error) {
	src := ""
	if file != nil {
		src = file.Name()
	}
	info, err := file.Stat()
	if err != nil {
		return 0, "", fmt.Errorf("stat import file: %w", err)
	}
	if info.IsDir() {
		return 0, "", fmt.Errorf(
			"import refused: %s is a directory, not a database dump",
			sanitizeUIText(src, 0),
		)
	}
	if info.Size() == 0 {
		return 0, "", fmt.Errorf(
			"import refused: %s is empty, not a database dump",
			sanitizeUIText(src, 0),
		)
	}
	head := make([]byte, dbTransferSniffBytes)
	read, err := io.ReadFull(file, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return 0, "", fmt.Errorf("read import header: %w", err)
	}
	engine, ok := sniffDBDumpHead(head[:read])
	if !ok {
		return 0, "", fmt.Errorf(
			"import refused: %s is not a recognized database dump (missing mysqldump/pg_dump header); refusing to write anything",
			sanitizeUIText(src, 0),
		)
	}
	if engine != scheme {
		return 0, "", fmt.Errorf(
			"import refused: %s looks like a %s dump but the project uses %s; refusing to write anything to the database",
			sanitizeUIText(src, 0),
			dbEngineDisplayName(engine),
			dbEngineDisplayName(scheme),
		)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", fmt.Errorf("rewind import file: %w", err)
	}
	return info.Size(), engine, nil
}

// dbEngineDisplayName renders a dump/connection engine for refusal messages.
func dbEngineDisplayName(engine string) string {
	if engine == "postgresql" {
		return "PostgreSQL"
	}
	return "MySQL"
}

// dbTransferKindLabel renders an op kind for progress and failure messages.
func dbTransferKindLabel(kind string) string {
	if kind == dbTransferOpKindImport {
		return "Import"
	}
	return "Dump"
}

// dbTransferEnvName is the container env var carrying the password for a
// scheme. The value travels via --env-file, never in argv.
func dbTransferEnvName(scheme string) string {
	if scheme == "postgresql" {
		return "PGPASSWORD"
	}
	return "MYSQL_PWD"
}

// dbDumpScriptForConn builds the in-container dump script. It mirrors
// buildLocalMySQLDumpCommandScript / buildLocalPostgresDumpCommand
// (internal/cmd/db_credentials.go): the same two-pass mysqldump posture with
// routines and triggers, so the desktop dump is byte-compatible with
// `govard db dump` instead of a second dump format. The password never enters
// the script: the container receives it via --env-file.
func dbDumpScriptForConn(conn dbConnection) string {
	user := shellQuoteForDB(conn.user)
	database := shellQuoteForDB(conn.database)
	if conn.scheme == "postgresql" {
		return "pg_dump --no-owner --no-privileges -U " + user + " " + database
	}
	common := []string{
		`"$DUMP_BIN"`,
		"--max-allowed-packet=" + conventions.MySQLMaxAllowedPacket,
		"--force", "--single-transaction", "--no-tablespaces",
		"-h" + conventions.DefaultDBHost,
		"-u" + user,
	}
	metadata := append(append([]string{}, common...), "--no-data", "--routines", "--triggers", database)
	data := append(append([]string{}, common...), "--no-create-info", "--skip-triggers", database)
	return conventions.MySQLDumpBinDetect + " && { " +
		strings.Join(metadata, " ") + " && " + strings.Join(data, " ") + "; }"
}

// dbImportScriptForConn builds the in-container import script. It mirrors
// buildLocalMySQLClientCommandScript / buildLocalPostgresImportCommand
// (internal/cmd/db_credentials.go): the mysql client runs with -f, and the
// session wrappers are written around the stream Go-side from
// dbImportSessionPrefix/Suffix, carrying the same bytes as the CLI's
// in-script echo/cat/echo wrapper.
func dbImportScriptForConn(conn dbConnection) string {
	user := shellQuoteForDB(conn.user)
	database := shellQuoteForDB(conn.database)
	if conn.scheme == "postgresql" {
		return "exec psql -U " + user + " -v ON_ERROR_STOP=1 " + database
	}
	return conventions.MySQLClientBinDetect + " && " +
		`exec "$DB_CLI" --max-allowed-packet=512M -u ` + user + " " + database + " -f"
}

// noteDBTransferOp records a running transfer's identity for stale-drop:
// every later event must carry this generation to reach the UI.
func noteDBTransferOp(opID string, generation uint64, project string, kind string) {
	dbTransferGens.Lock()
	defer dbTransferGens.Unlock()
	dbTransferGens.byOp[opID] = dbTransferIdentity{generation: generation, project: project, kind: kind}
}

// forgetDBTransferOp drops a finished transfer's identity so its late events
// (a copy loop draining after FinishOp) never reach the UI. The delete only
// fires for the same generation, so a recycled op id is never disturbed.
func forgetDBTransferOp(opID string, generation uint64) {
	dbTransferGens.Lock()
	defer dbTransferGens.Unlock()
	if ident, ok := dbTransferGens.byOp[opID]; ok && ident.generation == generation {
		delete(dbTransferGens.byOp, opID)
	}
}

// emitDBTransfer emits one transfer event when the op identity is still live
// and drops it stale otherwise: after CancelDBOp (which deletes the identity
// first) or FinishOp, nothing from the dying goroutine may reach the UI.
func (s *DBService) emitDBTransfer(opID string, generation uint64, event string, payload any) {
	dbTransferGens.Lock()
	ident, ok := dbTransferGens.byOp[opID]
	match := ok && ident.generation == generation
	dbTransferGens.Unlock()
	if !match {
		return
	}
	s.platform.Emit(event, payload)
}

// emitDBProgress folds written bytes into the structured db:progress payload.
// Completion always reports 100; mid-stream percent caps at 99 so the bar
// never claims completion before the process actually exited.
func (s *DBService) emitDBProgress(opID string, generation uint64, project string, kind string, written int64, total int64, done bool) {
	percent := 0
	if done {
		percent = 100
	} else if total > 0 {
		percent = int(written * 100 / total)
		if percent > 99 {
			percent = 99
		}
		if percent < 0 {
			percent = 0
		}
	}
	var message string
	if done {
		message = fmt.Sprintf("%s complete: %s", dbTransferKindLabel(kind), formatDBBytes(written))
	} else if total > 0 {
		message = fmt.Sprintf("%sing database... %s of ~%s (%d%%)",
			dbTransferVerb(kind), formatDBBytes(written), formatDBBytes(total), percent)
	} else {
		message = fmt.Sprintf("%sing database... %s", dbTransferVerb(kind), formatDBBytes(written))
	}
	s.emitDBTransfer(opID, generation, EventDBProgress, DBTransferProgress{
		OpID:          opID,
		Generation:    generation,
		Project:       sanitizeUIText(project, 0),
		Kind:          kind,
		BytesWritten:  written,
		TotalEstimate: total,
		Percent:       percent,
		Message:       sanitizeUIText(message, 0),
	})
}

// dbTransferVerb renders the -ing verb for a kind's progress message.
func dbTransferVerb(kind string) string {
	if kind == dbTransferOpKindImport {
		return "Import"
	}
	return "Dump"
}

// dbTransferFailureMessage renders a failure message for kind. A cancelled
// context always reads as a user cancel, never as the context error text.
func dbTransferFailureMessage(kind string, err error, ctx context.Context) string {
	if ctx.Err() != nil {
		return dbTransferKindLabel(kind) + " cancelled by user"
	}
	if err == nil {
		return dbTransferKindLabel(kind) + " failed"
	}
	return dbTransferKindLabel(kind) + " failed: " + err.Error()
}

// streamDBTransfer copies src to dst, reporting running bytes to onProgress:
// the first chunk reports immediately, later ones at most every
// logFlushInterval, and completion always reports. It checks ctx after every
// read: on cancel it stops before writing or emitting anything further, so no
// stale byte or event escapes a cancelled op.
func streamDBTransfer(ctx context.Context, src io.Reader, dst io.Writer, total int64, onProgress func(written int64, done bool)) error {
	buffer := make([]byte, dbTransferChunkSize)
	var written int64
	var lastEmit time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		read, readErr := src.Read(buffer)
		if read > 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if _, writeErr := dst.Write(buffer[:read]); writeErr != nil {
				return writeErr
			}
			written += int64(read)
			now := time.Now()
			if lastEmit.IsZero() || now.Sub(lastEmit) >= logFlushInterval {
				lastEmit = now
				onProgress(written, false)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				onProgress(written, true)
				return nil
			}
			return readErr
		}
	}
}

// runDBDump streams the dump process into dest through a sibling .partial
// file renamed into place only on success, mirroring writeDumpFile
// (internal/cmd/db_dump.go): a failed or cancelled dump never leaves a stub
// that looks like a dump, and an earlier file at dest survives. A .gz
// destination compresses the stream like the CLI.
func (s *DBService) runDBDump(ctx context.Context, opID string, generation uint64, conn dbConnection, dest string, estimate int64) {
	defer FinishOp(opID)
	defer forgetDBTransferOp(opID, generation)
	fail := func(err error) {
		s.emitDBTransfer(opID, generation, EventDBFailed, DBTransferFailed{
			OpID:    opID,
			Project: sanitizeUIText(conn.project, 0),
			Kind:    dbTransferOpKindDump,
			Message: sanitizeUIText(dbTransferFailureMessage(dbTransferOpKindDump, err, ctx), 0),
		})
	}
	var written int64
	progress := func(bytesWritten int64, done bool) {
		written = bytesWritten
		s.emitDBProgress(opID, generation, conn.project, dbTransferOpKindDump, bytesWritten, estimate, done)
	}

	partial := dest + ".partial"
	out, err := createDBDumpDestForDesktop(partial)
	if err != nil {
		fail(err)
		return
	}
	succeeded := false
	fileClosed := false
	closeFile := func() error {
		if fileClosed {
			return nil
		}
		fileClosed = true
		return out.Close()
	}
	defer func() {
		_ = closeFile()
		if !succeeded {
			_ = os.Remove(partial)
		}
	}()

	if ctx.Err() != nil {
		fail(ctx.Err())
		return
	}
	// Re-stat the destination at op start: the save dialog's Exists flag is
	// dialog-time, and free space sampled at call time may be gone by now. A
	// directory swapped in at dest is refused before anything streams, and a
	// filled disk fails here instead of mid-stream.
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		fail(fmt.Errorf(
			"dump refused: %s is a directory, not a dump file",
			sanitizeUIText(dest, 0),
		))
		return
	}
	if free, err := freeDiskSpaceForDesktop(nearestExistingDir(filepath.Dir(dest))); err != nil {
		fail(fmt.Errorf("refusing dump: cannot probe free disk space: %w", err))
		return
	} else if estimate > int64(free) {
		fail(fmt.Errorf(
			"insufficient disk space for dump of %s: estimated dump size %s exceeds free space %s at %s",
			sanitizeUIText(conn.project, 0),
			formatDBBytes(estimate),
			formatDBBytes(int64(free)),
			sanitizeUIText(filepath.Dir(dest), 0),
		))
		return
	}
	src, wait, err := openDBDumpReaderForDesktop(ctx, conn)
	if err != nil {
		fail(err)
		return
	}
	var dst io.Writer = out
	var gz *gzip.Writer
	if strings.HasSuffix(strings.ToLower(dest), ".gz") {
		gz = gzip.NewWriter(out)
		dst = gz
	}
	streamErr := streamDBTransfer(ctx, src, dst, estimate, progress)
	if closer, ok := src.(io.Closer); ok {
		_ = closer.Close()
	}
	waitErr := wait()
	if gz != nil {
		if closeErr := gz.Close(); closeErr != nil && streamErr == nil {
			streamErr = closeErr
		}
	}
	if closeErr := closeFile(); closeErr != nil && streamErr == nil {
		streamErr = closeErr
	}
	if streamErr != nil {
		fail(streamErr)
		return
	}
	if waitErr != nil {
		fail(waitErr)
		return
	}
	if err := os.Rename(partial, dest); err != nil {
		fail(err)
		return
	}
	succeeded = true
	s.emitDBTransfer(opID, generation, EventDBCompleted, DBTransferCompleted{
		OpID:    opID,
		Project: sanitizeUIText(conn.project, 0),
		Kind:    dbTransferOpKindDump,
		Path:    sanitizeUIText(dest, 0),
		Message: sanitizeUIText(fmt.Sprintf("Dump saved to %s (%s)", dest, formatDBBytes(written)), 0),
	})
}

// runDBImport streams src into the database writer. The header was validated
// synchronously in ImportDB, and re-validated here right after the op-start
// open (re-sniff plus fresh stat), so reaching the writer means the writer
// only ever sees a recognized dump of the project's own engine.
func (s *DBService) runDBImport(ctx context.Context, opID string, generation uint64, conn dbConnection, src string, total int64) {
	defer FinishOp(opID)
	defer forgetDBTransferOp(opID, generation)
	fail := func(err error) {
		s.emitDBTransfer(opID, generation, EventDBFailed, DBTransferFailed{
			OpID:    opID,
			Project: sanitizeUIText(conn.project, 0),
			Kind:    dbTransferOpKindImport,
			Message: sanitizeUIText(dbTransferFailureMessage(dbTransferOpKindImport, err, ctx), 0),
		})
	}
	progress := func(bytesWritten int64, done bool) {
		s.emitDBProgress(opID, generation, conn.project, dbTransferOpKindImport, bytesWritten, total, done)
	}

	if ctx.Err() != nil {
		fail(ctx.Err())
		return
	}
	in, err := os.Open(src)
	if err != nil {
		fail(err)
		return
	}
	defer func() { _ = in.Close() }()
	freshTotal, _, err := revalidateDBImportSourceFile(in, conn.scheme)
	if err != nil {
		fail(err)
		return
	}
	total = freshTotal
	var reader io.Reader = bufio.NewReader(in)
	if magic, peekErr := reader.(*bufio.Reader).Peek(2); peekErr == nil && len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, gzErr := gzip.NewReader(reader)
		if gzErr != nil {
			fail(gzErr)
			return
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}
	writer, wait, err := openDBImportWriterForDesktop(ctx, conn)
	if err != nil {
		fail(err)
		return
	}
	if ctx.Err() != nil {
		if closer, ok := writer.(io.Closer); ok {
			_ = closer.Close()
		}
		_ = wait()
		fail(ctx.Err())
		return
	}
	mysql := conn.scheme != "postgresql"
	if mysql {
		if _, err := io.WriteString(writer, dbImportSessionPrefix); err != nil {
			if closer, ok := writer.(io.Closer); ok {
				_ = closer.Close()
			}
			_ = wait()
			fail(err)
			return
		}
	}
	streamErr := streamDBTransfer(ctx, reader, writer, total, progress)
	if streamErr == nil && mysql {
		_, streamErr = io.WriteString(writer, dbImportSessionSuffix)
	}
	if closer, ok := writer.(io.Closer); ok {
		_ = closer.Close()
	}
	waitErr := wait()
	if streamErr != nil {
		fail(streamErr)
		return
	}
	if waitErr != nil {
		fail(waitErr)
		return
	}
	s.emitDBTransfer(opID, generation, EventDBCompleted, DBTransferCompleted{
		OpID:    opID,
		Project: sanitizeUIText(conn.project, 0),
		Kind:    dbTransferOpKindImport,
		Path:    sanitizeUIText(src, 0),
		Message: sanitizeUIText(fmt.Sprintf("Import from %s completed", src), 0),
	})
}

// dbTransferGens tracks the live identity of every running transfer so late
// events from a finished or cancelled op drop stale instead of reaching the
// UI under a recycled op id.
var dbTransferGens = struct {
	sync.Mutex
	byOp map[string]dbTransferIdentity
}{byOp: map[string]dbTransferIdentity{}}

type dbTransferIdentity struct {
	generation uint64
	project    string
	kind       string
}

var dbTransferGenCounter uint64

func resetDBTransferGens() {
	dbTransferGens.Lock()
	defer dbTransferGens.Unlock()
	dbTransferGens.byOp = map[string]dbTransferIdentity{}
}
