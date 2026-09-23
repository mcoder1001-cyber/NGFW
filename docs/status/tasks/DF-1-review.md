# DF-1 — independent review (task/DF-1 @ f2e1eaf)

Reviewer: independent review agent (did not write this code). Run directly on the host, slot 2.

## What I ran (my own runs, not the pasted ones)

- `tools/ci.sh --base main` in `/root/ngfw-wt/DF-1` → **CI GATE PASSED** (wall 0m46s, logs
  `/root/ngfw-wt/logs/ci/DF-1-20260924-004435-892741`); agent packages all `ok`, golangci-lint clean. Matches the pasted output.
- Host integration, one package at a time, `eval "$(tools/lab env 2)"; VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -count=1 -v ./internal/descriptors/<pkg>/`
  for interface, tapv2, af_packet, bond, l2, l3xc, memif → all PASS; rx-placement skips with
  `skip: no workers on host …`; `TestRestartSimulationOnHost`: "plan is empty (38 objects)", "Retrieve rebuilt 36 objects
  with equal Meta; plan = [create interface.mac-address/loop260, create interface.promisc/w2-tap63]", "second apply: plan is empty",
  "deleted 38 objects; Retrieve for owner w2r is empty".
- `systemctl show vpp -p NRestarts`: 2 before, 2 after every package (no VPP crash). Afterwards `vppctl show interface` = local0 only,
  no bridge domains, memif sockets = only VPP's id 0, `ip -br link | grep '^w2'` empty.
- `git merge-tree --write-tree HEAD main` → **CONFLICT in apps/agent/go.mod and go.sum** (see L6).

## Checklist

1. Contract compliance — no change under packages/schema, packages/proto, apps/agent/gen, packages/api-client. Models are agent-internal
   `.proto` stand-ins (D-055). OK.
2. Real verification — every package has a host test on `/run/vpp/api.sock` asserting through `Retrieve` (and the status file pastes
   `vppctl show …` with the prefixed objects, then empty). Not mocks. OK.
3. Restart safety — every object type has `Retrieve`; the simulation proves "restart with state intact → empty plan, equal Meta" but
   **not** the required "delete prefixed objects → start → recreated" leg (M2). No VPP restart.
4. Binapi provenance — all messages come from generated packages (`binapi/{interface,interface_types,bond,l2,l3xc,memif,tapv2,af_packet,
   fib_types,ip_types,ethernet_types,vlib,memclnt}`); the code compiles against them; `apps/agent/binapi/` and `tools/binapi-gen.sh`
   untouched. OK.
5. Shared host — slot prefix `w2` on every tag, loopback `loop2xx`, tap ids `2xx`, taps/veths `w2-*`, BD / memif socket ids from
   `VRX_VPP_TABLE_BASE=2000`; memif sockets under `/run/vrx-test/w2/`; cleanup in `t.Cleanup`; no pkill/killall; no daemons. OK.
6. Security — `exec.Command` only in two test files, fixed argv `/usr/sbin/ip link add|set|del w2-…`; no secrets. Host-side inputs
   are not restricted (L3).
7. Transaction semantics — Tag failure after create leaves an untaggable orphan (M3); dependency gaps around l2 mode (M5).
8. UI — n/a. 9. Scope creep — none of substance (the alias is D-065). 10. i18n — n/a.
11. Tests actually run — yes, re-run above; results match the status file.

## Findings (ranked)

### HIGH

