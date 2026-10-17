package tests

import (
	"strings"
	"testing"

	"govard/internal/desktop"
)

// The desktop data tab queries the project database with a read-only
// default: SELECT-style statements run, anything else needs an explicit
// per-query opt-in plus a confirm token, and connection info never carries
// the password.

const dbServiceTestPassword = "s3cret-test-pw"

func stubDBServiceDiscovery(t *testing.T) {
	t.Helper()
	restoreEnv := desktop.SetInspectDBContainerEnvForDesktopForTest(
		func(container string) (string, error) {
			if container != "sample-project-db-1" {
				t.Errorf("inspect container = %q, want sample-project-db-1", container)
			}
			return "MYSQL_USER=app-user\nMYSQL_PASSWORD=" + dbServiceTestPassword +
				"\nMYSQL_DATABASE=appdb\n", nil
		},
	)
	restorePort := desktop.SetPublishedDBContainerPortForDesktopForTest(
		func(container string, internalPort string) (string, error) {
			return "0.0.0.0:33061\n", nil
		},
	)
	t.Cleanup(func() {
		restorePort()
		restoreEnv()
	})
}

func stubDBServiceExecutor(t *testing.T, calls *int, output string) {
	t.Helper()
	restore := desktop.SetRunDBQueryForDesktopForTest(
		func(call desktop.DBQueryCall) (string, error) {
			*calls++
			if call.Container != "sample-project-db-1" {
				t.Errorf("query container = %q, want sample-project-db-1", call.Container)
			}
			if call.Database != "appdb" {
				t.Errorf("query database = %q, want appdb", call.Database)
			}
			return output, nil
		},
	)
	t.Cleanup(restore)
}

func TestDBService_SelectExecutesReadOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "id\n1\n")

	svc := desktop.NewDBService()
	res, err := svc.RunDBQuery("sample-project", "SELECT * FROM users", false, "")
	if err != nil {
		t.Fatalf("RunDBQuery(SELECT) error = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("executor calls = %d, want 1", calls)
	}
	if res.Statement != "SELECT" {
		t.Errorf("Statement = %q, want SELECT", res.Statement)
	}
	if !res.ReadOnly {
		t.Errorf("ReadOnly = false, want true")
	}
	if !strings.Contains(res.Output, "1") {
		t.Errorf("Output = %q, want executor output", res.Output)
	}
}

func TestDBService_ReadOnlyVariantsPass(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	queries := map[string]string{
		"  select 1":                      "SELECT",
		"-- leading comment\nSHOW TABLES": "SHOW",
		"/* block */ WITH x AS (SELECT 1) SELECT * FROM x": "WITH",
		"DESCRIBE users":              "DESCRIBE",
		"desc users":                  "DESC",
		"EXPLAIN SELECT * FROM users": "EXPLAIN",
		"SELECT ';'":                  "SELECT",
		"SELECT 1;":                   "SELECT",
	}
	for sql, wantStmt := range queries {
		res, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err != nil {
			t.Errorf("RunDBQuery(%q) error = %v, want nil", sql, err)
			continue
		}
		if res.Statement != wantStmt {
			t.Errorf("RunDBQuery(%q) Statement = %q, want %q", sql, res.Statement, wantStmt)
		}
	}
	if calls != len(queries) {
		t.Errorf("executor calls = %d, want %d", calls, len(queries))
	}
}

func TestDBService_WriteStatementsRefusedByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	for _, sql := range []string{
		"UPDATE users SET name = 'x'",
		"DELETE FROM users",
		"DROP TABLE users",
		"INSERT INTO users (name) VALUES ('x')",
		"CREATE TABLE t (id INT)",
		"ALTER TABLE users ADD COLUMN age INT",
		"TRUNCATE TABLE users",
	} {
		_, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err == nil {
			t.Errorf("RunDBQuery(%q) error = nil, want read-only refusal", sql)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0 (refused before execution)", calls)
	}
}

func TestDBService_WriteAllowedWithOptInAndConfirmToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "Query OK\n")

	svc := desktop.NewDBService()
	token := desktop.DBWriteConfirmTokenForTest("sample-project")
	res, err := svc.RunDBQuery("sample-project", "UPDATE users SET name = 'x'", true, token)
	if err != nil {
		t.Fatalf("RunDBQuery with opt-in + token error = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("executor calls = %d, want 1", calls)
	}
	if res.ReadOnly {
		t.Errorf("ReadOnly = true, want false for a write statement")
	}
	if res.Statement != "UPDATE" {
		t.Errorf("Statement = %q, want UPDATE", res.Statement)
	}
}

func TestDBService_WriteRefusedWithoutValidToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	token := desktop.DBWriteConfirmTokenForTest("sample-project")
	for name, tc := range map[string]struct {
		allowWrite bool
		token      string
	}{
		"flag off with valid token": {allowWrite: false, token: token},
		"flag on with wrong token":  {allowWrite: true, token: "bogus"},
		"flag on with empty token":  {allowWrite: true, token: ""},
	} {
		_, err := svc.RunDBQuery("sample-project", "DELETE FROM users", tc.allowWrite, tc.token)
		if err == nil {
			t.Errorf("%s: error = nil, want refusal", name)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0", calls)
	}
}

func TestDBService_MultiStatementRejected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	for _, sql := range []string{
		"SELECT 1; SELECT 2",
		"SELECT 1; DROP TABLE users",
	} {
		_, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err == nil {
			t.Errorf("RunDBQuery(%q) error = nil, want multi-statement refusal", sql)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0", calls)
	}
}

func TestDBService_EmptyQueryRejected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	for _, sql := range []string{"", "   ", "-- only a comment", "/* nothing */"} {
		_, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err == nil {
			t.Errorf("RunDBQuery(%q) error = nil, want rejection", sql)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0", calls)
	}
}

func TestDBService_GetDBInfoMasksCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)

	svc := desktop.NewDBService()
	info, err := svc.GetDBInfo("sample-project")
	if err != nil {
		t.Fatalf("GetDBInfo error = %v, want nil", err)
	}
	if info.Project != "sample-project" {
		t.Errorf("Project = %q, want sample-project", info.Project)
	}
	if info.Engine != "mysql" {
		t.Errorf("Engine = %q, want mysql", info.Engine)
	}
	if info.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", info.Host)
	}
	if info.Port != "33061" {
		t.Errorf("Port = %q, want 33061", info.Port)
	}
	if info.Database != "appdb" {
		t.Errorf("Database = %q, want appdb", info.Database)
	}
	if info.User != "app-user" {
		t.Errorf("User = %q, want app-user", info.User)
	}
	if !info.HasPassword {
		t.Errorf("HasPassword = false, want true")
	}
	if !strings.Contains(info.DisplayURL, "****") {
		t.Errorf("DisplayURL = %q, want masked password", info.DisplayURL)
	}
	for _, field := range []string{
		info.Project, info.Engine, info.Host, info.Port,
		info.Database, info.User, info.DisplayURL,
	} {
		if strings.Contains(field, dbServiceTestPassword) {
			t.Errorf("info field %q leaks the password", field)
		}
	}
}

func TestDBService_QueryOutputSanitized(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "\x1b[32m1\x1b[0m\x07\n")

	svc := desktop.NewDBService()
	res, err := svc.RunDBQuery("sample-project", "SELECT 1", false, "")
	if err != nil {
		t.Fatalf("RunDBQuery error = %v, want nil", err)
	}
	if strings.Contains(res.Output, "\x1b") || strings.Contains(res.Output, "\x07") {
		t.Errorf("Output = %q, want sanitized text", res.Output)
	}
	if strings.Contains(res.Output, dbServiceTestPassword) {
		t.Errorf("Output = %q, must never carry the password", res.Output)
	}
}

func TestDBService_ClassifierRejectsUnterminatedCommentWithoutPanicking(t *testing.T) {
	for _, sql := range []string{"/* nothing closes this", "--"} {
		_, _, err := desktop.ClassifyDBStatementForTest(sql)
		if err == nil {
			t.Errorf("ClassifyDBStatementForTest(%q) error = nil, want rejection", sql)
		}
	}
	// A trailing unterminated comment swallows nothing executable: the one
	// real statement still classifies.
	keyword, readOnly, err := desktop.ClassifyDBStatementForTest("SELECT 1 /* nope")
	if err != nil {
		t.Fatalf("ClassifyDBStatementForTest trailing comment error = %v, want nil", err)
	}
	if keyword != "SELECT" || !readOnly {
		t.Errorf("ClassifyDBStatementForTest = (%q, %v), want (SELECT, true)", keyword, readOnly)
	}
}

func TestDBService_WithModifyingCTERefusedByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	for _, sql := range []string{
		"WITH old AS (SELECT id FROM users) DELETE FROM users WHERE id IN (SELECT id FROM old)",
		"WITH ranked AS (SELECT id FROM users) UPDATE users SET name = 'x' WHERE id IN (SELECT id FROM ranked)",
		"WITH src AS (SELECT * FROM users) INSERT INTO archive SELECT * FROM src",
	} {
		_, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err == nil {
			t.Errorf("RunDBQuery(%q) error = nil, want refusal of data-modifying CTE", sql)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0", calls)
	}
}

