package deploy

import (
	"fmt"
	"regexp"
	"strings"
)

// SeedDB names the origin database to snapshot. The password travels on stdin
// (MYSQL_PWD) or a file, never in argv — same rule as COMPOSER_AUTH.
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

// SeedSpec is the resolved snapshot plan: dump/import argv without secrets,
// media endpoints, and the env-file rewrite mapping. The caller executes it.
type SeedSpec struct {
	DBDumpArgs   []string
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
		DBDumpArgs:   []string{"exec", source.DB.Container, "mysqldump", "-u", source.DB.User, "--single-transaction", "--skip-lock-tables", source.DB.Name},
		DBImportArgs: []string{"exec", "<sandbox-db-container>", "mysql", "-u", source.DB.User, source.DB.Name},
		DBPassword:   source.DB.Password,
		MediaSource:  source.MediaSource,
		EnvSource:    source.EnvSource,
		EnvMapping:   map[string]string{},
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
