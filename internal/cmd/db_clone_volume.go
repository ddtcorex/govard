package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"govard/internal/engine"
)

func runDBCloneVolume(cmd *cobra.Command, config engine.Config, options dbCommandOptions, extraArgs []string) error {
	if options.Environment != "local" {
		return fmt.Errorf("clone-volume is currently only supported for local environments")
	}

	if len(extraArgs) < 1 {
		return fmt.Errorf("source volume name is required")
	}
	sourceVolume := strings.TrimSpace(extraArgs[0])

	// Inspect if source volume exists
	checkCmd := exec.Command("docker", "volume", "inspect", sourceVolume)
	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("source volume '%s' does not exist or Docker is inaccessible: %w", sourceVolume, err)
	}

	targetVolume := fmt.Sprintf("%s_db-data", config.ProjectName)
	if config.Profile != "" {
		targetVolume = fmt.Sprintf("%s_db-data-%s", config.ProjectName, config.Profile)
	}

	if sourceVolume == targetVolume {
		return fmt.Errorf("source and target volumes are the same")
	}

	// Check if db container is running
	dbContainer := dbContainerName(config)
	if err := ensureLocalDBRunning(dbContainer); err == nil {
		// It is running
		pterm.Warning.Printf("Database container %s is currently running.\n", dbContainer)
		if !options.AssumeYes {
			proceed, err := confirmDestructive("Stop the database container and proceed with cloning?", "--yes")
			if err != nil {
				return err
			}
			if !proceed {
				return fmt.Errorf("aborted by user")
			}
		}

		pterm.Info.Println("Stopping database container...")
		dockerStop := exec.Command("docker", "stop", dbContainer)
		if err := dockerStop.Run(); err != nil {
			return fmt.Errorf("failed to stop database container: %w", err)
		}
	}

	if !options.AssumeYes {
		proceed, err := confirmDestructive(fmt.Sprintf("Clone all data from '%s' into '%s' (this replaces existing target data)?", sourceVolume, targetVolume), "--yes")
		if err != nil {
			return err
		}
		if !proceed {
			return fmt.Errorf("aborted by user")
		}
	}

	pterm.Info.Printf("Cloning data from '%s' to '%s'...\n", sourceVolume, targetVolume)

	checkTarget := exec.Command("docker", "volume", "inspect", targetVolume)
	if err := checkTarget.Run(); err != nil {
		composeVolumeName := "db-data"
		if config.Profile != "" {
			composeVolumeName = fmt.Sprintf("db-data-%s", config.Profile)
		}
		createVol := exec.Command("docker", "volume", "create",
			"--name", targetVolume,
			"--label", fmt.Sprintf("com.docker.compose.project=%s", config.ProjectName),
			"--label", fmt.Sprintf("com.docker.compose.volume=%s", composeVolumeName),
		)
		_ = createVol.Run()
	}

	spinner, _ := pterm.DefaultSpinner.Start("Copying raw volume files (this may take a moment)")

	if err := engine.CloneVolume(sourceVolume, targetVolume); err != nil {
		spinner.Fail("Volume clone failed")
		return err
	}

	spinner.Success("Volume successfully cloned!")
	pterm.Info.Println("You can now start your environment using 'govard env up'")

	return nil
}
