package bootstrap

import (
	"fmt"
	"govard/internal/conventions"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

func removeProjectContents(projectDir string) error {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read project directory: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == ".govard" || entry.Name() == ".govard.yml" {
			continue
		}
		if err := removeDirWithFallback(projectDir, entry.Name()); err != nil {
			return err
		}
	}

	return nil
}

// removeDirWithFallback removes projectDir/name, falling back to a
// throwaway alpine container running as root when the host user lacks
// permission to delete some of its contents - e.g. node_modules written
// by a root-running throwaway container (see nodeCreateProjectRunner in
// internal/cmd/bootstrap.go, which has no -u/--user mapping).
func removeDirWithFallback(projectDir string, name string) error {
	targetPath := filepath.Join(projectDir, name)
	if err := os.RemoveAll(targetPath); err == nil {
		return nil
	} else if _, statErr := os.Stat(targetPath); os.IsNotExist(statErr) {
		return nil
	} else {
		cmd := exec.Command("docker", "run", "--rm", "-v", projectDir+":/workspace", "-w", "/workspace", "alpine", "rm", "-rf", name)
		if runErr := cmd.Run(); runErr != nil {
			return fmt.Errorf("failed to remove %s (fallback failed: %v): %w", name, runErr, err)
		}
		return nil
	}
}

// containerBaseDir is the project's mount point inside whichever service
// container `runner` execs into - conventions.DefaultWorkDir for PHP
// frameworks, conventions.NodeWorkDir for Node-based ones (nextjs).
func runStagedCreateProject(projectDir string, runner func(command string) error, createInStage func(stageDir string) error, runnerCommand string, containerBaseDir string) error {
	if err := removeProjectContents(projectDir); err != nil {
		return err
	}

	stageDir, err := os.MkdirTemp(projectDir, "govard-create-project-")
	if err != nil {
		return fmt.Errorf("create staged project directory: %w", err)
	}
	defer func() {
		if err := removeDirWithFallback(projectDir, filepath.Base(stageDir)); err != nil {
			pterm.Warning.Printf("Could not remove staging directory %s: %v\n", stageDir, err)
		}
	}()

	if runner != nil {
		if strings.TrimSpace(runnerCommand) == "" {
			return fmt.Errorf("runner command is required for staged project creation")
		}
		if err := runner(buildStagedRunnerCommand(projectDir, stageDir, runnerCommand, containerBaseDir)); err != nil {
			return err
		}
	} else {
		if createInStage == nil {
			return fmt.Errorf("staged project creator is required")
		}
		if err := createInStage(stageDir); err != nil {
			return err
		}
	}

	if err := syncStagedProject(stageDir, projectDir); err != nil {
		return err
	}

	return nil
}

func buildStagedRunnerCommand(projectDir, stageDir, runnerCommand string, containerBaseDir string) string {
	containerStageDir := containerBaseDir
	if relStageDir, err := filepath.Rel(projectDir, stageDir); err == nil && relStageDir != "." {
		containerStageDir = path.Join(containerStageDir, filepath.ToSlash(relStageDir))
	}

	return fmt.Sprintf(
		"export GOVARD_STAGE_DIR=%s GOVARD_STAGE_HOST_DIR=%s; %s",
		conventions.ShellQuote(containerStageDir),
		conventions.ShellQuote(stageDir),
		runnerCommand,
	)
}

func syncStagedProject(stageDir, projectDir string) error {
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return fmt.Errorf("read staged project directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(stageDir, entry.Name())
		dstPath := filepath.Join(projectDir, entry.Name())
		if err := copyProjectPath(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}

func copyProjectPath(srcPath, dstPath string) error {
	info, err := os.Lstat(srcPath)
	if err != nil {
		return fmt.Errorf("stat staged path %s: %w", srcPath, err)
	}

	switch mode := info.Mode(); {
	case mode.IsDir():
		if err := os.MkdirAll(dstPath, mode.Perm()); err != nil {
			return fmt.Errorf("create project directory %s: %w", dstPath, err)
		}
		entries, err := os.ReadDir(srcPath)
		if err != nil {
			return fmt.Errorf("read staged directory %s: %w", srcPath, err)
		}
		for _, entry := range entries {
			if err := copyProjectPath(filepath.Join(srcPath, entry.Name()), filepath.Join(dstPath, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	case mode&os.ModeSymlink != 0:
		target, err := os.Readlink(srcPath)
		if err != nil {
			return fmt.Errorf("read staged symlink %s: %w", srcPath, err)
		}
		if err := os.Symlink(target, dstPath); err != nil {
			return fmt.Errorf("create project symlink %s: %w", dstPath, err)
		}
		return nil
	default:
		return copyProjectFile(srcPath, dstPath, info.Mode().Perm())
	}
}

func copyProjectFile(srcPath, dstPath string, mode os.FileMode) (err error) {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open staged file %s: %w", srcPath, err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dstPath), conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create project parent directory %s: %w", filepath.Dir(dstPath), err)
	}

	dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create project file %s: %w", dstPath, err)
	}
	defer func() {
		if cerr := dstFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy staged file %s: %w", srcPath, err)
	}

	return nil
}

func runComposerProjectCommand(projectDir string, runner func(command string) error, args ...string) error {
	if runner != nil {
		commandArgs := make([]string, len(args))
		for i, arg := range args {
			commandArgs[i] = conventions.ShellQuote(arg)
		}
		return runner("composer " + strings.Join(commandArgs, " "))
	}

	cmd := exec.Command("composer", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runPHPProjectScript(projectDir string, runner func(command string) error, scriptPath string, args ...string) error {
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return nil
	}

	if runner != nil {
		runnerScriptPath := scriptPath
		if relScriptPath, err := filepath.Rel(projectDir, scriptPath); err == nil && !strings.HasPrefix(relScriptPath, "..") {
			runnerScriptPath = path.Join(conventions.DefaultWorkDir, filepath.ToSlash(relScriptPath))
		}

		commandArgs := []string{"php", conventions.ShellQuote(runnerScriptPath)}
		for _, arg := range args {
			commandArgs = append(commandArgs, conventions.ShellQuote(arg))
		}
		return runner(strings.Join(commandArgs, " "))
	}

	cmd := exec.Command("php", append([]string{scriptPath}, args...)...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runPHPOneLiner(projectDir string, runner func(command string) error, code string) error {
	if runner != nil {
		return runner("php -r " + conventions.ShellQuote(code))
	}

	cmd := exec.Command("php", "-r", code)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func RunStagedCreateProjectForTest(projectDir string, runner func(command string) error, createInStage func(stageDir string) error, runnerCommand string) error {
	return runStagedCreateProject(projectDir, runner, createInStage, runnerCommand, conventions.DefaultWorkDir)
}
