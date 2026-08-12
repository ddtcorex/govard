package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestToolNPMRunsInConfiguredNodeContainer(t *testing.T) {
	projectRoot := t.TempDir()
	chdirForTest(t, projectRoot)
	writeRuntimeConfig(t, projectRoot, `project_name: sample-project
domain: sample.test
framework: magento2
stack:
  node_version: "24"
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "docker.log")
	installNodeToolDockerShim(t, shimDir)
	t.Setenv("NODE_TOOL_DOCKER_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := cmd.RootCommandForTest()
	root.SetArgs([]string{"tool", "npm", "--prefix", "app/design/frontend/Acme/Store/web/tailwind", "run", "build"})
	if err := root.Execute(); err != nil {
		t.Fatalf("run govard tool npm: %v", err)
	}

	logs := readRuntimeLog(t, logPath)
	want := fmt.Sprintf(
		"docker|run --rm -i --user %d:%d -v %s:/var/www/html -w /var/www/html node:24-alpine npm --prefix app/design/frontend/Acme/Store/web/tailwind run build",
		os.Getuid(), os.Getgid(), projectRoot,
	)
	if !strings.Contains(logs, want) {
		t.Fatalf("npm must use the configured one-shot Node runtime:\nwant: %s\nlogs:\n%s", want, logs)
	}
}

func TestOtherNodeToolsUseConfiguredNodeContainer(t *testing.T) {
	projectRoot := t.TempDir()
	chdirForTest(t, projectRoot)
	writeRuntimeConfig(t, projectRoot, `project_name: sample-project
domain: sample.test
framework: magento2
stack:
  node_version: "24"
`)

	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "docker.log")
	installNodeToolDockerShim(t, shimDir)
	t.Setenv("NODE_TOOL_DOCKER_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, tool := range []struct {
		name string
		want string
	}{
		{name: "npx", want: "node:24-alpine npx --version"},
		{name: "yarn", want: "node:24-alpine yarn --version"},
		{name: "pnpm", want: "node:24-alpine corepack pnpm --version"},
		{name: "grunt", want: "node:24-alpine npx grunt --version"},
	} {
		t.Run(tool.name, func(t *testing.T) {
			root := cmd.RootCommandForTest()
			root.SetArgs([]string{"tool", tool.name, "--version"})
			if err := root.Execute(); err != nil {
				t.Fatalf("run govard tool %s: %v", tool.name, err)
			}

			if logs := readRuntimeLog(t, logPath); !strings.Contains(logs, tool.want) {
				t.Fatalf("%s must use the configured Node runtime %q:\n%s", tool.name, tool.want, logs)
			}
		})
	}
}

func TestToolNodeCommandsExecIntoOwnContainerForNodeRuntimeFrameworks(t *testing.T) {
	for _, framework := range []string{"nextjs", "emdash"} {
		t.Run(framework, func(t *testing.T) {
			projectRoot := t.TempDir()
			chdirForTest(t, projectRoot)
			writeRuntimeConfig(t, projectRoot, fmt.Sprintf(`project_name: sample-project
domain: sample.test
framework: %s
`, framework))

			shimDir := t.TempDir()
			logPath := filepath.Join(shimDir, "docker.log")
			installNodeToolDockerShim(t, shimDir)
			t.Setenv("NODE_TOOL_DOCKER_LOG", logPath)
			t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			root := cmd.RootCommandForTest()
			root.SetArgs([]string{"tool", "npm", "--version"})
			if err := root.Execute(); err != nil {
				t.Fatalf("run govard tool npm: %v", err)
			}

			logs := readRuntimeLog(t, logPath)
			want := "docker|exec -i -w /app sample-project-web-1 npm --version"
			if !strings.Contains(logs, want) {
				t.Fatalf("npm must exec into the %s application container, not a one-shot Node image:\nwant: %s\nlogs:\n%s", framework, want, logs)
			}
			if strings.Contains(logs, "node:") {
				t.Fatalf("npm must not fall back to a one-shot Node image for a Node-runtime framework:\n%s", logs)
			}
		})
	}
}

func installNodeToolDockerShim(t *testing.T, shimDir string) {
	t.Helper()
	script := `#!/bin/sh
set -eu
if [ -n "${NODE_TOOL_DOCKER_LOG:-}" ]; then
  printf 'docker|%s\n' "$*" >> "$NODE_TOOL_DOCKER_LOG"
fi
`
	path := filepath.Join(shimDir, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write docker shim: %v", err)
	}
}
