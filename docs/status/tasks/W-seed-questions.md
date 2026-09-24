# W-seed — questions and conflicts for the manager

Rule conflicts are resolved with the higher-precedence rule and listed here (00-CONTEXT: never stop on a conflict).

## Q1. The A5 hooks are declared but not wired in `agent.go` (the manager decides)
`subsystems.Env` now has `Publish func(*vrxv1.Event)` and `Resync func()`. Features call them through `Wiring.Publish(ev)` and
`Wiring.RequestResync()` (`apps/agent/internal/subsystems/seams.go`). Both are no-ops until `agent.go` sets them. The envelope does
not give me `agent.go` (A5 is read-only, and "files you must not touch: everything else"), so the live wiring is not in this branch.
Features can build and unit-test against the hooks now. The wiring is a small change in `agent.Start`. `Env` is built before
`svc`, so the hook reads `svc` late:
```go
var svc *Service
wiring, err := subsystems.Register(reg, subsystems.Env{ /* as today */,
	Publish: func(ev *vrxv1.Event) { if svc != nil { svc.events().publish(ev) } },
	Resync:  func() { if svc != nil { go svc.Resync(context.Background()) } },
})
...
svc, err = NewService(...)   // `=` instead of `:=`
```
Options: (a) the manager adds these lines at merge, or in the first feature that needs them (F-neighbors-ra, EventKind 10);
(b) W-seed round 2 gets `agent.go`. Recommendation: (a), because it changes nothing until something publishes.

## Q2. "never: merge" (envelope) vs "git merge task/P08" (prompt)
I read the envelope's "never: merge" as "never merge into main or another task's branch". The prompt names `git merge task/P08`
into this branch before the final CI, and that is what I did (see W-seed.md, "P08 merge").

## Q3. "one commit" (prompt) vs two commits (hotspots §4.2)
tools/ci.sh's contract guard needs a `contract(…)` commit when `packages/schema` or `packages/proto` change. I made two commits,
as §4.2 says: `contract(wave-A): anchors` (schema + proto) and `chore(wave-A): hotspot anchors, seams and shells` (the rest). D-112
squashes them at merge anyway.

## Q4. Anchors beyond the prompt's list (additions, all comments)
- **C5 message anchors** where two or more features append fields or values at the same spot (§2 numbers): `Interface`,
  `Subinterface`, `Vrf`, `StaticRoute`, `RoutingConfig`, `ServicesConfig`, `AclConfig`, `ActionRequest.action`, and `EventKind`.
  In a proto message the anchors follow the allocated numbers, not board order.
- **C1 `SubinterfaceSchema`, `NextHopSchema`, `StaticRouteSchema`**: these match the Subinterface, NextHop and StaticRoute numbers in §2.
- **A1 const block**: one block of anchors for new domain constants, so the `const` lines are not all appended at one spot.
- **A4 F-unbound-chrony-syslog case**: its envelope names "one Action case for the DNS lookup". That makes four case anchors, not three.
- **P6 `relay.service.ts` `eventTopic()`**: §1 P6 says "one topic line + one EventKind case each".
- **P4 import list**: every new RPC method also imports its request and response types.
- **W2 `nav.test.ts`**: the anchors follow navigation order (interfaces, routing, firewall, vpn, services), not board order,
  so the `available` list keeps nav order when the features merge. Features and their groups: bonding, bridge-l2 and loopback
  under interfaces; vrf, neighbors-ra, rpf and P12 under routing; nat44-ed, object-model, acl and host-acl under firewall;
  P11 and wireguard under vpn; kea and unbound under services. A feature that places its item elsewhere moves its line at merge.
Every other hotspot uses board order (`plan/tasks.yaml`).

## Q5. Shared keys: features must still union them by hand
`Domains["services"]` (rpf, loopback?, kea, unbound), `Domains["vpn"]` (P11, wireguard), `BUILT_DOMAINS` `'vpn'` and `'services'`,
and the `available` test list: each of these features has its own anchor. Two features that add the same key still need a semantic union at merge
(launch plan §4). I did not seed the keys themselves: an empty `Domains` entry changes `Health.subsystems`, and a `BUILT_DOMAINS`
entry makes a placeholder screen reachable in the nav. Both would be behaviour changes.
