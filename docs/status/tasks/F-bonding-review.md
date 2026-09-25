# F-bonding review

Reviewer: independent review agent, 2026-09-25. Branch `task/F-bonding` @ `793d1f2`. I reviewed the branch's own diff,
`df67a8e...task/F-bonding` (65 files). I read it against the prompt, the envelope, wave-A-hotspots §0–§2, D-125, D-128,
D-129, D-132 (LOG entry in `228a7c5`: "list views never auto-poll full VPP dumps: manual refresh or ≥ 30 s, one walk at a
time in the agent"), TD-11c (`task/TD-11c` @ `ac313a1`, its topo change `0008a42` and its review) and WEB-1 (`task/WEB-1` @
`ca594d9`). I made no host runs and did not touch VPP.

## Verdict: **APPROVE WITH CHANGES**

The feature is solid:
- The contract is additive and numbered as allocated.
- The weight descriptor has a real Retrieve and is restart-safe.
- The rollback order is correct on the host and in the unit tests.
- The host evidence is complete and trace-free, and every acceptance line is pasted.

Four things must happen before the merge:
1. Remove the `dropPhantomOptionals` dependency (**F1**).
2. Fix the Bonds page polling under D-132, or get a manager waiver (**F2**).
3. At merge time, route the coretest hook through the D-129 seam (**F3**; merger-side, 2 lines).
4. Decide Q1 (drop `broadcast`) before the contract lands.

F4 and F5 are cheap defence-in-depth changes in files this task owns. I recommend doing them in the same fix round.

## What I ran
```
apps/agent  go test -count=1 ./internal/descriptors/bond/ ./internal/desired/ ./internal/subsystems/ ./internal/scheduler/
            ./internal/contracttest/ ./internal/descriptors/interface/            → ok ×6
            go test -count=1 ./internal/agent/ ./internal/descriptors/core/...    → ok (agent 7.7 s)   (TMPDIR=/tmp/g-rvb)
            go vet (bond, desired, subsystems, agent, coretest)                   → clean
packages/schema  vitest run src/semantic/bonding.test.ts                          → 12 passed (12)
main's tools/ci.sh (a copy from `git show main:tools/ci.sh`), `--base main`, run in the worktree:
  contract guard ok (3 contract commits named) · tools ok · install ok · generate + generated-output gate: clean ·
  forbidden patterns + gitleaks ok · then CI GATE FAILED at the D-128 packet-trace ban, on
  test/topology/interfaces/interfaces_test.go:291/297/302 only. Those are P08's lines from the speculative base df67a8e;
  TD-20 (a54b493) removed them on main. None of this branch's own files match the ban.
git merge-tree --write-tree --merge-base=df67a8e main task/F-bonding (the D-112 squash shape)
  → conflicts only in generated files: packages/api-client/src/generated/schema.d.ts, apps/cli/internal/api/operations_gen.go
    (rule 3: take main's side, `pnpm gen && make -C apps/cli gen docs`). Every hotspot hunk merges cleanly.
```
- I did not re-run the web suite, the API typecheck or the API e2e (it needs the slot database). For those I rely on the
  worker's two pasted green quick gates (`7131792`, `5466e9d`); commits after `7131792` are docs and web label changes.
- Q7 (SIGPIPE in the contract guard) is already fixed on main (D-127, `7edac8c`). The rebase picks the fix up.
- Build outputs I produced (`*/dist`) were removed afterwards. The tree is clean.

## Focus questions

**1. Is the contract additive and numbered? Yes.**
- `Interface.bond = 13` (§2). `rpc BondState` sits under the service anchor. Every message is prefixed `Bond*` and lives in
  the `// ----- F-bonding -----` section.
- The fake-agent UNIMPLEMENTED stub is in the contract commit. `proto.md` has its §11 section, and
  `F-bonding-contract.md` is present.
- No existing field was touched. The drift guard (`contracttest`) passes.
- The 8 rules are `interfaces.bonding-{name, member-exists, member-kind, member-unique, member-l3, load-balance,
  lacp-options, weight}` (`semantic/bonding.ts:74-211`). The agent builder repeats each of them with the same pointer.
- The duplicate-membership 400 points at the second membership. Members are ordered by bond id, then name, so "second" is
  deterministic.

