# F-bonding — LACP / XOR / round-robin / active-backup / broadcast bonds

Branch `task/F-bonding` (worktree `/root/ngfw-wt/F-bonding`), base `task/W-seed@df67a8e` (speculative, D-114), slot 6.
Contract changes: `F-bonding-contract.md`. Decisions and open questions: `F-bonding-questions.md`. WIP log: `F-bonding-wip.md`.

## What was built

| layer | what |
|---|---|
| schema (contract) | `interfaces.<BondEthernet<id>>.bond {mode, loadBalance?, members{<if>: {passive, longTimeout, weight?}}, numaOnly, id?}` in `domains/ext/bonding.ts`; 8 semantic rules `interfaces.bonding-*` (name/id, member exists, member kind, one bond per member, no L3 on members, loadBalance only xor/lacp, passive/longTimeout only lacp, weight only active-backup) |
| proto (contract) | `Interface.bond = 13` (`Bond`, `BondMember`), `rpc BondState` (`BondStateRequest/Response`, `BondStatus`, `BondMemberStatus`, `BondLacpState`, `BondLacpPort`), fake-agent stub, proto.md §11 |
| agent | `bond.member-weight` descriptor (dfkit spec, real Retrieve from `sw_member_interface_dump.weight`); `bond.bond` provides `interface/<name>` (KeyProvider, correct delete order); `desired.Bonds` / `desired.AssembleBonds` + `KindBond`; registry (`subsystems/bonding.go`, persisted claim store asserted) + projection hooks; `BondState` RPC (`rpc_bonding.go`: bond, member and LACP dumps, state machines decoded); coretest bond + lacp model |
| API | `GET /api/v1/state/interfaces/bonds` (`BondingController`, operationId `Bonding_bonds`): BondState merged with running + candidate, `live:false` on an agent without the RPC; config through the generic pointer routes |
| UI | **Interfaces → Bonds** (`/interfaces/bonds`): list with mode, load balance, status, active members and per-member LACP chips; drawer with live state, schema-driven bond form, members table + schema-driven member dialog, picker filtered to eligible NICs; en + fa (`bonding` namespace) |
| docs | `docs/user/interfaces/bonding.md` (LACP with 2 members, active-backup, CLI equivalent), see-also line in `basics.md`, `docs/agent/descriptors/bond.md` |
| host test | `test/topology/bonding` (own module): real agent + API + slot DB, fixture taps as NICs, no packets, no trace, no binding sweeps |

## Acceptance

All host evidence below is from `TestBondingOnHost` (`test/topology/bonding`, run with `run.sh`, slot 6, 2026-09-24 23:43,
host VPP 26.06, `PASS (41.75s)`); the earlier identical run at 23:31 also passed. Member NICs are fixture taps (untagged,
ids 6000–6002, host side `w6bm0…2` kept down): **no packet was sent, nothing was traced (D-128), no binding was swept
(D-126)**; the read-only V19 pre-flight and a read-only binding check ran first.

```
V19 pre-flight ok: no classify binding or classify DPO points at a missing table (0 warning(s))
systemctl show vpp -p NRestarts (before) = 1
binding check (read-only): tap6000 sw_if_index=4: classify l2/ip4/ip6=~0, no ACL, no SPD   (tap6001, tap6002 likewise)
...
systemctl show vpp -p NRestarts (after) = 1
--- PASS: TestBondingOnHost (41.75s)
```

- [x] **`vppctl show bond details` shows the committed LACP bond with both members and lb l34; Retrieve == desired**
```
vppctl show bond details:
BondEthernet6000
  mode: lacp
  load balance: l34
  number of active members: 0
  number of members: 2
    tap6000
    tap6001
  interface id: 6000
BondEthernet6001
  mode: active-backup
  load balance: active-backup
  number of active members: 1
    tap6002
      weight: 200, is_local_numa: 0, sw_if_index: 2
  interface id: 6001
vppctl show interface address BondEthernet6000:
BondEthernet6000 (up):
  L3 10.6.10.1/24
GET /state/drift → {"subsystems":["interfaces","vrfs","routing"],"changes":[],"ignored":[…unimplemented domains…]}
BondEthernet6000 bond: Retrieve={"loadBalance":"l34","members":{"tap6000":{"longTimeout":false,"passive":false},"tap6001":{"longTimeout":false,"passive":true}},"mode":"lacp","numaOnly":false}
                       running={"loadBalance":"l34","members":{"tap6000":{"longTimeout":false,"passive":false},"tap6001":{"longTimeout":false,"passive":true}},"mode":"lacp","numaOnly":false}
BondEthernet6001 bond: Retrieve={"members":{"tap6002":{"longTimeout":false,"passive":false,"weight":200}},"mode":"active-backup","numaOnly":false}
                       running={"members":{"tap6002":{"longTimeout":false,"passive":false,"weight":200}},"mode":"active-backup","numaOnly":false}
```
  LACP (no partner on taps, as expected — questions Q2): the evidence is configuration + `show lacp`:
