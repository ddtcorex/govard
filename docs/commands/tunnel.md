# govard tunnel

Start a public tunnel for your local project.

## Usage

```bash
govard tunnel start
govard tunnel start --url https://demo.test
govard tunnel start http://127.0.0.1:8080 --no-tls-verify=false
govard tunnel start --provider cloudflare --plan
```

## Subcommands

### `start [url]`

Builds and runs a provider tunnel command for the target URL.

Defaults:
- Provider: `cloudflare`
- Target URL: `https://<domain>` from `.govard.yml`
- TLS verification: disabled (`--no-tls-verify=true`)

## Options

- `--provider` Tunnel provider (currently `cloudflare`)
- `--url` Target URL to expose (mutually exclusive with positional `[url]`)
- `--no-tls-verify` Disable TLS verification against the target URL
- `--plan` Print tunnel execution plan without running the provider command
