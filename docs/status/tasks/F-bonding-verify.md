# F-bonding: focused verify of fix round 1

Reviewer: the author of `F-bonding-review.md` (@ `7c7e9b0`). Checked: `task/F-bonding` @ `79a9ee1b` against the review's
findings and the manager's fix list (`/root/ngfw-wt/F-bonding.fix1.md`). Only unit and web tests were run. There were no
host runs and I did not touch VPP.

## Verdict: **APPROVE**

Every finding and every fix-list item is done. Each code fix has a test that the pre-fix code fails. The branch diff
against `main` contains F-bonding only. Two things are left for later: one LOW follow-up (F4 scope, below) and one rebase
note (F3 → TD-23). Neither blocks the merge.

## Tests run by the reviewer
```
apps/agent  go test -race -count=1 ./internal/descriptors/bond/ ./internal/subsystems/ ./internal/contracttest/ ./internal/desired/
            → ok ×4 (bond 1.2 s, subsystems 6.8 s, contracttest 2.2 s, desired 1.3 s)
            go test -race -count=1 ./internal/agent/  → ok 14.3 s   (TMPDIR=/tmp/g-rvb)
packages/schema  vitest run src/semantic/bonding.test.ts        → 15 passed (15)
apps/web    vitest run src/domains/interfaces src/nav          → interfaces: 3 files, 20 passed; bonding+nav: 2 files, 13 passed
            (incl. "D-132: one timer of at least 30 s; the drawer reads the grid cache; Refresh walks once")
```
- **CI.** I did not re-run CI. The worker's pasted gate (`CI GATE PASSED`, 12m56s, on the merged tree `d4f2955b`) has a
  matching log directory: `/root/ngfw-wt/logs/ci/F-bonding-20260925-041306-3149891`. Its `08-agent.log` shows `ok` for
  every package, including subsystems, and `07-turbo.log` shows `Tasks: 30 successful`. Commit `79a9ee1b` changes only
  `F-bonding.md` and `F-bonding-questions.md`.
- **Pre-fix failures.** I could not run mutation checks myself: exporting a scratch copy was not permitted. For those
  I rely on the worker's pasted pre-fix failures (`F-bonding.md`, "Fix round 1") and on reading each test against the old
  code.

## Findings verified

