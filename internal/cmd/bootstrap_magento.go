package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
)

func ensureBootstrapMagentoEnvPHP(config engine.Config, opts bootstrapRuntimeOptions) error {
	if config.Framework != "magento2" {
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

	if err := os.MkdirAll(filepath.Dir(envPath), 0755); err != nil {
		return fmt.Errorf("failed to create app/etc: %w", err)
	}

	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Errorf("failed to generate random bytes: %w", err)
	}
	cryptKey := hex.EncodeToString(randomBytes)

	if remoteCfg, ok := config.Remotes[opts.Source]; ok {
		if metadata, err := remote.ProbeMagento2Environment(opts.Source, remoteCfg); err == nil {
			if strings.TrimSpace(metadata.CryptKey) != "" {
				cryptKey = strings.TrimSpace(metadata.CryptKey)
			}
		} else {
			pterm.Warning.Printf("Could not extract crypt/key from remote env.php (%v). Using fallback key.\n", err)
		}
	}

	containerName := fmt.Sprintf("%s-db-1", config.ProjectName)
	localDB := resolveLocalDBCredentials(containerName)

	template := fmt.Sprintf(`<?php
return [
    'backend' => [
        'frontName' => 'admin'
    ],
    'crypt' => [
        'key' => %q
    ],
    'db' => [
        'table_prefix' => '',
        'connection' => [
            'default' => [
                'host' => 'db',
                'dbname' => %q,
                'username' => %q,
                'password' => %q,
                'active' => '1'
            ],
            'indexer' => [
                'host' => 'db',
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
    ]
];
`, cryptKey,
		localDB.Database, localDB.Username, localDB.Password,
		localDB.Database, localDB.Username, localDB.Password,
	)

	if err := os.WriteFile(envPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write app/etc/env.php: %w", err)
	}

	pterm.Info.Println("Generated local app/etc/env.php for bootstrap.")
	return nil
}

func bootstrapMagentoMediaSyncArgs(opts bootstrapRuntimeOptions) []string {
	excludes := []string{
		"*.gz",
		"*.zip",
		"*.tar",
		"*.7z",
		"*.sql",
		"tmp",
		"itm",
		"import",
		"export",
		"importexport",
		"captcha",
		"analytics",
		"catalog/product.rm",
		"catalog/product/product",
		"opti_image",
		"webp_image",
		"webp_cache",
		"shoppingfeed",
		"amasty/blog/cache",
	}

	// Keep product images excluded by default to make media sync faster.
	// When explicitly requested, still exclude the cache folder.
	if opts.IncludeProduct {
		excludes = append(excludes, "catalog/product/cache")
	} else {
		excludes = append(excludes, "catalog/product")
	}

	args := make([]string, 0, len(excludes)*2)
	for _, pattern := range excludes {
		args = append(args, "--exclude", pattern)
	}
	return args
}
