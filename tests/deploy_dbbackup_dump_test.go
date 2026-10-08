package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/frameworks/magento2"
	"govard/internal/frameworks/wordpress"
)

// The backup step must not depend on a feature stock Magento ships disabled
// (`setup:backup` answers "Backup functionality is disabled"), and it must not
// put the database password on any command line: the process list of a shared
// target shows argv.
func TestMagento2DumpDoesNotUseSetupBackupOrPutThePasswordInArgv(t *testing.T) {
	recipe := magento2.DeployRecipe()
	if recipe.DBBackupProbe.Command == "" {
		t.Fatal("the Magento recipe declares no db:backup preflight probe")
	}
	if strings.Contains(recipe.DBBackupCommand, "setup:backup") {
		t.Fatalf("the dump still goes through setup:backup, which stock Magento rejects:\n%s", recipe.DBBackupCommand)
	}
	for _, want := range []string{"--defaults-extra-file=", "app/etc/env.php", "mariadb-dump", "mysqldump", "{{backup_path}}"} {
		if !strings.Contains(recipe.DBBackupCommand, want) {
			t.Errorf("the dump command lacks %q:\n%s", want, recipe.DBBackupCommand)
		}
	}
	if strings.Contains(recipe.DBBackupCommand, "--password") || strings.Contains(recipe.DBBackupCommand, " -p") {
		t.Errorf("the dump command passes a password flag:\n%s", recipe.DBBackupCommand)
	}
}

// Runs the generated shell with stand-ins for php and the dump tool: the option
// file carries the credentials, argv carries none, the dump lands where the
// engine asked, and the temporary option file is gone afterwards.
func TestOptionFileDumpShellFlow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dumpRC  int
		wantErr bool
	}{
		{name: "the dump succeeds"},
		{name: "the dump fails", dumpRC: 3, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			tmp := filepath.Join(root, "tmp")
			for _, dir := range []string{bin, tmp} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			// php stand-in: writes the option file named by its last argument and
			// prints the database name, which is the contract of the PHP fragment.
			php := "#!/bin/sh\nfor last; do :; done\nprintf '[client]\\npassword=\"s3cret\"\\n' > \"$last\"\nprintf 'shop'\n"
			dump := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + filepath.Join(root, "argv.txt") + "\n" +
				"first=\"$1\"; file=\"${first#--defaults-extra-file=}\"; cp \"$file\" " + filepath.Join(root, "optfile.txt") + "\n" +
				"printf 'dump body'\nexit " + string(rune('0'+tc.dumpRC)) + "\n"
			for name, body := range map[string]string{"php": php, "mysqldump": dump} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin+":/usr/bin:/bin")
			t.Setenv("TMPDIR", tmp)

			host := deploy.HostForTest(root, deploy.LocalRunner{})
			release := deploy.NewReleaseForTest("1", "abcdef", "main")
			release.Path = host.ReleasePath("1")
			if err := os.MkdirAll(release.Path, 0o755); err != nil {
				t.Fatal(err)
			}
			sc := deploy.StepContextForTest(host, deploy.Options{DBBackup: true, CommandTimeout: time.Minute})
			sc.Release = release
			sc.Vars = sc.Vars.Set("php_bin", "php").SetPath("release_path", release.Path)

			err := deploy.CoreDBBackup(magento2.DeployRecipe().DBBackupCommand)(context.Background(), sc)
			if tc.wantErr != (err != nil) {
				t.Fatalf("db:backup err = %v, want error %v", err, tc.wantErr)
			}
			argv, _ := os.ReadFile(filepath.Join(root, "argv.txt"))
			if strings.Contains(string(argv), "s3cret") {
				t.Fatalf("the password reached the dump tool's argv:\n%s", argv)
			}
			if !strings.HasPrefix(string(argv), "--defaults-extra-file=") {
				t.Fatalf("--defaults-extra-file must come first, argv:\n%s", argv)
			}
			if !strings.Contains(string(argv), "shop") {
				t.Fatalf("the database name was not passed, argv:\n%s", argv)
			}
			left, _ := os.ReadDir(tmp)
			if len(left) != 0 {
				t.Fatalf("the temporary option file was left behind: %v", left)
			}
			if !tc.wantErr {
				got, readErr := os.ReadFile(release.Database.Backup)
				if readErr != nil || string(got) != "dump body" {
					t.Fatalf("recorded backup = %q, %v", got, readErr)
				}
			}
		})
	}
}

// A target with no dump tool has to be refused before the first mutating task,
// as a configuration error (exit 4) like Laravel and Symfony, instead of failing
// inside the maintenance window.
func TestDBBackupPreflightRefusesATargetWithoutADumpTool(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	probe := deploy.DBBackupProbe{Command: "command -v govard-no-such-dump-tool >/dev/null 2>&1", Needs: "a dump tool"}

	sc := deploy.StepContextForTest(host, deploy.Options{DBBackup: true, DBBackupProbe: probe})
	err := deploy.CheckDBBackupForTest(context.Background(), sc)
	if !errors.Is(err, deploy.ErrDBBackupUnavailable) {
		t.Fatalf("err = %v, want ErrDBBackupUnavailable", err)
	}
	if !strings.Contains(err.Error(), "--db-backup") || !strings.Contains(err.Error(), "a dump tool") {
		t.Fatalf("the refusal must name the flag and what is missing: %v", err)
	}
	if got := cli.Code(fmt.Errorf("step deploy:check failed on local: %w", err)); got != cli.CodeConfig {
		t.Fatalf("exit code = %d, want %d", got, cli.CodeConfig)
	}

	for name, opts := range map[string]deploy.Options{
		"without the flag":  {DBBackupProbe: probe},
		"with a tool there": {DBBackup: true, DBBackupProbe: deploy.DBBackupProbe{Command: "true"}},
		"with no probe":     {DBBackup: true},
	} {
		if err := deploy.CheckDBBackupForTest(context.Background(), deploy.StepContextForTest(host, opts)); err != nil {
			t.Errorf("%s: unexpected refusal: %v", name, err)
		}
	}
}

func TestWordPressBackupProbesForWpCliAndFallsBackToADumpTool(t *testing.T) {
	recipe := wordpress.DeployRecipe()
	if !strings.Contains(recipe.DBBackupProbe.Command, "command -v wp") || !strings.Contains(recipe.DBBackupProbe.Command, "mysqldump") {
		t.Fatalf("the WordPress probe must accept wp-cli or a dump tool:\n%s", recipe.DBBackupProbe.Command)
	}
	command := recipe.DBBackupCommand
	for _, want := range []string{"command -v wp", "wp db export", "--defaults-extra-file=", "wp-load.php", "DB_PASSWORD"} {
		if !strings.Contains(command, want) {
			t.Errorf("the WordPress dump lacks %q:\n%s", want, command)
		}
	}
	if strings.Contains(command, "--password") {
		t.Errorf("the WordPress fallback passes a password flag:\n%s", command)
	}
}
