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

## P10 appliance packaging acceptance

All live cases below are **NOT RUN**. Package source, fixtures, syntax checks and offline unit graphs do not certify a working appliance.

| Case | Single-campaign execution when the target is available |
|---|---|
| Artifacts and signing | Rebuild the original seven shipping VPP packages; verify manifest and exact dependencies; build four product packages; run lintian, inspect content/permissions, verify Release/InRelease with the pinned exported signer; test wrong pins, malformed metadata and signing/output path overlap refusal |
| Safe installation | Fresh Ubuntu 26.04 chroot/VM with VPP masked: install/remove/upgrade/reinstall, preserved existing policy-rc.d, no premature daemon start, firstboot ordering and all failure rollback/credential retention cases |
| Identity and secrets | Real PostgreSQL migration and existing AuthService seed/readback; stable JWT/master key across retry/reboot; unknown/duplicate/overridden environment refusal; completion-marker crash recovery; delete bootstrap credentials only after successful verification |
| Firewall and boot | Real nft syntax/readback; early static base policy and exact management/punt interfaces; no unrelated table flush; offline and real distro systemd graph, failure propagation, firstboot/VPP/API/nginx boot sequencing |
| Runtime renderers | Exercise FRR/Kea/chrony/rsyslog/capture writes under the installed unit's actual capabilities and permissions, reconcile/restart and inspect daemon readback |

Known implementation/decision gaps are not lab-only deferrals: dynamic LCP punt synchronization now has source implementation and isolated tests, with target acceptance still NOT RUN; daemon-UID ownership requires the decision in `docs/decisions/PENDING-P10-agent-file-ownership.md`; source licensing metadata is unresolved for release. P10 remains running. Resolve these code/security/release boundaries before claiming full task completion or a releasable appliance.

### P10 dynamic admission follow-up

Source 00cb2cd3 implements per-host transaction-owned dynamic admissions in
`inet ngfw_base dynamic_punt_interfaces`; permanent bootstrap `punt_interfaces`
remains separate. All target cases below remain **NOT RUN**: actual nft element
add/delete and JSON identity/readback; LCP create/delete/rename/type-recreate
traffic admission/revocation; static/dynamic combined64 turnover; explicit and
default namespace transitions including malformed VPP readback; foreign/same-name
devices and orphan cleanup; nft/VPP restart/reboot resync; committed command with
lost reply, bounded compensation and DEGRADED recovery under installed unit.
Run them in this single campaign when access returns; source fixture success is
not traffic or boot acceptance. CAP_CHOWN/global `/etc` ownership decision and
release license remain unresolved, so P10 stays RUNNING.

### TD-19 provisioning and pinned installers

All target cases remain **NOT RUN**: genuine product artifact transfer and
installation on ngfw-b/ngfw-c; installed exact seven-package manifest version
readback; root-owned service-suppression policy and appliance ownership handover;
actual Go/containerlab downloads, installation and selected executable version;
Ubuntu 26.04 package availability and subsequent appliance boot. Offline fixture
success exercises redirected temporary files and blocked/fake remote/package
commands; it is not target installation acceptance. Execute these cases in this
single campaign when access returns. Missing authoritative third-party repository
key pins and other source reproducibility gaps remain implementation work and
are not lab-only deferrals or completed TD-19.

### Hosted fixture signing evidence

The dedicated `packaging-fixtures.yml` workflow adds isolated real-GPG temporary
key signing and gpgv verification to the offline fixture suite. Hosted run
[37034039364](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37034039364)
on `20df52065205b869873abec1acf673470867882f` passed at 2026-10-02T16:27:38Z:
**26 fixture tests PASS, 0 skipped**, including real temporary-key signing and
gpgv verification; seven strict-gate policy tests also passed. Local execution
still has 25 PASS / 1 signing SKIP because agent sockets are unavailable and
the strict wrapper correctly exits1. This bounded hosted proof does not establish actual
VPP provenance, real reprepro publication, production signing-key management or
fresh appliance installation/boot. Those target cases above remain NOT RUN.

### TEST-traffic-A inactive source foundation

The bounded support foundation frozen at `bd443810dd048ef767c4bba0e4b7153b2a0d907b`
has sixteen strict source cases; neither these nor structural capture parsing
prove forwarding. Seven composed executors, candidate/commit and rollback
lifecycle, shared-lock/lease renewal, protected run directories, capture
generation, packet correlation and forwarding/drop assertions remain genuine
**NOTIMPLEMENTED source work**. The live CLI refuses all stages. The stdout byte
cap continuation is split from this foundation and must ship before live
activation. These gaps are not lab-only deferrals; whole TEST-traffic-A is not DONE.

