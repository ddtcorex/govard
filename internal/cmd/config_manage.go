package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"govard/internal/engine"

	"github.com/spf13/cobra"
	"govard/internal/runtime"
)

var configCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:     "config",
	Aliases: []string{"cfg"},
	Short:   "Manage .govard.yml configuration from CLI",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Read a config value from .govard.yml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadFullConfig()
		if err != nil {

			return err
		}
		value, ok := getConfigValue(config, args[0])
		if !ok {
			return fmt.Errorf("unknown config key: %s", args[0])
		}
		_, err = io.WriteString(cmd.OutOrStdout(), value+"\n")
		return err
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Write a config value into .govard.yml",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadWritableConfig()
		if err != nil {
			return err
		}
		key, value := args[0], args[1]
		ok, err := setConfigValue(&config, key, value)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("unknown config key: %s", key)
		}
		wd, _ := os.Getwd()
		engine.NormalizeConfig(&config, wd)
		saveConfig(config)
		_, err = io.WriteString(cmd.OutOrStdout(), fmt.Sprintf("Config updated: %s = %s\n", key, value))
		return err
	},
}

func getConfigValue(config engine.Config, key string) (string, bool) {
	// The deploy block answers before the fixed-key switch: it carries a
	// free-form settings map, so a new recipe setting must not also have to be
	// listed here. `deploy` alone is reported as an unknown key on purpose —
	// the block is not a single value, and printing a struct would be worse than
	// naming the keys that exist.
	if rest, isDeploy := strings.CutPrefix(strings.ToLower(key), "deploy."); isDeploy {
		return getDeployValue(config.Deploy, rest)
	}

	// Simple key mapping for common fields
	switch strings.ToLower(key) {
	case "project_name":
		return config.ProjectName, true
	case "framework":
		return config.Framework, true
	case "domain":
		return config.Domain, true
	case "framework_version":
		return config.FrameworkVersion, true
	case "table_prefix":
		return config.TablePrefix, true
	case "php_version", "stack.php_version":
		return config.Stack.PHPVersion, true
	case "python_version", "stack.python_version":
		return config.Stack.PythonVersion, true
	case "node_version", "stack.node_version":
		return config.Stack.NodeVersion, true
	case "db", "services.db", "stack.services.db":
		return config.Stack.Services.DB, true
	case "db_version", "stack.db_version":
		return config.Stack.DBVersion, true
	case "services.web_server", "web_server", "stack.services.web_server":
		return config.Stack.Services.WebServer, true
	case "services.search", "search", "stack.services.search":
		return config.Stack.Services.Search, true
	case "services.cache", "cache", "stack.services.cache":
		return config.Stack.Services.Cache, true
	case "services.queue", "queue", "stack.services.queue":
		return config.Stack.Services.Queue, true
	}
	return "", false
}

func setConfigValue(config *engine.Config, key string, value string) (bool, error) {
	if rest, isDeploy := strings.CutPrefix(strings.ToLower(key), "deploy."); isDeploy {
		return setDeployValue(&config.Deploy, rest, value)
	}

	switch strings.ToLower(key) {
	case "project_name":
		config.ProjectName = value
	case "framework":
		config.Framework = value
	case "domain":
		config.Domain = value
	case "framework_version":
		config.FrameworkVersion = value
	case "table_prefix":
		config.TablePrefix = value
	case "php_version", "stack.php_version":
		config.Stack.PHPVersion = value
	case "python_version", "stack.python_version":
		config.Stack.PythonVersion = value
	case "node_version", "stack.node_version":
		config.Stack.NodeVersion = value
	case "db", "services.db", "stack.services.db":
		config.Stack.Services.DB = value
	case "db_version", "stack.db_version":
		config.Stack.DBVersion = value
	case "services.web_server", "web_server", "stack.services.web_server":
		config.Stack.Services.WebServer = value
	case "services.search", "search", "stack.services.search":
		config.Stack.Services.Search = value
		config.Stack.Features.Search = (value != "" && value != "none")
	case "services.cache", "cache", "stack.services.cache":
		config.Stack.Services.Cache = value
		config.Stack.Features.Cache = (value != "" && value != "none")
	case "services.queue", "queue", "stack.services.queue":
		config.Stack.Services.Queue = value
		config.Stack.Features.Queue = (value != "" && value != "none")
	default:
		return false, nil
	}
	return true, nil
}

