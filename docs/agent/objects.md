# Agent: the objects domain (`internal/objects`)

F-object-model (WBS D5.1). Package `ngfw/agent/internal/objects`; wiring in `internal/subsystems/object_model.go`,
builder/assembler in `internal/desired/object_model.go`, the state RPC in `internal/agent/rpc_object_model.go`.
There are no VPP objects in this domain.

## What is stable (for F-acl, F-host-acl-nftables, later NAT)

| API | contract |
|---|---|
| `Expand(doc *vrxv1.ObjectsConfig, ref string, opts ...Option) (Addresses, error)` | address object or address group → `Addresses{V4, V6 []netip.Prefix, Unresolved []string}`: host → /32 or /128, network → its prefix (masked), range → the minimal CIDR set, fqdn → the resolved addresses (`WithFQDN`), group → the union of its members, recursively. Output is masked, **aggregated** (covered prefixes dropped, sibling pairs merged), sorted, split by family, deterministic. An empty group expands to nothing |
| `ExpandService(doc, ref, opts...) ([]PortSpec, error)` | service object or service group → `PortSpec`s (sorted, deduplicated) |
| `ExpandServiceSpec(*vrxv1.ServiceSpec) ([]PortSpec, error)` | an ACL rule's inline service; `ServiceSpecOf(*vrxv1.ServiceObject)` converts an object |
| `PortSpec{Proto, SrcPortFirst, SrcPortLast, DstPortFirst, DstPortLast, TCPFlagsMask, TCPFlagsValue}` | the shape of `descriptors/acl.Rule`: Proto 0 = any; TCP/UDP/SCTP ports are inclusive ranges, 0–65535 = any; ICMP/ICMPv6: `SrcPort*` = type range, `DstPort*` = code range (0–255 = any); `tcp-udp` yields a TCP and a UDP entry, TCP flags only on the TCP one. One entry per protocol × source range × destination range |
| `Active(s *vrxv1.Schedule, now time.Time, loc *time.Location) (bool, error)` | recurring: the weekday and time of day **on the wall clock of `loc`** (nil = UTC; pass `system.timezone`), window `[start, end)`; once: `start ≤ now < end` with the absolute RFC 3339 instants. DST: a window inside the skipped spring-forward hour is never active, one inside the repeated fall-back hour is active twice |
| `ZoneInterfaces(doc, zone) ([]string, error)` | the zone's interfaces, sorted |
| `WithFQDN(FQDNLookup)`, `WithLimit(n)` | options; without `WithFQDN` every fqdn object is `Unresolved` |
| `MaxEntries = 10000`, `CheckLimit(ref, n) error`, `*LimitError{Ref, Count, Limit}` | the cap: one expansion above it fails with `*LimitError`; a consumer that multiplies expansions into rules (sources × destinations × services) checks the product with `CheckLimit` and reports the error as a DryRun issue **at the rule's pointer** |
| `ErrUnknownObject`, `ErrCycle`, `ErrInvalid` | `errors.Is` targets; cycles are defence in depth (the schema's `objects.*-group-members` rules reject them) |
| `RuntimeFor(stateDir, owner) *Runtime` | the running agent's objects runtime (nil if none); `subsystems.Wiring.ObjectModel()` returns the same. Use it for `FQDN` and `Subscribe` only |
| `(*Runtime).FQDN` | the `FQDNLookup` of the resolver: `Expand(doc, ref, WithFQDN(rt.FQDN))` |
| `(*Runtime).Subscribe(func(Change)) (unsubscribe func())` | **the in-agent notification**: called on the resolver goroutine after every change of the addresses an FQDN resolves to (`Change{Host, Objects, Addresses}`, including an expiry that empties them); keep it short. F-acl re-projects by asking for a resync of the agent's **stored desired state** (`Wiring.RequestResync()` once the A5 hook is set) — never by reading the applied store |
| `(*Runtime).FQDNStates(names...) []FQDNState` | what `FqdnObjectState` serves |

`Addresses.Unresolved` is a **warning** for the consumer (report `objects.fqdn-unresolved`-style issues at the rule), never
an error: an FQDN object that has no address yet expands to nothing (the schema comment on `objects.addresses`).

Anything else exported from the package (`Store`, `Snapshot`, `Register`, `Key`, `Value`, `Kinds`, descriptor names,
`Open`, `NetLookup`, `WriteMetrics`, the refresh and staleness constants) is for the wiring, diagnostics and tests and
may change.

### Project from the request, never from the applied store (review F3)

The objects a consumer expands against are **the transaction's own** `ds.GetObjects()` — the desired state the API sent
from PostgreSQL (rule 2: RPCs carry desired state). The product API sends every implemented domain in every Apply, so
`objects` is in the transaction whenever `acl` is; a transaction with `objects` present but empty (`{}`, or nil after
protobuf decoding — the same thing, proto.md §1) means *no objects*, not "use what is applied". If a partial Apply
(`vrx-agentctl`, tests) manages `acl` without `objects`, the consumer reports an error at the rule (e.g.
`acl.objects-required`) instead of guessing. Out-of-band re-projection (an FQDN change, a schedule tick) goes through a
resync of the agent's stored desired state, which carries its objects. The applied store (`Runtime.Snapshot`) is for
diagnostics and tests only and is deliberately not part of the stable API.

### Example (F-acl projection)

```go
if !in["objects"] { /* rules that reference objects: sink.Errorf(ptr, "acl.objects-required", …) */ }
doc := ds.GetObjects()          // the request's objects (nil = none)
rt := objects.RuntimeFor(stateDir, owner) // the resolver, for FQDN answers only
src, err := objects.Expand(doc, rule.GetSource().GetName(), objects.WithFQDN(rt.FQDN))
var le *objects.LimitError
switch {
case errors.As(err, &le): sink.Errorf(ptr, "acl.expansion-limit", "%v", err)
case err != nil:         sink.Errorf(ptr, "acl.object", "%v", err)
}
for _, n := range src.Unresolved { sink.Warnf(ptr, "acl.fqdn-unresolved", "FQDN object %q has no address yet; it matches nothing", n) }
svc, _ := objects.ExpandService(doc, rule.GetService().GetName())
if err := objects.CheckLimit(ptr, src.Len()*dst.Len()*len(svc)); err != nil { sink.Errorf(ptr, "acl.expansion-limit", "%v", err) }
on, _ := objects.Active(doc.GetSchedules()[rule.GetSchedule()], time.Now(), loc)
```

Schedules: the objects domain does not re-render anything when a schedule turns on or off. **Default (questions): the
consumer re-projects periodically, every 60 s**, and treats an inactive schedule as "rule not rendered". `Active` is a
pure function, so the consumer can also compute the next change itself.

## The domain in the agent

`objects` is an implemented domain (`subsystems.Domains["objects"]`, so it is in `Health.subsystems` and the API sends
it in every Apply). It is realised by an **agent-local descriptor family**, one descriptor per kind, registered in this
order: `objects.tag`, `objects.address`, `objects.address-group`, `objects.service`, `objects.service-group`,
`objects.schedule`, `objects.zone`.

- Key `objects.<descriptor>/<name>`; value: an `ObjectsConfig` holding exactly that one entry in its kind's map (the
  configuration message itself, so no agent-internal model is needed and Retrieve returns the document unchanged).
  Pointer `/objects/<kind>/<name>`.
