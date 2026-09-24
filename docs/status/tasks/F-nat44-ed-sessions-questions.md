# F-nat44-ed-sessions — questions and notes for the manager

Written and kept going (never waiting). Newest last.

## Q1 (info, contract): `contract(proto): nat sessions` is committed on this branch
Additive: `rpc NatSessions`, `rpc NatSummary`, `ActionRequest.nat_session_kill = 5` (§2 allocation) and the messages in
the `// ----- F-nat44-ed-sessions -----` section. Details: `F-nat44-ed-sessions-contract.md`. No `NatConfig` number used.

## Q2 (info, merge): edits outside my anchors that implementing a new domain forced — please keep them at merge
1. `apps/agent/internal/agent/service_test.go` (4 one-line hunks): with `nat` implemented, (a) the "unimplemented
   subsystem" case uses `system` instead of `nat`; (b) the two implemented-domain assertions compare with
   `implementedDomains()` (prefix `interfaces,vrfs,routing` still asserted) instead of the literal list; (c) the
   "dry run / idempotent apply sends only dumps" checks also allow `nat44_ed_output_interface_get` (DF-3's read-only
   cursor get). Every wave-A feature that adds a domain hits (a)–(c); the literal list would conflict on every merge.
2. `apps/agent/internal/descriptors/core/coretest/fakevpp.go` (A6 said "existing files read-only"): `New()` runs a new
   `extensions` list (3 lines + the var). Without it every agent test that Retrieves all domains fails on the first
   nat44 dump ("no handler"). My plugin model lives in my own `coretest/nat44ed.go` and registers itself in `init()`;
   the next features (bond, bridge, …) can do the same without touching `New()`. Proposal: keep it as the A6 seam.
3. `apps/api/src/testing/fake-agent.ts`: (a) one import line after the last import (the file has no import anchor)
   plus the spread under my P5 anchor; (b) a proven defect fixed in the generic `action` handler: `call.destroy(err)`
   never sent the UNIMPLEMENTED status, so a client hung until its deadline (my e2e got 504 instead of 501); now
   `call.emit('error', err)`. F-vrf-static-ecmp owns that handler's dispatch and may rewrite it.
4. `apps/agent/internal/agent/server.go` (A4): the `Action` signature's `_` became `stream` (every case needs it); if
   F-vrf-static-ecmp / F-neighbors-ra make the same rename the lines merge identically.

## Q3 (decision needed later, default taken): `/state/drift` and NAT lists
Retrieve never invents what VPP cannot report (proto.md §5): pool `name`/`description`, mapping `description`, and a
static mapping's `external.pool` (VPP stores the resolved address/interface). The API's drift compares arrays as a
whole, so `/state/drift` shows `/nat/pools` / `/nat/staticMappings` as changed whenever the running document carries
those labels. Options: (a) the API drift compares arrays of objects element-wise and skips leaves the agent flags;
(b) an agent hook to return stored labels for matched objects (needs `service.go`, A5); (c) accept and document.
Taken: (c) (the UI joins pool names by identity from the running document; `GET /state/nat/summary` does the same).

## Q4 (DF-3 gap, not fixed): one interface as a normal AND a twice-NAT interface pool
`nat44-ed.interface-address` is keyed by the interface only, so the schema-valid pair `{interface: X}` +
`{interface: X, twiceNat: true}` becomes `agent.duplicate-object` (400 at validation). A gap-only fix in
`descriptors/nat44ed` would key it `<if>[/twice-nat]` (ID change → a recreate of existing objects). Not done here.

## Q5 (A4 in the fake agent): the kill in API e2e
The fake agent's `action` handler has no per-feature dispatch (A4 covers only `server.go`), so the API e2e proves that
the kill reaches the agent's Action with the 5-tuple, is audited and maps the gRPC status (501); the success path (200,
the session gone from `vppctl`, audit entry) is proven against the real agent in the topology test. Proposal: the A4
owner's fake dispatch calls a `natSessionKill` case that `features/nat44-ed-sessions/fake.ts` can export.

## Q6 (info): VPP quirks met
- `nat44_user_session_v3_dump` filters by the user's address only, not its VRF (nat44_ed_api.c): the same inside address
  in two VRFs served by one worker lists both VRFs' sessions under each user. The pager tolerates it (counts come from the
  user dump); documented in proto.md §11 terms ("VPP's order of that user's sessions").
- ED deletes need the full 5-tuple (`nat44_ed_del_session` looks up the flow hash with the external host), so the kill
  requires the external endpoint (DF-3's `DeleteSession` made it optional).

## Q7 (lab, manager's call): the rig's netns veths need tx checksum offload off for NATed TCP/UDP
Root cause proven on slot 4 (19:03): NAT44-ED rewrites TCP/UDP that af_packet delivered with a PARTIAL checksum, the far
host drops the segment silently (ICMP passes). `ethtool -K <veth> tx off` inside the rig namespaces fixes it; my topology
test does that after `peers up`. Proposal: `tools/lab rig up` turns it off for every slot (NAT, cnat, det44 tests will all
hit it). Recorded as `### V-new (F-nat44-ed-sessions)` in docs/vpp-code-track.md (A7).

## Q8 (answer to the manager's incident note, VPP crash 18:41:08): not slot 4
At 18:40:59 slot 4 ran only the API e2e (PostgreSQL + the in-process fake agent, no VPP) and `pnpm gen`; this task's
first host run started at 18:57 (its log records `NRestarts (before) = 1`, i.e. after the crash). None of my code sweeps
classify bindings: the topology test's V19 guard (P08's `v19Guard`, copied) reads `classify_table_by_interface` and
resets the write-only ip/l2 bindings to `~0` on the two rig interfaces the agent just created — no loop over table
indexes. The agent code of this task sends only nat44-ed messages. D-126 noted; the NAT packet phase is ~12 s.
