# Execution stages — the DAG the manager runs

Plan of record: `docs/11-compressed-plan-fa.md` (21 days). Board: `plan/tasks.yaml`. Prompts: `prompts/`.
Gates are checklists the manager ticks itself with evidence in `docs/status/`; humans review later (2× rule).

```
S0 P01 ──► S1 { P02 ∥ P03 ∥ P04 ∥ P09 } ──► S2 { P05 ∥ P06 ∥ P07 } ──► S3 P08 ──► S4 waves A ∥ B ∥ C ──► S5 system ──► S6 freeze
                                    │           └─ P05a ──► { DF-1..8 ∥ RF-1..4 } ─┘
                                    └─ contracts-v1 tag (manager freezes, human reviews async)
```

| Stage | Tasks (parallel within a row) | Depends on | Gate (evidence required) | Target day |
|---|---|---|---|---|
| **S0** | P01 monorepo | — | gen/typecheck/lint/test/build green — **done 2026-09-23** | 1 |
| **S1 contracts + rails** | P02 schema (3 workers by domain group), P03 proto (2 workers), P04 lab tooling, P09 CI-lite | P01 | schema+proto pass acceptance; `contracts-v1` tag; `tools/lab status` shows VPP on vrx-a; CI runs gen-dirty/lint/typecheck/test | 1–2 |
| **S2 core** | P05 agent core (P05a interface first), P06 api core, P07 ui shell; then DF-1…DF-8, RF-1…RF-4 | S1 (P05a for factories) | each layer's tests green against contracts; factories: descriptor tables committed, integration green on host VPP | 3–5 |
| **S3 slice** | P08 interfaces end-to-end | P05, P06, P07, DF-1, DF-2 | ping through VPP (af_packet path recorded), live counters in UI, commit/rollback on MTU, `kill -9 vpp` → reconcile < 30 s | 6 |
| **S4 waves** | A: F-vlan-qinq, F-bonding, F-bridge-l2, F-loopback-bvi-gso-lldp-span, F-vrf-static-ecmp, F-neighbors-ra, F-rpf-adl-pbr, F-object-model, F-acl, F-host-acl-nftables, F-nat44-ed-sessions, F-nat44-ei-64-66-nptv6 · B: F-det44-map-dslite-cnat, P11, F-ikev2-native, F-wireguard, F-tunnels, F-pki, P12, F-ospf, F-isis-rip, F-bfd-redistribution, F-kea-dhcp-relay, F-unbound-chrony-syslog · C: F-mpls-srmpls, F-igmp-mfib, F-srv6, F-lisp, F-host-stack, F-lb, F-qos-flat, F-snmp, F-ipfix-sflow, F-dashboard-prom-alarms, F-capture-trace, F-vrrp-config-sync | P08 + relevant DF/RF | per feature: FAST-MODE DoD (Retrieve==desired, `vppctl show`, rollback clean, restart-safe) | 7–15 |
| **S5 system** | P13 cli, F-restconf-yang, F-sdk-terraform-ansible, F-aaa, F-backup-restore, F-ra-vpn, P10 deb, P14 iso, F-ab-upgrade, F-images, F-hardening-lite, F-licensing | S4 (partial ok) | packages install on a clean 26.04 VM; ISO boots unattended; CLI commit/rollback works | 16–18 |
| **S6 freeze** | INTEGRATE-E2E (tri topology when VMs exist), SECURITY-REVIEW, DOCS-GEN, STATUS-FINAL | all | no red on main; security review findings fixed or logged; final have/have-not report | 19–21 |

## Parallelism rules
- Max workers: 6 → 12 by host load. Critical path (P05 → P08 → waves) always has a worker.
- File ownership per worker is declared in the TASK ENVELOPE; two workers never edit the same package simultaneously.
- Factories (DF/RF) are the parallelism engine of S2 — start them the moment P05a is merged, don't wait for P05 to finish.
- Wave tasks are independent by construction; run as many as capacity allows, T1 first.

## Known hard dependencies on the human (see decision-policy §always-PENDING #6/#7)
- Data-plane NICs on vrx-a (or the other lab VMs) — until then tests use af_packet/veth (D-010).
- `handover: done` in `docs/lab/host-vrx-a.md` — needed by P12 (linux_cp/linux_nl) and NPTv6 (npt66).
- vSphere VMs `vrx-b/c`, `host-lan/wan`, `peer-frr/sswan` for the `tri` topology (S6 E2E).
