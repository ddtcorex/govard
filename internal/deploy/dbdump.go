package deploy

// DumpToolProbe is the shell test every plain-dump recipe uses to ask whether the
// target has a dump client at all.
const DumpToolProbe = `command -v mariadb-dump >/dev/null 2>&1 || command -v mysqldump >/dev/null 2>&1`

// optionFilePHPTail turns the credentials a recipe's PHP fragment read into an
// option file and prints the database name.
//
// The fragment must set $u (user), $w (password), $h (host, optionally
// `host:port` or `host:/socket`), $s (socket path or "") and $n (database name).
// The file is the only place the password goes: it is passed to the dump client
// with --defaults-extra-file, so it never appears in a process listing or in the
// command text a log captures. Output buffered before the name is dropped, so a
// plugin that echoes while the application boots cannot corrupt it. The fragment
// contains no single quote, which is what keeps `php -r '...'` one shell word.
const optionFilePHPTail = `$p=""; if(strpos($h,":")!==false){list($h,$p)=explode(":",$h,2); if(isset($p[0])&&$p[0]=="/"){$s=$p;$p="";}} ` +
	`$q=function($v){return "\"".addcslashes((string)$v,"\\\"")."\"";}; ` +
	`$o="[client]\nuser=".$q($u)."\npassword=".$q($w)."\nhost=".$q($h)."\n"; ` +
	`if($p!==""){$o.="port=".$q($p)."\n";} if($s!==""){$o.="socket=".$q($s)."\n";} ` +
	`file_put_contents($argv[1],$o); ob_end_clean(); echo $n;`

// OptionFileDumpCommand writes a plain SQL dump to {{backup_path}} using the
// credentials the PHP fragment reads from the application's own configuration.
//
// It runs in {{release_path}}. The option file is created private by mktemp,
// removed on every path, and a partial dump is removed when the client fails, so
// a failure leaves no credential file and no half-written backup behind.
func OptionFileDumpCommand(readCredentials string) string {
	return `cd {{release_path}} && opts="$(mktemp)" && { ` +
		`db="$({{php_bin}} -r 'ob_start(); ` + readCredentials + ` ` + optionFilePHPTail + `' "$opts")" && test -n "$db" && ` +
		`if command -v mariadb-dump >/dev/null 2>&1; then DUMP_BIN=mariadb-dump; ` +
		`elif command -v mysqldump >/dev/null 2>&1; then DUMP_BIN=mysqldump; ` +
		`else echo "db:backup: neither mariadb-dump nor mysqldump is installed on the target" >&2; false; fi && ` +
		`"$DUMP_BIN" --defaults-extra-file="$opts" --single-transaction --quick "$db" > {{backup_path}}; ` +
		`rc=$?; rm -f "$opts"; [ "$rc" -eq 0 ] || rm -f {{backup_path}}; exit "$rc"; }`
}