Actual laboratory acceptance remains **NOT RUN**. Once the executors and their
reviewed lifecycle exist and access returns, use this single campaign to check
VLAN→bridge/BVI→VRF/ECMP→uRPF/PBR→ACL→NAT44-ED/EI, selected PBR and both ECMP
paths, expected forwarding/drop, translation/endpoint independence, rollback
cleanup, capture loss and run identity. Both `whole_chain_proven=false` and
`packet_outcomes_proven=false` remain authoritative until real acceptance proves
them. No real rig, SSH, VPP, nft or capture operation was run for this foundation.

## Three ready-task source integrations — 2026-10-04

The owner explicitly authorized public publication and merging of the three selected source implementations while deferring aggregate CI. This is a scoped exception for F-bruteforce-block-host, F-igmp-mfib-host and F-capture-trace-host; it does not change workflow files, host privilege boundaries or other task acceptance. Independent source reviews and focused checks are recorded in the task reports. An environment-blocked or incomplete aggregate gate is **NOT PASS**.

Marking these rows merged records source integration only. The cases below remain **NOT RUN** until a provisioned target produces fresh evidence.

| Task | Required deferred acceptance | Explicit boundary |
|---|---|---|
| F-bruteforce-block-host | Real VPP ACL and nftables forwarding/local-in enforcement, trusted SSH/native-auth/scan observation, allowlist/TTL, rollback, restart/dataplane-loss replay and failed-update recovery | Fake VPP/nft unit evidence is not kernel or packet acceptance; bounded native polling and rate-limited logs can miss observations |
| F-igmp-mfib-host | Real static mFIB create/retrieve/rollback/recreate; IGMP packet membership/events and reconnect/restart under the existing explicit IGMP opt-in window | FRR PIM renderer/synchronization belongs to F-pim-frrsync; PIM neighbour reporting remains unavailable; optional BIER is not built |
| F-capture-trace-host | Actual API e2e with PostgreSQL/Valkey, dedicated-VPP recovery and rig packets, BPF globals restoration, file lifecycle and real T4 screenshot | Shared-VPP dispatch capture remains banned; a missing dedicated socket produces explicit NOTRUN/SKIP, never packet acceptance |

Aggregate CI for the combined integration tree remains deferred by this authorization. Existing local quick attempts encountered Unix-socket permission failures; no aggregate green result is claimed. Run the unchanged complete gate on the exact integrated SHA in an environment with the required socket permissions, preserve its failures and rerun after fixes. Record host SHA, slot, commands, actual results and artifacts for every live case.

## LDP, PIM and detector source batch — 2026-10-04

This batch requires the unchanged complete hosted quick gate before integration (D-233). The earlier three-row aggregate-CI exception above does not apply. Host-independent implementation and review are recorded per task; laboratory acceptance below remains **NOT RUN**, and source integration does not assert packet acceptance.

| Task | Required deferred acceptance | Supported source boundary |
|---|---|---|
| F-mpls-ldp-host | Real FRR LDP peers/labels, IPv4 EOS VPP forwarding, label ownership collision, LCP/label-range changes, withdrawal, restart and failed-apply recovery | At most 256 dynamic EOS IPv4 routes; NEOS and IPv6 forwarding are not built |
| F-pim-frrsync | Real FRR PIM neighbors/mroutes, VPP mFIB packet replication, remapping/withdrawal, reconnect/restart and failed-apply recovery | Default/global table 0; at most 256 dynamic routes; non-default VRFs are not built |
| F-bruteforce-detectors | Actual trusted SSH/native-auth/scan observation through API to agent, distinct-port threshold, allowlist/TTL, enforcement, restart/replay and rollback on provisioned VPP/nft/PostgreSQL/Valkey rig | Bounded source/observation windows fail unavailable beyond supported limits; unit evidence does not prove real traffic blocking |

Capture the exact integrated SHA, topology/slot, commands, raw outcomes and artifacts when running these cases. Scaling follow-up is recorded in `docs/tech-debt.md`; unsupported routes and thresholds must remain explicit.

## Eight historical Review recoveries — 2026-10-04

