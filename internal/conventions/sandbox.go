package conventions

// SandboxRemoteName is the reserved remote name for the synthetic sandbox
// rehearsal target.
//
// Unlike every other remote, `sandbox` has no configured identity: govard
// resolves it from the container `govard sandbox up` created, and a project's
// `remotes.sandbox` block may only add rehearsal *shape* (capabilities,
// protection, deploy settings) on top of it. Host, port, user, path and auth
// always come from the container, because a leaked identity does not fail
// loudly — the rehearsal would silently run against a different machine.
//
// The constant lives here rather than beside its first use because two
// packages need it and one of them cannot import the other:
// internal/engine's configuration validator has to recognise the name to exempt
// it from the host/user/path requirement, and internal/engine does not depend on
// internal/deploy. internal/deploy aliases it as deploy.SandboxRemoteName, which
// is how internal/cmd keeps reaching the sandbox through the deploy package.
const SandboxRemoteName = "sandbox"