// getDeployValue reads one key of the deploy block.
//
// The values are reported as the deploy will use them, not as they are stored:
// `keep_releases` answers with the effective count rather than 0 when the
// project never set one, because that is the number a deploy would prune by.
func getDeployValue(deploy engine.DeployConfig, key string) (string, bool) {
	if setting, isSetting := strings.CutPrefix(key, "settings."); isSetting {
		value, present := deploy.Settings[setting]
		if !present {
			// A setting the project never wrote is a key with nothing
			// configured, not a typo: only the recipe can say whether the key
			// exists, and `deploy plan` is where that is checked.
			return "", true
		}
		return formatSettingValue(value), true
	}
	switch key {
	case "keep_releases":
		return strconv.Itoa(deploy.KeepReleasesOr()), true
	case "command_timeout":
		return deploy.CommandTimeout, true
	case "maintenance_timeout":
		return deploy.MaintenanceTimeout, true
	case "lock_stale_after":
		return deploy.LockStaleAfter, true
	case "artifact_dir":
		return deploy.ArtifactDir, true
	case "db_backup":
		return strconv.FormatBool(deploy.DBBackup), true
	case "verify.url":
		return deploy.Verify.URL, true
	case "verify.timeout":
		return deploy.Verify.Timeout, true
	}
	return "", false
}

// formatSettingValue renders a `deploy.settings` value as the one line
// `config get` prints. A list is joined with commas and a map as `key=value`
// pairs, which is what the same key accepts back on `config set`.
func formatSettingValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case []string:
		return strings.Join(typed, ",")
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, formatSettingValue(item))
		}
		return strings.Join(items, ",")
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, key := range keys {
			pairs = append(pairs, key+"="+formatSettingValue(typed[key]))
		}
		return strings.Join(pairs, ",")
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// setDeployValue writes one key of the deploy block.
//
// A setting that is already a list stays a list: writing `shared_dirs` as one
// comma-separated argument would replace a list the deploy reads with a single
// string it cannot read, and the mistake would only show up at deploy time.
func setDeployValue(deploy *engine.DeployConfig, key string, value string) (bool, error) {
	if setting, isSetting := strings.CutPrefix(key, "settings."); isSetting {
		if deploy.Settings == nil {
			deploy.Settings = map[string]any{}
		}
		switch deploy.Settings[setting].(type) {
		case []string:
			deploy.Settings[setting] = splitSettingList(value)
		case []any:
			items := splitSettingList(value)
			anyItems := make([]any, 0, len(items))
			for _, item := range items {
				anyItems = append(anyItems, item)
			}
			deploy.Settings[setting] = anyItems
		case nil:
			deploy.Settings[setting] = value
		default:
			deploy.Settings[setting] = value
		}
		return true, nil
	}
	switch key {
	case "keep_releases":
		count, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || count < 0 {
			return false, fmt.Errorf("deploy.keep_releases must be a whole number, got %q", value)
		}
		deploy.KeepReleases = count
	case "command_timeout":
		deploy.CommandTimeout = value
	case "maintenance_timeout":
		deploy.MaintenanceTimeout = value
	case "lock_stale_after":
		deploy.LockStaleAfter = value
	case "artifact_dir":
		deploy.ArtifactDir = value
	case "db_backup":
		enabled, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return false, fmt.Errorf("deploy.db_backup must be true or false, got %q", value)
		}
		deploy.DBBackup = enabled
	case "verify.url":
		deploy.Verify.URL = value
	case "verify.timeout":
		deploy.Verify.Timeout = value
	default:
		return false, nil
	}
	return true, nil
}

// splitSettingList turns the one line `config get` prints back into the list it
// came from.
func splitSettingList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

// GetConfigValueForTest exposes getConfigValue to the tests/ package.
func GetConfigValueForTest(config engine.Config, key string) (string, bool) {
	return getConfigValue(config, key)
}

// SetConfigValueForTest exposes setConfigValue to the tests/ package.
func SetConfigValueForTest(config *engine.Config, key string, value string) (bool, error) {
	return setConfigValue(config, key, value)
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}