Owner authorized source recovery and final merging with complete CI waived. See `tasks/eight-review-recovery-20261004.md`. Source completion and live acceptance are separate. Default NIC binding/release on hardware, classify/sentinel current-host lifecycle, det44 current packet acceptance, and alarm rebuild on actual PostgreSQL/API restart remain deferred. Unsafe DS-Lite pool deletion tests are parked until a verified upstream VPP fix and reviewed safe harness exist.

NAT46: scoped descriptor acceptance was run on current VPP slot17 and passed create/retrieve/idempotent apply/delete; full IPv4-to-IPv6 packets, agent-restart timing, rollback and API/browser remain deferred. OSPF: restored FRR live and full topology tests; FRR-only adjacency/redistribution/withdrawal/event/config removal passed on slot11 (41.597s); root FIB mode fails closed on unverified shared-host preconditions. Full FRR/VPP adjacency/FIB, withdrawal/restart/rollback and API acceptance remain owed unless recorded in the recovery report. These two host rows remain parked; historical PASS evidence alone does not complete them.

## Five incomplete task completion campaign — 2026-10-04

Dataplane UI: observed VPP runtime/probe errors and actual startup preview/diff implemented, live read-only VPP probes passed. Deployed browser en/fa, startup commit/rollback and gated restart acceptance remain NOT RUN; VPP restart is the separate appliance privilege gate.

Management UI: certificate removal/rotation listener lifecycle and accurate revision/protocol status completed. Focused TLS/WSS15 and UI/locale11 tests passed with independent review. Real deployed API/DB/browser anti-lockout acceptance remains NOT RUN.

The owner waived hosted/full CI for this campaign. Focused tests, typechecks and independent source review are recorded separately; no waived gate is claimed passed.

Setup wizard: seven-step staged first boot/re-run and password/confirmed commit source completed. PPPoE disabled pending the real client. Real DHCP/NAT/LAN management, session-loss rollback and browser screenshots NOT RUN.

PKI: operational secure materializer, sealed cache adapter and mutation UI completed. Real browser/daemon acceptance NOT RUN. Native IKEv2 certificate consumers belong to the separate native task. Public CA/CRL and operational keys only; CA signing keys stay API-side.

OSPF: v3 contracts/rendering, fixture MD5 auth, Event20 and bounded on-demand state/UI completed. Production MD5 delivery remains explicitly PENDING-secret-channel per the task prompt. Real v2/v3 VPP FIB, withdrawal/restart/rollback and browser acceptance NOT RUN.

## Running-task source completion — 2026-10-04

Reviewed source merged through PR151 (tunnels, IS-IS/RIPng, native SA events), PR153 (fresh package staging), PR154 (WAN), PR155 (HA). Existing owner hosted/full CI waiver is retained; focused checks and integrated builds passed, not a complete hosted gate. Evidence: tasks/complete-running-20261004-wip.md, tasks/wan-integration-20261004.md and tasks/ha-integration-20261004.md.

| Task | Deferred laboratory acceptance (NOT RUN) | Source limit / separate decision |
|---|---|---|
| F-tunnels | Deployed browser, live tunnel/FIB/stats transitions, packet/restart/rollback exercises | Live unavailable/absent fields remain explicit |
| F-isis-rip | Dual-peer IPv4/IPv6 adjacency, route/FIB withdrawal, source password rotation, daemon/agent restart and rollback, deployed browser | Sealed production password references implemented; package fixtures are source evidence |
| F-multiwan-host | Timed packet failover/restore, 1000-flow weighted split/affinity, NAT session preservation, restart/rollback | Static gateways, default namespace/default VRF probes; unsupported VRF/netns unavailable; DHCP/PPPoE handoff not built |
| F-vrrp-config-sync | Dual-appliance VRRP master change/split-brain, peer-offline recovery, anti-lockout and configuration-sync reconciliation | Single automatic writer; mixed/unknown roles refuse; HA NAT/IPsec/ACL state synchronization unsupported and warns |

P11/native certificate trust option 1 was approved by the owner under D-234 on 2026-10-04; source implementation and independent review are complete. Real production sealed-cache/PKI/global-key/profile lifecycle, rotation, explicit revert and autonomous agent recovery after simulated profile/snapshot loss passed in disposable VPP. Certificate peer negotiation/packets, active-SA rotation, on-appliance/browser acceptance and deployment of this new source remain NOT RUN. P10 development packages are verified installed on 172.30.126.250; runtime file-ownership and release-license followups do not negate that installation. TD19 bootstrap trust remains pending. This source campaign has not restarted shared VPP, changed services or installed target packages.
