package tests

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/cmd"

	"github.com/pterm/pterm"
)

// runSaveConfigCommand runs one govard command against the current directory
// and returns the pterm output and the command error. The flags of the named
// command are reset before and after, as in remoteAddEnv.add.
func runSaveConfigCommand(t *testing.T, path []string, args ...string) (string, error) {
	t.Helper()
	root := cmd.RootCommandForTest()
	command, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("find %v: %v", path, err)
	}
	resetCommandFlags(command)
	t.Cleanup(func() { resetCommandFlags(command) })

	var captured bytes.Buffer
	pterm.SetDefaultOutput(&captured)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	t.Cleanup(func() {
		root.SetOut(nil)
		root.SetErr(nil)
	})

	root.SetArgs(append(append([]string{}, path...), args...))
	runErr := root.Execute()
	return captured.String(), runErr
}

func readConfigBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestRemoteAddInvalidNameExits4AndLeavesConfigUntouched(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	before := readConfigBytes(t, env.config)

	output, err := runSaveConfigCommand(t, []string{"remote", "add"}, "bad name", "--host", "h", "--user", "u", "--path", "/p")
	if err == nil {
		t.Fatalf("expected remote add with an invalid name to fail, output:\n%s", output)
	}
	if code := cli.Code(err); code != cli.CodeConfig {
		t.Fatalf("exit code = %d, want %d (configuration): %v", code, cli.CodeConfig, err)
	}
	if strings.Contains(output, "saved") {
		t.Fatalf("a failed remote add must not report success:\n%s", output)
	}
	if after := readConfigBytes(t, env.config); !bytes.Equal(before, after) {
		t.Fatalf(".govard.yml changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestRemoteAddOutOfRangePortExits4(t *testing.T) {
	for _, port := range []string{"0", "70000", "-1"} {
		t.Run(port, func(t *testing.T) {
			env := newRemoteAddEnv(t, remoteAddFreshYAML)
			before := readConfigBytes(t, env.config)

			output, err := runSaveConfigCommand(t, []string{"remote", "add"}, "staging", "--host", "h", "--user", "u", "--path", "/p", "--port", port)
			if err == nil {
				t.Fatalf("expected port %s to be refused, output:\n%s", port, output)
			}
			if code := cli.Code(err); code != cli.CodeConfig {
				t.Fatalf("exit code = %d, want %d (configuration): %v", code, cli.CodeConfig, err)
			}
			if !strings.Contains(err.Error(), "port") {
				t.Fatalf("error should name the port: %v", err)
			}
			if after := readConfigBytes(t, env.config); !bytes.Equal(before, after) {
				t.Fatalf(".govard.yml changed:\n%s", after)
			}
		})
	}
}

// A refused remote must not leave side effects behind: the key path is stored
// in the auth store before the config is saved, so validating only at save time
// would orphan an auth-store entry for a remote that never existed.
func TestRemoteAddRefusedRemoteLeavesNoAuthStoreEntry(t *testing.T) {
	cases := map[string][]string{
		"invalid name": {"bad name", "--port", "22"},
		"bad port":     {"staging", "--port", "70000"},
	}
	for label, extra := range cases {
		t.Run(label, func(t *testing.T) {
			env := newRemoteAddEnv(t, remoteAddFreshYAML)
			authStore := os.Getenv("GOVARD_AUTH_STORE_PATH")

			args := append(extra, "--host", "h", "--user", "u", "--path", "/p",
				"--auth-method", "keychain", "--key-path", filepath.Join(filepath.Dir(env.config), "id_test"), "--force")
			output, err := runSaveConfigCommand(t, []string{"remote", "add"}, args...)
			if err == nil || cli.Code(err) != cli.CodeConfig {
				t.Fatalf("expected a configuration error, got %v\n%s", err, output)
			}
			if _, statErr := os.Stat(authStore); !os.IsNotExist(statErr) {
				data, _ := os.ReadFile(authStore)
				t.Fatalf("auth store was written for a refused remote (stat err %v):\n%s", statErr, data)
			}
		})
	}
}

func TestRemoteAddFailsWhenTheConfigCannotBeWritten(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("a read-only file is still writable for root")
	}
	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	if err := os.Chmod(env.config, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(env.config, 0o644) })

	output, err := runSaveConfigCommand(t, []string{"remote", "add"}, "staging", "--host", "h", "--user", "u", "--path", "/p")
	if err == nil {
		t.Fatalf("expected remote add to fail when the file is read-only, output:\n%s", output)
	}
	if cli.Code(err) == cli.CodeConfig {
		t.Fatalf("an IO failure must not be reported as a configuration error: %v", err)
	}
	if strings.Contains(output, "saved") {
		t.Fatalf("a failed save must not print success:\n%s", output)
	}
}

func TestConfigSetInvalidValueExits4AndLeavesConfigUntouched(t *testing.T) {
	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	before := readConfigBytes(t, env.config)

	output, err := runSaveConfigCommand(t, []string{"config", "set"}, "table_prefix", "bad prefix!")
	if err == nil {
		t.Fatalf("expected config set with an invalid value to fail, output:\n%s", output)
	}
	if code := cli.Code(err); code != cli.CodeConfig {
		t.Fatalf("exit code = %d, want %d (configuration): %v", code, cli.CodeConfig, err)
	}
	if after := readConfigBytes(t, env.config); !bytes.Equal(before, after) {
		t.Fatalf(".govard.yml changed:\n%s", after)
	}
}

func TestDomainAddFailsWhenTheConfigCannotBeWritten(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("a read-only file is still writable for root")
	}
	env := newRemoteAddEnv(t, remoteAddFreshYAML)
	if err := os.Chmod(env.config, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(env.config, 0o644) })

	output, err := runSaveConfigCommand(t, []string{"domain", "add"}, "extra.test")
	if err == nil {
		t.Fatalf("expected domain add to fail when the file is read-only, output:\n%s", output)
	}
	var configErr *cli.ConfigError
	if errors.As(err, &configErr) {
		t.Fatalf("an IO failure must not be a ConfigError: %v", err)
	}
	if strings.Contains(output, "added to .govard.yml") {
		t.Fatalf("a failed save must not print success:\n%s", output)
	}
}
