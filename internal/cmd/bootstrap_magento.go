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
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

func ensureBootstrapMagentoEnvPHP(config engine.Config, opts BootstrapRuntimeOptions) error {
	if !engine.IsMagento2Family(config.Framework) {
		return nil
	}

	cwd, _ := os.Getwd()
	envPath := filepath.Join(cwd, "app", "etc", "env.php")

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
		if metadata, err := remote.ProbeMagento2Environment(opts.Source, remoteCfg); err == nil {
			if strings.TrimSpace(metadata.CryptKey) != "" {
				cryptKey = strings.TrimSpace(metadata.CryptKey)
			}
			if tablePrefix == "" {
				tablePrefix = metadata.DB.TablePrefix
			}
		} else {
			pterm.Warning.Printf("Could not extract crypt/key from remote env.php (%v). Using fallback key.\n", err)
		}
	}

	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	localDB := resolveLocalDBCredentials(config, containerName)

	template := buildBootstrapMagentoEnvPHP(cryptKey, localDB, tablePrefix)

	if err := os.WriteFile(envPath, []byte(template), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write app/etc/env.php: %w", err)
	}

	pterm.Info.Println("Generated local app/etc/env.php for bootstrap.")
	return nil
}

func buildBootstrapMagentoEnvPHP(cryptKey string, localDB dbCredentials, tablePrefix string) string {
	localDB = localDB.withDefaults()
	tablePrefix = engine.NormalizeTablePrefix(tablePrefix)

	return fmt.Sprintf(`<?php
return [
    'backend' => [
        'frontName' => %q
    ],
    'crypt' => [
        'key' => %q
    ],
    'db' => [
        'table_prefix' => %q,
        'connection' => [
            'default' => [
                'host' => %q,
                'dbname' => %q,
                'username' => %q,
                'password' => %q,
                'active' => '1'
            ],
            'indexer' => [
                'host' => %q,
                'dbname' => %q,
                'username' => %q,
                'password' => %q,
                'active' => '1'
            ]
        ]
    ],
    'resource' => [
        'default_setup' => [
            'connection' => 'default'
        ]
    ],
    'x-frame-options' => 'SAMEORIGIN',
    'MAGE_MODE' => 'developer',
    'session' => [
        'save' => 'files'
    ],
    'install' => [
        'date' => 'Mon, 01 May 2023 00:00:00 +0000'
    ]
];
`, conventions.DefaultAdminPath,
		cryptKey,
		tablePrefix,
		conventions.DefaultMagentoDBHost,
		localDB.Database, localDB.Username, localDB.Password,
		conventions.DefaultMagentoDBHost,
		localDB.Database, localDB.Username, localDB.Password,
	)
}

func runMagentoSearchHostFixViaCLI(cmd *cobra.Command, config engine.Config) error {
	host := "elasticsearch"
	if s := strings.ToLower(strings.TrimSpace(config.Stack.Services.Search)); s != "" && s != "none" {
		host = s
	}
	searchEngine := engine.ResolveMagentoSearchEngine(config)
	sql := engine.BuildMagentoSearchHostFixSQL(host, searchEngine)
	// Skip the --environment flag implicitly because we're running it locally
	err := runGovardSubcommand(cmd, "db", "query", sql)
	if err != nil {
		pterm.Warning.Printf("Could not fix search host via 'govard db query' (continuing): %v\n", err)
	}
	return err
}

func BuildBootstrapMagentoEnvPHPForTest(cryptKey, database, username, password string) string {
	return buildBootstrapMagentoEnvPHP(cryptKey, dbCredentials{
		Database: database,
		Username: username,
		Password: password,
	}, "")
}

func BuildBootstrapMagentoEnvPHPWithPrefixForTest(cryptKey, database, username, password, tablePrefix string) string {
	return buildBootstrapMagentoEnvPHP(cryptKey, dbCredentials{
		Database: database,
		Username: username,
		Password: password,
	}, tablePrefix)
}
