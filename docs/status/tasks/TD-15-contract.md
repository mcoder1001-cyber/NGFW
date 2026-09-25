# TD-15 — contract change (generated api-client)

**Commit subject:** `contract(api-client): document secretVersions on GET /config/revisions/{rev}`

## What changes
`packages/api-client/src/generated/schema.d.ts` — regenerated with `pnpm --filter @ngfw/api-client gen`, never
hand-edited. Additive only:

- `GET /api/v1/config/revisions/{rev}` 200 body gains the optional member
  `secretVersions?: { [ref: string]: number } | null` (secret versions pinned by the revision, `<kind>/<name>` →
  version; no values).
- `payload` gains a description string.

No field renamed, reshaped or removed; `packages/schema` and `packages/proto` are untouched.

## Why
ARCH-04 live drift: the handler has returned `secretVersions` since review M2 (the repo row carries it), but the
OpenAPI document did not list it. TD-15 documents it (the value is not a secret: ref names and integer versions, the
same information the secrets list already shows) and adds a compile-time guard (`SameKeys`) so the row and the DTO
cannot drift again. The alternative — stripping the member from the response — would change behaviour for clients
already reading it and was not chosen.

## Verification
The OpenAPI document before/after TD-15 differs only in this hunk (`diff before.json after.json`, see TD-15.md);
`pnpm --filter @ngfw/api-client test` and `pnpm --filter @ngfw/web typecheck` pass against the regenerated types.

## Process note
00-CONTEXT asks for a separate `contract/<id>` branch; this worker was given one worktree branch
(`worktree-agent-a52e6f5b9e6b01a70`) and told not to create others, so the contract commit is the FIRST commit on
that branch, separate from the implementation commit — the manager can cherry-pick it.