| item | fix (file:line) | test that fails on the old code | verified |
|---|---|---|---|
| **F1** `dropPhantomOptionals` | `BondDrawer.tsx:95` and `:119` now use `const cleaned = value`; the import is gone | none can fail: the call was a no-op for these forms. The existing "bond edit sends only the change" and member-patch tests stay green | ✓ `grep` finds no use in `bonding/`. `main` still exports the function (WEB-1 is not merged), so the branch no longer depends on WEB-1's merge order |
| **F2 web** (D-132) | `bonding/queries.ts:14` sets `BONDS_POLL_MS = 30_000`. The grid is the only timer (`BondsPage.tsx:223`). The drawer uses `useBondsCache` (`queries.ts:21`, `enabled:false`, no fetch of its own) and `useInterfacesOnce` (`:26`, `staleTime: Infinity`, no timer). Refresh buttons: `BondsPage.tsx:212`, `BondDrawer.tsx:150` | `BondsPage.test.tsx` "D-132: …" asserts `≥ 30 000` (old value 3 000). Opening the drawer must walk nothing; the old `useBondsState` fetched on mount. One click must give exactly one walk | ✓ Passes. `invalidateQueries(bondKeys.state)` prefix-matches the grid key (`[…,'bonds','grid']`), so the grid refetches once. The disabled cache query is not an "active" query and is not refetched, so there is no second walk |
| **F2 agent** (one walk at a time) | `rpc_bonding.go:44-48`, `:69-87`: a one-slot semaphore (`bondWalk`) around the whole `bondTable` (`sw_interface_dump` plus the bond/member/lacp dumps). A second caller waits up to 3 s (`bondWalkWait`), then gets `UNAVAILABLE`; a cancelled ctx gets its gRPC code | `TestBondStateOneWalkAtATime` (`rpc_bonding_test.go:301-349`). 6 concurrent callers see at most 1 `sw_bond_interface_dump` in flight (probe `coretest/bonding.go:55`, `SetBondDumpDelay`/`MaxBondDumpsInFlight`). A walk that outlasts the wait makes the second caller get UNAVAILABLE. Worker's pre-fix paste: `6 bond walks in flight at once, want 1` | ✓ Passes under `-race`. The API maps UNAVAILABLE (`agent.client.ts:208`) |
| **F3** coretest seam | `coretest/fakevpp.go`: F-nat44-ed's `extensions` seam, taken verbatim (+7 lines, the loop after `sanitizetest.Clean`). `coretest/bonding.go:86` has `func init() { extensions = append(extensions, (*VPP).installBonding) }` | refactor; every `coretest.New()` agent test covers it | ✓ **Rebase note for TD-23 (D-134):** take TD-23's `fakevpp.go` (drop this branch's seam hunk) and change `bonding.go:86` to `func init() { RegisterExtension("bonding", (*VPP).installBonding) }`. `installBonding` claims only bond-plugin messages (`bond_create2`, `bond_delete`, `bond_add_member`, `bond_detach_member`, `sw_interface_set_bond_weight`, `sw_bond_interface_dump`, `sw_member_interface_dump`, `sw_interface_lacp_dump`; `coretest/bonding.go:90-249`), so TD-23's `On()` collision guard cannot fire |
| **F4 API** | `ext/bonding.ts:44` `NON_ETHERNET_INTERFACE_RE` (loop, wg, ipip, gre, ipsec, vxlan_tunnel, vxlan_gpe_tunnel, gtpu_tunnel, geneve_tunnel, l2tpv3_tunnel, pppoe_session, mpls-tunnel, bvi, lisp_gpe, sr-tunnel) → `interfaces.bonding-member-kind` answers 400 with a pointer to the membership (`semantic/bonding.ts:44-50`). The builder mirrors it (`desired/bond.go:55`, `:157`) | `bonding.test.ts` "F4: tunnels …"; `bond_test.go` wg0 case | ✓ |
| **F4 agent** | `bond/member.go:64-66`: `ethernetMember()` (`:81-100`) runs before `bond_add_member`. It refuses a sub-interface (`type == IF_API_TYPE_SUB` or `sup_sw_if_index != sw_if_index`), device class `Loopback` or `bond`, and an all-zero `l2_address` | `ethernet_test.go` `TestMemberMustBeEthernet`: wg0 (no L2 address), a sub-interface and loop9 are refused and **0** `bond_add_member` calls are sent; an Ethernet tap still joins. Worker's pre-fix paste: `member wg0: err = <nil>, want ErrNotEthernet` | ✓ It uses the `sw_interface_dump` row that Create already fetched, so there is no new VPP call. Sufficiency: see below |
| **F5** member `mac` | new rule `interfaces.bonding-member-mac` (`semantic/bonding.ts:175-196`, pointer `/interfaces/<member>/mac`), mirrored in the builder (`desired/bond.go:161-164`) | `bonding.test.ts` "F5: …"; `bond_test.go` F5 case | ✓ |
| **F6** | `docs/agent/descriptors/bond.md:9-13`: one table, 3 rows, no blank line | docs | ✓ |
| **F7** | "Add bond" treats live bond names as taken (`BondsPage.tsx:177`) and the `model.ts:40` comment is corrected; the status column is 170 px | model test | ✓ (nit below) |
| **Q1** `broadcast` dropped | `642995f9 contract(schema,proto)`: enum `ext/bonding.ts:28`, help text, proto comment `dataplane.proto:3578`, `proto.md`, regenerated. Builder `desired/bond.go:48-53`: not configurable. `BondModeName` still names VPP's value for display (`:74-76`) | `bonding.test.ts` "Q1: broadcast is not a mode"; `bond_test.go` broadcast → `interfaces.bonding-mode` | ✓ It removes values from an enum that was never merged. `git grep -i broadcast main` in packages/schema, packages/proto/vrx, packages/api-client, apps/api/src, apps/web/src, docs/user and docs/contracts finds nothing bond-related. The only merged mentions are DF-1's agent-internal model (`descriptors/bond/bond_model.proto`, `bond.go:56-68`) and its descriptor row (`bond.md:11`). They describe what VPP and the descriptor can do, not the configuration contract, so there is nothing to reshape |
| **M1** TD-11b | `bond.go`: `RecordsNoOwnership()` (the owner tag). `weight.go:64-66`: `RecordsNoOwnership()` (the weight lives in the membership that `bond.member` owns). `member.go:122-127`: `CheckPersistent()` = `persist.Require(…, iface.Claims(owner))` | `ownership_test.go` `TestOwnershipDeclarations`: exactly one declaration each; the in-memory store is refused and a persisted one accepted. TD-11b's registration guard runs in every `subsystems.Register` test (`ErrUndeclaredDescriptors`, `stores.go:482`) | ✓ Passes under `-race` |
| **main merge** | `git diff main...HEAD` (merge-base `10059d57` = TD-4) | — | ✓ 69 files, all F-bonding-owned or generated. Every shared-hotspot hunk (projection, interfaces.go, subsystems, coretest seam, api app.module/agent.client/fake-agent, web router/nav/i18n/model.ts, schema interfaces/index/semantic index, proto, proto.md, basics.md) is a pure insertion with **0 deleted lines**. `main` has 2 newer commits (status/board), which the squash rebase picks up |

## F4: is "has an L2 address" enough to mean "Ethernet"? It is enough to prevent the crash, not to prove the member is physical

`ethernetMember` reads four `sw_interface_details` fields: `type`, `sup_sw_if_index`, `interface_dev_type` and `l2_address`.

**Enough to prevent the crash.**
- VPP 26.06 fills `l2_address` only when `sup_sw_if_index == sw_if_index` **and** `hi->hw_class_index ==
  ethernet_hw_interface_class.index` (`vnet/interface_api.c:269-279`).
- So a non-zero value means an Ethernet hardware class, and that class always has `hw_address` set. The
  `memcpy(mif->persistent_hw_address, mif_hw->hw_address, 6)` in `bond_add_member` (`vnet/bonding/cli.c:793`), the crash
  that F4 was about, is therefore safe. L3 tunnels (ipip, GRE-L3, wireguard, ipsec) report zeros and are refused.

**Not enough to prove "physical NIC".** Some virtual interfaces are also registered as Ethernet, carry a MAC, and pass the
agent check:

| interface | device class | registered at |
|---|---|---|
| BVI | `"BVI"` | `l2_bvi.c:51` |
| VXLAN | `"VXLAN"` | `vxlan.c:123` |
| GENEVE | `"GENEVE"` | — |
| GRE in TEB/ERSPAN mode | — | `vnet_eth_register_interface`, `gre/interface.c:429` |
| Pipe | `"Pipe"` | — |
| vhost-user | — | — |
| memif (Ethernet mode) | — | `memif.c:1075` |
| Xcrw | — | — |

- Loopback is Ethernet too, but it is refused by its device class.
- The schema name rule catches `bvi<N>`, `vxlan_tunnel<N>`, `geneve_tunnel<N>` and `gre<N>` by VPP name. It cannot catch a
  logical (tagged) name that a later tunnel family gives its interface, nor pipe, vhost-user or memif.
- None of these hits the `hw_address` crash. As bond members they are semantically wrong and VPP does not test them.

**LOW follow-up (not blocking).** In `ethernetMember` (`member.go:91-97`), also refuse the virtual Ethernet device classes:
`BVI`, `VXLAN`, `GENEVE`, `Pipe`, `vhost-user`, `Xcrw` and GRE's class. Add a test row. I prefer this denylist to an
allowlist of NIC drivers, which could miss a real driver.

## Nits (no action needed for the merge)
- `BondsPage.tsx:177`: `taken` reads `qc.getQueryData(bondKeys.state)` during render, which is not reactive. That cache is
  also only this agent's bonds, so a clash with a foreign or pre-existing `BondEthernet<id>` still fails at commit
  (`bond_create2` INSTANCE_IN_USE, rolled back). This is acceptable, and the `model.ts:40` comment is now accurate.
- `bond.md:11` (DF-1's `bond.bond` row) still lists `broadcast` among the descriptor's modes. That is correct for the
  descriptor. A "(not configurable, Q1)" note would help readers.
- Still open from the review, and waiting on TD-11c: collapse `TestBondingOnFake`'s two-step sub-interface removal
  (`b3a` + `b3`) and drop the workaround sentences in the user doc (review §6).

**APPROVE**