- Create/Update/Delete change the **store** — a derived record of applied state, not a source of truth — in memory, in
  place (O(1) per object); it is persisted to `<state dir>/objects-<owner>.json` (protobuf JSON, 0600, atomic replace)
  **once per transaction**: the scheduler's verification Retrieve writes it, else 200 ms after the last change, and at
  agent stop. Losing the last unwritten changes in a crash is harmless (Retrieve shows the older set, the next resync
  re-applies). 4 000 objects: 0.17 s in the store, 13.7 s for the whole scheduler transaction (review F1; the rest is
  the scheduler's own per-operation cost, questions Q9). Only FQDN objects reach the resolver.
- A **corrupt** store never stops the agent (review F2): it is moved to `objects-<owner>.json.corrupt-<unix time>`,
  logged as an ERROR and counted (`vrx_agent_objects_store_corrupt_total`); the store starts empty and the resync
  rebuilds it from the stored desired state. A failed write is logged and counted
  (`vrx_agent_objects_store_persist_errors_total`); the changes stay in memory and the next flush writes them.
- Dependencies are optional (ordering only): groups after their members, tagged objects after their tags.
- Apply, rollback, confirm-revert and resync work through the scheduler like any domain; after an agent restart
  Retrieve comes from the persisted store (D-063: real agent state, never an echo of the request). An explicitly empty
  `objects` (D-041) deletes every object.
- DryRun (defence in depth, the schema rules normally catch these first): a group whose expansion fails (cycle,
  unknown member, invalid leaf) is an ERROR `objects.object-model-expansion` at `/objects/<groupKind>/<name>`; a group
  above `MaxEntries` is a WARNING `objects.object-model-expansion-limit` there.

## FQDN resolver

- **What:** every `type: fqdn` address object of the store; objects sharing a host name share one resolution.
- **How:** Go's resolver, `PreferGo` (no cgo, **no exec, no shell** — 00-CONTEXT rule 9), over the system configuration
  (`/etc/resolv.conf`, `/etc/hosts` per nsswitch): the management resolver of the box, never configured by this task
  (the box's Unbound belongs to F-unbound-chrony-syslog). Names are queried fully qualified (trailing dot: no search
  domains), A and AAAA in parallel, 5 s budget. `VRX_OBJECTS_DNS_SERVERS=ip:port[,…]` replaces the system servers
  (test slots and their in-process responder).
- **Refresh:** a **fixed interval**, default **60 s**, `VRX_OBJECTS_FQDN_REFRESH_SEC` overrides it, clamped to
  **[30 s, 1 h]**. Go's resolver does not return TTLs (questions Q2: a `dnsmessage` lookup that does would change
  `apps/agent/go.mod`); the `Lookup` interface already carries a TTL and a TTL-reporting lookup is honoured with the same
  clamp. After a failure: retry in 30 s, doubling, at most the refresh interval.
