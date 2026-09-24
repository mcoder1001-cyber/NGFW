# F-object-model — questions and decisions for the manager

Written while working; nothing here blocks the task (each item states what I did meanwhile).

## Q1 — FQDN changes are state only (EventKind 11 not taken)
The A5 seam exists (`subsystems.Env.Publish`, `Wiring.Publish`) but `agent.go` does not set it (W-seed: "agent.go does
not set the hooks yet"), so an `EVENT_KIND_FQDN_CHANGED` published from `subsystems/object_model.go` would be dropped.
Envelope fallback taken: FQDN changes are exposed as state (`FqdnObjectState`) only; the web page polls it.
The in-agent notification F-acl needs exists: `(*objects.Runtime).Subscribe(func(objects.Change))`.
**Ask:** when the manager wires `Env.Publish` in `agent.go`, F-object-model (or F-acl) can add EventKind 11 + the
`objects.events` topic (P6) in one small contract commit; §2's number 11 stays reserved.

## Q2 — go.mod: fixed refresh interval instead of DNS TTLs
Querying the resolv.conf servers with `golang.org/x/net/dns/dnsmessage` makes `go mod tidy` (run by `pnpm gen`, the CI gen
gate) move `golang.org/x/net` from `// indirect` to a direct requirement, i.e. a change to `apps/agent/go.mod`, which this
envelope does not let me touch (D4). Options: (a) dnsmessage + go.mod change by the manager; (b) Go's `net.Resolver`
(PreferGo, the system resolver) with a fixed refresh interval clamped to [30 s, 1 h]. **Taken: (b)**, default 60 s,
`VRX_OBJECTS_FQDN_REFRESH_SEC` overrides it. The resolver's lookup is an interface returning `(addresses, ttl)`; a TTL of 0
means "unknown → fixed interval", so (a) is a drop-in follow-up (a dnsmessage lookup that returns TTLs; the clamp to
[30 s, 1 h] is already applied to whatever it returns) once x/net is promoted on main.

## Q3 — refresh interval is an agent setting, not configuration
`fqdn.refreshSec` would need a key line in `packages/schema/src/domains/objects.ts`, which has no C1 anchor and is P02b's
file (envelope: must not touch). The interval is therefore the agent environment variable above. If a per-box setting is
wanted in the configuration, the manager can seed an anchor in `ObjectsSchema` and `ObjectsConfig` (8 is allocated).

## Q4 — lifecycle of the resolver goroutine
`subsystems.Register` has no context and `Agent.Stop()` calls no `Wiring` hook, so `subsystems/object_model.go` starts the
resolver when the family registers and it stops (a) at process exit, (b) when the same owner registers again in the same
process (an in-process agent restart in tests closes the previous runtime first) or (c) through `Wiring.CloseObjectModel()`.
**Ask:** a one-line `a.wiring.Close()` in `Agent.Stop()` (A5, manager) would make (b) unnecessary.

## Q5 — two assertions in a file I do not own: `apps/agent/internal/agent/service_test.go`
`TestApplyRetrieveIdempotent` and `TestGRPCRoundTrip` compared `Retrieve`/`Health` subsystems with the literal
`"interfaces,vrfs,routing"`; every wave-A task that implements a domain (objects here, nat for F-nat44-ed-sessions, …)
breaks them. Changed both to `strings.Join(implementedDomains(), ",")` (two lines, commit c9584c2) so the next domain
needs no edit. If another feature makes the same change the hunks are identical; if it chose another fix, take either.

## Q6 — VPP crash 18:41:08 (manager's incident note, D-126): not F-object-model
F-object-model creates no VPP object and has no classify/policer code. Before 18:55 this task ran only unit tests (fake
VPP), the API e2e (fake agent) and web tests. The first real agent of this slot started at 18:56:33 (topology run 1,
`NRestarts=1` before and after), after the crash. Nothing of slot 3 needed re-creating.

## Q7 — no CLI command for FQDN state / where-used
`apps/cli` is P13's. The operations `ObjectModel_fqdn` and `ObjectModel_usage` are in the regenerated operation table;
a `show objects fqdn` / `show objects usage <name>` pair is a small follow-up for the CLI owner. The user doc gives the
REST calls and the configuration-mode commands (`set/merge/delete objects …`).

## Q8 — `tools/ci.sh` contract guard fails at random (pipefail + `grep -q`) — manager-owned file
`do_contract_guard` runs `git log --format=%s "$mb..$TIP" | grep -qiE '^contract(\(|:|!)'` under `set -euo pipefail`.
`grep -q` exits at the first match; when `git log` still has output to write it dies of SIGPIPE, the pipeline returns
141 and the guard reports "CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT" although the branch has six contract commits.
On this branch it failed 173 of 200 times (loop over the exact pipeline, `set -o pipefail`); whether a branch is hit depends
on how early its first contract commit appears in the log. **Proposed fix (one line):** read everything, e.g.
`if git log --format=%s "$mb..$TIP" | grep -iE '^contract(\(|:|!)' >/dev/null; then` (or capture the subjects in a
variable first). Meanwhile I re-ran `tools/ci.sh --base main` until the guard was not hit (attempt count in the status file).

---
## Fix round 1 (review 47beb50) — answers taken and new items

- **Q1–Q3:** the reviewer's recommendations are taken as they stand. EventKind 11 stays reserved. The event comes with the A5
  commit that wires `Env.Publish`. **Q2:** please open the TD item: promote `golang.org/x/net` on main, then a TTL-reporting
  `Lookup` replaces `NetLookup`. Q3: the interval stays an agent setting.
- **Q4 (done, manager-directed):** there is a generic close seam, `subsystems/lifecycle.go` (`Wiring.OnClose(f)`, `Wiring.Close()`).
  The closers are kept beside the Wiring, so `subsystems.go` is not edited. `Agent.Stop()` calls `a.wiring.Close()`
  (`agent.go`, +3 lines after `a.svc.Close()`). `CloseObjectModel` is dropped. The state RPC still reaches the runtime
  through `objects.RuntimeFor`, because agent core does not hand the Wiring to the gRPC server. An in-process re-open still
  closes the previous runtime (unit tests that build a Service without `Agent.Stop`).
- **Metrics (manager: "bump a metric"):** `agent/metrics.go` gets one line, `objects.WriteMetrics(w)`, next to
  `ifsanitize.WriteMetrics(w)`. It adds `vrx_agent_objects_store_corrupt_total`,
  `vrx_agent_objects_store_persist_errors_total` and `vrx_agent_objects_fqdn_stale_expired_total`.
- **F5:** see R1. On the merge I take main's version of the two `service_test.go` lines if main has one.

## Q9 — the scheduler's own cost per operation is O(N² log N) (P05 core, not changed here)
After F1 the store costs 0.17 s for 4 000 objects. A whole scheduler transaction of 4 000 objects still takes 13.7 s
(1 000: 0.9 s, 2 000: 3.6 s). The profile shows `scheduler.(*executor).dependents`, which runs `sortedKeys` (a full sort
of the transaction's keys) for every operation, at 74 % of the time, and `topo` at 19 % (it sorts `ready` for each
node). That is well inside the API's 60 s budget, but it grows quadratically. Every domain pays it; objects is just
the first domain with thousands of objects.
**Proposal (P05 owner/manager):** build the dependents index once per transaction, and use a heap for `ready`.

## Q10 — follow-ups from the review that stay open
- **F4 follow-up (P02b/manager):** put explicit `x-vrx-ui.objectKinds` on every `object-picker` field in
  `objects.ts`/`acl.ts`. The widget honours it already, and parsing the English help text is brittle.
- **F4 low companion:** while the objects query loads, or when a reference dangles, the single-value select can show
  blank. `WidgetProps` carries no value and ui-kit exports no form hook, so the fix needs a ui-kit change: either pass
  the current value to custom widgets, or export `useWatch`.
- **F9:** where-used duplicates the reference map of `acl.rule-references`. The follow-up is to export one reference
  walker from `packages/schema` (P02b).
- **F7 UI:** `staleSince` in `FqdnObjectState` (an additive field 8 on this task's own message) and in the UI column.
  Not done in this round; the WARN log and the metric carry it for now.
