# TD-10b — contract change (api-client, generated)

Branch `task/TD-10b`. The only contract file touched is `packages/api-client/src/generated/schema.d.ts`, regenerated
(`pnpm --filter @ngfw/api-client gen`) from the API's OpenAPI; never hand-edited. **Additive only**: no field, operation,
summary or schema is renamed or reshaped.

| Operation | Change | Why |
|---|---|---|
| `Users_setPassword` (`POST /api/v1/users/{name}/password`) | + `409` (`commit-busy`), + `503` (`audit-unavailable`) | manager addendum (TD-10a review M2): wait ≤ 1 s for the commit lock, then 409; review 2.3e: privileged routes fail closed when their audit row cannot be written first |
| `Auth_password` (`POST /api/v1/auth/password`) | + `409`, + `503` | same implementation as `Users_setPassword` |
| `Auth_createApiKey` (`POST /api/v1/auth/api-keys`) | + `503` | 2.3e fail closed |
| `Auth_deleteApiKey` (`DELETE /api/v1/auth/api-keys/{id}`) | + `503` | 2.3e fail closed |
| `Auth_logout` (`POST /api/v1/auth/logout`) | `204` description only: the session's refresh chain AND access tokens end; a Bearer token sent along counts | 2.3c per-session revocation |

Regenerated and unchanged (checked): `apps/cli/internal/api/operations_gen.go`, `docs/user/cli/reference.md`
(`make -C apps/cli gen docs`), `sdk/python/vrx/_generated`, `sdk/terraform/internal/provider/zz_*_gen.go` (`sdk/gen.sh`) —
none of them carries response descriptions or status codes of these operations.

Not in the contract: the cookies (`vrx_docs` next to `vrx_refresh`, Set-Cookie headers are not described in the OpenAPI),
the new environment settings (`VRX_TRUST_PROXY`, `VRX_SESSION_MAX_SEC`, `VRX_JWT_KEY_FILE`), the audit rows.