**H1 — The alias descriptor will fail every P05 transaction as wired today.**
`apps/agent/internal/descriptors/interface/alias.go:114-150`. Retrieve returns every VPP interface except local0, including interfaces
of other owners and our own interfaces the desired state does not alias. The scheduler contract (`scheduler/descriptor.go` "Retrieve
rules": "keep only objects owned by this agent") treats every retrieved key as owned: P05 (task/P05 `scheduler/reconciler.go`) plans a
Delete for each undesired key and `verify`/`diffErr` then reports `interface/<x> still present` → transaction FAILED → rollback, on every
commit (on the shared host other slots' interfaces are always present; in production `ens192`/DPDK NICs are). task/P05 already added the
opt-out `AbsenceDeleter` (`DeleteOnAbsence() bool`, written for exactly this descriptor) — DF-1 does not implement it.
Fix: add `func (*AliasDescriptor) DeleteOnAbsence() bool { return false }` (duck-typed, compiles without P05) + a unit test; and restrict
Retrieve to this owner's interfaces plus **untagged** ones (never interfaces tagged by another owner), which honours the ownership rule
even without the extension. Record the reconciler requirement in docs/agent/descriptors/interface.md (replacing the Q4 note).

**H2 — Physical / untagged interfaces cannot be configured by any DF-1 descriptor.**
`interface/dump.go:161-191` (`KeyFor`, `Index` resolve only through `"<owner>:<id>"` tags) is used by every attribute (admin-state, mtu,
mac, promisc, rx-mode, rx-placement), by the sub-interface parent, bond member, bridge-domain member, xconnect, l2 fib entry, l2 flags,
vlan-tag-rewrite and l3xc. A DPDK NIC has no tag and no creator (D-065), so "admin up ens224", "MTU on ens224", "VLAN 100 on ens224",
"ens224+ens256 in a bond / bridge" are impossible — these are the most basic router configurations and what P08/F-vlan-qinq/bonding need.
Passing the D-065 key does not help either: `Index("interface/w2-tap40")` finds the tag but fails with `ErrWrongKind` ("interface" ≠
"tapv2.tap"), and `interface/ens224` → `ErrNotFound`.
Fix: accept `interface/<name>` in every DF-1 interface field (depend on it, resolve with the alias rule); for untagged interfaces use the
DF-4 pattern already on main (`descriptors/acl/etype.go` `ClaimStore`: claim on Create, release on Delete, Retrieve reports claimed
untagged interfaces, P05 persists the store in the state dir; interfaces tagged by another owner are refused with an explicit error).
Retrieve of attributes on claimed NICs then needs the "creation default" of a NIC (MTU, rx-mode) — document per attribute.

**H3 — The alias name space does not match the consumer already on main (DF-4).**
`alias.go:131-150` names our interfaces by **tag id** (`w2-tap40`, `w2-bond20`, `w2-memif40`, `w2-af50`, `w2-tap11.100`) and foreign
ones by VPP name. DF-4 on main (`descriptors/acl/interfaces.go` `dumpInterfaces` / `index`: `byName[interface_name]`,
`register.go:15` `DefaultInterfaceKey` = `interface/<ifName>`) resolves a binding's interface by **VPP interface name** (`tap240`,
`BondEthernet20`, `memif2040/40`, `host-w2-af50`, `tap211.100`). For every non-loopback DF-1 interface the two differ: DF-4's (optional)
dependency `interface/tap240` never matches an alias (ordering silently lost), and a binding written with the alias name fails with
`ErrNoInterface`. D-065 says "DF-2…6 need no change" — that only holds if both sides use the same name.
Fix (needs a manager decision, log it): one rule for everybody. Recommended: alias name = VPP interface name (stable, because DF-1
already requires explicit tap/bond/memif ids and af_packet/sub-if names are derived); the alias with a creator verifies that the creator's
interface has that VPP name. Alternative: export `iface.ResolveName(table, owner, name)` and require DF-2…DF-8 to use it.

### MEDIUM

**M1 — Alias name fallback hands out other owners' interfaces.** `alias.go:79-83`: after the own-tag lookup, `find` matches any VPP
interface by name, including ones tagged by another owner (e.g. `tap301` of slot 3). Create then "verifies" it and returns its
sw_if_index as Meta, so any consumer resolving through the alias decorates a foreign interface. Fix: the VPP-name fallback only for
untagged interfaces; tagged-by-other → explicit `ErrForeignInterface` (as DF-4).

**M2 — Restart simulation lacks the "simulated loss" leg required by FAST MODE (3).**
`interface/restart_integration_test.go:325-371` only restarts with VPP state intact. Missing: after the restart, delete a subset of the
prefixed objects via binapi (e.g. `tap_delete_v2` on w2-tap62 — which cascades the sub-interface, BD member, vtr and flags — plus the
bridge domain and the memif socket), then Retrieve → plan must contain exactly those creates (in dependency order) → apply → Retrieve ==
desired, new Meta. Paste the log.

**M3 — A failed tag after create leaves an orphan that blocks every retry.**
`interface/subinterface.go:122-123`, `bond/bond.go:101-102`, `memif/memif.go:62-63`, `af_packet/host_interface.go:71-72` return
`(Meta, err)` when `sw_interface_tag_add_del` fails. The scheduler does not roll back a failed Create, the object is untagged (invisible
to Retrieve, never deleted), and the next Create fails with "already exists" (same sub_id / bond id / memif id / host_if_name) forever.
Fix: on tag failure delete the object just created (delete_subif / bond_delete / memif_delete / af_packet_delete) and return `nil, err`
(tap is fine: tag is inside tap_create_v3).

**M4 — Desired values VPP canonicalises cause perpetual plans.**
- `tapv2/tap.go:106,164`: `host_if_name` empty → the kernel/VPP name is reported → Update → `ErrRecreate` on **every** resync (tap and
  all dependents deleted and recreated each time). The fake even models this default (`tap_test.go:44`).
- `l3xc/l3xc.go:112,147`: path weight 0 is stored as 1 by VPP (`src/vnet/fib/fib_api.c:150`) → perpetual Update; unsorted desired paths
  (`SortPaths` is only a convention, :90) → perpetual Update.
- `interface/attributes.go:203`: `interface.mtu` equal to the creation default (e.g. `{9000,0,0,0}` on a tap/loopback whose link MTU is
  9000) is accepted, then invisible to Retrieve → perpetual Create and a verify failure. rx-mode already rejects this case
  (`ErrRxModeDefault`); mtu should too (needs the dump in Create).
Fix: reject in Create (require host_if_name; reject default MTU) or implement P05's `Normalizer` (weight 0→1, sorted paths).

**M5 — l2 mode dependents are not declared.** `l2/flags.go:39-41` depends only on the interface, but Create requires membership of an
owned bridge (`bridgeOf`) → correct order only by registration tie-break. A bridge-member recreate (`Update` = `ErrRecreate` on shg /
port type) sets the interface to L3 first, which resets the feature bitmap and memsets the L2 output config incl. the output VTR
(`/root/vpp/src/vnet/l2/l2_input.c:315-326`); since `l2.flags` and `l2.vlan-tag-rewrite` (`l2/vlan_tag_rewrite.go:35`) are not declared
dependents of the member, the reconciler does not re-create them → verify fails / silent loss until the next resync.
Fix: add `bridge_domain` to `Flags` and depend on `l2.bridge-domain-member/<bd>/<if>`; let vtr depend on the member (or xconnect) key
when the interface is in L2 mode; add a unit test for the recreate cascade.

### LOW

**L1** `interface/attributes.go:334,446` — the promisc/MAC "what I set" maps are keyed by sw_if_index and survive a VPP restart (pruned
only when the index is absent). After `kill -9 vpp` the index can be reused by a different interface → Retrieve reports promisc/MAC that
VPP does not have → never re-applied. Clear the maps when the VPP identity changes (DF-4's main-thread-PID approach, D-066).

**L2** Attribute Deletes act on `Meta.SwIfIndex` without re-checking ownership (`attributes.go:112-118` admin-state, `:231-245` mtu,
promisc, rx-mode, `l2/member.go:84-99`). With stale Meta after index reuse they touch another interface. Mtu/rx-mode already dump —
add an `OwnedID` check and treat "not ours / gone" as done.

**L3** Host-side inputs are unrestricted: `af_packet/host_interface.go:57-66` can attach VPP to any Linux netdev (incl. the management
NIC `ens192`); `tapv2/tap.go:112-117` can move a tap into any netns / enslave it to any Linux bridge. Belongs mainly in schema/F-*
validation, but a descriptor-level deny-list for the management interface is cheap defence in depth.

**L4** `memif/memif.go:39` — socket id 0 (VPP's shared `/run/vpp/memif.sock`) is allowed with no dependency; on the shared host a
master memif there listens on a socket no slot owns. Require an owned socket, or document it as production-only.

**L5** `docs/agent/descriptors/interface.md:36` — the "Creator keys" table is corrupted (an alias row was pasted into the sub-interface
row: 7 cells, wrong columns).

**L6** go.mod/go.sum conflict with main: DF-4 added `github.com/ftrvxmtrx/fd` and both sides added fsnotify/logrus as indirect.
Merge main in the fix round (take main's go.mod/go.sum, `go mod tidy`, re-run the gate).

**L7** Documented scope drops (memif secret / hw-addr / ring / buffer, tap host MAC, af_packet flags / frame sizes, bond enable_gso) are
not even create-only. Acceptable now; note that `memif_details.hw_addr` is reported and could round-trip.

**L8** Agent-internal `*.pb.go` have no generation script or gate (D-055 stand-ins) — regenerate/verify when P03b lands.

## Verified OK (answers to the focus questions)

- **Can a descriptor claim/modify/delete another slot's or a physical object?** Creators and attributes: no — Retrieve and resolution
  are by owner tag, Delete uses Meta from Create/Retrieve (stale-Meta caveat L2). Bridge domains: bd_tag + VPP refuses an existing id
  (`BD_ALREADY_EXISTS`, l2_bd.c:1341). Memif sockets: owned iff the file is directly in the owner's dir; id 0 never owned. Bonds/taps:
  tag. The only leak is the alias name fallback (M1).
- **Alias Delete** never sends a message (unit `TestAlias` asserts nothing but `sw_interface_dump` is sent; host `TestAliasOnHost`
  re-Creates after Delete). Reconciler requirement: H1.
- **MTU delete**: restores `{link_mtu,0,0,0}` (sub-if `{0,0,0,0}`), never zeros a hardware interface; vanished interface = no-op. Gap:
  default-equal desired (M4).
- **af_packet / tap Linux side**: tap netdev removed by `tap_delete_v2` (verified: no `w2-*` links after the runs); af_packet never
  deletes the veth (precondition, test-owned, removed in `t.Cleanup` with fixed argv).
- **Compared with DF-4's ClaimStore**: DF-4 supports untagged NICs by claiming them and refuses foreign-tagged ones; DF-1 refuses
  untagged interfaces outright (H2) and its alias accepts foreign-tagged ones (M1) — DF-1 should adopt DF-4's model.
- P05 overlap (Q4.2): task/P05's loopback `KeyProvider` also provides `interface/<name>` — the manager must pick one (with H3).

**APPROVE WITH CHANGES** — fix H1–H3 and M1–M5 (H3 needs a manager decision logged in LOG.md) and merge main (L6) before merging.
