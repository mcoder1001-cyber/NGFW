# F-loopback-bvi-gso-lldp-span — questions and decisions for the manager

Slot 7. Each item: what I decided (so work continues) and what the manager should confirm.

## Q1 — contract commits are on the task branch (FYI)
`contract(schema): …` and `contract(proto): …` are the first two commits after the salvage commit (P08 pattern),
described in `F-loopback-bvi-gso-lldp-span-contract.md`: `Interface.gso` 20, `Interface.mirror` 21,
`ServicesConfig.nsim` 9, `rpc LldpNeighbors` (+ `MirrorSession`, `NsimService`, `LldpNeighbor*`), the D-105 loopback
reservation rule. Numbers exactly as in the envelope.

## Q2 — schema examples live in the proto fixture corpus
`packages/schema/examples/loopback-bvi-gso-lldp-span-*.json` (owned) would fail `examples.test.ts` ("every other file
belongs to a sibling group": only the P02 prefixes are admitted; the test is not mine). Same finding as F-bridge-l2 Q4.
The valid example is `packages/proto/test/fixtures/loopback-bvi-gso-lldp-span-full.json` (round-trip + drift corpus);
invalid cases are in `semantic/loopback-bvi-gso-lldp-span.test.ts`. If the manager adds my prefix to `SIBLING`, I can
move/copy it.

## Q3 — `services` domain: built on F-rpf-adl-pbr's seam (merge F-rpf-adl-pbr FIRST)
Manager message 23:30: reuse F-rpf-adl-pbr's `Services` constant / map entry and `desired.ServicesMembers`, no second
`Services` constant. My base (task/F-bridge-l2) does not contain F-rpf-adl-pbr, so my branch carries a minimal seam that
the merger drops when F-rpf-adl-pbr is already on main (details and the exact union in the status file, "Expected union
at merge"). Summary: (a) my descriptor names are appended to `Domains["services"]` from an `init()` in my own
`subsystems/loopback_bvi_gso_lldp_span.go` (composes with F-rpf-adl-pbr's `Services: {…}` literal entry — no duplicate
key, no second constant; a private `servicesDomain = "services"` string names the key); (b) `lldp` and `nsim` are
registered with `ServicesMembers[...] = true` from an `init()` in my `desired/lldp.go`; (c) `desired/lldp_services_seam.go`
holds a byte-identical copy of F-rpf-adl-pbr's `ServicesMembers` declaration and its "every other non-empty member is
`agent.unsupported-field`" loop — **delete that one file at the merge**; (d) the P08 test hunks (`"services": {}` in the
canonical documents, `implementedDomains()` in the subsystem assertions) are byte-identical to F-rpf-adl-pbr's, so they
merge cleanly; (e) my write-only notes use F-rpf-adl-pbr's `agent.write-only` rule; until its `COVERAGE_RULES` line
(`apps/api/src/state/state.controller.ts`, not mine) is on main, the drift view on my branch alone shows
`services.lldp` / `services.nsim` as drift.
