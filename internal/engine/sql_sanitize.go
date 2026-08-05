package engine

import (
	"bufio"
	"errors"
	"io"
	"regexp"
	"strings"
)

var (
	sqlDefinerPattern = regexp.MustCompile(`DEFINER\s*=\s*[^*]+\*`)
	// Matches only the literal mysqlbinlog "sandbox mode" comment artifact
	// (e.g. `/*!999999\- enable the sandbox mode */`), anchored to the
	// `/*!999999` version-comment delimiter and closing `*/`. Must NOT be
	// a bare substring match: mysqldump uses --extended-insert by default,
	// so a whole table's rows can share one line, and real data containing
	// both the digits "999999" and the word "sandbox" (e.g. a PayPal/Braintree
	// "sandbox" config value next to an unrelated numeric ID) would otherwise
	// cause that entire INSERT statement to be silently dropped.
	sqlSandboxPattern       = regexp.MustCompile(`/\*!999999.*sandbox.*\*/`)
	sqlRowFormatPattern     = regexp.MustCompile(`ROW_FORMAT\s*=\s*FIXED`)
	sqlCollation0900Pattern = regexp.MustCompile(`utf8mb4_0900_ai_ci`)
	sqlCollation520Pattern  = regexp.MustCompile(`utf8(mb4)?_unicode_520_ci`)
)

func SanitizeSQLDump(input io.Reader, output io.Writer) error {
	reader := bufio.NewReader(input)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			sanitized, keep := SanitizeSQLLine(line)
			if keep {
				if _, writeErr := io.WriteString(output, sanitized); writeErr != nil {
					return writeErr
				}
			}
		}

		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func SanitizeSQLLine(line string) (string, bool) {
	// Optimization: Skip regex calls for the 99% of lines (INSERT data) that don't need changes.
	// We handle few special session variables first.
	if strings.Contains(line, "@@") {
		if strings.Contains(line, "@@GLOBAL.GTID_PURGED") || strings.Contains(line, "@@SESSION.SQL_LOG_BIN") {
			return "", false
		}
	}

	// Pattern checking with strings.Contains is much faster than running multiple regexes on every line.
	hasDefiner := strings.Contains(line, "DEFINER")
	hasRowFormat := strings.Contains(line, "ROW_FORMAT")
	has0900 := strings.Contains(line, "utf8mb4_0900_ai_ci")
	has520 := strings.Contains(line, "_unicode_520_ci")
	hasSandbox := strings.Contains(line, "/*!999999")

	// If none of these exist, return the line as-is immediately.
	if !hasDefiner && !hasRowFormat && !has0900 && !has520 && !hasSandbox {
		return line, true
	}

	if hasSandbox && sqlSandboxPattern.MatchString(line) {
		return "", false
	}

	if hasDefiner {
		line = sqlDefinerPattern.ReplaceAllString(line, "*")
	}
	if hasRowFormat {
		line = sqlRowFormatPattern.ReplaceAllString(line, "")
	}
	if has0900 {
		line = sqlCollation0900Pattern.ReplaceAllString(line, "utf8mb4_general_ci")
	}
	if has520 {
		line = sqlCollation520Pattern.ReplaceAllString(line, "utf8${1}_general_ci")
	}

	return line, true
}
