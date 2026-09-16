package tests

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/engine"
	// Side-effect import: the package init registers all 14 framework
	// definitions (including their seed definitions) into engine.
	_ "govard/internal/frameworks"
	"govard/internal/frameworks/magento2"
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
	joined := strings.Join(spec.DBDumpFlags, " ")
	for _, want := range []string{"--single-transaction", "--skip-lock-tables"} {
		if !strings.Contains(joined, want) {
			t.Errorf("dump flags must contain %s (the origin app user has no LOCK TABLES): %q", want, joined)
		}
	}
	if strings.Contains(joined, " -p") || strings.Contains(joined, "--password=") {
		t.Errorf("the password must never travel in argv (visible in ps): %q", joined)
	}
	// Minimal MariaDB images ship mariadb-dump without the mysqldump symlink
	// (found live: the origin env db container), while MySQL images only have
	// mysqldump — so the binary is probed at seed time, in this order.
	if len(spec.DBDumpCandidates) != 2 || spec.DBDumpCandidates[0] != "mariadb-dump" || spec.DBDumpCandidates[1] != "mysqldump" {
		t.Errorf("dump candidates = %v, want [mariadb-dump mysqldump]", spec.DBDumpCandidates)
	}
}

func TestResolveDumpBinaryPrefersMariaDBDump(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.answers["command -v"] = "/usr/bin/mariadb-dump\n"
	runtime := deploy.NewDockerCLIForTest(fake.run)
	got, err := deploy.ResolveDumpBinaryForTest(context.Background(), runtime, "shop-db-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "mariadb-dump" {
		t.Errorf("resolved = %q, want mariadb-dump", got)
	}
}

func TestResolveDumpBinaryFallsBackToMysqldump(t *testing.T) {
	fake := newFakeSandboxRuntime()
	fake.answers["command -v"] = "/usr/bin/mysqldump\n"
	runtime := deploy.NewDockerCLIForTest(fake.run)
	got, err := deploy.ResolveDumpBinaryForTest(context.Background(), runtime, "shop-db-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "mysqldump" {
		t.Errorf("resolved = %q, want mysqldump", got)
	}
}

func seedSandboxUpRequest(t *testing.T, root, origin string) deploy.SandboxRequest {
	t.Helper()
	return deploy.SandboxRequest{
		ProjectRoot:       root,
		ProjectName:       "seed-shop-sandbox",
		Profile:           deploy.SandboxProfileBasic,
		Requirements:      deploy.SandboxRequirements{},
		Repository:        origin,
		Out:               io.Discard,
		Probe:             func(ctx context.Context, host string, port int, timeout time.Duration) error { return nil },
		SeedOrigin:        "seed-shop",
		SeedOriginRunning: true,
		SeedBlueprintRev:  "rev-1",
		SeedDBContainer:   "seed-shop-php-1",
		SeedDBUser:        "magento",
		SeedDBPassword:    "magento",
		SeedDBName:        "magento",
	}
}

func freshSandboxFake() *fakeSandboxRuntime {
	fake := newFakeSandboxRuntime()
	fake.fail["image inspect"] = "No such image"
	fake.fail["{{.Id}}"] = "No such container"
	fake.answers["22/tcp"] = "127.0.0.1:2222\n"
	// A real mysqladmin ping prints "mysqld is alive"; the wait accepts
	// nothing less, so the fake answers the real contract.
	fake.answers["mysqladmin ping"] = "mysqld is alive\n"
	// The dump-binary probe asks per candidate; answer the first one the way
	// a MariaDB container would.
	fake.answers["command -v mariadb-dump"] = "/usr/bin/mariadb-dump\n"
	return fake
}

func TestSandboxUpWritesDerivedFromState(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	state, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, seedSandboxUpRequest(t, root, origin))
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if state.DerivedFrom == nil {
		t.Fatal("a seeded sandbox must record where it was seeded from")
	}
	if state.DerivedFrom.Origin != "seed-shop" {
		t.Errorf("derived from = %q, want the origin project", state.DerivedFrom.Origin)
	}
	if !fake.has("mariadb-dump -u magento") && !fake.has("mysqldump -u magento") {
		t.Errorf("the seed must dump the origin database, got: %v", fake.calls)
	}
}

func TestSandboxUpNoSeedSkipsSnapshot(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	request := seedSandboxUpRequest(t, root, origin)
	request.NoSeed = true
	state, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, request)
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if fake.has("mysqldump") {
		t.Errorf("--no-seed must not touch the origin database, got: %v", fake.calls)
	}
	if state.DerivedFrom != nil {
		t.Errorf("an unseeded sandbox must not claim a derivation: %+v", state.DerivedFrom)
	}
}

func TestSandboxUpCopiesMediaThroughTar(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	request := seedSandboxUpRequest(t, root, origin)
	request.SeedAppContainer = "seed-shop-php-1"
	request.SeedMediaSource = "/var/www/html/pub/media"
	request.SeedMediaTarget = "/home/deployer/media-seed"
	// Derive the sandbox container name the way Down will: fresh Up writes it
	// into state; the media pipe targets that same container.
	state, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, request)
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if !fake.has("tar -C /var/www/html/pub/media") {
		t.Errorf("the seed must stream media out of the origin app container, got: %v", fake.calls)
	}
	if !fake.has("tar -C /home/deployer/media-seed") {
		t.Errorf("the seed must stream media into the sandbox %s, got: %v", state.Container, fake.calls)
	}
}

