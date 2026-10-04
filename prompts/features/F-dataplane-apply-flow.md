# Task: F-dataplane-apply-flow — install the VPP start-up file from the UI/API (admin-only, audited, root executor)   (prepend 00-CONTEXT.md)

## Goal
Implement **the apply step of System › Dataplane** end to end in FAST MODE: an admin presses "Install and restart the engine",
the API gets a **root-issued, single-use approval token** bound to the rendering's sha256, and a **root executor unit** runs
`apply-startup.sh --mode product` with that sha256. Node never execs anything and never talks to VPP. Reference behaviour: TNSR
"dataplane … then restart the dataplane service" (WBS D0.6 in `plan/wbs.csv`). Decision D-196; debt TD-H10; review findings
RV-B R2 TD-17 #2 / R8 TD-17 #2 (operator confirmation ≠ approval; the API trigger must be admin-only, audited, sha256-bound,
a separate root unit) and RV-B R3 NIT 12 (state whether the API preview sha256 equals the script's `--expect-new-sha256`).
**High-risk row (security + writes /etc on an appliance): the manager spawns two reviewers.**

## Inputs to read first
- `docs/status/tasks/S-dataplane-ui-fixes-questions.md` Q2, `docs/status/tasks/S-dataplane-ui-fixes.md`, D-196 in `docs/decisions/LOG.md`
- `docs/tech-debt.md` TD-H10 (token: root 0600 under `/var/lib/ngfw/startup-apply/approvals/`, bound to `DataplanePreviewOut.sha256`)
- `docs/status/tasks/RV-B-review-R2.md` (TD-17 #2), `RV-B-review-R3.md` (NIT 12), `RV-B-review-R8.md` (TD-17 #2, #5), `S-apply-startup-mode-review.md` (#5)
- `deploy/vpp/apply-startup.sh` header :1-90 (product gate: marker `/etc/ngfw/appliance`, installed copy in `/usr/lib/ngfw/bin`, lab
  handover check; `--approve-rendering` must equal `--expect-new-sha256`; exit 3 = refused before any change) and
  `docs/agent/renderers/vppstartup.md` § "Product mode"; the package wrapper `ngfw-apply-startup` (`deploy/debian/build.sh` :117-127)
- the hook pattern `deploy/systemd/ngfw-power@.service` (a fixed, minimal root unit); `deploy/debian/ngfw/debian/ngfw-agent.tmpfiles` and
  `deploy/systemd/ngfw-api.service` (`/var/lib/ngfw` root 0755; the API user `ngfw` owns only `StateDirectory=ngfw/api`, 0700)
- `apps/api/src/features/dataplane/**` (preview route, `toDataplanePreview` sets `applyAvailable: false` :99), `apps/api/src/features/capture-trace/controller.ts`
  (`@MinRole('admin')` + `AuditService.write` pattern), `apps/api/src/auth/route-guard.test.ts` (`ADMIN_ONLY` pins)
- `apps/web/src/domains/system/dataplane/DataplanePage.tsx` (:201-206 disabled button), `apps/web/src/locales/{en,fa}/dataplane.json` (`apply.*`)
- `apps/agent/internal/agent/rpc_dataplane_startup.go` :186-226 and `apps/agent/cmd/ngfw-startupgen/main.go` (two rendering paths, NIT 12)

## Contract changes
None in `packages/schema` / `packages/proto` (the apply never crosses the API↔agent boundary). The OpenAPI grows by the new routes:
regenerate `packages/api-client` in its own `contract(api-client): …` commit, then `make -C apps/cli gen` (and `make -C apps/cli docs` if
its test says the reference is stale) — generated files are never hand-edited.

## Scope — build exactly this
1. **Root executor** `deploy/vpp/apply-executor.sh` + `deploy/vpp/apply-executor.{path,service}` (or a socket-activated pair — log the
   choice with options): the API writes a JSON request file (`O_EXCL`, 0600, atomic rename) into a spool dir it owns (default
   `/var/lib/ngfw/api/dataplane-apply/`, env-overridable); the root unit validates it strictly (fixed keys, `^[0-9a-f]{64}$` sums,
   integer revision, bounded size; content never reaches a shell) and answers into the same spool. Two verbs:
   - `issue`: dry-run `ngfw-apply-startup --doc <running doc>` (product mode), require its rendered sha256 == the requested sha256, write an
     approval record (root 0600, `/var/lib/ngfw/startup-apply/approvals/<sha256(token)>`: rendering sum, live sum, revision id, requesting
     admin, expiry ≤ 10 min) and return the token (≥ 128 bits from `/dev/urandom`) once;
   - `apply`: atomically move the record to `spent/` (a replay, an expired or unknown token → refused, logged), re-check revision + sums,
     then exec `ngfw-apply-startup --doc … --apply --expect-sha256 <live> --expect-new-sha256 <sum> --approve-rendering <sum>`; copy the
     work-dir result marker (`committed | rolled-back | console-needed`, exit 3 + gate line) into the answer.
   It also publishes an `available` answer (product gate dry-run passes, marker present). **Never** edits `apply-startup.sh`.
2. **API** `apps/api/src/features/dataplane/apply*.ts` (own `ApplyController`, registered in `index.ts`):
   `POST /api/v1/actions/dataplane/apply/approval {sha256}` → token + expiry; `POST /api/v1/actions/dataplane/apply {token}` → accepted;
   `GET /api/v1/state/dataplane/apply` → availability + last apply (state, gate/refusal line, stamp). Both POSTs `@MinRole('admin')`,
   audited (request, spend, observed result; resource `dataplane/startup`, sums and revision only — never the token). Only a **committed**
   revision is applied: an uncommitted `dataplane` change in the candidate → 409 problem+json pointer `/dataplane`. No executor answer
   within a bound → 503. `applyAvailable` in the preview = the executor's `available` answer (false on any lab/dev host).
3. **NIT 12**: a test-only file `apps/agent/cmd/ngfw-startupgen/preview_parity_test.go` proves that `run()` and the library path the agent's
   preview uses (`vppstartup.ReadHost` + `Generate`) give the same sum for the same document on the same fake host tree; where they can
   differ (ngfw-startupgen's management-NIC detection, `--current` switches) the approval binds the **executor's** dry-run sum, the API
   returns both sums, and a mismatch is refused (409). Write the statement into the user doc. No agent product code changes.
4. **UI** `apps/web/src/domains/system/dataplane/Apply*.tsx`: enable the button only for admins when `applyAvailable`; dialog with the
   diff, both sums, the restart/forwarding-interruption warning and type-the-first-8-hex confirmation; then progress from
   `GET /state/dataplane/apply`. en + fa strings in `dataplane.json` (fa glossary «ثبت» for commit, D-196; engine wording, D-155 —
   `locale.test.ts` must stay green).
5. **Packaging + docs**: install the executor and its units through `deploy/debian/build.sh` + `deploy/debian/ngfw/debian/ngfw-agent.install`
   + tmpfiles lines (spool dir owned by `ngfw`, approvals/spent root 0700); `docs/user/system/dataplane.md` apply section (who may press
   it, what the token binds, what happens on failure, CLI equivalent `ngfw-apply-startup`); replace the "none exists" sentence of the
   "Any future API/UI trigger" bullet in `docs/agent/renderers/vppstartup.md`.

## Acceptance (paste the evidence)
- [ ] Executor test (bash, fake root like `deploy/vpp/test-apply-startup.sh` / `NGFW_TEST_ROOT`, a fake `ngfw-apply-startup` that records
      argv): issue → apply runs with exactly the four sha flags; replayed / expired / unknown token, sum mismatch, malformed request → refused.
- [ ] API tests: operator → 403 on both POSTs, admin → token; uncommitted candidate → 409 `/dataplane`; no executor → 503 and
      `applyAvailable: false`; three audit rows written; route-guard pins added.
- [ ] Web tests: button disabled for non-admin / unavailable; confirmation needs the 8 hex; en/fa parity.
- [ ] Slot run (`eval "$(tools/lab env <N>)"`, API on your slot port, spool under `/run/ngfw-test/w<N>/`, executor in test mode): the
      curl sequence approval → apply → state, pasted; plus one read-only dry run `deploy/vpp/apply-startup.sh --mode product --doc <doc>`
      from your worktree (no `--apply`) showing ngfw-a refuses product mode (exit 3, gate line pasted).
- [ ] `tools/ci-slot.sh --base main` green in your worktree.

## Out of scope (do not build)
Running any real `--apply`, starting or installing the executor units on this host, touching `/etc/vpp/startup.conf`, `vpp.service` or
`/etc/ngfw` (handover pending, D-012); any change to `deploy/vpp/apply-startup.sh` (M-origin-sync touches it — merge main instead);
live VPP readings (TD-H13), the gRPC status-details pointer (S-dataplane-ui-fixes Q3), a second approver / four-eyes flow, scheduling
an apply, agent product code or proto changes, P10 unit hardening (F-hardening-lite).

## Open questions to surface, not to decide silently
- Spool + path unit vs a socket-activated root unit (pick one, log options). Token TTL (default 10 min) and whether the same admin must
  spend it. Whether the 503 bound should be configurable.

## Files you own
`apps/api/src/features/dataplane/apply*` · `apps/api/src/features/dataplane/index.ts` (register the controller) ·
`apps/api/src/features/dataplane/dataplane.controller.ts` (`applyAvailable` hunk) + `dataplane.test.ts` (its expectation) ·
`apps/api/src/auth/route-guard.test.ts` (two `ADMIN_ONLY` lines) · `packages/api-client/src/generated/**` + `apps/cli/internal/api/operations_gen.go`
+ `docs/user/cli/reference.md` (regenerated only) · `deploy/vpp/apply-executor*` · `deploy/debian/build.sh` (executor stage lines) ·
`deploy/debian/ngfw/debian/ngfw-agent.{install,tmpfiles}` (executor lines) · `apps/web/src/domains/system/dataplane/Apply*` ·
`apps/web/src/domains/system/dataplane/DataplanePage.tsx` (button hunk) · `apps/web/src/locales/{en,fa}/dataplane.json` (`apply.*`) ·
`apps/agent/cmd/ngfw-startupgen/preview_parity_test.go` (new, test only) · `docs/user/system/dataplane.md` (apply section) ·
`docs/agent/renderers/vppstartup.md` (that one bullet) · `docs/status/tasks/F-dataplane-apply-flow*`

## Rules
- Files you own: above. Everything else read-only; a needed edit elsewhere → `docs/status/tasks/F-dataplane-apply-flow-questions.md`.
- Shared VPP: read-only for this row (the preview reads files); never restart or kill VPP; `timeout 10` on every vppctl; packet trace banned (D-128).
- D-210a: write tests for your change and get them passing in your package (paste output); no full suite, no lint, no other packages' tests;
  run them through `tools/heavy.sh` (D-224), e.g. `tools/heavy.sh pnpm --filter @ngfw/api exec vitest run src/features/dataplane` and,
  from apps/agent, `../../tools/heavy.sh go test ./cmd/ngfw-startupgen/...`.
- Contracts: additive only — `contract(api-client): …` regeneration commit; no schema/proto change; renaming/reshaping = PENDING (decision-policy #1).
- Web parts: only after M-origin-sync landed (`git merge-base --is-ancestor origin/main main` succeeds in /root/NGFW); then `git merge main` into
  your branch and follow the product wording rules (packages/ui-kit/src/i18n/product-wording.ts, docs/status/tasks/network-defaults-web.md —
  they arrive with that merge).
- Secrets: the token is a credential — never logged, never in audit rows, status files or fixtures (redact as `<redacted>`).
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never tools/ci.sh directly), commit on your branch,
  `docs/status/tasks/F-dataplane-apply-flow.md` with pasted real output.
