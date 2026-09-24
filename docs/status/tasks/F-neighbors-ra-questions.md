# F-neighbors-ra — questions and decisions for the manager

Written while working (never waiting). Each item: what, options, what I did.

## Q1. Contract committed on the task branch (additive) — please review
`19935ee contract(schema): neighbours, RA, proxy-ARP/ND` and `61412e5 contract(proto): ListNeighbors, arp_flush,
neighbour event`; details in `F-neighbors-ra-contract.md`. Numbers only from wave-A-hotspots §2 (Interface 15–17,
Subinterface 13–15, Vrf 4, RoutingConfig 10, ActionRequest 4, EventKind 10). Building continues against them.

## Q2. `Wiring.Connected` has no F-neighbors-ra anchor (W-seed seeded none in that function)
The envelope asks for "one line in `Connected`" (re-subscribe neighbour events on every VPP (re)connect, like DF-8's
`Reconnected`). Options: (a) one line directly after `w.dhcpClient.Reconnected()`; (b) poll `Client.Connected()` from a
goroutine started in `Register` (misses a fast reconnect, and Register has no lifetime context). **Did (a)**; the line
calls a method of my owned `subsystems/neighbors_ra.go`. Listed under "Shared hunks"; a merge conflict with another
feature's line there is a trivial union.

## Q3. `Env.Publish` is not wired in `agent.go` yet (TD-8) → neighbour events are dropped until then
The watcher publishes `EVENT_KIND_NEIGHBOR_CHANGED` through `Wiring.Publish`. With a nil sink it does not even start
(no VPP subscription for nothing). The UI therefore polls the table every 5 s and additionally invalidates on the
`neighbor.events` WS topic, so it becomes event-driven the day TD-8 lands, without a UI change.

## Q4. Domain files have no import anchors (C1)
A key line under the anchor of `interfaces.ts` / `vrfs.ts` / `routing.ts` needs an `import … from './ext/neighbors-ra.js'`.
I added one import line after the last existing import of each file (Shared hunks). Other features will add theirs at
the same spot → trivial union at merge.

## Q5. `packages/schema/examples/neighbors-ra-*.json` is rejected by `examples.test.ts`
Its "every other file belongs to a sibling group" check only admits `nat|objects|acl|vpn|tunnels|services|ha` prefixes, and
the file is not mine. I added no example there; the feature document is `packages/proto/test/fixtures/neighbors-ra-full.json`
(directory-scanned by both proto corpus tests and the Go strict-decode test — all green), and the schema tests carry their
documents inline. Option for the manager: widen `SIBLING` to wave-A slugs.

## Q6. Contract side effect on a test I do not own
`packages/proto/test/desired-state.test.ts` asserts the exact `Vrf` object of `two-interfaces.json`; ts-proto fills an
absent repeated field with `[]`, so any new repeated `Vrf` field breaks it. One-line fix in the contract commit
(`proxyArpRanges: []` added to the expectation), listed under Shared hunks.

## Q7. VPP crash 18:41:08 (manager incident, D-126) — not this task
F-neighbors-ra had sent nothing to the host VPP before 18:41: until then only fake-backed unit tests (agent coretest
model, API fake agent) and the API e2e (host PostgreSQL + Valkey + in-process fake agent) ran. This feature has no
classify/policer code at all; its only binding-like objects (RA config, proxy-ARP interface, static neighbours) are
per-key and deleted only by key (never swept by index). Noted D-126 for every later host run.
