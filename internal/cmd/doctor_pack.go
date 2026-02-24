package cmd

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

type doctorPackEnvironment struct {
	Timestamp    string `json:"timestamp"`
	Govard       string `json:"govard_version"`
	GoVersion    string `json:"go_version"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	WorkingDir   string `json:"working_dir"`
}

func defaultDoctorPackDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".", ".govard", "diagnostics")
	}
	return filepath.Join(home, ".govard", "diagnostics")
}

// CreateDoctorDiagnosticsPack writes a diagnostics pack zip and returns its path.
func CreateDoctorDiagnosticsPack(outputDir string, cwd string, report engine.DoctorReport) (string, error) {
	if strings.TrimSpace(outputDir) == "" {
		outputDir = defaultDoctorPackDir()
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return "", fmt.Errorf("create doctor output dir: %w", err)
	}

	now := time.Now().UTC()
	packName := "doctor-pack-" + now.Format("20060102-150405")
	packDir := filepath.Join(outputDir, packName)
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		return "", fmt.Errorf("create doctor pack dir: %w", err)
	}

	reportPath := filepath.Join(packDir, "doctor_report.json")
	reportPayload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal doctor report: %w", err)
	}
	if err := os.WriteFile(reportPath, reportPayload, 0o600); err != nil {
		return "", fmt.Errorf("write doctor report: %w", err)
	}

	envPath := filepath.Join(packDir, "environment.json")
	envPayload, err := json.MarshalIndent(doctorPackEnvironment{
		Timestamp:    now.Format(time.RFC3339),
		Govard:       Version,
		GoVersion:    runtime.Version(),
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		WorkingDir:   cwd,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal doctor environment: %w", err)
	}
	if err := os.WriteFile(envPath, envPayload, 0o600); err != nil {
		return "", fmt.Errorf("write doctor environment: %w", err)
	}

	if err := writeConfigLayersSnapshot(filepath.Join(packDir, "config_layers.txt"), cwd); err != nil {
		return "", err
	}
	if err := writeRuntimeCommandSnapshots(filepath.Join(packDir, "runtime_commands.txt")); err != nil {
		return "", err
	}

	_ = copyIfExists(filepath.Join(cwd, engine.BaseConfigFile), filepath.Join(packDir, engine.BaseConfigFile))
	composeProjectName := ""
	if cfg, _, cfgErr := engine.LoadConfigFromDir(cwd, false); cfgErr == nil {
		composeProjectName = cfg.ProjectName
	}
	_ = copyIfExists(engine.ComposeFilePath(cwd, composeProjectName), filepath.Join(packDir, "govard-compose.yml"))
	_ = copyIfExists(remote.AuditLogPath(), filepath.Join(packDir, "remote-audit.log"))

	readme := []byte(
		"Govard diagnostics pack\n" +
			"- doctor_report.json: structured diagnostics report\n" +
			"- environment.json: runtime environment metadata\n" +
			"- config_layers.txt: resolved layered config file list\n" +
			"- runtime_commands.txt: docker/proxy command snapshots (best effort)\n" +
			"- govard.yml / govard-compose.yml: optional project snapshots\n" +
			"- remote-audit.log: optional remote audit history snapshot\n",
	)
	if err := os.WriteFile(filepath.Join(packDir, "README.txt"), readme, 0o600); err != nil {
		return "", fmt.Errorf("write doctor readme: %w", err)
	}

	zipPath := packDir + ".zip"
	if err := zipDirectory(packDir, zipPath); err != nil {
		return "", err
	}
	if err := os.RemoveAll(packDir); err != nil {
		return "", fmt.Errorf("cleanup pack directory: %w", err)
	}
	return zipPath, nil
}

func writeConfigLayersSnapshot(path string, cwd string) error {
	_, loaded, err := engine.LoadConfigFromDir(cwd, false)
	content := strings.Builder{}
	if err != nil {
		content.WriteString("failed to resolve config layers: ")
		content.WriteString(err.Error())
		content.WriteString("\n")
	} else if len(loaded) == 0 {
		content.WriteString("no config layers loaded\n")
	} else {
		content.WriteString("loaded config layers:\n")
		for _, layer := range loaded {
			content.WriteString("- ")
			content.WriteString(layer)
			content.WriteString("\n")
		}
	}
	if writeErr := os.WriteFile(path, []byte(content.String()), 0o600); writeErr != nil {
		return fmt.Errorf("write config layers snapshot: %w", writeErr)
	}
	return nil
}

func writeRuntimeCommandSnapshots(path string) error {
	builder := strings.Builder{}
	builder.WriteString("runtime command snapshots\n")
	builder.WriteString("generated_at: " + time.Now().UTC().Format(time.RFC3339) + "\n\n")

	if _, err := exec.LookPath("docker"); err != nil {
		builder.WriteString("docker CLI not found in PATH\n")
		return os.WriteFile(path, []byte(builder.String()), 0o600)
	}

	commands := []struct {
		title string
		args  []string
	}{
		{title: "docker version", args: []string{"docker", "version"}},
		{title: "docker compose version", args: []string{"docker", "compose", "version"}},
		{title: "docker ps (govard/proxy related)", args: []string{"docker", "ps", "--format", "{{.Names}}\t{{.Status}}"}},
		{title: "docker logs proxy-caddy-1", args: []string{"docker", "logs", "--tail", "200", "proxy-caddy-1"}},
		{title: "docker logs govard-proxy-caddy", args: []string{"docker", "logs", "--tail", "200", "govard-proxy-caddy"}},
	}

	for _, command := range commands {
		builder.WriteString("## " + command.title + "\n")
		builder.WriteString("$ " + strings.Join(command.args, " ") + "\n")
		output, err := runCommandCapture(command.args...)
		if err != nil {
			builder.WriteString("error: " + err.Error() + "\n")
		}
		trimmed := strings.TrimSpace(output)
		if trimmed == "" {
			builder.WriteString("(no output)\n")
		} else {
			builder.WriteString(trimmed + "\n")
		}
		builder.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(builder.String()), 0o600); err != nil {
		return fmt.Errorf("write runtime command snapshots: %w", err)
	}
	return nil
}

func runCommandCapture(args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	command := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("command timed out")
	}
	return string(output), err
}

func copyIfExists(source string, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return nil
	}

	target, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer target.Close()

	if _, err := io.Copy(target, file); err != nil {
		return err
	}
	return nil
}

func zipDirectory(sourceDir string, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("create diagnostics zip: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	err = filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)

		sourceFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer sourceFile.Close()

		writer, err := archive.Create(relative)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, sourceFile)
		return err
	})
	if err != nil {
		return fmt.Errorf("zip diagnostics pack: %w", err)
	}
	return nil
}
