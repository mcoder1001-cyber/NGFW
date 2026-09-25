# F-loopback-bvi-gso-lldp-span — WIP log (slot 7)

| time | state |
|---|---|
| 19:51 | envelope filled by the manager; session-limit stop at 20:57 while reading (salvage e6883c2 = envelope only) |
| 23:15 | resumed: read 00-CONTEXT, shared-host rules, template, prompt, hotspots, envelope + addenda, DF-7 lldp/span, F-bridge-l2 patterns, F-rpf-adl-pbr's services seam (read-only via git show) |
| 23:30 | contract (schema) in progress: ext/loopback-bvi-gso-lldp-span.ts, semantic rules, examples |
| 23:30 | contract commits cb1c84f (schema) + fb6418d (proto) |
| 23:45 | b32e0d5 agent: gso/nsim descriptors, DF-7 wiring, builders, LldpNeighbors RPC, coretest model, unit tests |
| 23:58 | 62e8be1 API controller + e2e (5/5 on the host PostgreSQL), b2041a4 regenerated client |
| 00:10 | 4ce9b9d web: LLDP / mirroring / nsim pages, en+fa |
| 00:15 | 2e71ecf gso host checks green (NRestarts 1 → 1), nsim opt-in, span stale-destination cleanup |
| 00:20 | usage-limit stop |
| 03:45 | resumed: eaa556b TD-11b declarations, D-132 polls/Refresh/serialised walk, WEB-1 dropPhantomOptionals removed |
| next | ERSPAN host test (DF-6 gre descriptor), topology test (API + agent + VPP), screenshots, docs, CI |
