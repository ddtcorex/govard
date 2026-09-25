package tests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
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
	in := "/*!50013 DEFINER=`shop_develop_dbuser`@`%` SQL SECURITY DEFINER */\nCREATE VIEW `inventory_stock_1` AS SELECT 1"
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
		DB:            deploy.SeedDB{Container: "shop-php-1", User: "magento", Password: "magento", Name: "magento"},
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

// A real Magento database does not fit in a Go string, and buffering it twice
// (once as the dump, once as the rewritten copy) is what made `sandbox up` a
// memory event on a multi-gigabyte origin. The seed streams the dump from the
// origin container into the sandbox container; the only dump bytes resident are
// one line at a time.
func TestSandboxSeedStreamsTheDumpWithoutBufferingIt(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	// ~64 MiB of dump, in 4 KiB lines: far more than the whole seed is allowed
	// to allocate, and small enough to run quickly. Every line is an extended
	// INSERT the stripper must pass through untouched.
	line := "INSERT INTO `catalog_product_entity` VALUES (" + strings.Repeat("1,", 2000) + "1);\n"
	const total = 64 << 20
	fake.bodies["mariadb-dump -u magento"] = []byte(strings.Repeat(line, total/len(line)))
	containers := deploy.NewDockerCLIForTest(fake.run)

	request := seedSandboxUpRequest(t, root, origin)
	request.SeedDBPassword = "s3cret-pw"

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if _, err := deploy.SandboxUp(context.Background(), containers, deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	runtime.ReadMemStats(&after)

	// TotalAlloc counts every byte ever allocated, so GC timing cannot hide a
	// buffer. A whole-dump copy is at least `total`; a stream is buffers.
	growth := after.TotalAlloc - before.TotalAlloc
	t.Logf("seeding a %d MiB dump allocated %d MiB", total>>20, growth>>20)
	if growth > 16<<20 {
		t.Fatalf("seeding a %d MiB dump allocated %d MiB: the dump is held whole instead of streamed",
			total>>20, growth>>20)
	}
}

func TestSandboxUpCopiesMediaThroughTar(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	// The archive the origin tar produces; the sandbox tar must read exactly it.
	archive := []byte("fake-tar-archive-bytes")
	fake.bodies["tar -C /var/www/html/pub/media -cf - ."] = archive
	containers := deploy.NewDockerCLIForTest(fake.run)

	request := seedSandboxUpRequest(t, root, origin)
	request.SeedAppContainer = "seed-shop-php-1"
	request.SeedMediaSource = "/var/www/html/pub/media"
	request.SeedMediaTarget = "/home/deployer/media-seed"
	// Derive the sandbox container name the way Down will: fresh Up writes it
	// into state; the media pipe targets that same container.
	state, err := deploy.SandboxUp(context.Background(), containers, deploy.LocalRunner{}, request)
	if err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if !fake.has("tar -C /var/www/html/pub/media") {
		t.Errorf("the seed must stream media out of the origin app container, got: %v", fake.calls)
	}
	if !fake.has("tar -C /home/deployer/media-seed") {
		t.Errorf("the seed must stream media into the sandbox %s, got: %v", state.Container, fake.calls)
	}
	// `mkdir` used to be handed the archive as stdin: it ignored it, and the
	// only way to provide it was to hold the whole archive in memory.
	if record, ok := fake.stdinOf("mkdir -p /home/deployer/media-seed"); ok {
		t.Errorf("mkdir must not be fed the media archive, got %d bytes", record.Bytes)
	}
	reader, ok := fake.stdinOf("tar -C /home/deployer/media-seed -xf -")
	if !ok {
		t.Fatalf("the sandbox tar must read the archive from its stdin, got: %v", fake.calls)
	}
	if reader.Bytes != int64(len(archive)) {
		t.Errorf("the sandbox tar read %d bytes, want the %d the origin wrote", reader.Bytes, len(archive))
	}
}

// The password must reach both mysql clients without ever becoming an argument:
// `ps` on a shared host reads argv, and an argv entry is what the seed used to
// hand over. It is passed by name (`-e MYSQL_PWD`), with the value in the
// runtime process's own environment.
func TestSandboxSeedNeverPutsThePasswordInArgv(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	containers := deploy.NewDockerCLIForTest(fake.run)

	request := seedSandboxUpRequest(t, root, origin)
	request.SeedDBPassword = "s3cret-pw"
	if _, err := deploy.SandboxUp(context.Background(), containers, deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}

	for _, call := range fake.calls {
		for _, argument := range call {
			if strings.Contains(argument, "s3cret-pw") {
				t.Fatalf("the password travelled in argv: %v", call)
			}
			if strings.HasPrefix(argument, "MYSQL_PWD=") {
				t.Fatalf("the password travelled as an argv environment entry: %v", call)
			}
		}
	}
	if !fake.hasArg("-e", "MYSQL_PWD") {
		t.Errorf("the runtime was never asked to pass MYSQL_PWD through by name: %v", fake.calls)
	}
	if !fake.hasEnvValue("MYSQL_PWD=s3cret-pw") {
		t.Errorf("the password must travel in the runtime's environment, got: %v", fake.envs)
	}
	// Both clients need it: the dump (no stdin) and the import (stdin).
	for _, client := range []string{"mariadb-dump -u magento", "mysql -u magento magento"} {
		carried := false
		for _, entry := range fake.envOf(client) {
			if entry == "MYSQL_PWD=s3cret-pw" {
				carried = true
			}
		}
		if !carried {
			t.Errorf("the %s invocation did not carry the password in its environment: %v", client, fake.envOf(client))
		}
	}
}

// The dump and the import are wired through two pipes, so one failure makes the
// other fail too — a dead import closes the pipe under the dump, and a dead dump
// truncates the stream under the import. Reporting only the first error checked
// names the symptom and hides the cause: the classic seeding failure
// (`Unknown collation` on a MariaDB sandbox) reached the operator as "broken pipe"
// from the dump, which says nothing about what to fix. Both sides are now
// reported with the side they came from, and "partial" still says the database is
// not usable.
func TestSandboxSeedReportsBothSidesWhenThePipesBreak(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	fake.bodies["mariadb-dump -u magento"] = []byte("INSERT INTO `x` VALUES (1);\nINSERT INTO `x` VALUES (2);\n")
	fake.partial["mariadb-dump -u magento"] = "mysqldump: Got error: 2013: Lost connection to MySQL server during query"
	fake.partial["mysql -u magento magento"] = "ERROR 1273 (HY000) at line 1: Unknown collation: 'utf8mb4_0900_ai_ci'"
	containers := deploy.NewDockerCLIForTest(fake.run)

	_, err := deploy.SandboxUp(context.Background(), containers, deploy.LocalRunner{}, seedSandboxUpRequest(t, root, origin))
	if err == nil {
		t.Fatal("a seed whose pipes broke must fail")
	}
	for _, want := range []string{"Unknown collation", "Lost connection", "partial"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure must report %q, got: %v", want, err)
		}
	}
}