```
                                                        actor state                      partner state
interface name            sw_if_index  bond interface   exp/def/dis/col/syn/agg/tim/act  exp/def/dis/col/syn/agg/tim/act
tap6001                   1            BondEthernet6000   1   1   0   0   0   1   1   0    0   0   0   0   0   0   1   0
  LAG ID: [(ffff,02-fe-57-d0-6b-ca,0009,00ff,0002), (ffff,00-00-00-00-00-00,0009,00ff,0002)]
  RX-state: EXPIRED, TX-state: TRANSMIT, MUX-state: DETACHED, PTX-state: NO_PERIODIC
tap6000                   4            BondEthernet6000   1   1   0   0   0   1   1   1    0   0   0   0   0   0   1   0
  LAG ID: [(ffff,02-fe-57-d0-6b-ca,0009,00ff,0001), (ffff,00-00-00-00-00-00,0009,00ff,0001)]
  RX-state: EXPIRED, TX-state: TRANSMIT, MUX-state: DETACHED, PTX-state: PERIODIC_TX
```
  (tap6001 is `passive`: actor `act` 0 and no periodic LACPDUs; `GET /state/interfaces/bonds` reports the same through
  `BondState`: `"rxState":"expired","muxState":"detached"`, partner system `00:00:00:00:00:00`.)

- [x] **Agent-restart simulation → bond + members + bond address back within 30 s** (agent stopped by PID; memberships,
  addresses and bonds deleted behind its back through binapi, members first — D-095 c; agent started; no config API call)
```
loss: bond_detach_member tap6000 (sw_if_index 4) from BondEthernet6000 → ok
loss: bond_detach_member tap6001 (sw_if_index 1) from BondEthernet6000 → ok
loss: bond_detach_member tap6002 (sw_if_index 2) from BondEthernet6001 → ok
loss: sw_interface_add_del_address BondEthernet6000 del_all → ok
loss: bond_delete BondEthernet6000 (sw_if_index 9) → ok
loss: sw_interface_add_del_address BondEthernet6001 del_all → ok
loss: bond_delete BondEthernet6001 (sw_if_index 10) → ok
agent log: {"time":"2026-09-24T23:43:31.86402377+03:30","msg":"reconcile start","mode":"resync","domains":["interfaces","vrfs","routing"]}
agent log: {"time":"2026-09-24T23:43:31.963503216+03:30","msg":"created","key":"bond.bond/BondEthernet6000"}
agent log: {"time":"2026-09-24T23:43:32.020457697+03:30","msg":"created","key":"bond.bond/BondEthernet6001"}
agent log: {"time":"2026-09-24T23:43:32.025587192+03:30","msg":"created","key":"bond.member/BondEthernet6000/tap6000"}
agent log: {"time":"2026-09-24T23:43:32.02721689+03:30","msg":"created","key":"bond.member/BondEthernet6000/tap6001"}
agent log: {"time":"2026-09-24T23:43:32.031701324+03:30","msg":"created","key":"bond.member/BondEthernet6001/tap6002"}
agent log: {"time":"2026-09-24T23:43:32.035053183+03:30","msg":"created","key":"bond.member-weight/BondEthernet6001/tap6002"}
agent log: {"time":"2026-09-24T23:43:32.066945322+03:30","msg":"reconcile done","mode":"resync","status":"APPLY_STATUS_APPLIED","summary":"created:11 unchanged:6","duration":202911424}
restart: bonds + members + weight + address back 0.413 s after the agent start (no config API call)
```
  After the restart `show bond details` again shows both bonds with their members and weight 200, and `/state/drift` is empty.

- [x] **Rollback removes bond and memberships; members return to plain L3 interfaces (Retrieve, not assumption)**
```
POST /config/rollback/1 → status applied, revision 3, summary {created:0 updated:0 deleted:9 unchanged:6 failed:0 reverted:0}
  results in order: delete bond.member-weight/BondEthernet6001/tap6002, bond.member/BondEthernet6001/tap6002,
  bond.member/BondEthernet6000/tap6001, bond.member/BondEthernet6000/tap6000, interface.admin-state/BondEthernet6001,
  bond.bond/BondEthernet6001, interface.admin-state/BondEthernet6000, interface-ip/BondEthernet6000/10.6.10.1/24,
  bond.bond/BondEthernet6000                       ← address before bond: bond.bond provides interface/<name> (F-B4)
Retrieve tap6000 after rollback: {"enabled":true,"promiscuous":false,"vrf":"default"}      (tap6001, tap6002 likewise)
vppctl show mode tap6000: l3 tap6000                                                        (tap6001, tap6002 likewise)
vppctl show bond (after rollback):
interface name   sw_if_index  mode          load balance  active members members
```
  (BondEthernet600x are no longer listed in `/state/interfaces`; `/state/drift` after the rollback is empty.)

