# Full Common UI migration

Owner approved the Common UI sample and requested the design for all NGFW pages.
The reference is `/Users/mohammad/webngf/frontend`; Common UI changes are authorized.
Preserve existing features, security, generated schemas, live APIs and atomic
configuration workflows. `usePgForm` may coordinate simple UI fields; existing RHF
remains authoritative where nested schema/validation/pointer semantics require it.

Integration branch: `codex/common-ui-migration-20261008`.
Base: `417e8fcd` (observed origin/main after fetch).
Integrator owns `packages/common-ui/**`, workspace dependency manifests/lockfile,
`packages/ui-kit/src/theme/**`, `apps/web/src/**` integration/import adjustments,
`tools/import-common-ui.py`, and migration/sample docs/prototype checkpoints.
Worker-owned changes are integrated by cherry-pick, not edits to their worktrees.

Workers: shell `codex/common-ui-shell-20261008`; schema forms
`codex/common-ui-forms-20261008`; grid `codex/common-ui-grid-20261008`, each in its own
managed worktree. Cross-review does not approve a worker's own implementation.

Acceptance: complete existing route inventory preserved; theme/shell/controls/forms/
grids coherent; existing UI-kit and web tests pass; production bundle fits the
unchanged budget and excludes developer routes; en/fa and RTL navigation preserved;
independent review; unchanged quick CI before any merge. Lab acceptance may be deferred
only as repository policy permits. No mocks added to production.
