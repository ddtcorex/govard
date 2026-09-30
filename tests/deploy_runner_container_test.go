package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

// A third implementation of the same seam has to be the same seam.
var _ deploy.Runner = deploy.ContainerRunner{}

// containerRunnerFixture is the shape a project supplies: the service container
// govard execs into, the account it execs as, and the two roots that map this
// machine's project tree onto the one inside that container.
func containerRunnerFixture() deploy.ContainerRunner {
	return deploy.ContainerRunner{
		Container:     "sample-php-1",
		User:          "www-data",
		LocalRoot:     "/home/u/sample",
		ContainerRoot: "/var/www/html",
	}
}

func TestContainerRunnerBuildsTheDockerExecArgv(t *testing.T) {
	runner := containerRunnerFixture()
	noUser := containerRunnerFixture()
	noUser.User = ""
	sibling := containerRunnerFixture()
	sibling.LocalRoot = "/srv/app"
	noRoot := containerRunnerFixture()
	noRoot.LocalRoot = ""

	cases := []struct {
		name    string
		runner  deploy.ContainerRunner
		workdir string
		command string
		want    []string
	}{
		{
			// The shape the recipe's own commands take: `git -C '<checkout>'` and
			// `tar -x -C '<output>'` name host paths, and the step runs where the
			// artifact is rather than where the project is.
			name:    "the local project root is rewritten to the container root",
			runner:  runner,
			workdir: "/var/www/html/artifacts",
			command: "cd /home/u/sample/artifacts && php -v",
			want: []string{
				"exec", "-u", "www-data", "-w", "/var/www/html/artifacts",
				"sample-php-1", "sh", "-c", "cd /var/www/html/artifacts && php -v",
			},
		},
		{
			name:    "an empty user leaves the container's own account in place",
			runner:  noUser,
			workdir: "/var/www/html",
			command: "git -C /home/u/sample rev-parse HEAD",
			want: []string{
				"exec", "-w", "/var/www/html",
				"sample-php-1", "sh", "-c", "git -C /var/www/html rev-parse HEAD",
			},
		},
		{
			// Every generated build command opens with the artifact directory, and
			// `Run` maps the caller's RunOptions.Dir before it gets here, so an
			// empty workdir means the project root inside the container.
			name:    "an empty workdir is the container root",
			runner:  runner,
			command: "composer install --no-dev",
			want: []string{
				"exec", "-u", "www-data", "-w", "/var/www/html",
				"sample-php-1", "sh", "-c", "composer install --no-dev",
			},
		},
		{
			// A sibling checkout whose name merely starts with the project root's
			// is a different directory. Rewriting it would build the wrong tree
			// and report success.
			name:    "a sibling directory that only shares a prefix is left alone",
			runner:  sibling,
			workdir: "/var/www/html",
			command: "git -C /srv/app-old rev-parse HEAD && cd /srv/app/vendor && php -v",
			want: []string{
				"exec", "-u", "www-data", "-w", "/var/www/html",
				"sample-php-1", "sh", "-c", "git -C /srv/app-old rev-parse HEAD && cd /var/www/html/vendor && php -v",
			},
		},
		{
			// A project root that matches everywhere is worse than no rewrite at
			// all: an unset one would otherwise insert the container root between
			// every character of every command.
			name:    "a runner with no local root rewrites nothing",
			runner:  noRoot,
			workdir: "/var/www/html",
			command: "cd /home/u/sample && php -v",
			want: []string{
				"exec", "-u", "www-data", "-w", "/var/www/html",
				"sample-php-1", "sh", "-c", "cd /home/u/sample && php -v",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			args, err := testCase.runner.Args(testCase.workdir, testCase.command)
			if err != nil {
				t.Fatalf("Args: %v", err)
			}
			if !slices.Equal(args, testCase.want) {
				t.Fatalf("args = %q, want %q", args, testCase.want)
			}
		})
	}
}

