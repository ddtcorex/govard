# Custom Recipe

The `custom` recipe lets you build a stack by selecting the services you want.

## Quick Start

```bash
govard init -r custom
```

You'll be prompted for:
- Web server (`nginx`, `apache`, or `hybrid`)
- Database (`mariadb`, `mysql`, or `none`)
- Cache (`redis`, `valkey`, or `none`)
- Search (`opensearch`, `elasticsearch`, or `none`)
- Queue (`rabbitmq` or `none`)
- Varnish (optional)
- PHP version, Node.js version, and web root
- Xdebug session cookie value (used for debug routing)

## Resulting Configuration

The prompt generates a `govard.yml` that you can edit later. The key fields are:

```yaml
recipe: custom
stack:
  web_root: "/public"
  services:
    web_server: nginx
    cache: redis
    search: opensearch
    queue: none
  db_type: mariadb
  db_version: "11.4"
  php_version: "8.4"
  node_version: "24"
  queue_version: "3.13.7"
  xdebug_session: "PHPSTORM"
  features:
    varnish: false
```

## Notes

- `stack.web_root` is appended to `/var/www/html` for the nginx/apache document root.
- `web_server: hybrid` runs nginx at the edge and forwards requests to an internal apache container.
- Set `db_type: none` to omit the database service entirely.
- Use `stack.services.cache` and `stack.services.search` to control optional services.
