## 2025-02-18 - [Insecure Temporary File Usage in Trust Logic]
**Vulnerability:** The `trustDarwin` function relied on a hardcoded path `/tmp/govard-ca.crt` to load the CA certificate. This file was expected to be placed there by the user or an external process.
**Learning:** Using predictable paths in shared directories like `/tmp` can lead to TOCTOU vulnerabilities or pre-creation attacks, where a malicious user creates the file first with bad content.
**Prevention:** Always use user-specific directories (like `~/.govard/ssl`) or secure temporary file creation APIs (`os.MkdirTemp`) that guarantee unique, private paths.
