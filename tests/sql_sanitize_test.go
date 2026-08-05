package tests

import (
	"strings"
	"testing"

	"govard/internal/engine"
)

func TestSanitizeSQLLineDefinerReplacement(t *testing.T) {
	line := "/*!50013 DEFINER=`alice`@`%` SQL SECURITY DEFINER */\n"
	sanitized, keep := engine.SanitizeSQLLine(line)
	if !keep {
		t.Fatal("expected line to be kept")
	}
	if !strings.Contains(sanitized, "/*!50013 */") {
		t.Fatalf("expected definer replacement, got: %s", sanitized)
	}
}

func TestSanitizeSQLLineDropsGTIDLines(t *testing.T) {
	line := "SET @@GLOBAL.GTID_PURGED='abc';\n"
	_, keep := engine.SanitizeSQLLine(line)
	if keep {
		t.Fatal("expected GTID line to be dropped")
	}
}

func TestSanitizeSQLDumpStream(t *testing.T) {
	input := strings.Join([]string{
		"CREATE TABLE test (id int);",
		"/*!50013 DEFINER=`alice`@`%` SQL SECURITY DEFINER */",
		"SET @@SESSION.SQL_LOG_BIN= 0;",
		"INSERT INTO test VALUES (1);",
		"",
	}, "\n")

	var output strings.Builder
	if err := engine.SanitizeSQLDump(strings.NewReader(input), &output); err != nil {
		t.Fatalf("sanitize dump: %v", err)
	}

	result := output.String()
	if strings.Contains(result, "@@SESSION.SQL_LOG_BIN") {
		t.Fatalf("expected SQL_LOG_BIN line removed, got: %s", result)
	}
	if !strings.Contains(result, "/*!50013 */") {
		t.Fatalf("expected definer replacement, got: %s", result)
	}
	if !strings.Contains(result, "INSERT INTO test VALUES (1);") {
		t.Fatalf("expected remaining payload preserved, got: %s", result)
	}
}

func TestSanitizeSQLLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
		keep     bool
	}{
		{
			name:     "Leave normal line",
			line:     "CREATE TABLE `test` (id int);",
			expected: "CREATE TABLE `test` (id int);",
			keep:     true,
		},
		{
			name:     "Remove GTID_PURGED",
			line:     "SET @@GLOBAL.GTID_PURGED='...';",
			expected: "",
			keep:     false,
		},
		{
			name:     "Remove SQL_LOG_BIN",
			line:     "SET @@SESSION.SQL_LOG_BIN= 0;",
			expected: "",
			keep:     false,
		},
		{
			name:     "Remove Sandbox lines",
			line:     `/*!999999\- enable the sandbox mode */`,
			expected: "",
			keep:     false,
		},
		{
			name: "Keep extended-insert row containing sandbox and 999999 as real data",
			line: "INSERT INTO `core_config_data` VALUES (1,'default',0,'general/store_information/name','MyStore')," +
				"(999999,'default',0,'somekey','someval'),(2,'default',0,'payment/paypal/environment','sandbox');",
			expected: "INSERT INTO `core_config_data` VALUES (1,'default',0,'general/store_information/name','MyStore')," +
				"(999999,'default',0,'somekey','someval'),(2,'default',0,'payment/paypal/environment','sandbox');",
			keep: true,
		},
		{
			name:     "Strip DEFINER",
			line:     "/*!50003 CREATE*/ /*!50017 DEFINER=`root`@`localhost`*/ /*!50003 TRIGGER `test`*/",
			expected: "/*!50003 CREATE*/ /*!50017 */ /*!50003 TRIGGER `test`*/",
			keep:     true,
		},
		{
			name:     "Remove ROW_FORMAT=FIXED",
			line:     "CREATE TABLE `test` (id int) ENGINE=InnoDB ROW_FORMAT=FIXED;",
			expected: "CREATE TABLE `test` (id int) ENGINE=InnoDB ;",
			keep:     true,
		},
		{
			name:     "Map utf8mb4_0900_ai_ci",
			line:     "COLLATE=utf8mb4_0900_ai_ci",
			expected: "COLLATE=utf8mb4_general_ci",
			keep:     true,
		},
		{
			name:     "Map utf8mb4_unicode_520_ci",
			line:     "CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_520_ci",
			expected: "CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci",
			keep:     true,
		},
		{
			name:     "Map utf8_unicode_520_ci",
			line:     "COLLATE=utf8_unicode_520_ci",
			expected: "COLLATE=utf8_general_ci",
			keep:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, keep := engine.SanitizeSQLLine(tt.line)
			if keep != tt.keep {
				t.Errorf("SanitizeSQLLine() keep = %v, want %v", keep, tt.keep)
			}
			if got != tt.expected {
				t.Errorf("SanitizeSQLLine() got = %v, want %v", got, tt.expected)
			}
		})
	}
}
