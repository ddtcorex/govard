package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"govard/internal/conventions"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func ensureBootstrapFrameworkEnvironment(config engine.Config, opts BootstrapRuntimeOptions) error {
	definition, ok := frameworks.Get(config.Framework)
	if !ok || definition.BootstrapEnvironmentPath == "" || definition.RenderBootstrapEnvironment == nil {
		return fmt.Errorf("framework %q does not provide a bootstrap environment renderer", config.Framework)
	}
	cwd, _ := os.Getwd()
	envPath := filepath.Join(cwd, definition.BootstrapEnvironmentPath)

	if info, err := os.Lstat(envPath); err == nil && (info.Mode()&os.ModeSymlink) != 0 {
		if _, err := os.Stat(envPath); err != nil {
			if err := os.Remove(envPath); err != nil {
				return fmt.Errorf("failed to remove env.php symlink: %w", err)
			}
		} else {
			return nil
		}
	} else if err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(envPath), conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create app/etc: %w", err)
	}

	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Errorf("failed to generate random bytes: %w", err)
	}
	cryptKey := hex.EncodeToString(randomBytes)

	tablePrefix := engine.NormalizeTablePrefix(config.TablePrefix)
	if remoteCfg, ok := config.Remotes[opts.Source]; ok {
		if definition.ProbeRemoteBootstrapMetadata != nil {
			metadata, err := definition.ProbeRemoteBootstrapMetadata(opts.Source, remoteCfg)
			if err == nil {
				if remoteKey := strings.TrimSpace(metadata.Private[definition.BootstrapEnvironmentMetadataKey]); remoteKey != "" {
					cryptKey = remoteKey
				}
				if tablePrefix == "" {
					tablePrefix = metadata.TablePrefix
				}
			} else {
				pterm.Warning.Printf("Could not extract remote bootstrap metadata (%v). Using generated fallback secret.\n", err)
			}
		}
	}

	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	localDB := resolveLocalDBCredentials(config, containerName)

	template := definition.RenderBootstrapEnvironment(cryptKey, types.BootstrapEnvironmentDatabase{
		Database: localDB.Database,
		Username: localDB.Username,
		Password: localDB.Password,
	}, tablePrefix)

	if err := os.WriteFile(envPath, []byte(template), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write framework bootstrap environment: %w", err)
	}

	pterm.Info.Println("Generated local framework bootstrap environment.")
	return nil
}

func runFrameworkSearchHostFixViaCLI(cmd *cobra.Command, config engine.Config) error {
	definition, ok := frameworks.Get(config.Framework)
	if !ok || definition.BuildSearchHostFixSQL == nil {
		return nil
	}
	sql := definition.BuildSearchHostFixSQL(config)
	// Skip the --environment flag implicitly because we're running it locally
	err := runGovardSubcommand(cmd, "db", "query", sql)
	if err != nil {
		pterm.Warning.Printf("Could not fix search host via 'govard db query' (continuing): %v\n", err)
	}
	return err
}
