# Task: F-bruteforce-detectors-b — push the live auto-block set to the data plane (enforcement half)   (prepend 00-CONTEXT.md)

> Split of board row F-bruteforce-detectors (D-171). **-a must be merged first** (contract EventKind 18, detectors, API
> subscriber, slot driver up to the API's block list). This row closes the loop: 10 bad logins → the source is dropped by the
> box, removal/expiry re-permits, an allow-listed source never reaches the data plane.

## Goal
Today a block lives only in the API's `auto_block` table: nothing hands it to the agent, so neither the VPP `_gb.*` deny ACLs
nor the nftables `b4_bad` set ever contain it (`grep -rn autoBlock apps/api/src` finds no consumer outside
`features/auto-block`). Build the push the design always assumed — "a system-owned, TTL'd Global Blocking entry, no config
commit per block" (`docs/status/tasks/F-bruteforce-block.md:3-4`, `docs/user/security/auto-block.md:7`) — and prove it on a
slot. Also own TD-H2 (`docs/tech-debt.md`: Go host test of the `_gb.*` descriptor on real VPP, owner "the next row touching
`desired/global_blocking.go`").

## Inputs to read first
- `docs/status/tasks/F-bruteforce-detectors-a.md` (what -a built) and `docs/status/tasks/F-bruteforce-block-host.md` (1a/1b/1c
  enforcement evidence, TD-H2 gap, the `b4_bad` / `in__gb` shapes).
- API: `apps/api/src/features/auto-block/auto-block.service.ts` (`block`, `unblock`, `manualBlock`, `sweep`, `list`),
  `apps/api/src/agent/agent.client.ts` + `apps/api/src/testing/fake-agent.ts` (anchored feature hunks), the reconnect handling
  in `apps/api/src/telemetry/relay.service.ts` (read only).
- Agent: `apps/agent/internal/desired/global_blocking.go` (`aclExpander.globalBlocking`, `reconstructGlobalBlocking` :267),
  `desired/hostacl.go` + `renderers/nftables/blocking.go` (the `b4_bad` set), D-172 (always-on `acl-plugin-in/out-ip4-fa` deny
  counters instead of the global stats flag), TD-H4 (never flip `acl_stats_intf_counters_enable`).
- `docs/status/wave-BC-numbers.md` (rules: RPC and message names start with the feature noun; new messages in the task's
  `// ----- <task-id> -----` section at the end of the proto).

## Contract changes
Decide the transport and log it with its options in your status file (it is yours to decide, 2× rule):
(a) **additive RPC (recommended)** `AutoBlockSync(AutoBlockSyncRequest{owner, generation, entries})` → `AutoBlockSyncResponse`
in a `// ----- F-bruteforce-detectors-b -----` section; names/numbers proposed in your own `### F-bruteforce-detectors-b`
section of `docs/status/wave-BC-numbers.md`, confirmed by the manager in your envelope; commit first on
`contract/F-bruteforce-detectors-b` (`contract(proto): …`, regenerated stubs, `-contract.md`, questions file, keep going);
(b) a `CommitService.systemCommit` of a reserved list — rejected by the design note above and `systemCommit` defers while an
admin edits the candidate (a brute force would go unblocked); only if you can show neither problem applies.

## Scope — build exactly this
1. **API push** (`features/auto-block/**`, one anchored method hunk in `agent.client.ts` and `fake-agent.ts`): after every
   change of the live set (block, unblock, manual block, expiry sweep) send the full current set with a monotonic generation,
   coalesced (≤ 1 send per second), and resend it on every agent (re)connect. Allow-listed and loopback sources can never be in
   the set (re-check at send time too — anti-self-lockout, RV-H2 R2). Unit tests with the fake agent.
2. **Agent side** (`agent/rpc_autoblock*.go`, `subsystems/autoblock*.go` from -a, `desired/autoblock*.go`): owner-checked
   handler; keep the last set per owner (bounded by `security.autoBlock.maxEntries`), ignore an older generation, then
   `RequestResync()`; the projection adds the set as a runtime Global Blocking list (a reserved name the schema cannot produce,
   enabled, every L3 interface of the owner, inbound, `protectHost: true`) before the ACL and host-firewall builders run (one
   anchored call in `agent/projection.go`), and `reconstructGlobalBlocking` skips it (one anchored hunk in
   `desired/global_blocking.go`) so `/state/drift` never shows it. Restart safety: after an agent restart the set is back within
   30 s (API resend on reconnect, or an agent-local 0600 journal — choose, log). Fake-VPP unit tests: set → `_gb.*` ACL entries
   + `b4_bad` elements; older generation ignored; empty set removes them; allow-listed entry refused by the API, never sent.
3. **TD-H2** (`test/topology/autoblock/`, `NGFW_INTEGRATION=1`): a Go host test that lets the agent emit `_gb.*` on the af_packet
   rig (`tools/lab rig up w<N>`, prefixed, your table range) and shows a listed source dropped on through-traffic (always-on
   acl-plugin deny counters, per-interface counters; no trace) and re-permitted when the list empties.
4. **End to end** (extend -a's driver): 10 wrong-password SSH logins → the source is in `b4_bad` (slot netns `nft list set`) and in
   the `_gb.*` ACL on VPP → its new connections fail; `POST /api/v1/actions/auto-block/unblock` (and, with a short `blockSec`,
   expiry) → removed from both, connections work again; an allow-listed source doing the same never appears in either.
5. **Docs**: `docs/user/security/auto-block.md` — enforcement now live; what "system-owned" means; CLI equivalent.

## Acceptance (paste the evidence; `.txt` under `docs/status/tasks/F-bruteforce-detectors-b-evidence/`)
- [ ] Blocked: `nft list set …b4_bad` (slot netns) and `vppctl show acl-plugin acl` (your `_gb.*` entry) contain the source;
      the next SSH attempt from it times out
- [ ] Unblock and expiry: both places empty again, SSH reaches sshd (log line)
- [ ] Allow-listed source: never in the API set, never in `b4_bad`, never in `_gb.*` (outputs before/after)
- [ ] TD-H2 test output (PASS) with the deny-counter delta; `/state/drift` clean while blocks exist
- [ ] Agent-restart simulation → set restored within 30 s; stale generation ignored (unit test output)
- [ ] NRestarts before/after every host step; own unit tests output; `tools/ci-slot.sh --base main` tail

## Out of scope (do not build)
Detectors, EventKind 18, the API subscriber (that is -a; fix only a proven defect, name the test) · threshold/escalation changes ·
user-visible Global Blocking UI changes · a config-document field for blocked entries (runtime state, D-171) · flipping the VPP-
global acl stats flag (TD-H4) · the API→agent secret channel · `apps/web/**`, `packages/ui-kit/**`.

## Open questions to surface, not to decide silently
- Transport decision (a)/(b) and the manager's confirmation of the names/numbers.
- Whether the product agent should also persist the set (power loss while the API is down) — state the gap if you choose resend-only.

## Files you own
`apps/api/src/features/auto-block/**` · `apps/api/src/agent/agent.client.ts` and `apps/api/src/testing/fake-agent.ts` (one anchored
hunk each) · `packages/proto/ngfw/v1/dataplane.proto` (your section, contract commit) + regenerated stubs · `docs/status/wave-BC-numbers.md`
and `docs/contracts/proto.md` (own sections) · `apps/agent/internal/agent/rpc_autoblock*.go` · `apps/agent/internal/subsystems/autoblock*.go` ·
`apps/agent/internal/desired/autoblock*.go` · `apps/agent/internal/desired/global_blocking.go` and `apps/agent/internal/agent/projection.go`
(one anchored hunk each, `// wave-BC: F-bruteforce-detectors-b`, listed under `## Shared hunks`) · `test/topology/autoblock/**` ·
`docs/user/security/auto-block.md` · `docs/status/tasks/F-bruteforce-detectors-b*`.

## Rules
- Files you own: the list above. Everything else is read-only; a needed edit elsewhere → `docs/status/tasks/F-bruteforce-detectors-b-questions.md`.
- Shared VPP: slot prefix `w<N>` on every object, tables N000–N999 (`eval "$(tools/lab env <N>)"`); never restart or kill VPP;
  `timeout 10` on every vppctl; packet trace banned (D-128: no `trace add` / `show trace` / `clear trace`); never `vppctl delete
  host-interface` by hand (`tools/lab rig down` does it). Test sshd only in your slot netns, stopped by PID. Daemons: none.
- D-210a: write tests for your change and get them passing in your package; paste the output; no full suite, no lint, no other
  packages' tests. Run them through `tools/heavy.sh` (D-224), e.g. from `apps/agent`: `../../tools/heavy.sh go test ./internal/desired/... ./internal/agent/...`;
  API: `tools/heavy.sh pnpm --filter @ngfw/api exec vitest run src/features/auto-block`.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`;
  renaming/reshaping = PENDING (decision-policy #1).
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your
  branch, `docs/status/tasks/F-bruteforce-detectors-b.md` with pasted real output (D-175 evidence rules), stop every process you started.