- [x] **Member already in another bond → 400 problem+json with `pointer` to the second membership**
```
commit with tap6001 in two bonds → 400 {"type":"https://vrx.dev/problems/validation","title":"Validation failed","status":400,
  "tier":"semantic","warnings":[],"detail":"semantic validation failed","instance":"/api/v1/config/commit",
  "errors":[{"pointer":"/interfaces/BondEthernet6001/bond/members/tap6001",
             "message":"'tap6001' is already a member of BondEthernet6000 (/interfaces/BondEthernet6000/bond/members/tap6001); an interface belongs to at most one bond"}]}
```
  Also covered by the API e2e (`apps/api/test/e2e/bonding.e2e.test.ts`, 6/6 with the fake agent on the slot DB) and the
  agent's own DryRun check (`TestBondingValidation`: the same pointer with rule `interfaces.bonding-member-unique`).

- [x] **UI screenshot against the real endpoint** — production build under `vite preview` on the slot web port, real
  API + agent + VPP, one pending change (BondEthernet6000 numaOnly) for the pending mark. Taken with headless
  Chrome-for-Testing 154 + playwright-core from the npx cache, script and browser kept in the worker scratchpad (nothing
  installed system-wide, nothing committed but the PNGs; Playwright is not installed on the host):
```
screenshots:
  bonds-list-en.png + bonds-drawer-en.png  html dir/lang=ltr/en  rows=["BondEthernet6000","BondEthernet6001"]  pageErrors=0
  bonds-list-fa.png + bonds-drawer-fa.png  html dir/lang=rtl/fa  rows=["BondEthernet6000","BondEthernet6001"]  pageErrors=0
```
  `F-bonding-screens/bonds-list-en.png`, `bonds-drawer-en.png`, `bonds-list-fa.png`, `bonds-drawer-fa.png`.

- [x] **`tools/ci.sh --base main` green in the worktree** (`TMPDIR=/tmp/g-w6`, at 7131792; docs-only commits after it)
```
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m03s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   2m11s
  forbidden patterns (+ gitleaks)                    0m05s
  lint · typecheck · unit tests · build (turbo)   3m44s
  apps/agent: make lint test build                   1m19s
  apps/cli: make lint test build                     0m21s
  test/ Go modules, unit mode (test/integration/smoke test/topology/bonding test/topology/interfaces)   0m11s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(W-seed): verify
  mode quick · wall time 7m57s · logs /root/ngfw-wt/logs/ci/F-bonding-20260924-233408-709209
CI GATE PASSED
```
  (the warning is W-seed's commit, not this branch's)

### Tests added
| where | what |
|---|---|
| `packages/schema/src/semantic/bonding.test.ts` | schema defaults/idempotence, rejects, `bondIdOf`, every rule with its pointer, proto fixture clean (12 tests) |
| `apps/agent/internal/descriptors/bond/weight_test.go` | weight Create/Retrieve/Update/Delete, normalisation, 0 refused, xor refused (INVALID_ARGUMENT), detached member, KeyProvider |
| `apps/agent/internal/desired/bond_test.go` | `KindBond`, alias creator only with a bond leaf, builder objects + pointers, 11 error cases, assembler canonical form |
| `apps/agent/internal/agent/rpc_bonding_test.go` | on the coretest model with DF-1's real descriptors: apply (pointers), untagged members claimed not tagged, Retrieve == canonical, re-apply empty, BondState (LACP decode, names filter, owner check), rollback order + claims released, loss + resync; active-backup weights (in-place update), mode change re-creates the bond with members/address; 9 validation pointers; pre-existing `BondEthernet<id>` |
| `apps/agent/internal/subsystems/bonding_test.go` | registry/domain + persisted claim store guard |
| `apps/api/test/e2e/bonding.e2e.test.ts` | 6 e2e: commit + live state, duplicate membership 400, mode-option pointers, removal, no RPC (`live:false`), auth |
| `apps/web/src/domains/interfaces/bonding/BondsPage.test.tsx` | rows/paging, eligible members, statuses, forms from the one schema; screen: list, drawer, picker offers only eligible NICs, add member (unconfigured NIC added enabled), remove member, bond edit sends only the change, add bond, fa/RTL + readonly + nav |
| `test/topology/bonding` | the host test above |

