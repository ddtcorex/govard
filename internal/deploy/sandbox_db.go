package deploy

import (
	"fmt"
	"regexp"
	"strings"
)

// The database a `full` sandbox provides has to be the one the project runs.
// Debian's own MariaDB is a single series (10.11 on the pinned release), and a
// Magento 2.4.6 deploy rehearsed against it fails at db:migrate with "Current
// version of RDBMS is not supported" — a failure that looks like a project
// defect. A requested series therefore comes from the official MariaDB
// repository, the same shape as sury for PHP.
const (
	sandboxDBEngineMariaDB = "mariadb"
	sandboxDBEngineMySQL   = "mysql"

	// sandboxDBDefault is the `--db` value that keeps the base distribution's
	// own database server.
	sandboxDBDefault = "default"

	sandboxMariaDBKeyring = "/usr/share/keyrings/mariadb-release.asc"
	sandboxMariaDBSource  = "/etc/apt/sources.list.d/mariadb.list"
	sandboxMariaDBPin     = "/etc/apt/preferences.d/mariadb"
)

// sandboxDBSeries is `major.minor`, the unit MariaDB's repository is keyed on.
var sandboxDBSeries = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// sandboxMariaDBSources lists the series the official repository publishes in
// a form the sandbox's Debian release can install, measured against the
// repository: 10.11 and the 11.x long-term series are built for bookworm, while
// 10.6 is built only for Debian 11 (whose OpenSSL 1.1 cannot be installed here)
// and Ubuntu 22.04, whose glibc and OpenSSL 3 match bookworm's. Any other series
// has no installable package, so it is refused up front instead of failing
// halfway through an image build.
var sandboxMariaDBSources = map[string]struct{ Distro, Codename string }{
	"10.6":  {"ubuntu", "jammy"},
	"10.11": {"debian", sandboxDebianCodename},
	"11.4":  {"debian", sandboxDebianCodename},
	"11.8":  {"debian", sandboxDebianCodename},
}

func sandboxMariaDBSupported() string {
	return "10.6, 10.11, 11.4, 11.8"
}

// sandboxDBVersionPrefix extracts `major.minor` from a project's db_version,
// which may carry a patch level.
var sandboxDBVersionPrefix = regexp.MustCompile(`^([0-9]+\.[0-9]+)(\.[0-9]+)*$`)

// ParseSandboxDB validates an explicit database request, `<engine>:<series>`
// (for example `mariadb:10.6`) or `default`. It returns the normalized value,
// empty for the base distribution's server. The value is rendered into the
// image definition, so anything that is not an engine and a series is refused.
func ParseSandboxDB(value string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" || trimmed == sandboxDBDefault {
		return "", nil
	}
	engine, series, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", fmt.Errorf("unsupported sandbox database %q; use mariadb:<series> (for example mariadb:10.6) or default", value)
	}
	return sandboxDBFor(engine, series, "the requested database")
}

// ResolveSandboxDBDefault resolves the database a new `full` sandbox provides
// from the project's stack. A stack that names no MariaDB or MySQL keeps the
// base distribution's server (empty result). A MySQL stack is refused: the
// sandbox can only provide MariaDB, and quietly handing back another engine is
// the defect this exists to prevent.
func ResolveSandboxDBDefault(engine, version string) (string, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	version = strings.TrimSpace(version)
	switch engine {
	case sandboxDBEngineMariaDB, sandboxDBEngineMySQL:
	default:
		return "", nil
	}
	if version == "" {
		if engine == sandboxDBEngineMySQL {
			return "", sandboxDBMismatch(engine, version)
		}
		return "", nil
	}
	match := sandboxDBVersionPrefix.FindStringSubmatch(version)
	if match == nil {
		return "", fmt.Errorf("the project database version %q is not a series such as 10.6; set stack.db_version or pass --db mariadb:<series> (or --db default)", version)
	}
	return sandboxDBFor(engine, match[1], "the project database")
}

func sandboxDBFor(engine, series, subject string) (string, error) {
	switch engine {
	case sandboxDBEngineMariaDB:
		if !sandboxDBSeries.MatchString(series) {
			return "", fmt.Errorf("unsupported MariaDB series %q; use major.minor such as 10.6 or 11.4", series)
		}
		if _, ok := sandboxMariaDBSources[series]; !ok {
			return "", fmt.Errorf("%s asks for MariaDB %s, which the official MariaDB repository does not provide in an installable form for the sandbox's Debian release (available: %s); pass --db with one of those or --db default", subject, series, sandboxMariaDBSupported())
		}
		return sandboxDBEngineMariaDB + ":" + series, nil
	case sandboxDBEngineMySQL:
		return "", sandboxDBMismatch(engine, series)
	default:
		return "", fmt.Errorf("unsupported sandbox database engine %q; the sandbox provides mariadb", engine)
	}
}

func sandboxDBMismatch(engine, version string) error {
	named := engine
	if version != "" {
		named += " " + version
	}
	return fmt.Errorf("the project database is %s, but the sandbox can only provide MariaDB; rehearsing on a different engine proves nothing about this project. Pass --db mariadb:<series> to choose a MariaDB series anyway, or --db default for the distribution's own", named)
}

// sandboxDBParts splits a normalized database value, `mariadb:10.6`.
func sandboxDBParts(db string) (engine, series string) {
	engine, series, _ = strings.Cut(db, ":")
	return engine, series
}

// sandboxDBRepositoryBlock is the Dockerfile step that points apt at the
// requested MariaDB series and makes it win over Debian's own, which is a
// higher version: without the pin apt would still install the distribution's.
func sandboxDBRepositoryBlock(db string) string {
	_, series := sandboxDBParts(db)
	source := sandboxMariaDBSources[series]
	return fmt.Sprintf(`RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends curl gnupg ca-certificates; \
    curl -fsSL https://mariadb.org/mariadb_release_signing_key.pgp -o %s; \
    echo "deb [signed-by=%s] https://dlm.mariadb.com/repo/mariadb-server/%s/repo/%s %s main" > %s; \
    printf 'Package: *\nPin: origin dlm.mariadb.com\nPin-Priority: 1001\n' > %s; \
    apt-get update

`, sandboxMariaDBKeyring, sandboxMariaDBKeyring, series, source.Distro, source.Codename, sandboxMariaDBSource, sandboxMariaDBPin)
}

// sandboxDBVerifyBlock fails the build when the installed server is not the
// requested series: a silently wrong database is worse than no image.
func sandboxDBVerifyBlock(db string) string {
	_, series := sandboxDBParts(db)
	return fmt.Sprintf(`
RUN set -eux; \
    mariadbd --version; \
    mariadbd --version | grep -Eq 'Ver %s[.-]' || { echo "govard: the installed MariaDB is not the requested %s series" >&2; exit 1; }
`, regexp.QuoteMeta(series), series)
}
