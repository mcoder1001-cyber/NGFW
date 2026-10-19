# Appliance 211 — Layer 2 pairs active; tester interval has no ingress and links 9/11 down

Branch: `codex/route-ethernet9-11-211-20261019`; worktree `/root/.codex/worktrees/b538/NGFW`.
Owned files: this status and its adjacent task envelope; no product code changed in this worktree.
Base: `cb148aafb288301f9fe18eb064a4fc9610ad1698`.
Initial local/remote operational checkpoint: `cb83f65014dc9d1e7c544d34a6563d84f78abc6e`. Confirmed-configuration local/remote checkpoint: `f3a1c5956ee6c09758a28c4c4f0e45e415bdd3af`. Both successful GitHub pushes verified with `git ls-remote`.
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

Independent source review APPROVE. Focused subsystem tests PASS (0.113s), subsystem vet PASS, all19 strict corroboration cases PASS with race (1.099s). Go1.26.0 static stripped build succeeded. Generated sources/modules checked unchanged. An initial race command overlapped generation and failed on a missing generated file. After generation completed, the full focused subsystem package race command passed (1.311s), without changing tests or checks. The complete unchanged local and hosted quick gates are still running; no full-gate pass or merge is claimed.

Installed only the corrected agent binary, SHA256 `86a65b51dfbd5da268533ebbaae3c5b2b2143549547a7ce9f1176c083b499f5c`. Original SHA256 `4ca376e628686eb0b51ad480461f4fcd5150b00a62cf0ed270ac4f8384a5d6d1`. Root-private rollback material on211: `/root/ngfw-route9-11-repair-20261019/ngfw-agent.before` and `agent-state.before.tar.gz`; receipt hashes and before/after VPP process evidence are beside them. Staging is `/root/ngfw-route9-11-staging`.

Agent restart stopped dependent ngfw-api via its existing systemd Requires relationship; starting ngfw-api restored it. All three services are active. No service definition, environment, privilege, policy boundary, VPP source/startup or package was changed. Post-repair ordinary validate passed with exactly four xconnect creates; confirmed commit persisted revision4. Initial PPPoE inventory error was not a persistent permission/package defect; subsequent authenticated interface observations have no errors. No PPPoE repair was performed.

## Tester follow-up on 2026-10-19

User said they would now start tester transmission. Root and independent reviewer took fresh read-only snapshots. At11:45:25→11:46:04 UTC (39seconds), no RX packets, TX packets, drops, transmit errors or bytes increased on any of the four ports. Independent44-second observation agrees. VPP `show interface` state is administrative state; use `show hardware-interfaces` to determine actual carrier.

At11:45:26 UTC, ports5/6/9 had10Gbps carrier; port11 carrier was down. At11:46:10 UTC, both9 and11 were physically down;5/6 remained up. All four administrative states remained up and all four reciprocal xconnect mappings remained installed. Port9 hardware local/remote fault counts rose by4 during the link transition. Exact cause (tester settings, cable or optics) is unknown; do not infer a specific physical fault from counters alone.

Accumulated errors since the earlier initial-frame test are no longer zero: VPP reports port5 output `interface is down`4 times and port11 output `interface is down`3 times. Total drop counters at the new baseline are5=11,6=14,9=28,11=14. These totals did not change in the measured interval. They indicate intervening link outages, not a new packet-loss measurement during the idle interval.

The user was asked whether tester cables/link settings are being changed and to check tester TX/link. Keep configured L2 pairs unchanged while awaiting external tester/link recovery. No counters were cleared and no capture/trace/host configuration changes were made. Counter snapshots and parser are private at `/root/.cache/ngfw-route9-11-211/{tester-start,tester-interval1}.json` and `sample-counters.py`.

PR250 remains unmerged. Current observed published repair checkpoint is `cc1656787d3c9a8b6d851ced2dca8501209e1aff`; local full Turbo35tasks passed including API823 and web694tests. Full agent vet, lint and race suites passed; the developer continues the remaining complete quick gate and hosted run39246565687. Do not claim the final complete gate until it actually passes. GitHub main remainscb148aafb with green last mainCI.

## Second tester follow-up

User confirmed cable/tester settings were being changed, then said traffic was now being sent. Fresh configured-port sampling11:51:39→11:52:33 UTC (54seconds) again showed zero RX/TX/drop/error deltas on5/6/9/11. Independent all-port hardware and VPP sampling11:52:04→11:52:32 UTC (28seconds) found no increasing packet counters on any of17 physical data ports. Hardware and software counters agree; no alternate high-volume ingress was discovered.

At the first all-port sample, only5/6/7/8 had10Gbps carrier. During the repeat,7/8 lost link; final carrier was up only on5/6. Ports9/11 remained down. Ports7/8/10 have small historic RX totals17/15/18, not evidence of a current tester stream. The user was asked which web-displayed pair is under test. Do not equate chassis printed numbers to PCI-derived web labels without verification, and do not reconfigure other pairs on that assumption. L2 mappings remain5↔6 and9↔11. Root/independent reviewer performed only show/sampling commands, with no counter reset, packet trace or mutation.

## Actual packet evidence and outstanding work

User tester is ready; root announced activation and requested traffic start. At11:24:17 UTC, deltas relative to precommit were:

- Ethernet5 RX0/TX1; Ethernet6 RX1/TX0.
- Ethernet9 RX1/TX1; Ethernet11 RX1/TX1.
- New drops, errors and RX misses: zero on all four ports.

A later independent VPP counter read found total RX/TX: port5=12/2, port6=12/1, port9=27/2, port11=16/2. Compared with precommit, this is forwarded traffic in all four directions: 5→6=1 frame, 6→5=2 frames, 9→11=2 frames, 11→9=2 frames. Historical drops stayed11/10/25/14. All services remain active, VPP PID167929/NRestarts0 unchanged. These few frames show initial forwarding evidence, not a sustained tester run or complete endpoint reception proof. Observe actual tester traffic in both directions before claiming complete packet acceptance. Do not reset existing counters; historic drops predate activation. Root-private API snapshots, short-lived session, and command helpers are in `/root/.cache/ngfw-route9-11-211`; never publish the session or full protected snapshots.

Layer3 addressing remains unspecified. Proposed lab addresses were not accepted. Keep both L2 pairs active for the user test; remove the relevant reciprocal cross-connects before applying later requested L3 addresses/routes.

Exact next read-only command:

```sh
python3 /root/.cache/ngfw-route9-11-211/apply-l2.py status
```

For counters use authenticated `/api/v1/state/interfaces` or read-only VPP show commands and compare against `interfaces-precommit.json`. Renew the short-lived session only through normal authenticated login if expired.

Initial operational `tools/ci.sh check --base origin/main` PASS (14seconds). GitHub main CI was green at startup; unrelated PRs244–249 and board were inspected without adopting their work. This task's observed workers: root operational agent, lcp_repair developer and routing_review independent reviewer. Other workers' liveness is unverifiable.
