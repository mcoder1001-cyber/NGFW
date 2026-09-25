# F-licensing — contract change (additive, generated)

- `packages/api-client/src/generated/schema.d.ts`: regenerated (`pnpm -C packages/api-client gen`) for two NEW routes,
  nothing renamed or removed: `GET /api/v1/state/license`, `PUT /api/v1/system/license` (tag `licensing`).
- `apps/cli/internal/api/operations_gen.go`: regenerated (`make -C apps/cli gen docs`) — two new operations.
- No `packages/schema` or `packages/proto` change: nothing licence-related crosses the API↔agent boundary (D-040) and
  the licence is not part of the configuration document.
- Commit containing the regenerated files: `0fcd967 chore(gen): regenerate api-client and CLI operations for licensing routes`.
