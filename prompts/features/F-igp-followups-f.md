# Task: F-igp-followups-f — HA configuration sync (D9.2): API-to-API push of the running revision minus syncExclude, cluster view, force-sync   (prepend 00-CONTEXT.md)

> Split 6 of 6 of board row F-igp-followups (D-173; RV-A "F-config-sync"). Needs -a merged (`HaCluster.syncExclude`); runs after -e (both touch
> `apps/web/src/domains/system/ha/HaPage.tsx`). The manager may promote this split to its own row `F-config-sync`.

## Goal
Today `ha.cluster` is stored and shown but nothing acts on it (S-rva-agent-gates: `agent.unsupported-field` at `/ha/cluster`). Build the sync:
after a successful commit on the primary, its API pushes the running document minus `syncExclude` (and minus every secret leaf) to the peer API,
which commits it as a revision of kind `cluster-sync`. Reference: TNSR "HA configuration synchronization"; WBS D9.2. API-only: no proto change,
no agent data-plane change (the agent never talks to the peer).

## Inputs to read first
- `docs/status/tasks/F-vrrp-config-sync.md` "Not built → Config sync (D9.2)" (the design: pinned peer certificate + cluster key, peer endpoint `kind: 'cluster-sync'`,
  `GET /api/v1/state/ha/cluster`, `POST /api/v1/actions/ha/sync`, local-unsynced-edits refusal, SY1 public-route entry)
- `docs/status/wave-BC-numbers.md` § F-vrrp-config-sync ("Config sync is API-to-API (no proto). Revisions from a peer carry `config_revision.kind = 'cluster-sync'`
  (text column — no migration)"), slot port offset TCP `3000+100·SLOT+50` = cluster node B API (slots 14–32: `NGFW_HTTP_PORT + 50`, shared-host-rules §1)
- `packages/schema/src/domains/ha.ts` (`HaClusterSchema`: peer, interface, vrf, port, `secretRef`, `syncExclude` from -a), `apps/api/src/db/schema.ts:109` (`kind` column)
- The commit engine `apps/api/src/commit/{commit.service.ts,validation.service.ts}` (read-only: find the post-commit hook/bus event other features use; if
  none exists, ask — do not edit the engine) and `apps/api/src/features/mgmt-tls/**` (certificate handling)
- `apps/api/src/auth/route-guard.test.ts` (`PUBLIC` :17 / `ADMIN_ONLY` :42 sets — SY1 rule: one anchored line per set), `docs/decisions/PENDING-secret-channel.md`
- `docs/user/system/vrrp-config-sync.md` (today: "`ha.cluster` is stored, not applied"), `docs/tech-debt.md` S-rva-web-fixes hand-off (3) (`secretRef` picker)

## Contract changes
None in proto. If the API needs a schema field beyond -a (e.g. a peer certificate fingerprint), commit `contract(schema): …` first with a number-free
additive key, `-contract.md`, questions file. Never reshape `ha.cluster`.

## Scope — build exactly this
1. **Peer trust**: the cluster key (`ha.cluster.secretRef`, resolved inside the API from the encrypted secret store) authenticates each push (HMAC over the body +
   timestamp, replay window ≤ 60 s); the peer's TLS certificate is pinned (SHA-256 fingerprint stored with the cluster config or learned once by an admin
   action — choose one, log it as a decision with options). Plain HTTP to the peer is refused.
2. **Push**: after every successful commit (and on `POST /api/v1/actions/ha/sync`, admin-only, audited) build running − `syncExclude` − every secret leaf
   − `ha.cluster` itself, send to `https://<peer>:<port>/api/v1/cluster/sync`; bounded timeout, retry with backoff, state recorded (last push, peer revision, error).
3. **Receive** (`POST /api/v1/cluster/sync`, in the SY1 `PUBLIC` set because it is key-authenticated, not session-authenticated): verify key + timestamp, refuse
   while the local candidate has unsynced edits (409 problem+json), otherwise validate (schema → semantic → agent DryRun) and commit as revision kind
   `cluster-sync` with the audit entry naming the peer; merge rule: the local `syncExclude`d pointers and secrets are kept from the local running document.
