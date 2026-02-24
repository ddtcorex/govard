# govard config profile

Show or apply the runtime profile selected for the detected framework and framework version.

## Usage

```bash
govard config profile
govard config profile --framework laravel --framework-version 11
govard config profile --json
govard config profile apply
govard config profile apply --framework magento2 --framework-version 2.4.7
```

## Behavior

- `govard config profile` is read-only.
- `govard config profile apply` writes the selected profile into `.govard.yml`.
- This command does not modify application dependency files (`composer.json`, lockfiles, `package.json`, lockfiles).
- `--json` outputs a stable machine-readable payload for automation/desktop integration.

## Options

- `--framework` Override detected framework.
- `--framework-version` Override detected framework version.
- `--json` Output selected profile as JSON.
