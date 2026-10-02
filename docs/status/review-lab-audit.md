# Review backlog audit — 2026-10-02

Base: `origin/main` / `471c61fa381caca6cbd96b1b6f74ac9b6da35334`. Independent audit of the fourteen stale `review` board rows. This is source/ancestry inspection, not a fresh test run. Every implementation commit below is already an ancestor of main (`git merge-base --is-ancestor` returned zero for all fourteen). No fourteen new product merges are required or claimed.

The product owner explicitly authorized merging code while lab access is unavailable and collecting deferred acceptance in one campaign. This waives the lab evidence blocker only: it does not turn NOT RUN into PASS, erase missing functionality, or authorize pending privileged/security decisions.

| Board row | Already integrated commit(s) | Safe board transition | Missing code or scope boundary | Deferred acceptance |
|---|---|---|---|---|
| F-system-identity | 8e66159e | review → running | SystemState RPC/API and public login banner absent; resolved restart remains privilege handover dependent | Temporary root goldens, real API pointers, host invariance, restart/no-rewrite, DNS state, en/fa browser |
| F-dataplane-ui | 41879710, ed79efa1 | review → running | live VPP thread readback absent; preview only; apply intentionally gated by handover | Real startup preview/diff, installed-vs-live facts, hugepages/plugin/NIC failure, en/fa browser; no unauthorized restart |
| F-management-ui | f58b9072, cf44b45a | review → running | HTTPS WebSocket upgrades absent; certificate removal retains running cert until restart; transport decision unresolved | Real DB-backed commit validation, TLS rotation/min-version/rollback, stream transport, no secret exposure, browser |
| F-tunnels | 23d64f45, 327d7c1c | review → running | VXLAN-GPE, GTP-U, L2TPv3, PPPoE, IPIP 6RD and TunnelState absent | GRE/IPIP/VXLAN forwarding, aliases/MTU/VRF/bridge membership, delete/restart; advanced types after implementation, opt-ins preserved |
| F-ospf | d75ca74e | review → running | OSPFv3, interface auth, Event20, API state, live neighbours UI and user docs absent | FRR v2/v3 neighbours/LSDB and linux-nl FIB, auth failures, events, withdrawal, browser |
| F-isis-rip | cf75a151 | review → running | RIPng, version/family/auth additions, Event21, OSI punt, RIP reader, API state/live UI absent | IS-IS/RIP/RIPng convergence, OSI punt and IPv4/v6 FIB, auth, withdrawal/events, browser |
| F-capture-trace | 04802324, 58de175c | review → merged | Capture scope integrated; Trace/PG unsupported by documented host/API capabilities; shared-host trace restriction remains | Capture filters/drop/error flags, 0600 artifact lifecycle, stream download/RBAC/path safety, retention/restart, browser; no shared trace |
| F-vrrp-config-sync | c20240c4, 497b5d6b | review → running | Config sync, pinned peer TLS, cluster endpoint, state/events not implemented; secret channel boundary pending | Both VRRP engines/failover, nondefault VRF skip, later peer sync/unsynced edits/secret boundaries and cluster browser |
| WEB-4a | 33400072 | review → merged | D123 prebuilt-screen scope complete; live protocol state belongs to feature rows | Real SchemaForm save/remove and problem mapping, en/fa RTL light/dark browser; protocol state when feature rows implemented |
| WEB-4b | 7a29d185 | review → merged | D123 prebuilt-screen scope complete; backend VRRP/cluster state belongs to feature row | HA/cluster forms, delete merge patch, pointers, secret reference handling, en/fa RTL light/dark browser |
| TD-17 | 40b0fa38 | review → merged | Installed approval-gated script scope complete; package wiring belongs P10 and UI approval flow F-dataplane-ui | Installed appliance paths, exact SHA approval, mandatory dead-man/rollback/locks/health; host restart only after handover |
| TD-21 | bfc9aa60 | review → merged | Pure scheduler scope complete; no lab dependency | No separate lab blocker; full unit/race/scale gate must remain green |
| TD-26 | af90b3ca | review → merged | Tolerant VRF delete scope complete; upstream NAT64 unlock/durable tombstones are follow-ups | Real NAT64 locked IPv6 table, delete/recreate/resync, agent restart and VPP boot identity change |
| TD-27 | b56cb8ad | review → merged | Source SPAN/LLDP sanitation scope complete; destination rows belong their live source owner | Actual reused sw_if_index, source dump readback, quarantine on uncleared SPAN, LLDP optional plugin, bound probe retention |

## One deferred campaign

Manager records each case in `docs/status/DEFERRED-ACCEPTANCE.md`: exact main SHA, host/VPP/FRR version, slot, command, expected/actual outcome, artifacts, fix SHA and rerun. Until run, outcomes are NOT RUN. Once access returns: preflight/handover and locks → full CI on slot12 → management anti-lockout/auth/rollback/TTL → grouped daemon/dataplane/restart tests → en/fa light/dark screenshots → cross-feature regression. Preserve failures and fix code before rerun. No VPP restart or expanded privilege is implied by lab waiver.

## Evidence boundaries

Audit read the feature status reports, board, contributing/manager rules and current implementation contracts. Historical per-feature checks are recorded in their reports; this audit did not rerun them. TD17 reports two old sandbox process/lock failures and their baseline reproduction; they remain honest historical evidence rather than a fresh all-green claim. All listed code is already on main, whose current hosted quick gate should be checked by the manager before board reconciliation.
