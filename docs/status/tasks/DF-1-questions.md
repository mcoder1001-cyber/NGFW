# DF-1 — questions for the manager (none blocking; work continued)

## Q1 — FIB table (VRF) dependency key for l3xc
`l3xc.l3xc` depends on the FIB table of every path with a non-zero table id. The DF task prompts name the
key `vrf/<id>`; P05 core (task/P05) has not committed its VRF descriptor yet, and the README's naming
examples suggest `ip.table/<id>`. DF-1 uses `l3xc.TableDescriptor = "vrf"` (one constant).
Options: (a) P05 names its descriptor `vrf` → nothing to do; (b) P05 uses `ip.table` → change the
constant in a follow-up (one line + one unit-test expectation). Recommendation: (b) is fine, just tell DF-1.

## Q2 — interface key scheme: full creator key vs generic `interface/<name>`
The P05a contract routes every key to a descriptor by its first segment, so an interface's key is the key
of the descriptor that created it: `interface.loopback/loop201`, `tapv2.tap/w2-tap10`,
`interface.subinterface/w2-tap11.100`, `bond.bond/…`, `memif.memif/…`, `af-packet.host-interface/…`.
DF-1 uses these full keys in every interface field and dependency (docs/agent/descriptors/interface.md).
The branches of DF-2, DF-3, DF-4, DF-5 and DF-6 instead build `interface/<name>` (a descriptor named
`interface` that does not exist), so their dependencies would never be satisfied by DF-1/P05 objects.
Options:
- (a) **Full creator keys everywhere** (DF-1 as built). Other DF-* store the full key in their model's
  interface field and pass it through (`scheduler.Key(o.GetInterface())`); DF-1 exports
  `iface.ParseRef` / `iface.Resolve` for resolution by owner tag. Cost: a small follow-up in each DF-*.
- (b) A generic alias object `interface/<name>` whose value names the creator key (desired-state builder
  emits one per interface; its Dependencies point at the creator; Create/Delete are no-ops, Retrieve
  derives it from the tag table). Cost: one extra descriptor + builder work in P08; keeps DF-2…6 as is.
- (c) Rename all creators to one descriptor `interface` with a type field. Breaks P05's `interface.loopback`
  and the per-plugin package split; not recommended.
Recommendation: (a); DF-1 can add the alias of (b) later if P08 prefers it. Decision needed before P08.

## Q3 — attributes VPP cannot report back (promisc, configured MAC)
`interface.promisc` and `interface.mac-address` are reported by Retrieve only for what the running agent
set; after an agent restart the scheduler plans one idempotent Create for each (host-verified in
`TestRestartSimulationOnHost`), and a MAC/promisc removed from the desired state while the agent was down
is not reverted. Acceptable for P05's "empty plan after restart" criterion, or should P05 persist an
owner table for these two (state dir) as the README allows for tag-less objects?
