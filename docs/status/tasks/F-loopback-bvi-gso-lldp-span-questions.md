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

## Q4 — gso.interface is readable (decision; envelope said write-only)
The envelope lists gso.interface as write-only. VPP has no GSO getter, but feature_is_enabled("ip4-output", "gso-ip4")
is real VPP state, so Retrieve reports GSO when that read-back AND the applied-once record of the running VPP boot
(index + logical name, D-080) agree — exactly F-bridge-l2's mactime.enable pattern. Options: (a) write-only (re-applied
every resync, never in Retrieve, drift note) (b) read-back + record (chosen): no echo of desired state (D-063 holds),
GSO shows in /state/interfaces `config` and a lost enable is detected and re-applied. The record also keeps a resync from
stacking the feature (VPP stacks it on every enable; one delete after three creates leaves it off on the host).

## Q5 — LLDP system name: no default from system.hostname in the agent
The schema help says "Defaults to system.hostname". The agent stores only its implemented domains, so after a restart a
resync would not know the hostname and would change lldp.global's value. The agent applies services.lldp.systemName as
given; unset = VPP keeps its current name (VPP's default is none). If the product wants the hostname default, the API
(or the LLDP screen) should fill systemName when saving. Please confirm.

## Q6 — TD-11b ownership declarations (manager message 03:40)
Every descriptor this feature registers declares its ownership: gso.interface, nsim.cross-connect, nsim.output,
lldp.interface, span.mirror → CheckPersistent (claims on untagged interfaces; gso/nsim also the D-076 BootStore);
nsim.config → CheckPersistent (BootStore); lldp.global → RecordsNoOwnership. My base has no TD-11b, so the checks
follow dfkit/persist's structural protocol locally (`<pkg>/ownership.go`: a store must have Persistent() == true); after
the rebase onto TD-11b they can call dfkit.CheckClaims / dfkit.CheckBoot (mechanical). On my branch alone the product
stores have no Persistent() yet, so the checks would fail if called — nothing calls them before TD-11b's guard.
`subsystems/loopback_bvi_gso_lldp_span_test.go` asserts exactly-one declaration for each of my descriptors.
F-bridge-l2's l2 / l3xc / mactime descriptors are not mine (they need their own declarations).

## Q7 — D-132 / WEB-1 applied
LLDP table and the mirroring page's /state/interfaces poll at most every 30 s and have a Refresh button; the agent's
LldpNeighbors walk is serialised (one walk at a time). My calls to P08's dropPhantomOptionals are removed (WEB-1: a
no-op); on my branch alone (no WEB-1 presence toggle) SchemaForm may still materialise nsim's optional crossConnect —
fixed by WEB-1 in the merged tree.

## Q8 — coretest hook line and the TD-23 extension registry (D-134)
`coretest/fakevpp.go` gets one hook line (`v.installLoopbackBviGsoLldpSpan()`, after sanitizetest.Clean) as F-bridge-l2
did (its Q9). My handler also answers `feature_is_enabled` for ip4-output/gso-ip4 and hands every other arc to
F-bridge-l2's mactime answer (replicated, 3 lines) — one handler per message in the fake. At the rebase onto TD-23 this
becomes one registration line in its extension registry.
