# Reviewer R3 — contracts / API & backward compatibility   (prepend 00-CONTEXT.md, then ../REVIEW-PROMPT.md)

Mandatory when the diff touches `packages/schema`, `packages/proto`, `packages/api-client`, generated code, API routes/DTOs or
`docs/04-api-datamodel.md`.

## Check
1. `git diff --name-only main...task/<id> -- packages/schema packages/proto apps/agent/gen packages/proto/gen packages/api-client/src/generated`.
   Any hit needs a commit whose subject starts with `contract(` **and** `docs/status/tasks/<id>-contract.md`. Reshaping or renaming
   an existing field, message, enum value, route or event code → BLOCKER (decision-policy #1: a PENDING decision, not a review
   finding to argue). Additive changes that follow `docs/04-api-datamodel.md` are fine.
2. Generated code is regenerated, never hand-edited (`pnpm gen` then `git status --porcelain` on the generated dirs is clean).
3. Backward compatibility: old clients and stored configs still load (new fields optional with defaults, enums extended not
   renumbered, proto field numbers never reused, JSON names stable); a migration exists for any stored shape change and is reversible
   or documented as not.
4. API behaviour: problem+json errors with `pointer`s as the spec says, status codes (400 validation, 404, 409 busy/conflict),
   pagination and filtering consistent with sibling routes, OpenAPI/api-client updated with the route.
5. Agent ↔ API ↔ web agree: the same field names and units end to end (ms vs s, bytes vs packets).

## Output
`docs/status/tasks/<id>-review-R3.md` — findings, verdict line.
