package cmd

import (
	"fmt"
	"govard/internal/engine"
	"os"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

func loadConfig() engine.Config {
	wd, _ := os.Getwd()
	config, _, err := engine.LoadConfigFromDir(wd, false)
	if err != nil {
		pterm.Warning.Printf("Failed to load layered config: %v\n", err)
		return engine.Config{}
	}
	return config
}

func loadFullConfig() (engine.Config, error) {
	return loadFullConfigWithProfile("")
}

func loadFullConfigWithProfile(profile string) (engine.Config, error) {
	wd, _ := os.Getwd()
	config, _, err := engine.LoadConfigFromDirWithProfile(wd, true, profile)
	if err != nil {
		return engine.Config{}, fmt.Errorf("could not load config: %w", err)
	}
	return config, nil
}

func loadWritableConfig() (engine.Config, error) {
	wd, _ := os.Getwd()
	config, err := engine.LoadBaseConfigFromDir(wd, true)
	if err != nil {
		return engine.Config{}, fmt.Errorf("could not load %s: %w", engine.BaseConfigFile, err)
	}
	return config, nil
}

func saveConfig(config engine.Config) {
	if err := engine.ValidateConfig(config); err != nil {
		pterm.Error.Printf("Config validation failed: %v\n", err)
		return
	}
	writableConfig := engine.PrepareConfigForWrite(config)

	data, err := yaml.Marshal(&writableConfig)
	if err != nil {
		pterm.Error.Printf("Failed to marshal config: %v\n", err)
		return
	}
	err = os.WriteFile(engine.BaseConfigFile, data, 0644)
	if err != nil {
		pterm.Error.Printf("Failed to write %s: %v\n", engine.BaseConfigFile, err)
	}
}

func runUp() {
	// Call up command logic
	upCmd.Run(upCmd, []string{})
}
