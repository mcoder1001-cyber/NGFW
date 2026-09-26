# F-lb — questions for the manager (written and kept going; nothing waits on an answer)

## Q1 — contract commits on task/F-lb (review, please)
`87244806 contract(schema): services.lb …`, `6e0d2dec contract(proto): LbService (ServicesConfig 11), LbState and
LbFlushVip RPCs`, `bbdcd36a contract(schema): … 0.0.0.0/8 reserved`, `63ea12c5 contract(api-client): regenerate`.
Numbers exactly as allocated (ServicesConfig 11; new `Lb*` messages from 1; RPCs `LbState`, `LbFlushVip`; no
ActionRequest member, no EventKind). Besides the six allocated messages the RPCs need their request/response messages
(`LbStateRequest/Response`, `LbServerState`, `LbFlushVipRequest/Response`) — all `Lb`-prefixed, in the F-lb section.
Details: `docs/status/tasks/F-lb-contract.md`.

## Q2 — missing `wave-BC: F-lb` anchors (hunks marked "unanchored" in F-lb.md)
The anchor pass seeded no F-lb anchor in: `packages/schema/src/domains/services.ts` (key block + import),
`packages/schema/src/index.ts`, `packages/schema/src/semantic/index.ts` (import + spread), `docs/contracts/proto.md`,
`apps/agent/internal/agent/projection.go` (`project()` block), `apps/agent/internal/subsystems/subsystems.go` (domain
constants block and the `register()` family block — the `Domains` entry has its anchor), `apps/web/src/nav/nav.ts` +
`nav.test.ts` (`'services'`), `apps/api/src/testing/fake-agent.ts` (the import line of `features/lb/fake.ts`). Per the
envelope I appended at the end of each block. Expect trivial unions at merge.

## Q3 — `Services` domain / services seam collides with other branches (merge note)
Main has no `services` domain yet; F-kea-dhcp-relay, F-unbound-chrony-syslog and F-rpf-adl-pbr each add
`Services = "services"` + a `Domains[Services]` entry. I add the same constant and entry (envelope: "whoever lands first
adds the key"); the second lander moves `lb.conf`, `lb.vip`, `lb.as`, `lb.intf-nat` into the existing entry. For the
projection I copied F-rpf-adl-pbr's `desired.ServicesMembers` seam into `apps/agent/internal/desired/lb_services_seam.go`
(the F-loopback precedent, D-131): **delete that file** once F-rpf-adl-pbr is on the base; `lb.go` then registers `lb`
in its map and its `projectServices` reports the unsupported members. The two registry-derived assertions in
`internal/agent/service_test.go` (D-129 F5) take main's version at rebase.

## Q4 — drift view needs `agent.write-only` in COVERAGE_RULES (F-rpf-adl-pbr adds it)
The agent's DryRun notes `/services/lb` as `agent.write-only` (Retrieve never reports lb objects, D-063).
`apps/api/src/state/state.controller.ts` (P2, not mine) only skips `agent.unsupported-field` /
`agent.unimplemented-domain` on main, so until F-rpf-adl-pbr's `COVERAGE_RULES` change (+ `'agent.write-only'`) merges,
`GET /api/v1/state/drift` shows a committed `services.lb` as drift. No action on this branch; merge order note.

## Q5 — D-090 (2) command: `lb conf` does not collect (decision taken, please confirm)
VPP 26.06 `lb_conf_command_fn` returns before `lb_garbage_collection()` on an empty line, and every `lb conf` argument
overwrites a global — DF-7-questions Q1 option (a) as written collects nothing. Options: (a) `lb conf` with the
globals owner's desired values (not constant: user values in a CLI line, and "keep current" values are unknowable);
(b) **the constant `lb vip 0.0.0.0/32 del`** — parses, runs the GC, then fails the lookup of a VIP that never exists
(0.0.0.0/8 refused as a VIP by schema + projection; a success reply is reported loudly); (c) leave removed VIPs until
a VPP restart. I took (b) (ALLOWLIST row active). Timing: VPP frees a removed AS only 10 s after removal and visits a
VIP at most every 60 s, so the GC runs once, 65 s after the last lb delete (debounced timer), not right after the
transaction. **Not yet verified on the host** (host runs closed): the manager-window run `VRX_LB_GLOBALS=1` proves it.

## Q6 — NAT VIP SNAT-key crash hazard (V20 follow-up, source reading only) — decision taken
VPP keys the SNAT mapping of NAT port VIPs by (AS address, target port) only. A changed NAT VIP (delete + add) and its
removed predecessor share one mapping; a GC of the predecessor frees the live mapping, the next GC `pool_put`s NULL
(likely SIGSEGV on the shared VPP). Decision: `lb.GCSafe` skips the GC while any such pair appears twice in
`lb_as_dump` (logged); the schema forbids two NAT VIPs sharing a (server, target port). Consequence: after a NAT VIP
change nothing is collected until VPP restarts (documented). Options were (a) skip GC (taken), (b) forbid changing NAT
VIPs, (c) doc only. Not reproduced on purpose (D-064: no crash tests on the shared VPP); V-item text appended to V20.

## Q7 — FlushVIP defect in DF-7's helper (fixed under the gap rule)
`vl_api_lb_flush_vip_t_handler` memcpy's `un.ip6` regardless of the family and ignores the lookup result; DF-7's IPv4
encoding never matched, so every DF-7 host test flush (`VRX_DF7_LB=1`) flushed an uninitialised VIP index — possibly
other slots' flows. Fixed (ip46 layout + in-use guard); worth a note to whoever ran DF-7's lb host test.

## Q8 — globals owner on slots: `settings` → agent.unsupported-field
Slot agents (VRX_GLOBALS_OWNER=0) never send `lb_conf` and report `services.lb.settings` as `agent.unsupported-field`
(so the drift view skips it). VIPs still work there with whatever the globals owner set (GRE source defaults to
255.255.255.255 — traffic evidence needs the globals owner's `ip4Source`). "Slots require it" could not be enforced:
lb_conf has no getter.

## Q9 — open question from the prompt: ship an in-product LB given V20?
Default taken: ship behind the T3 label with the visible write-only/GC notice (UI + user docs). Product-owner call.

## Q10 — host steps pending TD-25 (host runs closed at spawn)
Not run: `TestLbOnHost` (`VRX_INTEGRATION=1 VRX_LB_HOST=1`, slot 2: `show lb vips verbose`, restart simulation, loss,
removal), `TestLbGarbageCollectOnHost` (`+ VRX_LB_GLOBALS=1`, exclusive globals lock, manager window), the optional GRE
tcpdump evidence, and the UI screenshot against the real endpoint. Commands are in F-lb.md "Pending host steps".