**2. Is `bond.member-weight` declarative, and does Retrieve read VPP? Yes to both.**
- `descriptors/bond/weight.go`: a dfkit spec (D-077). It depends on `bond.member/<bond>/<member>`.
- Retrieve reads `sw_member_interface_dump.weight`. It skips 0, VPP's "never set" value (VPP 26.06 `bond_set_intf_weight`
  accepts 0; `bond_add_member` zeroes the member).
- Retrieve reports only memberships whose member passes `OwnedRef(…, bond.member)`, which keeps it consistent with
  `bond.member`'s Retrieve.
- Update is in place. Delete sets 0, and a vanished membership counts as a successful delete.

**3. Is it restart-safe? Yes.**
- The host simulation deletes the memberships first (D-095c), then the addresses and the bonds.
- It shows `created bond.member-weight/BondEthernet6001/tap6002`, with everything back 0.413 s after the agent start.
- `TestBondingOnFake` repeats the loss plus resync. Weight lost on its own (reset to 0 behind the agent's back) is also
  re-created, because Retrieve then reports it missing.

**4. `bond.bond` as the KeyProvider for `interface/<name>`: does it satisfy D-125 without TD-11c? Yes, for everything the
bond owns.**
- `bond.go:54-63` makes a delete-only plan see an edge from every object that references `interface/<bond>` to
  `bond.bond`. Those objects are the bond's admin state, MTU, addresses, memberships and sub-interfaces.
- The result: weight, then members, then admin state, then address, then bond. This is shown on the host
  (`F-bonding.md`, rollback list), in `TestBondingOnFake` (the call order asserts that address and `delete_subif` both
  come before `bond_delete`), and in `TestBondProvidesAlias`.
- Without the KeyProvider, the bond is deleted first (rank order) and the transaction fails.
- **Manager action:** the envelope predates D-125 and does not name this obligation. Record on the board row or envelope
  that the obligation is met by `bond.go:54-63` and those tests, so the merge gate is explicit.

**5. Is it consistent with TD-11c's alias-aware topo? Yes. There is no double-provide conflict once both merge.**
- `topoThrough.targets()` (TD-11c `0008a42`) looks in three places, in order: nodes, then node KeyProviders (`alias`),
  then non-node objects from the Retrieve snapshot.
  - **Delete plan.** `bond.bond` is a node, so `interface/<bond>` resolves through its KeyProvider before TD-11c's
    through-path is tried. The through-path would reach the same node anyway: the retrieved alias names its creator,
    because the `bond` device class is in `dump.go` `kindByDevType`.
  - **Create plan.** The alias object `interface/<bond>` is itself a node, and `nodes[dep]` wins. The alias depends on its
    `Creator` `bond.bond/<name>` (`interfaces.go` KindBond case), so there is no cycle and no second provider.
- `bond.bond` is the only provider of `interface/BondEthernet*`.
- `executor.dependents()` already follows KeyProviders, so the recreate cascade (mode change) is unchanged.
- The hunks are disjoint: TD-11c edits `reconciler.go`, the `core.Register` line and `stores.go`, while F-bonding only
  touches the anchors.

**6. Is Q4 (sub-interface removal) TD-11c's gap? Confirmed.**
- A sub-interface's own attributes depend on the observe-only `interface/<p>.<id>`, which has no planned provider. P08 has
  the same problem: core (`interface-ip`) is registered first, so its delete runs after `interface.subinterface`'s. TD-11c's own test
  `TestDeleteOrderFollowsAliasToCreator/sub-interface_removed,_parent_stays` covers it. `KeyFor` maps sub-interfaces by
  type.
- The KeyProvider moves this failure from the bond's own address to the sub-interface's attributes. Both shapes fail and
  roll back cleanly, so the KeyProvider is strictly better.
- **After TD-11c merges:** collapse `TestBondingOnFake`'s two-step removal (`rpc_bonding_test.go`, txn `b3a` + `b3`) into
  one step, drop the workaround sentences in `docs/user/interfaces/bonding.md:30-32` and `bond.md`, and assert that
  `interface-ip/BondEthernet6000.100/…` is deleted before `delete_subif`. Whichever of the two merges second does this.

**7. Q1, broadcast mode: I recommend removing it now.**
- TNSR omits it.
- It duplicates every frame on every member. Against a switch-side LAG that means duplicate delivery and MAC flapping, so
  it is a trap for an operator.
- Nothing in the WBS needs it.
- Narrowing the enum after the contract lands is a reshape (decision-policy #1 → PENDING), while adding it back later is
  additive and cheap.
- Change set (≈15 lines):
  - `ext/bonding.ts:24` enum and the `:89` help text;
  - the proto comments (`Bond.mode`, `BondStatus.mode`);
  - `desired/bond.go:51` `bondModes` (keep `BondLBName`'s decoding of VPP's `broadcast`);
  - `mode.broadcast` in the en/fa locales;
  - `docs/user/interfaces/bonding.md:12`;
  - the schema tests.
- If the manager prefers to keep it, keep it as specified. This is the manager's product call, not a defect.

## Findings (ranked)

### F1: MEDIUM, fix before merge. BondDrawer depends on the P08 export that WEB-1 deletes
`apps/web/src/domains/interfaces/bonding/BondDrawer.tsx:31` (import), `:93`, `:117` (calls).
- **What goes wrong.** D-131 and WEB-1 fix round 1 (`98fd042`, "H1 drop P08 dropPhantomOptionals") remove
  `dropPhantomOptionals` from `apps/web/src/domains/interfaces/model.ts`. Whichever of WEB-1 and F-bonding merges second
  fails `pnpm typecheck` at the pre-merge hook. If F-bonding lands first, WEB-1's merger would have to edit a file F-bonding
  owns.
- **Why the call can go.** The call is a no-op here. It only drops *optional object* members, and neither
  `bondFormSchema()` (mode, loadBalance, numaOnly, id) nor `memberSchema()` (passive, longTimeout, weight) has one.
- **Fix.** Delete the import and use `value` directly: `const cleaned = value` in `saveBond` and in `saveMember`.
- **Test.** Keep the existing BondsPage tests. "bond edit sends only the change" must stay green.

### F2: MEDIUM, fix before merge or get a D-132 waiver. The Bonds page polls BondState every 3 s on up to two timers, and each call walks the whole interface table
`apps/web/src/domains/interfaces/bonding/queries.ts:10` (`BONDS_POLL_MS = 3_000`), `:17` (`useBondsState` has its own
`refetchInterval`), `BondsPage.tsx:216` (the grid's timer), `BondDrawer.tsx:63-64` (the drawer adds `useBondsState` plus
P08's `useInterfacesState`, also 3 s); agent `apps/agent/internal/agent/rpc_bonding.go:36-61, :65`.

**Does it walk a full VPP table on a timer?** Yes, but only one table is walked in full, and that walk holds no barrier.

| per BondState call | VPP message | barrier | walks |
|---|---|---|---|
| once | `sw_interface_dump` (`iface.Dump`) | no (thread-safe, `interface_api.c:1730`) | every interface (all sub-interfaces too) |
| once | `sw_bond_interface_dump` | yes | the bonds (few) |
| per bond | `sw_member_interface_dump` | yes | that bond's members |
| once, LACP bonds | `sw_interface_lacp_dump` | yes | `bm->neighbors` (bond members only) |

- **The barrier-held walks are bounded**, so the stall that D-132 guards against (a long walk under the worker barrier)
  does not occur here.
- **The letter of D-132 is still broken:**
  - The page auto-refreshes a list RPC that walks a VPP table every 3 s.
  - Two unsynchronised timers share one cache entry (the grid's `fetchQuery` has `staleTime: 1000`), so an open drawer
    makes up to 2 BondState calls plus 1 InterfaceState call per 3 s per tab.
  - The agent does no single-flight, so concurrent tabs walk concurrently. The rule says "one walk at a time in the agent".
- **Precedent.** F-bridge-l2's verify BLOCKed on a 10 s poll (`54690a5`) and was fixed in `8e6741d`.
- **Fix (≈15 lines, the F-bridge-l2 pattern).**
  - Web: `useBondsState` loses its own timer, and the drawer reads the grid's cache.
  - Web: set `BONDS_POLL_MS` to at least 30 s and add a Refresh button. LACP convergence after a commit is then one click,
    and the commit mutation can invalidate `bondKeys.state`.
  - Web: add a test asserting `BONDS_POLL_MS >= 30_000` and that there is no second timer.
  - Agent: add a `sync.Mutex` around `bondTable` in `Service.BondState` (like F-nat44-ed's `rpc_nat44_ed.go:44`).
- **Alternative.** The manager records a D-132 exemption for bounded state RPCs (barrier walks ≤ bonds × members). If so,
  keep the one-timer and single-flight parts anyway.

### F3: MEDIUM, merger action. The coretest hook must become the D-129 `extensions` seam
`apps/agent/internal/descriptors/core/coretest/fakevpp.go:95` (`v.installBonding()`).
- F-B8's reason is valid: every `coretest.New()` user now retrieves `bond.bond`.
- D-129 makes the `extensions` seam of F-nat44-ed and F-rpf-adl-pbr the single A6 hook. F-bridge-l2 and F-neighbors-ra
  still carry direct lines too.
- **At merge:**
  - Drop the `fakevpp.go` line.
  - Add `func init() { extensions = append(extensions, (*VPP).installBonding) }` to `coretest/bonding.go`, which F-bonding
    owns.
  - If F-bonding is the first of these to merge, the merger adds the seam from F-nat44-ed's shape: the
    `var extensions []func(*VPP)` declaration plus the loop after `sanitizetest.Clean`.
- **Order is safe.** `installBonding` registers only `bond_*` and `sw_interface_lacp_dump` handlers, so running it after
  `installIfExt` and the sanitizer changes nothing.
- Nit: `bondModels sync.Map` (`coretest/bonding.go:56`) never deletes its per-VPP entry. This is a test-only leak;
  `t.Cleanup(func(){ bondModels.Delete(v) })` or a field reached through the seam fixes it.

### F4: MEDIUM (forward risk, cheap now). "Members are physical" is enforced only by name; VPP 26.06 does not check that a member is Ethernet
`packages/schema/src/semantic/bonding.ts:43-47` (`notMemberKind`: bonds and `loop<N>` only),
`apps/agent/internal/desired/bond.go:145-152`, `apps/agent/internal/descriptors/bond/member.go:45-72` (owned by this task).
- **The VPP gap.** `bond_add_member` (`/root/vpp/src/vnet/bonding/cli.c:691-800`) rejects only a bond as a member. It then
  runs `memcpy(mif->persistent_hw_address, mif_hw->hw_address, 6)` (`:793`).
- **Scenario.**
  - For a non-Ethernet hardware class `hw_address` is NULL. Examples are L3 tunnels: ipip, GRE-L3, wireguard, ipsec itf.
    I checked this in the source; it was not run on the host.
  - A document with `interfaces.<tunnel>` and that tunnel as a bond member passes all 8 rules and the builder. Applying
    it very likely SIGSEGVs the shared VPP, the D-128 class of outage for every slot.
  - Today's product wiring has no tunnel family, so the only route is an untagged tunnel made by hand. The route opens
    as soon as F-wireguard, df6 or P11 wire their interfaces.
- **Fix, both in owned files:**
  - In `MemberDescriptor.Create`, refuse the member before `bond_add_member` when its `sw_interface_details` is a
    sub-interface (`Type == IF_API_TYPE_SUB`) or carries no L2 address (all-zero `L2Address`; VPP fills it only for Ethernet
    hardware, `interface_api.c:269-279`). Return INVALID_ARGUMENT with a clear message, plus a coretest case.
  - Optionally extend `notMemberKind` with the known virtual prefixes (`wg`, `ipip`, `gre`, `ipsec`, `vxlan_tunnel`,
    `bvi`) for a 400 with a pointer.

### F5: LOW. A member's `mac` is not rejected, but VPP rewrites the member's MAC
`packages/schema/src/semantic/bonding.ts:50-58` (`l3Leaf`), `desired/bond.go:138-175`.
- **What VPP does.** `bond_add_member` gives the 2nd and later members the bond's MAC (`cli.c:795-812`) and restores it on
  detach (`:403`).
- **What goes wrong.** An `interfaces.<member>.mac` puts `interface.mac-address` and the bond in conflict:
  - Retrieve then differs from the desired state, which is permanent drift.
  - A re-apply sets the member back to its own MAC. The member then stops receiving frames sent to the bond's MAC,
    because `bond_add_member` sets the member to non-promiscuous L3 (`:820-825`).
- **Fix.** Add `mac` to the forbidden member leaves: one line in `l3Leaf` (or a separate `interfaces.bonding-member-mac`),
  mirrored in the builder, plus a test.

### F6: LOW, docs. The descriptor table is split
`docs/agent/descriptors/bond.md:13`: the blank line after the `bond.member` row ends the table. The `bond.member-weight`
row then renders as plain text with pipes. Remove the blank line.

### F7: LOW, nits
- `apps/web/src/domains/interfaces/bonding/model.ts:40` says `nextBondName` avoids ids "on the data plane", but
  `BondsPage.tsx:174` passes only candidate keys. A clash with a pre-existing or foreign `BondEthernet<id>` then fails at
  commit (`bond_create2` INSTANCE_IN_USE, rolled back). Either pass the live bond names or fix the comment.
- The Status column truncates "no active m…" in `bonds-list-en.png`. Widen it or shorten the label.
- The `interfaces.ts` import sits at line 2 with a tag comment, while siblings import at line 15. There is no textual
  conflict either way; just be aware of it at merge.

## Out-of-envelope lines
| line | verdict |
|---|---|
| `coretest/fakevpp.go:95` `v.installBonding()` | justified (F-B8). Becomes the D-129 seam at merge (F3) |
| `apps/web/src/domains/interfaces/model.ts:31` `delete props['bond']` | allowed by the prompt ("only if that breaks the drawer"). The breakage is evidenced: two P08 drawer tests sent no PATCH. The hunk does not touch WEB-1's deletion hunk. Keep it even after WEB-1's presence toggle, since bonds are edited on the Bonds page |
| `bond.member-weight` registered in `subsystems/bonding.go:30`, not in `bond.Register` | fine: an owned file, and DF-1's `descriptors/interface/registry_test.go` pins `bond.Register` to two descriptors. `registerBonding` also asserts the persisted IfaceClaims store (§4 checklist: "every Register in the A1 hunk passes a Wiring store") ✓ |
| `desired/interfaces.go` | exactly the A3 allowance: one const, one `KindOf` case, one creator case (9 lines) ✓ |

## Checklist (REVIEW-PROMPT)
| # | check | result |
|---|---|---|
| 1 | contract | ✓ additive, 3 `contract(…)` commits, `F-bonding-contract.md` |
| 2 | real verification | ✓ `test/topology/bonding` runs on host VPP through the real agent and API. It asserts `show bond details`, `show lacp`, Retrieve == running and `show mode` (members back to l3) |
| 3 | restart safety | ✓ pasted (members first, D-095c; 0.413 s). No VPP restart; NRestarts 1 → 1 |
| 4 | VPP API provenance | ✓ only `binapi/bond`, `binapi/lacp`, `binapi/interface`, `binapi/classify` (read-only check). `binapi/` is untouched |
| 5 | shared host | ✓ slot 6 prefix, ids 6000–6002, fixture taps, `flock -s` only during runs, cleanup pasted, the `vppctl` wrapper refuses `trace` |
| 6 | security | ✓ no `exec` in product code (test-only fixed argv), route behind the global guard (e2e auth case), no secrets |
| 7 | transactions | ✓ rollback order proven on host and on the fake. A mode change re-creates the bond with its dependents |
| 8 | UI honesty | ✓ real endpoint, 4 screenshots (en/fa), no stubs |
| 9 | scope creep | none. A CLI `show bonds` was correctly left out (Q5 → CLI owner) |
| 10 | i18n | ✓ en/fa 92/92 keys, logical CSS only |
| 11 | CI | main's gate on the un-rebased branch stops at the D-128 ban on the base's P08 lines (see "What I ran"). Re-run the gate after the rebase (Q6) |

**APPROVE WITH CHANGES.** Before merge: F1, F2 (or a D-132 waiver), the Q1 decision, and F3 at merge time. In the same
round, recommended: F4, F5 and F6.