func TestContainerRunnerMapsPathsInsideTheProjectRoot(t *testing.T) {
	runner := containerRunnerFixture()

	cases := []struct {
		name  string
		local string
		want  string
	}{
		{name: "an empty path is the project root itself", local: "", want: "/var/www/html"},
		{name: "the project root maps onto the container root", local: "/home/u/sample", want: "/var/www/html"},
		{name: "a path below the root keeps its shape", local: "/home/u/sample/app", want: "/var/www/html/app"},
		{name: "a trailing separator is not part of the path", local: "/home/u/sample/app/", want: "/var/www/html/app"},
		{name: "an unclean path is cleaned first", local: "/home/u/sample/./app/../app", want: "/var/www/html/app"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := runner.ContainerPath(testCase.local)
			if err != nil {
				t.Fatalf("ContainerPath(%q): %v", testCase.local, err)
			}
			if got != testCase.want {
				t.Fatalf("ContainerPath(%q) = %q, want %q", testCase.local, got, testCase.want)
			}
		})
	}
}

func TestContainerRunnerRefusesAPathItCannotMap(t *testing.T) {
	runner := containerRunnerFixture()

	cases := []struct {
		name  string
		local string
	}{
		{name: "a path outside the project root", local: "/tmp/elsewhere"},
		{name: "a sibling that only shares a prefix", local: "/home/u/sample-old"},
		{name: "the parent of the project root", local: "/home/u"},
		{
			// LocalRunner hands a relative directory to the OS, which resolves it
			// against govard's own working directory. Resolving it against the
			// project root here would silently be a different rule.
			name:  "a relative path",
			local: "artifacts",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := runner.ContainerPath(testCase.local)
			if !errors.Is(err, deploy.ErrContainerPathUnmappable) {
				t.Fatalf("ContainerPath(%q) err = %v, want ErrContainerPathUnmappable", testCase.local, err)
			}
			if got != "" {
				t.Fatalf("ContainerPath(%q) = %q, want no path at all: a refused mapping must not fall back", testCase.local, got)
			}
			// The message has to name the remedy: a refusal the operator cannot
			// act on is the same as a silent fallback.
			for _, want := range []string{testCase.local, runner.LocalRoot, "--output"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("message %q must name %q", err.Error(), want)
				}
			}
		})
	}
}

func TestContainerRunnerRefusesToMapWithoutARoot(t *testing.T) {
	// Half-configured is not a mapping: with no local root to measure from, and
	// with no container root to map into, every answer would be a guess.
	noLocal := containerRunnerFixture()
	noLocal.LocalRoot = ""
	if got, err := noLocal.ContainerPath("/var/www/html"); !errors.Is(err, deploy.ErrContainerPathUnmappable) || got != "" {
		t.Fatalf("ContainerPath = (%q, %v), want a refusal with no path", got, err)
	}

	noContainer := containerRunnerFixture()
	noContainer.ContainerRoot = ""
	if got, err := noContainer.ContainerPath(""); !errors.Is(err, deploy.ErrContainerPathUnmappable) || got != "" {
		t.Fatalf("ContainerPath = (%q, %v), want a refusal with no path", got, err)
	}
}

