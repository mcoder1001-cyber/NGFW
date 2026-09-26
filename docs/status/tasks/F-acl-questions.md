# F-acl — questions and decisions for the manager

Worker: slot 3, branch `task/F-acl`. Each entry says what I did meanwhile (never waiting).

## Q1 (A5) — wire `subsystems.Env.Resync` in `agent.go` — RESOLVED by TD-8 (merged into this branch at 6fe374ce)
The re-projection trigger (schedules every 60 s, FQDN change events) lives in my `subsystems/acl.go` and calls
`Wiring.RequestResync()`, which is a no-op while `Env.Resync` is nil. `agent.go` (read-only for me) builds `Env` before the
`Service` exists, so the hook needs a late binding. Proposed hunk in `agent.Start` (3 lines, no behaviour change for
other families):
```go
var svcRef atomic.Pointer[Service]                       // before subsystems.Register
… subsystems.Env{…, Resync: func() { if s := svcRef.Load(); s != nil { go s.Resync(context.Background()) } }}
svcRef.Store(svc)                                        // after NewService
```
`Service.Resync` takes the transaction lock, so it serialises with Apply. My watcher asks only when an **applied** rule's
schedule changed state or an FQDN object used by an applied rule changed addresses, and coalesces requests (≥ 5 s apart).
Meanwhile: implemented and unit-tested with a fake `Env.Resync`; without the hook the watcher logs once
`acl re-projection needed but the agent's resync hook is not wired` and a schedule change is applied at the next commit.

## Q2 (core) — gRPC message size for a 100 000-rule list
The whole configuration travels in one `ApplyRequest`/`DryRunRequest` (proto.md §1) and comes back in `RetrieveResponse`.
The agent's `grpc.NewServer()` (agent.go) has the default 4 MiB receive limit, and the API's `DataplaneClient`
(agent.client.ts, construction options are not under my anchor) the default 4 MiB receive limit. A 100k-rule list is
≈ 6–8 MB of protobuf, so the 100k commit will fail with `RESOURCE_EXHAUSTED` until both are raised (e.g. 64 MiB:
`grpc.MaxRecvMsgSize(64<<20)` on the server, `'grpc.max_receive_message_length': 64 << 20` on the client). The API's
body limit (8 MiB, app.ts) also caps a merge patch of 100k rules. Meanwhile: 10k is measured end to end; the 100k step is
opt-in and its failure point (and the per-layer timings that do work: projection, `acl_add_replace`) are reported.

## Q3 (D-131) — removing F-rpf-adl-pbr's `pbr.acl-ref` stand-in
The stand-in (`subsystems/rpf_adl_pbr.go` `registerACLBridge`/`aclRefs`, test `TestRpfAdlPbrWithoutFAcl`) exists only on
`task/F-rpf-adl-pbr`, which is not in my base, so I cannot delete it on my branch. It already deactivates itself when
`acl.acl` is registered (my registration uses the name `acl.acl`). Proposal for the merger (F-rpf-adl-pbr merges first):
delete `registerACLBridge` + its call, the `aclRefs` type and `TestRpfAdlPbrWithoutFAcl` in the F-acl merge commit; my
branch carries the replacement test (a PBR/ABF policy naming an F-acl list applies) once `abf` is registered on main.

