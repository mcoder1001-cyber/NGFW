# Risks — read this before committing budget

| # | Risk | Impact | Mitigation |
|---|---|---|---|
| 1 | **VPP expertise is rare.** Debugging a vector-graph data plane is nothing like Linux networking. | Schedule slips 2–3×. | Hire/contract one senior VPP engineer before week 1. Budget for FD.io community time and for reading CSIT. |
| 2 | **linux-cp ↔ FRR integration is the classic swamp**: MTU/state sync, IPv6 ND, VRF mapping, route churn. | P3 doubles in length. | Prototype it in week 2, not month 6. Test with a full BGP table early. Follow `lcpng` and Netgate's patches. |
| 3 | **NIC/driver reality.** DPDK PMD bugs, firmware mismatches, SR-IOV and hotplug edge cases. | Field escalations. | Fix a small supported-hardware matrix. Refuse to support "any NIC". Keep a lab unit of every SKU. |
| 4 | **Restart-safety.** VPP crashes lose all runtime state. | Customer outage. | The reconciler + "datastore is truth" rule is the whole answer. Test by `kill -9 vpp` in CI, every night. |
| 5 | **Transactional commit across 6 daemons** has no clean primitive. | Half-applied configs. | Validate-then-apply, ordered backends, file snapshots, confirmed-commit timer, DEGRADED state. |
| 6 | **Feature parity with TNSR is ~15 years of accumulated work.** | Never "done". | Don't chase parity. Pick a wedge (high-throughput IPsec concentrator, CGNAT box, BGP edge) and win it first. |
| 7 | **Performance claims without a lab are marketing fiction.** | Lost deals, lost trust. | Buy TRex hardware in month 1. Publish repeatable numbers with the test profile. |
| 8 | **GPL contamination** if someone links FRR/strongSwan code. | Legal exposure to your source. | Process boundary rule in CONTRIBUTING.md + an automated licence scan in CI. |
| 9 | **MUI X Pro licensing** if you use the Pro DataGrid features. | Surprise cost / compliance issue. | Decide MIT DataGrid vs Pro in week 1 and record it. |
| 10 | **Node.js as a bottleneck** if someone pipes per-packet or per-flow data through it. | API meltdown at scale. | Node handles config + aggregated telemetry only. Session/route browsing is paged at the agent. |
| 11 | **Security of the agent socket** — it is root-equivalent. | Full compromise from a web bug. | Minimal typed gRPC surface, no passthrough shell, per-method authz, fuzz the agent inputs. |
| 12 | **Certification/compliance** (if selling to government) can invalidate architecture choices late. | Rework. | Get the requirement list before P0 ends. |

## The honest summary

The web UI and the Node.js API are maybe 25% of this product's effort. The other 75% is
the data plane, the reconciler, the FRR bridge, and the test lab. Plan, staff and budget
accordingly — most teams that attempt this underestimate exactly that ratio.
