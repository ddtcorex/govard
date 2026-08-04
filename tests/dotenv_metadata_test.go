package tests

import (
	"testing"

	"govard/internal/frameworks/shared/dotenv"
)

func TestParseDotenvDatabaseURL(t *testing.T) {
	info, err := dotenv.ParseDatabaseURLForTest("mysql://etl_user:s3cr3t@127.0.0.1:3307/etl_dev?serverVersion=8.0")
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}
	if info.Host != "127.0.0.1" {
		t.Fatalf("host mismatch: got %q", info.Host)
	}
	if info.Port != 3307 {
		t.Fatalf("port mismatch: got %d", info.Port)
	}
	if info.Username != "etl_user" {
		t.Fatalf("username mismatch: got %q", info.Username)
	}
	if info.Password != "s3cr3t" {
		t.Fatalf("password mismatch: got %q", info.Password)
	}
	if info.Database != "etl_dev" {
		t.Fatalf("database mismatch: got %q", info.Database)
	}
}

func TestResolveDotenvDBInfoPrefersDatabaseURL(t *testing.T) {
	info, err := dotenv.ResolveDatabaseInfoForTest(
		"mysql://correct_user:correct_pass@db.example:3309/correct_db",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("expected resolve success, got error: %v", err)
	}
	if info.Host != "db.example" || info.Port != 3309 || info.Username != "correct_user" || info.Database != "correct_db" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestResolveDotenvDBInfoFromDiscreteVars(t *testing.T) {
	info, err := dotenv.ResolveDatabaseInfoForTest(
		"",
		"db.internal",
		"3310",
		"etl",
		"secret",
		"etl_dev",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("expected resolve success, got error: %v", err)
	}
	if info.Host != "db.internal" {
		t.Fatalf("host mismatch: got %q", info.Host)
	}
	if info.Port != 3310 {
		t.Fatalf("port mismatch: got %d", info.Port)
	}
	if info.Username != "etl" {
		t.Fatalf("username mismatch: got %q", info.Username)
	}
	if info.Password != "secret" {
		t.Fatalf("password mismatch: got %q", info.Password)
	}
	if info.Database != "etl_dev" {
		t.Fatalf("database mismatch: got %q", info.Database)
	}
}