// The cap on one line is a refusal, and it has to survive the same wiring: the
// stripper's own error used to reach the operator as the dump's broken pipe, so
// the one sentence that names the real problem — a line over the cap — never
// appeared. The limit is injectable so the refusal is exercised through the path
// that runs, not only through the stripper helper.
func TestSandboxSeedReportsAnOverlongLineThroughTheRealPath(t *testing.T) {
	fake := freshSandboxFake()
	fake.bodies["mariadb-dump -u magento"] = []byte(strings.Repeat("x", 4096) + "\nINSERT INTO `x` VALUES (1);\n")
	containers := deploy.NewDockerCLIForTest(fake.run)

	err := deploy.StreamDatabaseDumpForTest(context.Background(), containers, "origin", "sandbox", nil,
		[]string{"mariadb-dump", "-u", "magento"}, []string{"mysql", "-u", "magento", "magento"}, 1024)
	if err == nil {
		t.Fatal("a dump line over the cap must fail the seed")
	}
	if !strings.Contains(err.Error(), "line longer than") {
		t.Fatalf("the cap's own refusal must survive the pipes, got: %v", err)
	}
	if !strings.Contains(err.Error(), "partial") {
		t.Fatalf("the failure must still say the database is partial, got: %v", err)
	}
}

// A dump that dies midway leaves the sandbox database partially imported. The
// import may still exit 0 after reading a truncated stream, so the seed has to
// report the dump's own failure and say the database is partial.
func TestSandboxSeedFailsLoudlyOnAPartialDump(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	fake.bodies["mariadb-dump -u magento"] = []byte("INSERT INTO `x` VALUES (1);\n")
	fake.partial["mariadb-dump -u magento"] = "mysqldump: Got error: 2013: Lost connection to MySQL server during query"
	containers := deploy.NewDockerCLIForTest(fake.run)

	_, err := deploy.SandboxUp(context.Background(), containers, deploy.LocalRunner{}, seedSandboxUpRequest(t, root, origin))
	if err == nil {
		t.Fatal("a dump that failed midway must fail the seed, not leave a half-imported database silently")
	}
	if !strings.Contains(err.Error(), "partial") {
		t.Errorf("the error must say the database is partial: %v", err)
	}
	if !strings.Contains(err.Error(), "Lost connection") {
		t.Errorf("the dump's own failure must survive: %v", err)
	}
}

