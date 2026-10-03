# F-restconf-yang — RESTCONF + YANG from the schema

Merged in PR #50 (2026-09-27). Complete in-container; no host follow-up.

## Delivered

- **`packages/yang`** — a deterministic Zod→YANG 1.1 generator (via the JSON Schema), one module per root key,
  checked in under `generated/` with a golden drift test, wired into `pnpm gen`.
- **RESTCONF (RFC 8040)** — `apps/api/src/features/restconf-yang`: data GET/PUT/PATCH/DELETE and
  `operations/ngfw:commit|confirm|rollback` over the candidate/commit engine; running/candidate reads; secret
  redaction; `ietf-restconf:errors` bodies; host-meta, API resource, ietf-yang-library. Excluded from OpenAPI; a
  `/api/v1/system/yang` read surface backs the web download card.
- **Web** — `System › RESTCONF / YANG` download card (en/fa).
- **Docs** — `docs/user/system/restconf-yang.md`.

## Decisions (documented defaults for the prompt's open questions)

- Namespace `urn:ngfw:<key>`, organization `NGFW`.
- Writes require an explicit `ngfw:commit` (like `/api/v1`), not auto-commit.
- RESTCONF is not in the OpenAPI document (`@ApiExcludeController`).
- `/.well-known/host-meta` is guarded like every other route.

## Known limitations / manual gates

- `pyang`/`yanglint` are not installed on the build image, so YANG validity is a **manual gate**; the golden test
  guards output drift, and the generator handles the XSD-incompatible regex constructs the schema uses today.
- `tools/ci.sh` `GEN_PATHS` does not include `packages/yang/generated` (manager-owned). Drift is instead caught by the
  golden test in `packages/yang`; if the schema changes and the modules are not regenerated, that test fails.
- A schema list with no natural key becomes a keyless (`ordered-by user`) YANG list — a documented deviation (a YANG
  config list normally needs a key); there are none in the current schema.
- Only the top addressed node is module-qualified in a response; deep RFC 8040 encoding of augmented nodes is not
  reproduced (FAST MODE).
