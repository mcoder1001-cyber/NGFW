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

## Q9 (tools/ci.sh bug, manager-owned): the contract guard fails a branch whose commit subjects exceed 8 KiB
`TMPDIR=/tmp/g-w4 tools/ci.sh --base main` stops at "CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT" although the
branch carries `contract(proto): nat sessions`, `contract(schema): nat adjacent pools` and `contract(api-client): …`.
Cause: under `set -o pipefail`, `git log --format=%s "$mb..$TIP" | grep -qiE '^contract…'` — `grep -q` exits at the first
match, git log's second 8 KiB write gets SIGPIPE (exit 141) and pipefail turns the `if` false. My branch's subjects since
the merge base with main are 8 383 bytes (it still contains P08's and W-seed's history); reproduced 8/8 in a shell with
`set -o pipefail` (`PIPESTATUS` = `141 0`). Fix: `grep -iE … >/dev/null` (no `-q`) or capture the log in a variable
first. Until then I ran the full gate without the guard (`tools/ci.sh quick`, green — see the status file) and list the
contract commits by hand.

## Q10 (D-128 acknowledged): `vppctl trace` / `show trace` removed from the topology test
The port-forward step used `vppctl trace add af-packet-input 40` + `show trace max 5000` in runs 1–5 (18:59–19:13,
before the D-128 message; NRestarts stayed 1 → 1). Removed (commit below): the out2in translation is now proven by
`vppctl show nat44 sessions filter i2o saddr 10.4.1.2 filter i2o sport 80` (the static session `o2i 10.4.2.110:8080`,
`external host 10.4.2.2:41001`, "static translation") and the same row in `GET /state/nat/sessions` (`static: true`),
next to the tcpdump in the lan netns. The trace excerpt in the status file is from run 5 and marked as such.

## Fix round 1 (review 0ee338e, D-129)
- D-129 noted: Q2 all four out-of-anchor edits kept (the coretest `extensions` seam = A6, one copy at merge); Q3 (c)
  kept, (d) is the manager's tech-debt row; Q4 goes to a later task; Q7 the manager adds `tx off` in `tools/lab rig up`
  (my test's two `ethtool` lines can go then). L5: no rebase by me.
- **Q11 (info): the API e2e was not run this round.** Slot 4 (`vrx_w4`) now belongs to F-nat44-ei, and the e2e
  global setup creates and DROPS the slot database — running it would destroy that task's database. No other slot was
  assigned for this round, so the API side is covered by its unit tests (vitest, no DB) plus the agent and web tests;
  the e2e file is updated (`pageSize=257` is the new bad value) and runs unchanged on a free slot.
- **Q12 (tech debt, review L3):** the RPCs still build `nat44ed.New(s.vpp, s.owner)` per call (read-only helpers,
  never touch claims) because `Service` has no Wiring handle (A5). When A5 gets a seam, expose the registered plugin as
  `Wiring.Nat44ED()` from `subsystems/nat44_ed.go` and use it. L4 is done (streaming `EachUserSession`, a gap-only DF-3
  helper).
- **Q13 (merge note): main's ci.sh vs the branch's old deploy/vpp harness.** The gate with main's ci.sh fails only in the
  apply-startup harness step. The branch's pre-D-103 `deploy/vpp/test-apply-startup.sh` has no `VRX_TEST_SHARD`, so
  main's ci.sh runs four full copies of it in parallel, and they collide on scenarios 24 and 26. Serially, the same copy
  passes 101/101. The file is not mine, and the L5 rebase replaces it with main's copy. After the rebase the step runs
  main's sharded harness and should pass.

## Fix round 1, part 2 (after the 03:40 reset; CONTINUE-quota "Also new")
- **D-132** done: NatSessions and the NatSummary computation take one per-agent walk slot (one VPP session walk at a
  time; a caller whose deadline passes gets DEADLINE_EXCEEDED); no UI timer below 30 s (unfiltered grid 30 s, filtered
  grid Refresh only, summary 30 s); Refresh button present.
- **WEB-1**: the NAT forms no longer call `dropPhantomOptionals` (2 calls removed).
- **TD-11b** (not yet on this branch): every descriptor this task registers is a `natcommon.Descriptor` (the 10 nat44-ed
  descriptors + `nat44-ed.vrf-table`). TD-11b itself gives `natcommon.Descriptor` its `CheckPersistent()` (natcommon is
  read-only for me). Global singletons return nil, and every other descriptor requires a `Persistent()` claim store,
  which `registerNat44ED` already passes: `natcommon.WithClaims(Wiring.KeyedClaims("nat"))`, whose file store
  implements `Persistent()` on task/TD-11b@14daf722. So no change is needed in my files. After the rebase, the guard
  runs in every NAT agent test (`newSvc` → `subsystems.Register`).
- **Q13 resolved (not a flake, not this task):** both fix-round gate runs failed only in the apply-startup harness step.
  This branch has no diff under `deploy/`, `tools/` or `apps/agent/cmd/vrx-startupgen` against its W-seed base, and
  `vrx-startupgen` plus its testdata are byte-identical to main's. The step fails because main's ci.sh shards a
  harness copy that cannot shard (the pre-D-103 file from the old base). Main's own `deploy/vpp`, run sharded exactly as
  main's ci.sh runs it against this tree's `vrx-startupgen` (the same binary as the gate's), passes 138/138 at load 59
  (logs `/root/ngfw-wt/logs/F-nat44-ed-sessions-mainharness-shard{1..4}.log`). I did not merge main: a trial
  `git merge-tree HEAD main` shows about 35 conflicts, mostly in files I do not own (`agent.go`, `service.go`,
  `seams.go`, `stores.go` add/add from the re-cut W-seed). That is the L5 rebase the merger does.
