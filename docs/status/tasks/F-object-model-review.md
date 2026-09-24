# F-object-model — review (architecture focus)

Reviewer: independent review agent, 2026-09-24. Branch `task/F-object-model` @ `31249d3` (13 commits).
Diff base: the branch was built on `task/W-seed@df67a8e`, and I reviewed `git diff df67a8e...task/F-object-model` (86 files).
**During this review the manager reset `task/W-seed` onto main (P08 squash-merged, `c2ca3ed`) and re-created the anchor
commit as `a303f0b`**, so `task/W-seed...task/F-object-model` now points at a different merge base (see R1).

## Verdict up front
The architecture holds. The `objects` store is a derived record of applied state, and PostgreSQL stays the only source of
truth (details in §A). The resolver is bounded, safe across restarts and runs no shell. The contract is additive and uses
no §2 number. Where-used is computed from the document. Binding the picker through `SchemaField` is the right approach.
Four owned-file defects must be fixed before F-acl codes against this API:
1. the store rewrites the whole document with an fsync for every object, so the cost grows quadratically (measured below)
2. a corrupt store stops the whole agent, which treats it like a source of truth
3. the "stable API" document tells consumers to fall back to the applied store
4. the exported picker defaults to address objects for fields it cannot classify

The branch also has to be rebased onto the reset W-seed.

## A. Architecture: is the agent-local objects store a second source of truth?
**No, on three conditions. The branch meets conditions 1 and 3 and breaks condition 2 in two places (F2, F3).**

For a domain with no data-plane object, the "actual state" that rule 2's Retrieve has to reflect is the agent's record of
what it applied. That is the same kind of state as the P08 claim and boot stores in the state dir. D-063 requires it to be
real persisted state and not an echo of the request, and it is. The store stays a cache as long as:
1. **Only the scheduler writes it, and only from desired state the API sends from PostgreSQL.** Met:
   Create/Update/Delete are its only writers (`objects/descriptors.go`). The API always sends every implemented domain
   (`commit/validation.service.ts:66-70`, `commit.service.ts:272`). Resync overwrites the store. The topology run deleted
   the file and the objects were back in 0.28 s. Rollback and confirm-revert go through the scheduler.
