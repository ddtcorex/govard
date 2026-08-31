# audit-module fixture

Neutral Magento-2-shaped tree used by the gated live lint audit tests in
`tests/integration/audit_glint_live_test.go`. It deliberately covers all
three audit target modes from a single checked-in tree:

- `project/` satisfies `internal/frameworks/magento2.isMagentoProject`
  (a regular `bin/magento` plus a `composer.json` requiring
  `magento/product-community-edition`), so it resolves as a `project` target.
- `project/app/code/Govard/AuditSample/` carries only `etc/module.xml`, which is
  how Magento itself registers an in-app module and how
  `classifyMagentoModule` recognizes one without a Composer manifest. Starting
  a resolve there yields a `module_in_project` target.
- `standalone/` is a separately distributed module: its own `composer.json` of
  type `magento2-module`, with no Magento project anywhere in its ancestry, so
  it resolves as a `standalone` target.

`project/.govard.yml` pins `stack.php_version: "7.4"` so the project-mode test
exercises the oldest supported launcher without needing a running container.

> **Keep `project/.govard.yml` tracked.** Many developers carry a personal global
> gitignore rule for `.govard.yml` (it is normally per-machine project config, and
> this repository's own `.gitignore` excludes `.govard.local.yml` for the same
> reason). That rule silently excludes this fixture file too, and `git status`
> then reports a clean tree while the fixture is incomplete for everyone else. It
> is committed with `git add -f`; if you ever regenerate this tree, re-check with
> `git ls-files tests/integration/projects/magento2/audit-module/` rather than
> trusting `git status`. Without it the project fixture has no base config at all
> and the `.govard.local.yml` layer the module-in-project tests write has nothing
> to layer onto.
The tests copy this tree into a temp directory first, so a run never mutates it;
one of them digests the tree before and after a run to prove the source mount
stayed read only.

Nothing here is installed or executed: the lint runner analyzes project and
module-in-project targets straight off the read-only source mount and never runs
Composer for them, so no `vendor/` tree is required.
