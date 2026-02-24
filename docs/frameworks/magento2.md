# Magento 2

Govard provides deep integration for Magento 2 development with automated configuration and optimized stack.

## Auto-Configuration

The `govard config auto` command automates the injection of environment settings into `app/etc/env.php`.

### What is configured?

1. **Database**: Host, Name, User, Password
2. **Redis Cache**: Configures Default and Page cache
3. **Redis Session**: Uses the same Redis service for sessions
4. **Varnish**: Configures the Full Page Cache application type
5. **Elasticsearch/OpenSearch**: Search engine configuration
6. **Base URLs**: Automatic HTTPS URL setup

### Execution

Run this command after `govard env up` has finished starting the containers:

```bash
govard config auto
```

**Requirements:**
- Magento 2 must be installed
- Containers must be running (`govard env up`)
- `bin/magento` must be available

### Configuration Details

#### Database Connection

```php
'db' => [
    'table_prefix' => '',
    'connection' => [
        'default' => [
            'host' => 'db',
            'dbname' => 'magento',
            'username' => 'magento',
            'password' => 'magento',
            'model' => 'mysql4',
            'engine' => 'innodb',
            'initStatements' => 'SET NAMES utf8;',
            'active' => '1'
        ]
    ]
]
```

#### Redis Cache

- **Default Cache**: `redis:6379` (db 0)
- **Page Cache**: `redis:6379` (db 1)

#### Redis Sessions

- **Host**: `redis:6379`
- **Database**: 2

#### Varnish

Sets `system/full_page_cache/caching_application` to `2` (Varnish).

#### Elasticsearch/OpenSearch

- **Engine**: `opensearch`
- **Host**: `elasticsearch`
- **Port**: `9200`

## Stack Components

### Web Server

**Nginx** (default) - Optimed for serving from `pub/` folder:
- Static file caching
- PHP-FPM proxying
- Security headers

### PHP-FPM

Pre-configured with:
- PHP 7.4, 8.1, 8.2, 8.3, or 8.4 (default 8.4)
- Required extensions: intl, gd, bcmath, soap, xsl, zip, sockets
- Optimized `memory_limit` (4G)
- Xdebug 3 support (toggle with `govard debug`)
- Node.js + Grunt tooling for frontend builds (via `govard/php-magento2`)
- Node.js is bundled in the `govard/php-magento2` image (version is tied to the image build)

### Database

**MariaDB 11.4** (default) with:
- Database: `magento`
- User: `magento` / `magento`
- Root: `root` / `root`

### Varnish 7.x

Custom VCL with:
- Cache bypass for Admin and Checkout routes
- Support for `X-Magento-Tags` purging
- Grace periods for stale-content delivery
- Custom `X-Govard-Cache` headers for HIT/MISS debugging

### Grunt LiveReload

Govard proxies `/livereload.js` to the Grunt LiveReload port (`35729`) inside the PHP container.
Make sure Grunt is running with LiveReload enabled, then include:

```html
<script src="/livereload.js"></script>
```

### Xdebug Session Cookie

Set `stack.xdebug_session` to match your IDE helper cookie (default `PHPSTORM`).
You can provide multiple values separated by commas (e.g. `PHPSTORM,VSCODE`).

### Redis Architecture

Single Redis service for cache and sessions:
1. **redis**: Cache (default + FPC) and sessions (db 2)

## Magento CLI

Run Magento CLI commands directly:

```bash
govard tool magento [command]
```

Examples:

```bash
govard tool magento cache:clean
govard tool magento cache:flush
govard tool magento setup:upgrade
govard tool magento indexer:reindex
govard tool magento maintenance:enable
govard tool magento maintenance:disable
```

**User**: Commands run as the project runtime user (`stack.user_id:stack.group_id`
when configured, otherwise `www-data`) to keep file ownership aligned with the
workspace configuration.

## Development Workflow

### Daily Development

```bash
# Start environment
govard env up

# Configure (first time or after env changes)
govard config auto

# Enter container
govard shell

# Enable Xdebug when needed
govard debug on

# Check logs
govard env logs -e  # Error only
```

### Cache Management

```bash
# Clean all caches
govard tool magento cache:clean

# Clean specific cache
govard tool magento cache:clean config full_page

# Flush cache
govard tool magento cache:flush
```

### Database Operations

```bash
# Access database
govard db connect

# Import database
govard db import < dump.sql

# Export database
govard db dump > dump.sql
```

## Troubleshooting

### Permission Issues

If you see permission errors:

```bash
govard shell
# Inside container:
chmod -R 777 var/ generated/
```

### Cache Not Clearing

Varnish may cache responses. To purge:

```bash
# Restart Varnish
docker restart {project}-varnish-1

# Or restart all
govard env stop && govard env up
```

### Elasticsearch Connection

If search doesn't work:

```bash
govard tool magento config:set catalog/search/engine opensearch
govard tool magento config:set catalog/search/opensearch_server_hostname elasticsearch
govard tool magento config:set catalog/search/opensearch_server_port 9200
```
