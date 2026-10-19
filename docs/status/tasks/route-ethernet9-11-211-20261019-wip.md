# Appliance 211 — Layer 2 pairs active, initial traffic verification underway

Branch: `codex/route-ethernet9-11-211-20261019`; worktree `/root/.codex/worktrees/b538/NGFW`.
Owned files: this status and its adjacent task envelope; no product code changed in this worktree.
Base: `cb148aafb288301f9fe18eb064a4fc9610ad1698`.
Initial local/remote operational checkpoint: `cb83f65014dc9d1e7c544d34a6563d84f78abc6e`, successful GitHub push verified with `git ls-remote`.
Current source checkpoint: `git rev-parse HEAD`; publish every subsequent documentation checkpoint and verify the remote SHA.

## Current requested scope and result

The user first requested routing Ethernet9↔Ethernet11, then explicitly requested Layer 2 first for an initial test, later Layer 3, and added a separate Ethernet5↔Ethernet6 pair. The user supplied normal web administrator credentials; no credentials are recorded in Git. HTTPS authentication succeeded without auth changes or bypass.

Both reciprocal Layer 2 cross-connect pairs are now applied and persistently confirmed as running revision **4**, parent revision3, transaction `9bec043f-d1a9-4808-8b24-ac0bfd15e04f`. Four operations returned `ok`, no requested changes were unapplied, and sync is in-sync. Final candidate diff is empty and there is no pending confirmation.

| Display port | Actual interface | PCI | Peer |
|---|---|---|---|
| Ethernet5 | enp2s0f0np0 | 0000:02:00.0 | Ethernet6 |
| Ethernet6 | enp2s0f1np1 | 0000:02:00.1 | Ethernet5 |
| Ethernet9 | enp3s0f0np0 | 0000:03:00.0 | Ethernet11 |
| Ethernet11 | enp3s0f2np2 | 0000:03:00.2 | Ethernet9 |

Full authenticated physical inventory verified these PCI-based UI labels. All four ports are admin/link up at10Gbps, MTU9000. Independent reviewer verified exact reciprocal `show mode` entries and automatic hardware promiscuous/unicast acceptance on all four. There is no shared bridge-domain and no traffic configuration crossing the two pairs.

The only configuration change is `routing.l2.xconnects` with those four mappings (other L2 maps default empty). No IP, VRF, NAT, ACL, management, physical identity or unrelated interface values changed. enp4s0 remains172.30.110.211/24. VPP PID167929 and NRestarts0 were preserved throughout.

## Prerequisite repair and recovery

Before staging, revision3 had17 enabled physical dataplane ports in default VRF without IP addresses, empty static routes and ACL attachments, and an empty/unlocked candidate. Initial L2 validation failed with HTTP400 tier agent, `/interfaces`, `basepolicy.namespace`: malformed namespace binary readback despite `show lcp` explicitly reporting `<unset>`. No failed validation was committed or bypassed.

Reused the strict reviewed PR249 approach on the installed source base cb148aafb in a separate developer branch `codex/repair-211-lcp-20261019`. Product source SHA `98e5525c0b183586984557e673c95de2649f0f58`; documentation checkpoint `b02d077af414fe703cddbee41d3ee335f02980cb`. PR250: https://github.com/mcoder1001-cyber/NGFW/pull/250 (draft, not merged).

Independent source review APPROVE. Focused subsystem tests PASS (0.113s), subsystem vet PASS, all19 strict corroboration cases PASS with race (1.099s). Go1.26.0 static stripped build succeeded. Generated sources/modules checked unchanged. An initial race command overlapped generation and failed on a missing generated file; author is rerunning after generation, without changing tests or checks. The complete unchanged local and hosted quick gates are still running; no full-gate pass or merge is claimed.

Installed only the corrected agent binary, SHA256 `86a65b51dfbd5da268533ebbaae3c5b2b2143549547a7ce9f1176c083b499f5c`. Original SHA256 `4ca376e628686eb0b51ad480461f4fcd5150b00a62cf0ed270ac4f8384a5d6d1`. Root-private rollback material on211: `/root/ngfw-route9-11-repair-20261019/ngfw-agent.before` and `agent-state.before.tar.gz`; receipt hashes and before/after VPP process evidence are beside them. Staging is `/root/ngfw-route9-11-staging`.

Agent restart stopped dependent ngfw-api via its existing systemd Requires relationship; starting ngfw-api restored it. All three services are active. No service definition, environment, privilege, policy boundary, VPP source/startup or package was changed. Post-repair ordinary validate passed with exactly four xconnect creates; confirmed commit persisted revision4. Initial PPPoE inventory error was not a persistent permission/package defect; subsequent authenticated interface observations have no errors. No PPPoE repair was performed.

## Actual packet evidence and outstanding work

User tester is ready; root announced activation and requested traffic start. At11:24:17 UTC, deltas relative to precommit were:

- Ethernet5 RX0/TX1; Ethernet6 RX1/TX0.
- Ethernet9 RX1/TX1; Ethernet11 RX1/TX1.
- New drops, errors and RX misses: zero on all four ports.

These few frames show initial forwarding evidence, not a sustained tester run or complete endpoint reception proof. Observe actual tester traffic in both directions before claiming complete packet acceptance. Do not reset existing counters; historic drops predate activation. Root-private API snapshots, short-lived session, and command helpers are in `/root/.cache/ngfw-route9-11-211`; never publish the session or full protected snapshots.

Layer3 addressing remains unspecified. Proposed lab addresses were not accepted. Keep both L2 pairs active for the user test; remove the relevant reciprocal cross-connects before applying later requested L3 addresses/routes.

Exact next read-only command:

```sh
python3 /root/.cache/ngfw-route9-11-211/apply-l2.py status
```

For counters use authenticated `/api/v1/state/interfaces` or read-only VPP show commands and compare against `interfaces-precommit.json`. Renew the short-lived session only through normal authenticated login if expired.

Initial operational `tools/ci.sh check --base origin/main` PASS (14seconds). GitHub main CI was green at startup; unrelated PRs244–249 and board were inspected without adopting their work. This task's observed workers: root operational agent, lcp_repair developer and routing_review independent reviewer. Other workers' liveness is unverifiable.
