<?php
// The shared-file verification reads this file, and `--db-backup` reads the
// default connection from it, exactly as a real Magento deploy does. The values
// are the ones tests/integration/deploy_sandbox_test.go provisions in the
// sandbox database; they are not a real credential.
return [
    'db' => [
        'connection' => [
            'default' => [
                'host' => 'localhost',
                'dbname' => 'govard_deploy',
                'username' => 'govard_deploy',
                'password' => 'sandbox-dump-secret-7f3a',
            ],
        ],
    ],
];