2. **Nothing treats it as authoritative.** Broken twice:
   - (F2) `OpenStore` *fails closed* on a corrupt file (`store.go:42`). A cache of PostgreSQL state then stops the whole
     agent from starting: interfaces, routing and the data-plane rebuild after `kill -9 vpp`. The error message's own
     remedy is to move the file aside and let the next resync re-apply, which shows that failing open is safe.
   - (F3) `docs/agent/objects.md:21,37` publishes `Runtime.Snapshot()` (the applied store) as a stable *input to consumer
     projection*, with `if doc == nil { doc = rt.Snapshot() }`. That makes a consumer's desired projection (F-acl's ACL
     rules) depend on agent-local applied state instead of the request (rule 2: "RPCs carry desired state"). It is also
     ambiguous: an explicitly empty `objects` in the transaction is also nil (the assembler's own comment, proto.md §1).
3. **Configuration reads (GET /config, where-used, drift's expected side) come from PostgreSQL.** Met: where-used runs over
   the running or candidate document (`usage.ts`), and drift compares PostgreSQL's running document with Retrieve.

The FQDN answers are runtime state, like a DHCP lease: persisted only as a cache and kept out of Retrieve (proto.md §5).
That is correct. The process-global `runtimes` registry (`runtime.go:44`, reached through `RuntimeFor` from
`rpc_object_model.go`) is a workaround while agent core is read-only (A5). Accept it for now and replace it with a Wiring
reference when Q4's seam exists.

## B. Findings (ranked)

**F1 — Medium-High (required). Each object write clones, marshals and fsyncs the whole store, so a transaction costs O(N²).**
Code: `apps/agent/internal/objects/store.go:84-106` (`mutate`: `proto.Clone(s.doc)` + `protojson.Marshal` of the whole
document + `atomicWrite` with fsync on every Create/Update/Delete). `runtime.go:72,157` adds a full `Snapshot()` and a
resolver sync on every write, including non-FQDN objects.

Measured on this branch: sequential Create through the descriptor, on **tmpfs** (fsync is nearly free there, so disk is
slower):

| objects | time | per object |
|---:|---:|---:|
| 500 | 2.3 s | 4.5 ms |
| 1 000 | 9.0 s | 9 ms |
| 2 000 | 36.4 s | 18 ms |
| 4 000 | 2 min 34 s | 39 ms |

Method: `go test -overlay` with a scratch test, nothing written to the tree.

Failure scenario: a commit, rollback or resync after a lost store with about 2 500 or more objects goes past the API's
agent deadline (`VRX_AGENT_TIMEOUT_MS` = 60 s, `apps/api/src/config.ts:26`). The API returns 504 and records the commit as
failed or unknown while the agent keeps applying, so PostgreSQL and the agent diverge until the next resync. A restart
with a lost store takes minutes.

Fix, owned files only:
- (a) mutate in place under the lock (clone only the entry)
- (b) coalesce persistence: mark dirty and flush from one writer goroutine, at most every ~100–200 ms and at `Close`.
  Losing the last unflushed writes in a crash is safe for the reason in §A: Retrieve then shows the older set and the
  next resync re-applies.
- (c) call `syncFQDN` only when an `addresses` entry of type `fqdn` is added, changed or removed.
- Add a unit test: 5 000 Creates finish in under 5 s.

**F2 — Medium (required). A corrupt objects store stops the whole agent.**
Code: `apps/agent/internal/objects/store.go:42`, test `descriptors_test.go` `TestStoreCorruptFailsClosed`.
Failure scenario: after a torn write or disk error, `vrx-agent` does not start at all, so no domain converges and the data
plane is not rebuilt, all because of a cache.
Fix: rename the file to `objects-<owner>.json.corrupt-<unix>`, log ERROR, start empty. The API's resync re-applies it, the
same path the topology test proves. Flip the test to assert the move-aside and the empty Retrieve. (The claim stores fail
closed for a reason that does not apply here: they guard adoption and deletion of VPP objects.)

**F3 — Medium (required, cheap now, expensive after F-acl). The documented consumer pattern falls back to the applied store.**
Code: `docs/agent/objects.md:21,32-38`, `objects/runtime.go:131`.
Failure scenario: F-acl copies the example. A partial Apply (`vrx-agentctl`, tests) or a transaction with an explicitly
empty `objects` then expands ACL references against whatever was last applied. The resulting ACL is not a function of the
request, and determinism breaks during a rebuild.
Fix: document "expand from `ds.GetObjects()` whenever `objects` ∈ the transaction's domains (nil = empty); the product API
always sends it". Out-of-band re-projection (FQDN change, schedule tick) goes through the agent's stored desired state
(resync), never through `Snapshot()`. Move `Snapshot` out of the stable table ("diagnostics/tests") and fix the example
to test `in["objects"]`, not `nil`.

**F4 — Medium (required, downstream). `ObjectPicker` defaults to address objects for fields it cannot classify.**
Code: `apps/web/src/domains/firewall/object-model/model.ts:76-89` (final `return PICKER_KINDS.address`), `ObjectPicker.tsx:53-72`.
Failure scenario: `acl.ts:230,246,257` mark `attachments[].list`, `macipAttachments[].list` and the host attachment `list`
as `widget: 'object-picker'` with no help text. Once F-acl passes `objectModelWidgets`, those selects offer *address
objects* as an enum, and the correct ACL list cannot be chosen.
Fix: when there is no `objectKinds` hint, no kinds in the help text and no known property name, render the plain field
(delegate to `SchemaField` without an enum) instead of guessing. Add a test for `acl.attachments[].list`.
Follow-up (manager or P02b, questions file): put explicit `x-vrx-ui.objectKinds` on every `object-picker` field in
`objects.ts`/`acl.ts`. The widget already honours it, and parsing English help text is brittle.
Low companion issue: while the objects query loads, or when a reference dangles, `enum` is `[]` or lacks the current
value, and MUI renders the select blank. Always include the current value, labelled as missing.

**R1 — Required process item (not a code defect). The branch base is stale.**
`task/W-seed` is now `a303f0b` (the anchor commit re-created on the P08-squash main).
`git merge-tree task/W-seed task/F-object-model` gives 17 conflicts; `main` gives 13. They are all trivial: add/add at the
re-created anchors, which is a union (checked on `projection.go`), plus generated files, which need regeneration.
Merge or rebase onto `task/W-seed@a303f0b`, run `pnpm gen && make -C apps/cli gen docs`, and re-run
`TMPDIR=/tmp/g-w3 tools/ci.sh --base main`. The pasted CI (48962f2) is against the old base. The Q8 guard race is fixed on
main (D-127, `7edac8c`), so one clean run is enough.

**F5 — Low-Medium (manager, merge process). The Q5 hunks in `service_test.go` diverge between wave-A branches.**
`apps/agent/internal/agent/service_test.go:148,676`. F-object-model, F-acl and F-host-acl-nftables carry this exact
text. F-rpf-adl-pbr has the same code without the trailing comment. F-nat44-ed-sessions and F-nat44-ei-64-66-nptv6 add a
`strings.HasPrefix(…, "interfaces,vrfs,routing")` guard. So the second of these merges conflicts on the same two lines.
Also, Health's subsystems *are* `implementedDomains()` (`service.go:673`), so the Health assertion is now tautological.
Fix (manager): land one canonical version on main before the first wave-A merge, preferably the nat44 variant (equality
plus the P08-core prefix, which is not tautological). Every branch then takes main's side.

**F6 — Low. The persisted refresh time is trusted without a bound.**
`objects/resolver.go:194-213` (`load`), due checks at `:267` and `:348`.
Failure scenario: a wall-clock step backwards (VM boot before chrony syncs), or a persisted future `nextRefresh`, stops
refreshes for the size of the step. The comparisons use the wall clock, and loaded times have no monotonic reading.
Fix: in `load`/`sync(initial)`, clamp `NextRefresh` to at most `now + r.refresh`. In `resolveDue`, treat
`NextRefresh > now + MaxRefresh` as due.

**F7 — Low (question for the manager: decide and log). Last-good answers are kept forever.**
`objects/resolver.go:409-410` (NXDOMAIN or no records is a failure, so the last-good answers stay; decision 6).
Scenario: an allow rule keeps permitting an address whose DNS record was deliberately removed for as long as the failures
continue. That is pfSense-like behaviour, but it is a security policy choice.
Options: (a) keep forever (current); (b) a maximum staleness, for example 24 h, after which the object expands to nothing
with a warning.
Recommendation: (b), with `staleSince` shown in `FqdnObjectState` and the UI later.

**F8 — Low (test code). The fake-agent handler can hang.**
`apps/api/src/testing/fake-agent.ts:633`: `void import(…).then(…)` never calls `cb` if the import or the handler throws,
so the test call hangs until its deadline. Add `.catch((e) => cb(e as Error))`.

**F9 — Low (follow-up). Where-used duplicates the reference map in `acl.rule-references`.**
`apps/api/src/features/object-model/usage.ts:1-30,206-233` walks the ACL reference paths that `semantic/acl.ts`
(`acl.rule-references`) already encodes. The two lists must stay in sync, and F-acl or NAT must edit an F-object-model file
to add a reference. Follow-up: export one reference walker from `packages/schema` (P02b-owned, so through the manager).
Cosmetic: the header cites "names unique within their kind (vdom.md #2)", but D-062 makes addresses, services and groups
one namespace. `definedAs` handles both cases.

Checked and fine:
- **Hotspot discipline:** every shared hunk sits directly under its own anchor and is listed in the status file.
- **Checklist line "every Register passes a Wiring store":** the objects family uses its own persisted store in `StateDir`.
  It owns no VPP object, so it needs no claims, and nothing is kept in memory only.
- **Contract `e94ad5f`:** proto, generated files, the P5 stub, proto.md §11 and `-contract.md` only. Additive. Names carry
  the `FqdnObject*` prefix. No §2 number is used (EventKind 11 and ObjectsConfig 8–9 stay free).
- **Rules and host hygiene:** no `exec`, `child_process`, `vppctl` or `pkill`, and no VPP object. Test secrets come from
  `runSecret()`. The global AuthGuard covers the new routes. Every UI string goes through `t()`, and the CSS uses logical
  properties only.
- **Topology test:** runs under `flock -s`, kills only PIDs it started, and checks `NRestarts` before and after.

## C. Resolver assessment (the brief's specific questions)
- **Bounded:**
  - 5 s budget per lookup, with A and AAAA in parallel
  - hosts resolved one at a time, bursts of 4, then 250 ms spacing (≤ 16 hosts/s)
  - retries start at 30 s and double up to the refresh interval
  - in an outage the loop falls behind but never storms
- **Restart-safe:** the state is persisted (0600, atomic write). On start it reloads, keeps the fresh entries and spreads
  the due ones over 30 s. The topology run confirms: 0 queries in the first 3.3 s after a restart. The loop starts from
  the owned `subsystems/object_model.go`, not from `agent.go`.
- **No shell, no user input into resolv:** Go resolver with `PreferGo`. Nothing ever writes `/etc/resolv.conf` or daemon
  configuration. The configured FQDN goes only to `LookupNetIP` as a DNS name, and the schema `hostname` pattern plus Go's
  name check bound it. `VRX_OBJECTS_DNS_SERVERS` is operator environment, parsed as `ip:port`.
- **Last-good:** kept per address family. An authoritative "no A" with AAAA answers clears v4, which is correct.
  NXDOMAIN or no records keeps the last-good answers (see F7).
- **1 h dormancy:** sound. A rollback or a resync after a lost store gets the answers back without a query, and they are
  re-queried at once because they are due, so the stale window is short. Pruning can take up to about 2 h in practice
  when no FQDN object is left (the loop sleeps `MaxRefresh`). That is harmless.

## D. Answers to Q1–Q4 (recommendations to the manager)
- **Q1 (EventKind 11 not taken; FQDN changes are state only, and the page polls every 5 s):** agree. Keep 11 reserved.
  Wire `Env.Publish` in `agent.go` in the same A5 commit that seeds F-acl's resync hook. The event (EventKind 11 plus the
  `objects.events` topic, P6) then becomes one small contract follow-up owned by F-object-model. The in-agent
  `Runtime.Subscribe` is enough for F-acl now.
- **Q2 (fixed 60 s interval, clamped to 30 s–1 h, vs `dnsmessage` TTLs):** accept (b) for wave A. `Lookup` already carries a
  TTL, so (a) is a drop-in replacement. Open a TD item: the manager promotes `golang.org/x/net` to a direct dependency on
  main (D4), then a TTL-reporting lookup replaces `NetLookup`. It is not a merge condition.
- **Q3 (refresh interval as agent environment, not configuration):** agree. It is operational tuning, not user
  configuration, and a field would mean anchoring P02b's `objects.ts` for no user story. Keep `ObjectsConfig` 8–9 free
  until someone asks.
- **Q4 (resolver lifecycle):** yes, add the hook, as a *generic* seam. `Agent.Stop()` → `a.wiring.Close()`, and
  `Wiring.Close` runs closers that families register from their own files (`w.onClose(rt.Close)`). P11, P12 and
  F-wireguard background work then reuse it. Once it exists, drop `CloseObjectModel` and the in-process re-open close
  path, and pass the runtime through Wiring instead of the global `RuntimeFor` registry.
- **Q5 (out-of-list `service_test.go` edit):** justified and disclosed. Every domain feature must touch those assertions,
  but the sibling branches diverge, so see F5 for the manager action.
- **Q6:** consistent with D-128 (root cause `show trace`). **Q7:** agree, a CLI follow-up for P13. **Q8:** fixed on main
  (D-127).

## E. What I ran (unit only, no host runs)
```
$ go vet ./internal/objects/ ./internal/subsystems/ ./internal/desired/ ./internal/agent/      (clean)
$ go test -race -count=1 ./internal/objects/ ./internal/subsystems/ ./internal/desired/
ok  ngfw/agent/internal/objects 2.029s · ok ngfw/agent/internal/subsystems 1.185s
$ TMPDIR=<short> go test -race -count=1 ./internal/agent/
ok  ngfw/agent/internal/agent 15.780s
  (with the long scratch TMPDIR, 3 socket tests fail on the 108-char sun_path limit: environment, not the branch)
$ npx vitest run src/features/object-model/usage.test.ts            (apps/api)  Tests 4 passed (4)
$ npx vitest run src/domains/firewall/object-model src/nav/nav.test.ts src/locales/locales.test.ts   (apps/web)
  ✓ model.test.ts (5) ✓ ObjectsPage.test.tsx (3) ✓ nav.test.ts (5) ✓ locales.test.ts (12)
F1 scale probe (go test -overlay, scratch file, tmpfs): 500→2.26 s · 1000→9.0 s · 2000→36.4 s · 4000→2m34.5 s
```
`packages/{schema,ui-kit,api-client}/dist` were built for the TS tests and then removed. I did not run `tools/ci.sh` or
the topology suite; that has to happen after R1 anyway.

**APPROVE WITH CHANGES** — required before merge: F1, F2, F3, F4 (owned files) and R1 (rebase onto `task/W-seed@a303f0b`
+ regenerate + a green `tools/ci.sh --base main`). Manager actions: F5, the Q4 seam, and the F7 decision. F6, F8 and F9 are follow-ups.
