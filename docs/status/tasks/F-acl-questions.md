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
