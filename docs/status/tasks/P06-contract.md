# P06 — contract note: `packages/api-client` regenerated

`packages/api-client/src/generated/schema.d.ts` is listed in `CONTRACT_PATHS` (tools/ci.sh), so the regeneration is
committed as `contract(api-client): …`. Nothing is hand-edited: the file is `openapi-typescript` output of the OpenAPI
3.1 document that `apps/api` builds from its Nest decorators + the Zod domain schemas of `packages/schema`.

- **Additive only.** The previous client had one path (`GET /api/v1/health`), which is unchanged. New: `/api/v1/auth/**`,
  `/api/v1/config/**`, `/api/v1/state/**`, `/api/v1/actions/{action}`, `/api/v1/secrets/**`, `/api/v1/audit`, the
  components `RootConfig`, `<Key>Config` × 13 (from packages/schema) and `Problem` (RFC 9457).
- `packages/schema` and `packages/proto` are **not** touched.
- Gen pipeline change (P06 owns packages/api-client): `gen` first builds the API's workspace dependencies
  (`pnpm --filter "@ngfw/api^..." run build`), and `@ngfw/api` is a devDependency so turbo orders `gen` after the
  schema/proto generators; `lint` now also runs `redocly lint openapi.json` (recommended ruleset; `redocly.yaml`).
