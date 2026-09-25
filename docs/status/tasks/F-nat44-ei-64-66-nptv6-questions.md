# F-nat44-ei-64-66-nptv6 — questions and notes for the manager

Written and kept going (never waiting). Newest last.

## Q1 (info, contract): `contract(proto): nat session variants` is committed on this branch
Additive: `optional NatSessionVariant variant` on `NatSessionsRequest` (5), `NatSessionsResponse` (8) and
`NatSessionKillAction` (7), the enum in my proto section. No new RPC, no `ActionRequest` member, no `NatConfig`
number. Details: `F-nat44-ei-64-66-nptv6-contract.md`.

## Q2 (info, merge): edits outside my files that implementing the sibling translators forced — please keep them
1. `apps/agent/internal/desired/nat_test.go` (F-nat44-ed-sessions' file, one hunk): the "siblings are warnings" case
   asserted that `mode: "ei"`, `nat64` and `nat66` are *not applied*; now EI projects its objects and a disabled
   nat64/nat66 projects nothing. Only that case changed.
2. `apps/agent/internal/agent/rpc_nat44_ed_test.go` (one line): its dry run no longer lists `/nat/nat64
   agent.unsupported-field` (its document has `nat64: {enabled: false}`, which now programs nothing).
3. `apps/agent/internal/agent/service_test.go` (three lines, the same as F-nat44-ed-sessions' Q2.1c): the "only dumps"
   checks also allow `nat44_ei_output_interface_get` (DF-3's read-only cursor get).
4. `apps/web/src/domains/firewall/nat44-ed-sessions/NatPage.test.tsx` (two assertions): the NAT44-ED tabs are asserted
   as the first four (`slice(0, 4)`); the siblings' tabs follow.
5. `apps/agent/internal/descriptors/core/coretest/nat44ed.go` is untouched: my `coretest/nat44ei.go` replaces its
   `nat44_ei_show_running_config` stub through the `extensions` seam (it runs later). D-134/TD-23 hoists that seam:
   my three `init()` registrations (`coretest/{nat44ei,nat64,npt66}.go`) become three registration lines at rebase.

## Q3 (info, base): F-nat44-ed-sessions' fix round 1 merged into my branch (speculative base, D-114)
My base was `task/F-nat44-ed-sessions@acc1877`. Its fix round (bf093407: per-call caps, streamed pager with
`EachUserSession`, `external_nat_*` = 0.0.0.0 without twice-NAT, 30-s summary) changed the pager API my EI adapter uses,
so I merged the ED head into my branch (merge commit d3740a12, no conflicts; generated code regenerated = identical) and
adapted (10e17f32). I did not merge any other feature. When ED merges to main first, my rebase onto main needs no
further adaptation of that API.

## Q4 (decision taken, npt66 write-only — the prompt's open question)
npt66 has no dump in VPP 26.06. **Taken: write-only (D-063) + V-new** (`### V-new (F-nat44-ei-64-66-nptv6)` (a) in
docs/vpp-code-track.md: `npt66_binding_dump`, ~0.5 day, plus the interface-delete hook). No D-076 record is needed:
VPP's add overwrites the interface's binding and enables the features only once (fake + host evidence in the status
file). Options: (a) write-only + V-item (taken), (b) wait for the V-item. The product owner may prefer (b) for restart
claims — the limitation (a binding removed while the agent was down stays until VPP restarts) is in the user page.

## Q5 (VPP findings, V-new (b)–(d)): NAT64 session ports, NAT64 FIB lock leak, EI port forwards
- `nat64_st_details` carry the remote port in `il_port` and no `r_port` (nat64_api.c sets il_port twice). The agent
  corrects the page from the BIB; a fixed VPP (r_port set) is left alone.
- nat64 locks a tenant VRF's IPv6 table on every prefix add and static-BIB add/delete and never unlocks: such a VRF
  cannot be deleted until VPP restarts; the agent's VRF delete then fails verify (DEGRADED). On slot 4 the IPv6 table
  4064 now holds `nat64-hi:24` locks (my test runs; cleared only by a VPP restart — **the manager's nightly gc will see
  `w4:w4-n64` in `show ip6 fib`; it is this leak, not a forgotten object**). The topology test keeps its slot VRF in the
  configuration and removes the IPv4 table through binapi.
- nat44-ei reserves a port forward's external port on a pool address (NO_SUCH_ENTRY otherwise): the builder refuses
  it with a pointer (`nat.ei-port-forward-pool`) — found by host run 1.

## Q6 (info): NAT64 is multi-tenant on the inside only
VPP keys the outside (out2in BIB/session) in FIB 0 and routes the translated IPv4 packet in the inside interface's IPv4
table. The topology test puts only the lan interface into the slot VRF (the prefix is chosen by the inside IPv6 FIB, so
the slot /96 is really used) and adds a route in that VRF to the wan subnet via the wan interface. Documented in the
user page. No agent change.

## Q7 (TD-11b / TD-8 / D-132 / WEB-1, from the CONTINUE notes)
- TD-11b: every descriptor I register is a `natcommon.Descriptor` (nat44ei, nat64, nat66, npt66), which TD-11b gives
  `CheckPersistent()`; my wiring passes `natcommon.WithClaims(Wiring.KeyedClaims("nat"))`, so the guard passes without a
  change of mine. The npt66 Create makes one VPP call (no partial create).
- TD-8 seams: not needed by this feature (no events, no dynamic sources, no metrics).
- D-132: EI / NAT64 walks are serialised in the agent (`natVariantWalk`); the EI / NAT64 grids and the drift status
  refresh every 30 s at most (no timer with a session-level EI filter) and have a Refresh button. F-nat44-ed-sessions'
  own walks are not under my mutex (its code; its unfiltered grid still polls at 5 s on its branch) — one shared
  "one walk at a time" lock for all NAT walks would be a small follow-up in rpc_nat44_ed.go.
- WEB-1: my subtree form does not call `dropPhantomOptionals`.

## Q8 (incident note): VPP crash 2026-09-25 04:27:21 (NRestarts 1 → 2) — not slot 4
`received signal SIGSEGV … faulting address 0x0`, frame #0 `ip4_sas + 0x31` called from an API handler of a plugin
(frames #1–#4 unnamed, then `vl_msg_api_socket_handler`); systemd-coredump took a core. Slot 4's last VPP message was
at ~04:20:40 (end of `TestNatEI6466Screenshots`, log `…-shots-2.log`: NRestarts 1 before and after); from 04:22 on this
task ran only CI in unit mode (no VPP connection) and, at 04:36, read-only `vppctl show` commands. None of this task's
code calls `ip4_sas`-reaching messages (npt66 add/del, nat44-ei/nat64/nat66 config, dumps). Every topology run of this
task logged NRestarts 1 → 1.

## Q9 (CI, manager): main's ci.sh vs this branch's inherited P08 test file
Main's gate (D-128 packet-trace ban) fails on `test/topology/interfaces/interfaces_test.go` lines 291–302 (`trace add`,
`show trace`), P08's pre-merge copy that this branch carries through its base (ED ← W-seed ← P08). Main's copy has no
trace; the D-112 rebase onto main replaces the file. The branch's own `tools/ci.sh quick` is green.