## Q4 — time zone of ACL schedules
`objects.Active(schedule, now, loc)` needs `system.timezone`, but the agent never stores `system` (not an implemented
domain, `mergeDomains`), so a resync could not see it. Decision: evaluate schedules on the agent process's local time
zone (`time.Local`, i.e. the box's `/etc/localtime`, which the system feature sets from `system.timezone`) for commits
and resyncs alike — never a mix. Documented in `docs/user/firewall/acl.md`.

## Q5 — one ACL projection environment per process
`project()` (projection.go) receives only the document, so the FQDN lookup and the D-071 globals flag reach the ACL
builder through a registry that `subsystems/acl.go` sets at registration (`desired.SetACLEnv`); the last registered
agent in a process wins. That is exact for the product (one agent per process) and for the tests (one agent at a time).
Proposal for later: pass an env from `Service` through `project()` (core change) — no action needed now.

## Q6 — TD-11b ownership declarations for the acl family (done, gap edit in descriptors/acl)
TD-11b (merging) refuses to start an agent whose descriptors declare neither `RecordsNoOwnership()` nor
`CheckPersistent() error`. TD-11b does not touch `descriptors/acl`, so I added `descriptors/acl/ownership.go`:
acl.acl, acl.macip-acl, acl.interface-binding, acl.macip-interface-binding and acl.stats-enable declare
`RecordsNoOwnership()` (owner tags; the stats switch is never deleted, V7); acl.etype-whitelist declares
`CheckPersistent()` over its claim store (structural `Persistent() bool`, the dfkit/persist protocol) — on this branch
`KeyedClaims` has no `Persistent()` yet (TD-11b adds it), so the guard passes only after TD-11b is merged; nothing on
this branch calls the guard. Test: `subsystems.TestACLDescriptorsDeclareOwnership`. My tracker wrappers
(`actions/acl.TrackedACL/TrackedMacip`) embed the DF-4 descriptors, so the declaration is promoted.

## Q7 — one shared hunk outside an anchor: `coretest/fakevpp.go` `New()` calls `v.installACL()`
Now that `acl` is an implemented domain, every agent-level test's Retrieve reaches `acl_dump`; the fake needs the
acl plugin model from the start (the owned `coretest/acl.go`). There is no extension hook in `New()`, so it gets one
line (like P08's `v.installIfExt()`). Also `agent/service_test.go` used `acl` as its example of an unimplemented
domain; that assertion now uses `management` (one line, like F-object-model's Q5).

## Q8 — the 100 000-rule step needs a manager window (and Q2's limits)
Measured on the host (slot 3, `VRX_ACL_SCALE=10000 test/topology/acl/run.sh`, NRestarts 1 → 1): raw `acl_add_replace`
of 10 000 rules 0.013 s, CSV import 2.8 s, commit 10.8 s end to end (agent reconcile 2.26 s of it), rule editor first
page 0.04 s (running) / 0.7 s (candidate). A 10 000-rule `ApplyRequest` is ≈ 1.2 MB of protobuf, so 100 000 rules are
≈ 12 MB: above the 4 MiB gRPC default on both sides (Q2) — the 100k commit will fail with RESOURCE_EXHAUSTED until the
limits are raised; the raw `acl_add_replace` probe and the CSV import (≤ 64 MiB, streamed) do not depend on it. The
step is opt-in and ready: `eval "$(tools/lab env 3)"; VRX_ACL_SCALE=100000 test/topology/acl/run.sh -run TestACLTopology`
(it prints NRestarts before/after the step, the raw `acl_add_replace`/`acl_del` time of a 100 000-rule probe ACL
tagged `w3-probe:scale`, the import/commit/first-page timings, and removes everything again). Please run it in a
manager window (D-064) or tell me when I may; the unit-level projection of 100 000 rules takes 2.6 s
(`internal/desired TestACLListLimitAndProjectionTime`).

## Q9 — hit counters are pulled every 30 s, not pushed over WS
The prompt says "hit-counter columns refreshed from WS". A WS topic would need the API to poll the agent for every
subscribed list anyway (AclState is unary; StreamStats is core and interface-only). Decision: the editor asks for the
counters of exactly the visible rows (≤ 1000, stats segment only, no VPP dump) every 30 s and on *Refresh* (D-132:
nothing that walks VPP below 30 s). A WS topic can be added later behind the same route without changing the UI's
data shape.

## Q10 — counters availability comes from VPP, not from the D-071 role
Slot agents are never the globals owner, so they never switch the counters on. When the flag is already on (the globals
owner, or a test under `flock -x` on the globals lock) the per-rule counters are real, so AclState reports them as
available whatever the role; when it is off, `counters_available=false` with the reason. The flag is read with the
read-only CLI `show acl-plugin tables mask` (V7: no API getter). My topology test switched it on once (opt-in
`VRX_ACL_STATS_GLOBALS=1`, flock -x) and — per the envelope — never off: it is on on the shared VPP now.

## Q11 — a candidate edit of a 100 000-rule list takes ~40 s (datastore, not F-acl)
`POST /actions/acl/lists/{name}/rules/bulk` and the CSV import write the new rules array with ONE
`DatastoreService.putCandidate`. With a 100k-rule candidate that call takes ~40 s (bulk disable of 1 000 rules 43 s, a
one-rule move 40 s; import 25 s): the datastore parses the whole document (Zod), redacts it twice for the before/after of
the edit and walks it for secret changes, then writes a ~15 MB jsonb row. A per-rule pointer edit (the generic routes) has
the same cost. Suggestion for the datastore owner: redact/diff only the edited subtree (the acl domain has no secrets).
Reads are fine now (first page 2.5 s, decision 8 in F-acl.md). Not changed by me (datastore is not mine).

## Q12 — ui-kit SchemaForm fills absent optional objects (TCP flags edited as JSON text)
The web sub-worker found that SchemaForm fills an absent optional object (`service.spec.tcpFlags`) with empty fields, so
every TCP rule failed validation; the rule dialog edits that one field as JSON text (`optionalObjectsAsJson()` in
`domains/firewall/acl/model.ts`). Same cause as P08-questions Q2; a ui-kit fix would let the dialog use the plain form.

## Q13 — screenshots wait for TD-25 (V19 sanitizer cap)
Since the 04:27 VPP restart the TD-3 sanitizer refuses interface creates on the shared VPP (122 freed classify indices >
cap 64). Manager: host runs paused until TD-25. The screenshot run is ready (`TestACLScreenshots`, also a no-rig mode that
creates no interface and no MACIP classify tables). Also recorded: my first three passing topology runs deleted the rig
interfaces while the test's foreign ACL was still bound to host-w3l0 (VPP cleared the list itself, NRestarts unchanged);
the test now unbinds the foreign ACL first (D-095c) and cleans up through the API in `t.Cleanup` even when a step fails.

## Q14 — fold with F-host-acl-nftables at the rebase (manager note)
Both branches add an `ACL` constant and a `Domains["acl"]` entry in `subsystems.go`, and each reports the other's leaves as
`agent.unsupported-field`. When F-host-acl-nftables is on main, I merge main and fold per its review Q8: one `ACL` const and
one `Domains` entry (my six names + host-acl's), my VPP assembly first then host-acl's in `assemble()`, and both
unsupported-field blocks removed (mine: `acl.host` / `acl.hostAttachments` in `desired/acl.go`).