func TestDBService_ReadOnlyFalsePositivesPass(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	for _, sql := range []string{
		"SELECT * FROM users WHERE note = 'please delete this row'",
		"SELECT id, updated_at FROM users WHERE status = 'drop table'",
		"SELECT `update`, `delete` FROM audit_log",
		"-- delete everything below\nSELECT * FROM users",
		"/* drop table users */ SELECT COUNT(*) FROM users",
		"WITH recent AS (SELECT id FROM users) SELECT * FROM recent",
	} {
		res, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err != nil {
			t.Errorf("RunDBQuery(%q) error = %v, want nil (no real write word)", sql, err)
			continue
		}
		if !res.ReadOnly {
			t.Errorf("RunDBQuery(%q) ReadOnly = false, want true", sql)
		}
	}
	if calls != 6 {
		t.Errorf("executor calls = %d, want 6", calls)
	}
}

func TestDBService_ExplainAnalyzeRefusedByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	res, err := svc.RunDBQuery("sample-project", "EXPLAIN SELECT * FROM users", false, "")
	if err != nil {
		t.Fatalf("RunDBQuery(EXPLAIN) error = %v, want nil", err)
	}
	if !res.ReadOnly {
		t.Errorf("RunDBQuery(EXPLAIN) ReadOnly = false, want true")
	}

	_, err = svc.RunDBQuery("sample-project", "EXPLAIN ANALYZE SELECT * FROM users", false, "")
	if err == nil {
		t.Errorf("RunDBQuery(EXPLAIN ANALYZE) error = nil, want refusal (executes on PG)")
	}

	token := desktop.DBWriteConfirmTokenForTest("sample-project")
	_, err = svc.RunDBQuery("sample-project", "EXPLAIN ANALYZE SELECT * FROM users", true, token)
	if err != nil {
		t.Errorf("RunDBQuery(EXPLAIN ANALYZE with opt-in) error = %v, want nil", err)
	}
	if calls != 2 {
		t.Errorf("executor calls = %d, want 2 (plain EXPLAIN + opted-in ANALYZE)", calls)
	}
}

func TestDBService_SelectIntoAndForUpdateRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	for _, sql := range []string{
		"SELECT * INTO archive_users FROM users",
		"SELECT * INTO OUTFILE '/tmp/users.csv' FROM users",
		"SELECT * FROM users WHERE id = 1 FOR UPDATE",
		"select id from users for  update",
	} {
		_, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err == nil {
			t.Errorf("RunDBQuery(%q) error = nil, want refusal", sql)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0", calls)
	}
}

func TestDBService_DestructiveBareStatementsRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	for _, sql := range []string{
		"LOAD DATA INFILE '/tmp/x.csv' INTO TABLE users",
		"DISCARD ALL",
		"COPY users FROM '/tmp/x.csv'",
		"SET SESSION sql_mode = 'x'",
		"RESET QUERY CACHE",
		"GRANT SELECT ON users TO 'app'",
		"REVOKE SELECT ON users FROM 'app'",
		"SELECT * FROM a UNION SELECT * FROM b",
	} {
		_, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err == nil {
			t.Errorf("RunDBQuery(%q) error = nil, want refusal", sql)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0", calls)
	}
}

func TestDBService_WriteTokenNormalizedBothSides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "Query OK\n")

	if got := desktop.DBWriteConfirmTokenForTest("Sample-Project"); got != desktop.DBWriteConfirmTokenForTest("sample-project") {
		t.Fatalf("token generator not case-insensitive: %q", got)
	}

	svc := desktop.NewDBService()
	_, err := svc.RunDBQuery("sample-project", "DELETE FROM users", true, "  CONFIRM-WRITE:SAMPLE-PROJECT  ")
	if err != nil {
		t.Fatalf("RunDBQuery with padded mixed-case token error = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("executor calls = %d, want 1", calls)
	}
}

func TestDBService_LockingReadsRefusedByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	for _, sql := range []string{
		"SELECT * FROM users WHERE id = 1 FOR SHARE",
		"SELECT * FROM users WHERE id = 1 LOCK IN SHARE MODE",
		"SELECT * FROM users WHERE id = 1 FOR KEY SHARE",
		"select id from users for  share",
		"SELECT * FROM users WHERE id = 1 FOR NO KEY UPDATE",
	} {
		_, err := svc.RunDBQuery("sample-project", sql, false, "")
		if err == nil {
			t.Errorf("RunDBQuery(%q) error = nil, want refusal of locking read", sql)
		}
	}
	if calls != 0 {
		t.Errorf("executor calls = %d, want 0", calls)
	}
}

func TestDBService_LockWordInLiteralStillPasses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	stubDBServiceDiscovery(t)
	var calls int
	stubDBServiceExecutor(t, &calls, "ok\n")

	svc := desktop.NewDBService()
	res, err := svc.RunDBQuery(
		"sample-project",
		"SELECT * FROM docs WHERE body = 'lock in share mode explained'",
		false, "",
	)
	if err != nil {
		t.Fatalf("RunDBQuery lock words in literal error = %v, want nil", err)
	}
	if !res.ReadOnly {
		t.Errorf("ReadOnly = false, want true")
	}
	if calls != 1 {
		t.Errorf("executor calls = %d, want 1", calls)
	}
}