4. **State**: `GET /api/v1/state/ha/cluster` (members, local/peer revision, last sync, error, unsynced edits); the agent warning at `/ha/cluster` must stop
   claiming the field is ignored — ask the manager for that one line in `desired/vrrp.go` (owned by S-vrrp-product-fixes) via the questions file.
5. **UI** (after -e): cluster view on the HA page — members, revision per node, sync status, "Sync now" (admin, confirm), `syncExclude` editor, and a
   secret-reference picker for `ha.cluster.secretRef` (closes tech-debt hand-off (3)); en + fa.
6. **Tests**: API unit tests (HMAC, replay, pinning mismatch, exclusion + secret stripping, unsynced-edits 409, DryRun failure → no commit); ONE two-node run on your
   slot: API A on `NGFW_HTTP_PORT`, API B on the node-B port, each with its own slot database (`ngfw_w<N>` and a second slot-prefixed database; if
   `tools/slot-check.py` refuses the name → questions file), fake agents or your slot agent: commit on A → revision `cluster-sync` on B (psql + curl pasted),
   excluded pointer unchanged on B, secret leaves never in B's revision or any log.
7. **Docs**: rewrite the cluster part of `docs/user/system/vrrp-config-sync.md` (setup, key, pinning, exclusions, what is never synced, CLI equivalent).

## Acceptance (paste the evidence)
- [ ] own API + web tests green (paste)
- [ ] two-node run: push, receive, `cluster-sync` revision, exclusion kept, 409 on unsynced edits, wrong key → 401/403, pin mismatch → refused (paste)
- [ ] `grep -rn` of the two API logs for the test key value returns nothing (paste the command and empty result)
- [ ] `tools/ci-slot.sh --base main` green (tail pasted)

## Out of scope (do not build)
Syncing secrets (PENDING-secret-channel / secret-sync decision — excluded and stated); VRRP/keepalived behaviour and state (-d, S-vrrp-product-fixes);
HA session/state sync (F-ha-state-sync); more than two members; agent-to-agent traffic; any proto change; auto-failover of the API itself.

## Open questions to surface, not to decide silently
Pinning by fingerprint field vs trust-on-first-use action; whether the peer accepts pushes while its own VRs are Master; secret sync (owner, PENDING).

## Files you own
`apps/api/src/features/cluster-sync/**` (new), one `// wave-BC: F-igp-followups-f` line in `apps/api/src/app.module.ts` and in each `route-guard.test.ts`
set it needs, `packages/api-client/**` (generated), `apps/web/src/domains/system/ha/cluster/**` (new) + one anchored mount line in `HaPage.tsx`,
`apps/web/src/locales/{en,fa}/ha.json` (cluster keys), `docs/user/system/vrrp-config-sync.md` (cluster sections), `docs/status/tasks/F-igp-followups-f*`.

## Rules
- Files you own: above. Everything else read-only; a needed edit elsewhere → `docs/status/tasks/F-igp-followups-f-questions.md`.
- Web parts: only after M-origin-sync landed (`git merge-base --is-ancestor origin/main main` succeeds in /root/NGFW); then `git merge main` into your branch
  and follow the product wording rules (packages/ui-kit/src/i18n/product-wording.ts, docs/status/tasks/network-defaults-web.md — they arrive with that merge).
- Shared VPP: not used; never restart or kill VPP (any vppctl: slot prefix, `timeout 10`, packet trace banned — D-128). Slot ports/DB only (shared-host-rules §1); stop both API processes by PID before you finish.
- D-210a: write tests for your change and get them passing in your package (paste output); run them through `tools/heavy.sh` (D-224), e.g.
  `tools/heavy.sh pnpm --filter @ngfw/api exec vitest run <files>`; no full suite, no lint, no other packages' tests.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`; renaming/reshaping = PENDING.
- Secrets: test cluster key `NGFW_TEST_PSK_FIGPF_<n>` only; never a key, HMAC or certificate private key in a log, fixture or status file (`<redacted>`).
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your branch,
  `docs/status/tasks/F-igp-followups-f.md` with pasted real output.
