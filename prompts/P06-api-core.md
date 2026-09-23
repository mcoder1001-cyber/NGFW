# Task P06 — vrx-api core: datastore + commit engine + auth   (prepend 00-CONTEXT.md)

## Goal
The NestJS control plane: candidate/running datastore, transactional commit through
the agent, revisions/rollback, confirmed commit, auth/RBAC/audit, live telemetry relay.

## Read first
`docs/04-api-datamodel.md`, `packages/schema` (P02), `packages/proto/gen/ts` (P03).

## Build exactly this
1. **Persistence** (Prisma or Drizzle — pick one, justify): tables from docs/04
   (`config_revision`, `config_candidate`, `audit_log`, `app_user`, `api_key`, `secret`,
   `system_event`). Migrations committed. Seed: one admin user from `VRX_BOOTSTRAP_ADMIN_PASSWORD`.
2. **Datastore service**: `getRunning()`, `getCandidate(user)`, `patchCandidate(pointer,
   merge-patch)`, `putCandidate(pointer, value)`, `deleteCandidate(pointer)`, `diff()`,
   `discard()`. Single-writer lock with owner + timestamp; `409` with lock owner otherwise.
3. **Validation pipeline**: schema (Zod) → semantic (P02 functions) → agent `DryRun`.
   Errors are RFC 9457 problem+json with `errors[]{pointer,message}`.
4. **Commit engine**: `POST /config/commit?confirm=<sec>&comment=` → validate → agent
   `Apply(txn)` → on APPLIED persist new revision (full snapshot, hash, author, comment,
   parent) and promote candidate to running; on FAILED/ROLLED_BACK return 422 with per-object
   results and leave running untouched. `POST /config/commit/confirm`, `POST /config/rollback/{rev}`
   (creates a new revision whose payload = old one, then applies), `GET /config/revisions`,
   `GET /config/export`, `POST /config/import` (to candidate only).
5. **Routes**: everything under `/api/v1/config/**`, `/api/v1/state/**`
   (`interfaces`, `routes?vrf=&page=`, `neighbors`, `system`), `/api/v1/actions/**`
   (501 for now). Generic `PATCH/PUT/DELETE /api/v1/config/{path...}` operating on the
   JSON pointer into the candidate, so new schema domains need no new controller.
6. **Auth**: local users with argon2id; JWT access (15 min) + rotating refresh cookie
   (httpOnly, SameSite=Strict); API keys (`Authorization: ApiKey`); roles admin/operator/readonly
   enforced by a guard on every route (readonly: GET only; operator: no user/AAA changes).
   Rate limit login; lockout after 10 failures.
7. **Audit**: interceptor writes user, ip, route, before/after diff, result for every mutation.
8. **Telemetry relay**: subscribe to agent `StreamStats`/`StreamEvents`; fan out on
   `WS /api/v1/stream` with `{subscribe:[topics]}`; per-connection topic filter; heartbeat.
9. **OpenAPI** generated from Nest decorators + Zod → `pnpm gen` updates `packages/api-client`.
10. Tests: unit for datastore/diff/lock; e2e with real Postgres (testcontainers) and the
    lab VM's agent: patch MTU → diff shows it → commit → `vppctl show int` reflects it →
    rollback → reverted; commit with `confirm=5` and no confirm → reverted after 5 s;
    invalid overlapping IPs → 400 with pointer; readonly user PATCH → 403.

## Acceptance
- [ ] All tests green; OpenAPI valid (`redocly lint`); `packages/api-client` regenerated
- [ ] No route without an auth guard (write a test that enumerates routes and asserts it)
- [ ] `grep -rn "child_process\|execSync" apps/api/src` is empty

## Out of scope
UI. NAT/ACL/VPN. RESTCONF. External AAA. CLI.
