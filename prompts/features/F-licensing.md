# Task: F-licensing — simple licence and entitlement enforcement   (prepend 00-CONTEXT.md)

## Goal
A minimal, offline licensing scheme (WBS D12.2 in `plan/wbs.csv`, reduced to "simple"): a signed licence file (Ed25519) that carries customer,
serial/host binding, expiry and entitlements (feature flags + limits such as throughput tier or max tunnels); the API verifies it and
enforces entitlements **at commit validation time**; the UI shows status. No online activation, no DRM, no obfuscation — the data plane is
never degraded at runtime by licensing.

## Inputs to read first
- P06 (merged) under `apps/api/src/`: `commit/validation.service.ts` (schema → semantic + secret existence → agent DryRun, `ValidationTier`
  union — you add one `license` stage between the secret-existence check and the agent DryRun, a shared hunk under your anchor),
  `common/problem.ts` (problem+json), `db/schema.ts` + `migrations/` (Drizzle; `pnpm -C apps/api db:generate` — DB migration protocol in your
  envelope), `auth/` (roles), `audit/system-events.service.ts` (the `system_event` table — use it for the expiry event instead of a new bus
  topic); TD-2/TD-4 own `auth/**`/`users/**` (read-only for you)
- `packages/schema` root keys (`system`, `interfaces`, `nat`, `vpn`, …) — entitlements are expressed as JSON-pointer predicates over the document
- `docs/decisions/LOG.md` D-001, D-040 (nothing licence-related goes to the agent), D-046, D-059
- Node `crypto` Ed25519 (`crypto.verify(null, …)`)

## Scope — build exactly this
1. **Format** `tools/license/`: licence = canonical JSON `{version, licenseId, customer, issuedAt, notBefore, expiresAt, binding{machineIdHash?|serial?},
   entitlements{features[], limits{…}}}` + detached Ed25519 signature (base64), packed as one `.vrxlic` file. `tools/license/vrx-license` CLI
   (Node or Go): `keygen` (writes the private key **outside the repo**, prints path), `issue`, `verify`, `inspect`. Test keys are generated at test
   time, never committed. `tools/` is not a pnpm workspace member and has no Go module: keep the CLI dependency-free (a plain ESM script on
   `node:crypto` that shares the canonical-JSON/verify code with the API feature, or a Go `main` with stdlib only) — a new workspace package,
   Go module or dependency is a questions-file item (`pnpm-workspace.yaml`, lockfiles and `tools/ci.sh` are manager-owned); its tests run from
   the API's vitest suite so `tools/ci.sh` covers them. The product's public key is embedded in the API build via a config file inside your own
   `apps/api/src/features/licensing/` (not `apps/api/src/config.ts`, which is shared).
2. **API** `apps/api/src/features/licensing/`: `PUT /api/v1/system/license` (admin, upload), `GET /api/v1/state/license` (status: valid / grace /
   expired / invalid, entitlements, days left — never the raw signature or customer PII beyond name), a validation stage that rejects a commit
   using an unlicensed feature (e.g. `vpn.ikev2` present without `ipsec` entitlement) with 403/422 problem+json carrying the `pointer` of the first
   offending node; a **grace period** (e.g. 30 days after expiry: warn, still commit); no licence = "community" entitlement set (defined in one table).
   Expiry event recorded through `SystemEventsService` (`system_event`, subsystem `licensing`; F-dashboard-prom-alarms may consume it) — no new
   bus topic. OpenAPI; regenerate `packages/api-client` (and `make -C apps/cli gen docs`).
3. **UI** `apps/web/src/domains/system/licensing/`: licence page (status, entitlements table, upload), banner when in grace/expired; en + fa
   (`locales/*/licensing.json`).
4. **Tests**: signature verify (valid, tampered byte, wrong key, not-yet-valid, expired, grace), host binding mismatch, entitlement predicates on
   sample documents, commit rejected with the right pointer, running config never removed when a licence expires.
5. **Docs**: `docs/user/system/licensing.md` — file format, entitlement table, grace rules, how support issues a licence.

Files you own and shared hotspots: your TASK ENVELOPE is authoritative. Main pieces: `tools/license/**`, `apps/api/src/features/licensing/**`,
`apps/web/src/domains/system/licensing/**`, `apps/web/src/locales/{en,fa}/licensing.json`, `docs/user/system/licensing.md`,
`test/topology/licensing/**`. Shared files: lines under your anchor only (`app.module.ts`, the validation stage in P06's validation service,
DB schema/migrations if you store the licence in PostgreSQL, web router/nav/i18n) — the manager resolves at merge.

## Acceptance (paste the evidence)
- [ ] `vrx-license issue` → upload → `GET /state/license` shows valid + entitlements; tampered file → 400 (pasted)
- [ ] Commit adding an unlicensed feature → rejected with `pointer`; within grace → accepted with a warning; expired past grace → rejected for new
      features but the running config stays applied (agent Retrieve unchanged — pasted)
- [ ] `git grep -n "PRIVATE KEY"` and fixtures contain no private key; logs contain no licence signature
- [ ] UI screenshot of the licence page against the real endpoint; `tools/ci.sh --base main` green

## Out of scope (do not build)
Online activation / licence server / call-home; hardware dongles, TPM binding; per-throughput rate limiting in VPP (no agent/VPP work at all); billing;
tamper-resistance/obfuscation of the check; enforcement in the CLI (P13) beyond what the API rejects; VDOM/tenant licences (D-003).

## Open questions to surface, not to decide silently
The entitlement matrix (which features are "community" vs paid) is a product-owner decision — ship a clearly marked sample table. Host binding by
`/etc/machine-id` hash vs DMI serial (VMs clone machine-ids) — default: optional serial binding, flag it.