func TestContainerRunnerRefusesBeforeItReachesDocker(t *testing.T) {
	// The control case matters as much as the refusals: a stub that cannot be
	// reached would make every "docker was not called" assertion below pass for
	// the wrong reason.
	cases := []struct {
		name        string
		command     string
		opts        deploy.RunOptions
		wantErr     error
		wantReached bool
	}{
		{
			name:        "a mappable directory reaches docker",
			command:     "php -v",
			opts:        deploy.RunOptions{Dir: "/home/u/sample/artifacts"},
			wantReached: true,
		},
		{
			name:    "a directory outside the project root is refused first",
			command: "php -v",
			opts:    deploy.RunOptions{Dir: "/tmp/elsewhere"},
			wantErr: deploy.ErrContainerPathUnmappable,
		},
		{
			name:    "stdin is refused first",
			command: "php -v",
			opts:    deploy.RunOptions{Stdin: "a private key"},
			wantErr: deploy.ErrContainerStdinUnsupported,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			argvFile := dockerStub(t, "")
			_, err := containerRunnerFixture().Run(context.Background(), testCase.command, testCase.opts)

			if testCase.wantErr == nil {
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
			} else {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("err = %v, want %v", err, testCase.wantErr)
				}
				var commandErr *deploy.CommandError
				if errors.As(err, &commandErr) {
					t.Fatalf("err = %v, want the refusal itself, not a failed command", err)
				}
			}

			got := dockerArgv(t, argvFile)
			if !testCase.wantReached {
				if len(got) > 0 {
					t.Fatalf("docker was called with %q, want no call at all", got)
				}
				return
			}
			want := []string{
				"exec", "-u", "www-data", "-w", "/var/www/html/artifacts",
				"sample-php-1", "sh", "-c", "php -v",
			}
			if !slices.Equal(got, want) {
				t.Fatalf("docker argv = %q, want %q", got, want)
			}
		})
	}
}

func TestContainerRunnerBoundsWhatItKeepsOfAChattyStep(t *testing.T) {
	// 300 KiB on stderr, past the 256 KiB the engine keeps. The tail is the part
	// that explains a failure, so it is the part that has to survive.
	argvFile := dockerStub(t, "dd if=/dev/zero bs=1024 count=300 2>/dev/null | tr '\\0' 'e' >&2\n")

	result, err := containerRunnerFixture().Run(context.Background(), "bin/magento setup:di:compile", deploy.RunOptions{Dir: "/home/u/sample"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(dockerArgv(t, argvFile)) == 0 {
		t.Fatal("the stub was never called")
	}
	if !strings.Contains(result.Stderr, "dropped") {
		t.Fatalf("stderr must say how much was dropped, got %d bytes", len(result.Stderr))
	}
	if !strings.HasSuffix(result.Stderr, strings.Repeat("e", 16)) {
		t.Fatalf("stderr must keep the tail of the output, got %q", result.Stderr[len(result.Stderr)-32:])
	}
	if len(result.Stderr) >= 300*1024 {
		t.Fatalf("stderr is %d bytes; the capture is not bounded", len(result.Stderr))
	}
}

func TestContainerRunnerHonoursRunOptionsTimeout(t *testing.T) {
	argvFile := dockerStub(t, "sleep 30\n")
	start := time.Now()

	_, err := containerRunnerFixture().Run(context.Background(), "composer install", deploy.RunOptions{Timeout: 300 * time.Millisecond})
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout took %s; the runner must kill the client, not wait for it", elapsed)
	}
	var commandErr *deploy.CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("err = %v, want *deploy.CommandError", err)
	}
	if !strings.Contains(commandErr.Error(), "timed out") {
		t.Fatalf("error must name the timeout, got %q", commandErr.Error())
	}
	if !strings.Contains(commandErr.Command, "composer install") {
		t.Fatalf("the operator is shown %q, want the command they wrote", commandErr.Command)
	}
	if len(dockerArgv(t, argvFile)) == 0 {
		t.Fatal("the stub was never called")
	}
}

// dockerStub puts a fake `docker` first on PATH and returns the file it appends
// its argv to. The body runs after the argv is recorded, so a test can both
// prove the runner reached docker and change what docker does.
func dockerStub(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$GOVARD_TEST_DOCKER_ARGV\"\n" + body
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the docker stub: %v", err)
	}
	t.Setenv("GOVARD_TEST_DOCKER_ARGV", argvFile)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvFile
}

// dockerArgv is what the stub recorded, and empty when it never ran.
func dockerArgv(t *testing.T, argvFile string) []string {
	t.Helper()
	recorded, err := os.ReadFile(argvFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read the recorded docker argv: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(recorded), "\n"), "\n")
}
