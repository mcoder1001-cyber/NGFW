# Multi-WAN live failover acceptance

2026-10-04, source fixture `0b1343a4a1664f0c83e2dba6e763443997f900ef`, product `d44d44eb6efd88754c1964996f7973b16febc57a`. Branch `codex/closeout-multiwan-live`, isolated worktree `/root/ngfw-wt/codex-closeout-multiwan-live`. Owned fixture/docs only; no product changes.

`TestMultiWANRealAPI` PASS32.73s with no skips: real API, real source-current agent, real device-bound ICMP probes, disposable VPP. Raw evidence: [final2.txt](closeout-multiwan-evidence/final2.txt). Agent SHA256 `8658f0111afb301935815d462200debb359e75f5ffe7a6c71920c6758f5fce3a` matches the parent's d44 host build manifest; ctl/preflight hashes printed in raw evidence.

| Actual gate | Result |
| --- | --- |
| Commit | API status applied; initial monitor states observed from actual ICMP |
| Preferred WAN1 | Installed default selected WAN1, 3/3 replies TTL63 |
| Physical WAN1 peer down | Actual monitor loss100%; installed default WAN2 within2.530s, 3/3 replies TTL64 |
| Physical peer restored | Real bound probe works; full failback2.652s including diagnostics, installed default WAN1, 3/3 TTL63 |
| Actual agent stop/restart | Durable configuration replay, actual installed WAN1, 3/3 TTL63 |
| Rollback | API applied; monitor snapshot empty/no agentError within3s; no forwarding default (drop DPO); 0/3 packets |
| Cleanup | API removes own objects, no owned VPP interfaces; agent/API stopped, PostgreSQL role/database dropped; disposable VPP stopped; four owned namespaces absent; shared VPP MainPID1014/NRestarts0 unchanged |

The agent and VPP run inside owned `ns-w7-mw-router`; the real API stays outside and uses the agent Unix socket. LAN and both WAN peer namespaces contain only test veths. Linux-CP and Linux-NL stay enabled. Slot-owner agent drives no FRR; fixture assigns LCP addresses and monitor endpoint routes only inside the private router namespace. The data endpoint198.18.7.2 has no specific VPP /32 route, so its packet gates exercise the dynamically installed default. Different peer default TTLs64/65 identify the selected packet path at the LAN (63/64).

All sysctl changes are confined to owned namespaces: router `ping_group_range=0 0` permits the real agent's restricted datagram ICMP socket; peer `arp_announce=2` selects an on-link ARP sender for replies sourced from loopback endpoints; peer TTLs mark packet path. Peer default route is reinstalled after link UP because the kernel removes it during interface DOWN. No root routes, physical NICs, shared VPP, FRR, or system daemon units are modified. Slot7 is released.

## Preserved failures and fixture corrections

Attempts1/10 failed to compile because of fixture local variable mistakes. Attempt2 real API applied but the new namespace's datagram ICMP policy was unavailable. Attempts3/4 used the same monitor/data address: Linux-NL imported its /32 viaWAN1 and shadowed the correct dynamically switched default, so failover data failed. Attempt5 disabled Linux-NL, which broke the LCP monitor path; final fixture retains it and separates monitor/data endpoints. Attempts6–9 failed restoration. Attempt11 capture proved echo requests reached WAN1 but peer ARPed with off-link sender198.18.7.1 after neighbour cache flush; VPP/AF_PACKET/LCP remained UP with correct routes/addresses. Namespace-local arp_announce2 corrected endpoint ARP and attempt12 passed the complete flow. The first strengthened final run proved all packet/timing gates but asserted the asynchronous monitor snapshot immediately after rollback; the product watcher refreshes at1Hz. Final source now requires snapshot removal within3s and preserves that failed run as final.txt.

These are fixture failures, not new product fixes. Passing partial evidence was never promoted to full acceptance while restoration failed.

## Remaining host acceptance

This closes only static-gateway IPv4/default-VRF failover/restoration/restart/rollback packet cases. Weighted1000-flow split, sticky affinity and per-member NAT/session cleanup, ABF/PBR binding, IPv6, browser presentation, scale/campaign repetition are not executed here. Unsupported non-default VRF/netns probe ownership and DHCP/PPPoE gateway handoff remain declared source limitations. The whole F-multiwan-host row must not be called DONE from this bounded proof. Physical195/250 integration and TEST-A/B/C orchestration remain separate acceptance.

Run: `NGFW_MULTIWAN_PRODUCT_ROOT=/root/.codex/worktrees/6189/NGFW NGFW_MULTIWAN_BIN_DIR=<source-current agent/ctl/preflight directory> python3 test/topology/multiwan-host-acceptance/run.py`. Slot7 must be reserved first. The runner uses the heavy semaphore and finite4m Go timeout, owns its namespaces/processes, and removes them on exit.

Final source-only guard `tools/ci.sh check --base d44d44eb6` PASS11s. The complete quick/API/product integration gates are manager-owned; this fixture run does not substitute for them.

Independent reviewer: `/root/management_acceptance`, read-only review of frozen source and raw evidence; final approval recorded by manager. Publication is manager-owned; local source commits alone are not remote checkpoints.
