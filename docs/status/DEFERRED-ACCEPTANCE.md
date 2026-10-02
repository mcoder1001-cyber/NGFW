# Central deferred acceptance campaign

Owner authorization, 2026-10-02: remove Telegram; merge reviewed code without waiting for laboratory access; collect deferred tests here and continue development. This supersedes historical lab-only BLOCK verdicts. Real code defects and the complete hosted quick CI remain merge blockers. NOT RUN never means PASS. Existing security, secret-channel, shared-host and privilege handover decisions remain applicable.

## Campaign order when access returns

1. Record exact main SHA, appliance/VPP/FRR versions, host reachability, slot ownership and handover. Use existing locks and CI slot12. No unauthorized VPP restart.
2. Run full CI, then management anti-lockout, auth/MFA, commit/confirmed rollback and expiry cases.
3. Run grouped forwarding, daemon state, withdrawal and permitted restart cases; then real API/browser en/fa RTL light/dark acceptance.
4. Preserve every failure, fix code, record fix SHA and rerun affected and cross-feature cases.

Each case must record SHA, slot, command, expected/actual outcome, artifacts, failure/fix and rerun. Initial outcome for all cases below: **NOT RUN**.

## Integrated review backlog

The exact fourteen-row matrix, implementation SHAs, missing functionality and acceptance cases are in [review-lab-audit.md](review-lab-audit.md), incorporated into this campaign. Seven complete scoped implementations were already on main; seven partial features require further code. Board reconciliation is not fourteen new product merges.

## Active feature acceptance

| Feature | Deferred acceptance | Functional boundary |
|---|---|---|
| Dashboard/Prometheus, PR58 | Real VPP stats traffic, bind/close/restart, metrics growth, alarm transitions and delivery, API browser | Aggregate counters only; no fabricated directional/link stats |
| Notifications | SMTP TLS/auth, HMAC webhook DNS/IP filtering, rules/dedup/retry/reload/shutdown, real alarm/commit/link/VPN events, en/fa browser | SMTP/webhook only; Telegram removed by owner instruction |
| Setup wizard | Real first-boot LAN access/DHCP/NAT, password, one confirmed commit and rollback/session loss, stale candidate/cancel/reopen, browser | No relaxation of management anti-lockout |
| System identity | Installed readback, DNS facts, bounded public login banner, failure pointers, restart/no rewrite, browser | Restart privilege boundary retained; daemon status unknown unless observed |
| Multi-WAN | Two WANs traffic failover/restore, weighted flows, NAT session cleanup, queue retry/restart, browser | Static IPv4/default VRF first; dynamic gateway, VRF and ABF remain explicit code follow-ups |

## Remaining host acceptance rows

TEST-trafficA, F-lb-host, F-srv6-host, F-mpls-srmpls-host, F-rule-expiry-host, F-global-blocking-host, F-pppoe-client-host: execute their existing task acceptance against real slots. AutoBlock, multicast and LDP require code recovery and fresh reviews before their forwarding/expiry/detector, PIM/IGMP and label/neighbour/restart acceptance. No unavailable test is marked passed.

## Recovery checkpoint

Workspace maintenance removed unpublished local work. GitHub main and remote dashboard checkpoint were recovered. Rebuilt features must pass fresh tests and review; prior chat claims alone do not certify recovered code. Publish incremental branch checkpoints so subsequent interruptions do not discard work.
