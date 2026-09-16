package tests

import (
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestResolveSeedSpecRefusesStoppedOrigin(t *testing.T) {
	_, err := deploy.ResolveSeedSpec(deploy.SeedSource{OriginRunning: false})
	if err == nil {
		t.Fatal("seeding from a stopped origin must fail loudly, not build an empty sandbox")
	}
}

func TestStripDefinerRemovesForeignDefiners(t *testing.T) {
	in := "/*!50013 DEFINER=`hyva-compat-modules-develop_dev8_sutunam_info`@`%` SQL SECURITY DEFINER */\nCREATE VIEW `inventory_stock_1` AS SELECT 1"
	got := deploy.StripDefiner(in)
	if strings.Contains(got, "DEFINER") {
		t.Fatalf("foreign definer survived: %q", got)
	}
	if !strings.Contains(got, "CREATE VIEW `inventory_stock_1`") {
		t.Fatalf("the statement body must survive: %q", got)
	}
}

func TestDumpArgsAreSingleTransactionWithoutLocks(t *testing.T) {
	spec, err := deploy.ResolveSeedSpec(deploy.SeedSource{
		OriginRunning: true,
		DB:            deploy.SeedDB{Container: "shop-php-1", User: "magento", Password: "magento", Name: "magento", Engine: "mariadb"},
	})
	if err != nil {
		t.Fatalf("seed spec: %v", err)
	}
	joined := strings.Join(spec.DBDumpArgs, " ")
	for _, want := range []string{"--single-transaction", "--skip-lock-tables"} {
		if !strings.Contains(joined, want) {
			t.Errorf("dump args must contain %s (the origin app user has no LOCK TABLES): %q", want, joined)
		}
	}
	if strings.Contains(joined, " -p") || strings.Contains(joined, "--password=") {
		t.Errorf("the password must never travel in argv (visible in ps): %q", joined)
	}
}
