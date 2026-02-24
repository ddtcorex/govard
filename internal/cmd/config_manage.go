package cmd

import (
	"fmt"
	"govard/internal/engine"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage govard.yml configuration from CLI",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Read a config value from govard.yml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config := loadFullConfig()
		value, ok := getConfigValue(config, args[0])
		if !ok {
			return fmt.Errorf("unknown config key: %s", args[0])
		}
		_, err := io.WriteString(cmd.OutOrStdout(), value+"\n")
		return err
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Write a config value into govard.yml",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		config := loadWritableConfig()
		key, value := args[0], args[1]
		if !setConfigValue(&config, key, value) {
			return fmt.Errorf("unknown config key: %s", key)
		}
		engine.NormalizeConfig(&config)
		saveConfig(config)
		_, err := io.WriteString(cmd.OutOrStdout(), fmt.Sprintf("Config updated: %s = %s\n", key, value))
		return err
	},
}

func getConfigValue(config engine.Config, key string) (string, bool) {
	// Simple key mapping for common fields
	switch strings.ToLower(key) {
	case "project_name":
		return config.ProjectName, true
	case "domain":
		return config.Domain, true
	case "framework_version":
		return config.FrameworkVersion, true
	case "php_version":
		return config.Stack.PHPVersion, true
	case "db_type":
		return config.Stack.DBType, true
	case "services.web_server", "web_server":
		return config.Stack.Services.WebServer, true
	case "services.search", "search":
		return config.Stack.Services.Search, true
	case "services.cache", "cache":
		return config.Stack.Services.Cache, true
	}
	return "", false
}

func setConfigValue(config *engine.Config, key string, value string) bool {
	switch strings.ToLower(key) {
	case "project_name":
		config.ProjectName = value
	case "domain":
		config.Domain = value
	case "framework_version":
		config.FrameworkVersion = value
	case "php_version":
		config.Stack.PHPVersion = value
	case "services.web_server", "web_server":
		config.Stack.Services.WebServer = value
	case "services.search", "search":
		config.Stack.Services.Search = value
	case "services.cache", "cache":
		config.Stack.Services.Cache = value
	default:
		return false
	}
	return true
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}
