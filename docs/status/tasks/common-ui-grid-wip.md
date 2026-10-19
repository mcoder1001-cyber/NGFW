# Common UI grid migration

- Branch: codex/common-ui-grid-20261008
- Base: 417e8fcd
- Owned: packages/ui-kit/src/data-grid/**; packages/common-ui/src/extensions/PgDataGrid.tsx; docs/status/tasks/common-ui-grid*
- Completed: ServerDataGrid renders Common UI PgDataGrid. The complete existing MUI grid props, API ref, server paging/sorting/filtering, cancellation, localized labels, errors, selection and export are forwarded. Common UI extension applies compact table/header/footer styling without client paging or query changes.
- Local implementation SHA: a0e5a018.
- Remote SHA: none. Actual push failed HTTP 403: authenticated CLI account hightwig does not have repository write permission.
- Tests: not executed yet; this isolated checkout has no node_modules and root owns new Common UI package dependencies/exports. Existing ServerDataGrid tests remain unchanged and applicable.
- Remaining: root package integration, typecheck, UI grid tests, independent review, unchanged complete quick gate.
- Current failures: no dependency installation in isolated checkout; tools/ci.sh check --base main failed board validation because python3-yaml is missing. Forbidden-pattern checks passed; gitleaks unavailable. Full quick gate not run.
- Next command: pnpm --filter @ngfw/ui-kit typecheck && pnpm --filter @ngfw/ui-kit test -- src/data-grid/ServerDataGrid.test.tsx

Implementation choice: existing Geereed cannot preserve ACL imperative/selection/filtering semantics. Per manager direction, Common UI exposes an additive PgDataGrid server-capable extension alongside its Geereed component. Existing APIs and security feature behavior stay intact.