- **Failure:** the **last good answers are kept** (per family: an AAAA timeout keeps the old IPv6 answers while new IPv4
  answers are used); `error` and `failures` say what happened; a WARN log line `fqdn resolution failed; last-good
  addresses kept`. NXDOMAIN or no A/AAAA at all counts as a failure too (the old answers stay); a name that never
  resolved has no address and expands to nothing.
- **Maximum staleness (D-129):** a failing family keeps its last good answers for at most **24 h** after it last
  answered (`VRX_OBJECTS_FQDN_MAX_STALE_SEC`, clamped to 60 s – 30 days). After that they are dropped with a WARN
  `fqdn last-good answers expired …` (`stale_for`, `max_stale`), counted (`vrx_agent_objects_fqdn_stale_expired_total`),
  and subscribers are told: the object expands to nothing until the name resolves again.
- **Clock steps (review F6):** a persisted next refresh more than one interval ahead is pulled in at start, and one
  more than 1 h ahead at run time counts as due.
- **Persistence and restart:** results live in `<state dir>/objects-fqdn-<owner>.json` (0600). A (re)started agent
  loads them (`fqdn state reloaded … fresh=N due=M`), keeps every fresh entry's next refresh and spreads the due ones
  (overdue while the agent was down, or never resolved) evenly over 30 s: **no query storm**. The loop itself starts at
  most 4 lookups back to back, then waits 250 ms, and writes the state file when idle or at most every 5 s while busy.
  An unreadable state file is only a cache: it is ignored with a warning.
- **Lifecycle:** started by `subsystems/object_model.go` when the family registers (never from `agent.go`), which also
  registers `rt.Close` with the generic close seam `Wiring.OnClose`; `Agent.Stop` calls `Wiring.Close()` (resolver
  stopped, pending store changes and FQDN state written). An in-process re-open for the same state dir and owner
  (unit tests without `Stop`) closes the previous runtime first.
- **State RPC:** `FqdnObjectState` (docs/contracts/proto.md §11) → the API's `GET /api/v1/state/objects/fqdn`.
  Nothing of it is in Retrieve (proto.md §5). No event is published yet (questions Q1: EventKind 11 stays reserved).
