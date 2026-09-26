# F-vlan-qinq — WIP log

Branch `task/F-vlan-qinq` (speculative on `task/W-seed`, D-114/D-120; `task/W-seed`@df67a8e merged at 18:12 on the
manager's A1 instruction), slot 5 (`w5`, API 3500, web 5500, metrics 9151, DB `vrx_w5`, rig 10.5.{1,2}.0/24). Started 17:27.

| time | step | state |
|---|---|---|
| 17:35 | read 00-CONTEXT, shared-host rules, template, prompt, hotspots, envelope, P08 status, DF-1 docs | done |
| 17:50 | schema tests (9 cases, no rule missing) · agent QinQ tests → **defect found and fixed** (Q1: removing a sub-interface rolled back; e771ecb) · API e2e (4) | committed |
| 18:10 | UI: SubinterfaceTable + tag-stack formatter, en/fa `vlan-qinq`, drawer swap, i18n anchors; Vitest | committed 41ce6c1 |
| 18:12 | manager A1: merged `task/W-seed` (TD-5 rings/quiesce, D-113 rig, P08 fix round 2) before any host run | fb29ebd |
| 18:20 | topology module `test/topology/vlan-qinq` (commit/duplicate 400, packets, restart-safety, rollback, cleanup) | b8fa14e |
| 18:21 | host run 1: all PASS, NRestarts 0 → 0; V-new (af_packet loses the 802.1ad TPID) recorded | 0159129 |
| 18:25–18:37 | screenshot runs (NRestarts 0 → 0); table fitted to the drawer, RTL label fixed | f764283 |
| 18:41 | host run 2: **VPP SIGSEGV PC 0x0 during the packet phase (NRestarts 0 → 1)** → host runs stopped, Q0 written; packet phase opt-in, no 802.1ad frames (root cause later found by the review: the test's own `show trace`, D-128) | 60d1abf, f764283 |
| 18:46 | user docs page + basics.md line | 42aaa03 |
| 18:47–19:12 | CI (passed on run 3 at c06785c), status file | 01b80ff |
| 19:37 | review 674b03e: APPROVE WITH CHANGES (H1 `show trace`, M1 guard, M2 host run) | — |
| 19:42 | fix round 1 (slot 12): no packet trace in the test, counter-delta proof, dead dot1ad branch removed (H1, L1) | abe6d50 |
| 19:43 | host run of the committed test, packet-free: PASS, NRestarts 1 → 1 (M2) | status |
| 19:50 | tag-stack view types as `Pick<>` of the schema/client types (L2) | 5cd5010 |
| 20:57 | usage-limit stop while correcting Q0; manager salvaged the docs | d4801d8 |
| 22:41 | continued: Q0 / status / D-VQ-4 checked, M1 re-checked against main 7e2c272 (D-128b), CI at the final HEAD | fix-round-1 commits |

## Left
Nothing beyond the final report (fix round 1).