## Shared hunks (all pure insertions under the `wave-A: F-bonding` anchors, 189 lines, 0 deletions)
| file | hunk |
|---|---|
| `packages/schema/src/domains/interfaces.ts` | import of `interfaceBondField` (top of file, tagged `// wave-A: F-bonding`; no import anchor exists) + `bond: interfaceBondField,` |
| `packages/schema/src/index.ts` | `export * from './domains/ext/bonding.js';` |
| `packages/schema/src/semantic/index.ts` | import `bondingValidators` + `...bondingValidators,` |
| `packages/proto/vrx/v1/dataplane.proto` | `rpc BondState` (service anchor), `Bond bond = 13;` (Interface anchor), `// ----- F-bonding -----` messages |
| `docs/contracts/proto.md` | `### F-bonding: BondState (+ Interface.bond 13)` |
| `apps/api/src/testing/fake-agent.ts` | `bondState` UNIMPLEMENTED stub (real fake: `features/bonding/fake.ts`, `installBondingFake`) |
| `apps/api/src/app.module.ts` | import `bondingFeature` + `...bondingFeature.controllers,` + `...bondingFeature.providers,` |
| `apps/api/src/agent/agent.client.ts` | `type BondStateResponse` import + `bondState()` method |
| `apps/agent/internal/desired/interfaces.go` | `KindBond` const, `KindOf` case (`BondID(name)`), creator case (`bond.bond/<name>` only with a bond leaf) |
| `apps/agent/internal/subsystems/subsystems.go` | `bondingBond`, `bondingMember`, `bondingWeight` in `Domains[Interfaces]`; `w.registerBonding(r)` at the end of `Register()` |
| `apps/agent/internal/agent/projection.go` | `desired.Bonds(p, ds.GetInterfaces())` in `project()`; `desired.AssembleBonds(ds, kvs, stored, nameOf)` in `assemble()` |
| `apps/web/src/router.tsx` | lazy route `interfaces/bonds` |
| `apps/web/src/nav/nav.ts` / `nav.test.ts` | `bonds` item in the Interfaces group (label `bonding:nav`) / `'bonds'` in the available list |
| `apps/web/src/i18n.ts` | en/fa imports, `'bonding'` namespace, `bonding:` in en and fa |
| `docs/user/interfaces/basics.md` | one see-also line at the end ("Not in this release" untouched) |
| `apps/web/src/domains/interfaces/model.ts` | the one named line `delete props['bond']` (the drawer broke: F-B7) |
| `apps/agent/internal/descriptors/core/coretest/fakevpp.go` | `v.installBonding()` in `New()` (F-B8; A6 read-only rule could not be kept, as for F-bridge-l2) |
| generated | `apps/agent/gen`, `packages/proto/gen/ts`, `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go` (`Bonding_bonds`) — regenerate on merge (rule 3) |

## Out of scope (not built)
VLAN sub-interfaces on bonds beyond showing they work through DF-1/P08 (unit-tested: create, retrieve, recreate with the
bond; removal ordering limit Q4); bridge domains; LLDP; linux-cp pairs for bonds; DPDK binding / startup.conf; GSO; MAC
override beyond `interface.mac-address`; no second bond descriptor (only `bond.member-weight` is new); a CLI `show bonds`
command (Q5).

## Open questions / decisions
See `F-bonding-questions.md`: F-B1 bond names `BondEthernet<id>`; F-B2 examples in the proto fixture; F-B3 weight 1–255;
F-B4 KeyProvider (please confirm for the D-125 merge order); F-B5 fixture taps; F-B6 weight registered in subsystems;
F-B7 drawer exclusion; F-B8 fakevpp line; F-B9 no react-hook-form; Q1 broadcast mode; Q2 LACP without partner; Q3 D-126
vs the agent's sanitizer; Q4 sub-interface removal ordering (TD-11c); Q5 CLI `show bonds`; Q6 base W-seed, not rebased.

## Cleanup (after the last host run)
```
vppctl show bond                              → header only (no BondEthernet6xxx)
vppctl show interface | grep tap60xx/BondEthernet6xxx → none
ip -o link | grep ': w6'                      → none (fixture taps' host devices gone)
processes (vrx-agent / vrx-api / vite preview of slot 6) → none (all stopped by PID by the test)
vrx_w6 database/role                          → dropped by pg-test.sh ("nothing named vrx_w6 / vrx_w6 remains")
lab lock                                      → released (held only during the runs, D-094)
systemctl show vpp -p NRestarts               → 1 before and after every host run of this task
apps/agent/bin, */dist                        → removed at the end
```
