package magento2

import (
	"fmt"

	"govard/internal/conventions"
	"govard/internal/engine"
)

// BootstrapEnvironmentDatabase is the local database connection data required
// to render Magento's app/etc/env.php during a clone bootstrap.
type BootstrapEnvironmentDatabase struct {
	Database string
	Username string
	Password string
}

// BuildBootstrapEnvironment renders Magento's app/etc/env.php. Its contents
// are framework-owned; generic bootstrap orchestration only obtains the local
// database values and writes the declared environment file.
func BuildBootstrapEnvironment(cryptKey string, localDB BootstrapEnvironmentDatabase, tablePrefix string) string {
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