func TestSandboxDownVolumesRemovesDerivedVolumes(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	if _, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, seedSandboxUpRequest(t, root, origin)); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	before := len(fake.calls)
	downRequest := seedSandboxUpRequest(t, root, origin)
	downRequest.Volumes = true
	if _, err := deploy.SandboxDown(context.Background(), runtime, downRequest); err != nil {
		t.Fatalf("sandbox down: %v", err)
	}
	after := fake.calls[before:]
	joined := ""
	for _, call := range after {
		joined += strings.Join(call, " ") + "\n"
	}
	if !strings.Contains(joined, "volume ls") || !strings.Contains(joined, "seed-shop-sandbox") {
		t.Errorf("down --volumes must list the derived volumes, got:\n%s", joined)
	}
}

func TestSandboxDownKeepsVolumesByDefault(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	if _, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, seedSandboxUpRequest(t, root, origin)); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	before := len(fake.calls)
	if _, err := deploy.SandboxDown(context.Background(), runtime, seedSandboxUpRequest(t, root, origin)); err != nil {
		t.Fatalf("sandbox down: %v", err)
	}
	for _, call := range fake.calls[before:] {
		if strings.Contains(strings.Join(call, " "), "volume rm") {
			t.Errorf("plain down must keep volumes, but removed: %v", call)
		}
	}
}

func TestMagentoEnvRewritePointsAtSandbox(t *testing.T) {
	in := []byte(`'db' => ['connection' => ['default' => ['host' => '127.0.0.1', 'dbname' => 'magento']]], 'system' => ['default' => ['web' => ['unsecure' => ['base_url' => 'https://shop.test/']]]]`)
	mapping := map[string]string{"base_url": "https://shop-sandbox.test/"}
	got, skipped, err := magento2.RewriteMagentoEnvForSandbox(in, mapping)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none (the key is present)", skipped)
	}
	if !strings.Contains(string(got), "https://shop-sandbox.test/") {
		t.Errorf("base_url was not rewritten: %s", got)
	}
	if !strings.Contains(string(got), "'dbname' => 'magento'") {
		t.Errorf("unrelated keys must pass through untouched: %s", got)
	}
}

func TestMagentoEnvRewriteLocalizesServiceHosts(t *testing.T) {
	in := []byte(`'connection' => ['default' => ['host' => 'db', 'dbname' => 'magento']], 'cache' => ['frontend' => ['backend_options' => ['server' => 'redis', 'port' => '6379']]], 'session' => ['save' => 'redis', 'redis' => ['host' => 'redis']]`)
	got, _, err := magento2.RewriteMagentoEnvForSandbox(in, map[string]string{})
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// A single-container sandbox runs every service on loopback: any
	// compose-network hostname left over would fail DNS at the first bin/magento
	// call (found live: "getaddrinfo for db failed").
	for _, want := range []string{`'host' => '127.0.0.1'`, `'server' => '127.0.0.1'`, `'port' => '6379'`, `'dbname' => 'magento'`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("want %s in rewritten env, got: %s", want, got)
		}
	}
	for _, gone := range []string{`'host' => 'db'`, `'server' => 'redis'`, `'host' => 'redis'`} {
		if strings.Contains(string(got), gone) {
			t.Errorf("compose hostname must not survive (%s): %s", gone, got)
		}
	}
	// 'save' => 'redis' is a driver selector, not a hostname: it stays.
	if !strings.Contains(string(got), `'save' => 'redis'`) {
		t.Errorf("driver selectors are not hostnames and must survive: %s", got)
	}
}

func TestMagentoEnvRewriteSkipsAbsentKeysLoudly(t *testing.T) {	in := []byte(`'db' => ['connection' => ['default' => ['host' => '127.0.0.1']]]`)
	mapping := map[string]string{"base_url": "https://shop-sandbox.test/"}
	got, skipped, err := magento2.RewriteMagentoEnvForSandbox(in, mapping)
	if err != nil {
		t.Fatalf("an absent key must not fail the seed: %v", err)
	}
	if len(skipped) != 1 || skipped[0] != "base_url" {
		t.Errorf("skipped = %v, want [base_url]", skipped)
	}
	if string(got) != string(in) {
		t.Errorf("nothing present means byte-identical output, got: %s", got)
	}
}

func TestSandboxSeedRegistryServesMagento2(t *testing.T) {
	definition, ok := engine.SandboxSeedFor("magento2")
	if !ok {
		t.Fatal("magento2 registered no sandbox seed definition")
	}
	if definition.EnvPath != "app/etc/env.php" {
		t.Errorf("env path = %q, want app/etc/env.php", definition.EnvPath)
	}
	if definition.MediaPath != "pub/media" {
		t.Errorf("media path = %q, want pub/media", definition.MediaPath)
	}
	if definition.Rewrite == nil {
		t.Fatal("magento2 registered no env rewriter")
	}
}

func TestSandboxSeedRegistryIgnoresUnknownFrameworks(t *testing.T) {
	if _, ok := engine.SandboxSeedFor("no-such-framework"); ok {
		t.Fatal("an unregistered framework must report no seed definition")
	}
}

func TestSandboxSeedHandsOwnershipToDeployer(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	runtime := deploy.NewDockerCLIForTest(fake.run)

	if _, err := deploy.SandboxUp(context.Background(), runtime, deploy.LocalRunner{}, seedSandboxUpRequest(t, root, origin)); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	// Exec runs as container root, so every file the seed writes lands
	// root-owned; the deploy pipeline runs as the deploy user and must own
	// the tree, or deploy:check fails on a non-writable deploy path (found
	// live: exit 1 on mkdir/test -w).
	want := fmt.Sprintf("chown -R %d:%d", deploy.SandboxUserUID, deploy.SandboxUserGID)
	if !fake.has(want) {
		t.Errorf("the seed must hand the tree to the deploy user (%q), got: %v", want, fake.calls)
	}
}
