# Common UI shell WIP

Branch: `codex/common-ui-shell-20261008`; baseline `417e8fcd`.
Owned files: see envelope.
Completed: real Sidenav/PgGlobalAppBar/PgBreadcrumb/PgIconButton shell, responsive 250/64 rail and mobile drawer, existing route inventory, translated unavailable markers, preserved transactional/license/stream banners, auth/settings and theme mode action, Common UI gradient page canvas/header.
Checks: source inspection only. `ls node_modules/.bin/tsc` fails: dependencies absent in this isolated worktree. No passing build/test claim.
Remaining: root portable library integration; Sidenav localization and exact-current selection contract; build/typecheck and independent review.
Next command after root dependency integration: `pnpm --filter @ngfw/web typecheck` (confirm actual package name).
Local implementation checkpoint: `69dcd548`. Remote publication FAILED: `git push -u origin codex/common-ui-shell-20261008` returned HTTP 403, permission denied to hightwig. Manager notified; connector publication required. No remote SHA exists for this checkpoint.

Localization/current/end contracts applied in 39fd28dd; native appbar menu controls and translated Sidenav labels. Additional owned files en/fa common.json. git diff --check 417e8fcd..HEAD passed. Dependencies remain absent; root integration must run typecheck/tests.