// The refusal is the point: an error that quotes an environment value would put
// the secret in a log, which is the other half of keeping it out of argv.
func TestSandboxExecErrorOmitsEnvironmentValues(t *testing.T) {
	containers := deploy.NewDockerCLIForTest(func(ctx context.Context, command deploy.SandboxCommand) (string, error) {
		return "", &deploy.CommandError{
			Command:  "docker " + strings.Join(command.Args, " "),
			ExitCode: 1,
			Stderr:   "Access denied for user",
		}
	})

	err := containers.ExecStream(context.Background(), "sandbox", []string{"MYSQL_PWD=s3cret-pw"}, nil, nil, "mysql", "-u", "magento")
	if err == nil {
		t.Fatal("a failing exec must fail")
	}
	if strings.Contains(err.Error(), "s3cret-pw") {
		t.Fatalf("the error printed an environment value: %v", err)
	}
	if !strings.Contains(err.Error(), "MYSQL_PWD") {
		t.Errorf("the error should still name the variable it passed: %v", err)
	}
}

// The streaming stripper is the whole-dump stripper applied one line at a time;
// anything else would change what reaches the sandbox.
func TestStripDefinerStreamMatchesTheStringVersion(t *testing.T) {
	const dump = "CREATE TABLE `a` (id int);\n" +
		"/*!50013 DEFINER=`staging_dbuser`@`%` SQL SECURITY DEFINER */\n" +
		"CREATE VIEW `v` AS SELECT 1;\n" +
		"INSERT INTO `a` VALUES (1);\n"
	var out bytes.Buffer
	if err := deploy.StripDefinerStream(strings.NewReader(dump), &out); err != nil {
		t.Fatalf("strip: %v", err)
	}
	if got, want := out.String(), deploy.StripDefiner(dump); got != want {
		t.Fatalf("streamed strip =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(out.String(), "DEFINER") {
		t.Fatalf("a foreign definer survived the stream: %q", out.String())
	}
}

// A line past the cap is a refusal, not a buffer: an unbounded line is how a
// bounded stream turns back into an out-of-memory event.
func TestStripDefinerStreamRefusesAnEndlessLine(t *testing.T) {
	var out bytes.Buffer
	err := deploy.StripDefinerStreamForTest(strings.NewReader(strings.Repeat("x", 4096)+"\n"), &out, 1024)
	if err == nil {
		t.Fatal("a line past the cap must be refused")
	}
	if !strings.Contains(err.Error(), "1024") {
		t.Errorf("the refusal must name the limit it exceeded: %v", err)
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

func TestMagentoEnvRewriteSkipsAbsentKeysLoudly(t *testing.T) {
	in := []byte(`'db' => ['connection' => ['default' => ['host' => '127.0.0.1']]]`)
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
	if definition.DBRewrite == nil {
		t.Fatal("magento2 registered no database rewrite for the seeded base_url")
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

// The seed rewrites one env file, but a framework's live configuration does not
// always live in a file: Magento's base_url lives in the database the seed just
// imported. A seeded sandbox that keeps the origin's base_url answers the
// verify check with a redirect to the origin domain — found on a real Magento 2
// rehearsal — so the seed has to point the framework's database at itself.
func TestSandboxSeedRunsTheDBRewrite(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	fake.answers["80/tcp"] = "127.0.0.1:32768\n"
	fake.answers["cat app/etc/env.php"] = "<?php return array('db' => array('table_prefix' => 'mg_'));\n"
	containers := deploy.NewDockerCLIForTest(fake.run)

	var gotContent []byte
	var gotBaseURL string
	calls := 0
	rewrite := func(envContent []byte, baseURL string) []string {
		calls++
		gotContent = envContent
		gotBaseURL = baseURL
		return []string{"UPDATE mg_core_config_data SET value='http://127.0.0.1:32768/' WHERE path='web/unsecure/base_url'"}
	}

	request := seedSandboxUpRequest(t, root, origin)
	request.Profile = deploy.SandboxProfilePHP
	request.SeedDBPassword = "s3cret-pw"
	request.SeedEnvSource = "app/etc/env.php"
	request.SeedAppContainer = "seed-shop-php-1"
	request.DBRewrite = rewrite
	if _, err := deploy.SandboxUp(context.Background(), containers, deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if calls != 1 {
		t.Fatalf("the db rewrite ran %d times, want once", calls)
	}
	if gotBaseURL != "http://127.0.0.1:32768/" {
		t.Errorf("base url = %q, want the sandbox web url", gotBaseURL)
	}
	if !strings.Contains(string(gotContent), "table_prefix") {
		t.Errorf("the rewrite must see the origin env content, got %q", gotContent)
	}
	// The import plus the one statement travel the same mysql shape.
	imports := 0
	for _, call := range fake.calls {
		if strings.Contains(strings.Join(call, " "), "mysql -u magento magento") {
			imports++
		}
	}
	if imports != 2 {
		t.Errorf("mysql ran %d times, want the import plus the one statement", imports)
	}
	for _, call := range fake.calls {
		for _, argument := range call {
			if strings.Contains(argument, "s3cret-pw") {
				t.Fatalf("the password travelled in argv: %v", call)
			}
		}
	}
}

// Without a web tier the sandbox has no URL of its own: a rewrite would point
// the database at nothing, so `basic` must skip it even when one is registered.
func TestSandboxSeedSkipsTheDBRewriteWithoutAWebPort(t *testing.T) {
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	fake := freshSandboxFake()
	containers := deploy.NewDockerCLIForTest(fake.run)

	calls := 0
	request := seedSandboxUpRequest(t, root, origin)
	request.DBRewrite = func(envContent []byte, baseURL string) []string {
		calls++
		return []string{"UPDATE x SET y='z'"}
	}
	if _, err := deploy.SandboxUp(context.Background(), containers, deploy.LocalRunner{}, request); err != nil {
		t.Fatalf("sandbox up: %v", err)
	}
	if calls != 0 {
		t.Fatalf("the db rewrite ran %d times without a web port, want none", calls)
	}
}
