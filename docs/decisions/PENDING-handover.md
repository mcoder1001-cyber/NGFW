# PENDING: handover

- raised: 2026-09-23 by manager (cycle 1)
- decision: **<empty — product owner fills in>**
- parked tasks: P12 (FRR/linux-cp), the NPTv6 part of F-nat44-ei-64-66-nptv6; and (since 13:17) binding the six new data NICs to DPDK

## Context
VPP 26.06 on the host was brought up by a separate agent. `docs/lab/host-vrx-a.md` says `handover: pending`, so nobody
edits `/etc/vpp/startup.conf`, packages or `vpp.service`, and nobody restarts VPP (D-012). Two planned features need plugins
that are on disk but not loaded: `linux_cp_plugin.so` + `linux_nl_plugin.so` (P12) and `npt66_plugin.so` (NPTv6).

## Options
| # | Option | Cost now | Reversal cost | Risk |
|---|---|---|---|---|
| 1 | Product owner flips `handover: done`; the manager owns VPP config from then on (changes via the startup.conf generator, logged) | 0 | low | manager restarts VPP only under lock, between integration phases |
| 2 | The bring-up agent adds `plugins { plugin linux_cp_plugin.so { enable } plugin linux_nl_plugin.so { enable } plugin npt66_plugin.so { enable } }` to startup.conf and restarts VPP once; handover stays pending | ~10 min | trivial | none |
| 3 | Keep everything parked until the 21-day freeze | 0 | — | P12 and NPTv6 slip out of the plan |

## Also needed from the product owner (no decision, just information)
The six new vmxnet3 NICs (`ens161/193/224/225/256/257`) — which vSphere port group is each on (lan / wan / dmz / p2p-ab / p2p-bc / spare)? With that mapping the generator can emit `dpdk { dev 0000:04:00.0 name lan … }` and tests get the DPDK path.

## Recommendation
Option 2 now (one restart in a quiet window: enable `linux_cp`, `linux_nl`, `npt66` **and** add the six `dpdk { dev }` lines once the mapping is known), option 1 when the bring-up agent is finished.

## What continues meanwhile
Everything else (73 tasks). P12's schema/API/UI parts can still be built with `t.Skip` on the plugin-dependent tests.
