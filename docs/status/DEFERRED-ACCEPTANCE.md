# Central deferred acceptance campaign

Owner authorization, 2026-10-02: remove Telegram; merge reviewed code without waiting for laboratory access; collect deferred tests here and continue development. This supersedes historical lab-only BLOCK verdicts. Real code defects and the complete hosted quick CI remain merge blockers. NOT RUN never means PASS. Existing security, secret-channel, shared-host and privilege handover decisions remain applicable.

## Campaign order when access returns

1. Record exact main SHA, appliance/VPP/FRR versions, host reachability, slot ownership and handover. Use existing locks and CI slot12. No unauthorized VPP restart.
2. Run full CI, then management anti-lockout, auth/MFA, commit/confirmed rollback and expiry cases.
3. Run grouped forwarding, daemon state, withdrawal and permitted restart cases; then real API/browser en/fa RTL light/dark acceptance.
4. Preserve every failure, fix code, record fix SHA and rerun affected and cross-feature cases.

Each case must record SHA, slot, command, expected/actual outcome, artifacts, failure/fix and rerun. Initial outcome for all cases below: **NOT RUN**.

## Integrated review backlog

The exact fourteen-row matrix, implementation SHAs, missing functionality and acceptance cases are in [review-lab-audit.md](review-lab-audit.md), incorporated into this campaign. This historical audit distinguishes prior integrations from missing source at the time of review. Current completion and remaining functionality are recorded in each task closeout; deferred live acceptance is not PASS. Board reconciliation is not fourteen new product merges.

## Active feature acceptance

| Feature | Deferred acceptance | Functional boundary |
|---|---|---|
| Dashboard/Prometheus, PR58 | Real VPP stats traffic, bind/close/restart, metrics growth, alarm transitions and delivery, API browser | Aggregate counters only; no fabricated directional/link stats |
| Notifications | SMTP TLS/auth, HMAC webhook DNS/IP filtering, rules/dedup/retry/reload/shutdown, real alarm/commit/link/VPN events, en/fa browser | SMTP/webhook only; Telegram removed by owner instruction |
| Setup wizard | Real first-boot LAN access/DHCP/NAT, password, one confirmed commit and rollback/session loss, stale candidate/cancel/reopen, browser | No relaxation of management anti-lockout |
| System identity | Installed readback, DNS facts, bounded public login banner, failure pointers, restart/no rewrite, browser | Restart privilege boundary retained; daemon status unknown unless observed |
| Multi-WAN | Two WANs traffic failover/restore, weighted flows, NAT session cleanup, queue retry/restart, browser | Static gateway forwarding, owned VRF routes and ABF group expansion are implemented; probes support the default namespace/default VRF. DHCP/PPPoE gateway handoff remains separate code work; non-default probe VRFs/netns refuse unavailable. |

## Remaining host acceptance rows

TEST-trafficA, F-lb-host, F-srv6-host, F-mpls-srmpls-host, F-rule-expiry-host, F-global-blocking-host, F-pppoe-client-host: execute their existing task acceptance against real slots. AutoBlock, multicast and LDP source recovery and reviews are recorded in their integration reports; their forwarding/expiry/detector, PIM/IGMP and label/neighbour/restart laboratory acceptance remains NOT RUN. No unavailable test is marked passed.

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

Known implementation/decision gaps are not lab-only deferrals: dynamic LCP punt synchronization now has source implementation and isolated tests, with target acceptance still NOT RUN; daemon-UID ownership requires the decision in `docs/decisions/PENDING-P10-agent-file-ownership.md`; source licensing metadata is unresolved for release. P10 is merged for the verified development-package installation. Ownership/identity source followups are addressed by DEC-agent-file-ownership-20261008 in the completion campaign; their final hosted and installed-service acceptance remain separate. Authoritative product-license metadata remains unresolved and is not lab-only.

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
not traffic or boot acceptance. The former CAP_CHOWN/global `/etc` source boundary is addressed by the
reviewed completion-campaign ownership/identity changes, without broad writable
`/etc`; final hosted and appliance acceptance remain required. P10 stays merged
for the verified development installation. Product release-license authority
remains unresolved and is not a lab-only test.

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

### TEST-traffic-A composed source acceptance

PR166 integrates the leased in-tree composed transaction and owned NAT fixture,
candidate/commit/rollback lifecycle, canonical locks, protected captures, packet
correlation, CLI/API counter readback and cleanup guards. The original bounded
foundation remains covered by source fixtures. Independent source review and
fixtures do not prove actual forwarding.

Actual laboratory acceptance remains **NOT RUN**: execute
VLAN→bridge/BVI→VRF/ECMP→uRPF/PBR→ACL→NAT44-ED/EI under a manager lease and idle
owned window. Retain both-path/PBR, forwarding/drop, translation and endpoint
independence, rollback/residue, capture-loss and VPP-identity evidence. The runner
can mark whole_chain_proven only after real packet, readback and cleanup checks
pass. No live campaign result is claimed by this source batch.

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

Setup wizard: seven-step staged first boot/re-run and password/confirmed commit source completed. The wizard PPPoE option remains disabled; PR165 integrates agent client wiring, while real ISP/kernel PPP and deployed wizard acceptance remain NOT RUN. Real DHCP/NAT/LAN management, session-loss rollback and browser screenshots NOT RUN.

PKI: operational secure materializer, sealed cache adapter and mutation UI completed. Real browser/daemon acceptance NOT RUN. Native IKEv2 certificate consumers belong to the separate native task. Public CA/CRL and operational keys only; CA signing keys stay API-side.

OSPF: v3 contracts/rendering, fixture MD5 auth, Event20 and bounded on-demand state/UI completed. PR164 wires OSPF/RIPv2 MD5 password selection through the existing versioned sealed channel and adds RIPv2 per-interface key-chain authentication. Live authenticated peer/rotation acceptance remains NOT RUN. Real v2/v3 VPP FIB, withdrawal/restart/rollback and browser acceptance NOT RUN.

## Running-task source completion — 2026-10-04

Reviewed source merged through PR151 (tunnels, IS-IS/RIPng, native SA events), PR153 (fresh package staging), PR154 (WAN), PR155 (HA). Existing owner hosted/full CI waiver is retained; focused checks and integrated builds passed, not a complete hosted gate. Evidence: tasks/complete-running-20261004-wip.md, tasks/wan-integration-20261004.md and tasks/ha-integration-20261004.md.

| Task | Deferred laboratory acceptance (NOT RUN) | Source limit / separate decision |
|---|---|---|
| F-tunnels | Deployed browser, live tunnel/FIB/stats transitions, packet/restart/rollback exercises | Live unavailable/absent fields remain explicit |
| F-isis-rip | Dual-peer IPv4/IPv6 adjacency, route/FIB withdrawal, source password rotation, daemon/agent restart and rollback, deployed browser | Sealed production password references implemented; package fixtures are source evidence |
| F-multiwan-host | Timed packet failover/restore, 1000-flow weighted split/affinity, NAT session preservation, restart/rollback | Static gateways and owned VRF/ABF forwarding implemented; probes remain default namespace/default VRF only. Non-default probe VRF/netns refuse unavailable; DHCP/PPPoE gateway handoff is not built. |
| F-vrrp-config-sync | Dual-appliance VRRP master change/split-brain, peer-offline recovery, anti-lockout and configuration-sync reconciliation | Single automatic writer; mixed/unknown roles refuse; HA NAT/IPsec/ACL state synchronization unsupported and warns |

P11/native certificate trust option 1 was approved by the owner under D-234 on 2026-10-04; source implementation and independent review are complete. Real production sealed-cache/PKI/global-key/profile lifecycle, rotation, explicit revert and autonomous agent recovery after simulated profile/snapshot loss passed in disposable VPP. Certificate peer negotiation/packets, active-SA rotation, on-appliance/browser acceptance and deployment of this new source remain NOT RUN. P10 development packages are verified installed on 172.30.126.250; runtime file-ownership and release-license followups do not negate that installation. TD19 bootstrap trust remains pending. This source campaign has not restarted shared VPP, changed services or installed target packages.

### Seven ready tasks: dataplane Apply source closeout (2026-10-04)

PR162 implements the root-only Unix-socket executor, single-use actor/document/
preview/installed SHA-bound approval, audited administrator API/UI action and
existing product apply-startup rollback guards. Exact source head
`c434481c0c5f55608b8bb4fc5e12274c9633eb65` passed the complete unchanged hosted
quick gate [37217602615](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37217602615)
and merged as `f8fcd6c2fc8cfddfe8c34397681acec44724225d`.
Installed socket activation/permissions, real startup preview and approval,
privileged apply/deadman/health rollback and en/fa browser acceptance remain
**NOT RUN**. Execute under appliance ownership and the existing exclusive lock;
source and isolated fixture success do not certify deployment.

### Seven ready tasks: IGP source closeout (2026-10-04)

PR164 additive RIPv2 MD5 key chains and decoded OSPF/RIP sealed-secret selectors
passed unchanged complete hosted quick
[37220789126](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37220789126)
at `14771daa5574bc36678f014a54db9acba7bd949b`, then merged as
`8c537ce2ff09fe4d91239149736e4ba2e24d30fd` after independent source/security
review. Live FRR authenticated peers, key rotation/removal, convergence and real
API/browser acceptance remain **NOT RUN**. Existing unrelated historical FRR
observations are not fresh acceptance of this merge.

### Seven ready tasks: Multi-WAN source closeout (2026-10-04)

PR159 fixes address-family matching by VRF/prefix for dual-stack WAN groups
sharing member interfaces. Previously-integrated monitor/default-route/weighted
ECMP/SNAT/ABF wiring is not claimed as new code. Complete cumulative hosted quick
[37222055557](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37222055557)
passed at `2dd48e0afa3456e4494651995a233c02290a7663`; merge
`bee8ed4b22a6270b61186ecabd0b8f2716aaa174` followed independent review.
Real failover/restore, 1000-flow weighted split and affinity, NAT cleanup,
agent-originated ABF/readback and rollback remain **NOT RUN**. Dynamic
DHCP/PPPoE gateway handoff remains separate implementation work.

### Verified bounded Multi-WAN acceptance (2026-10-04)

The prior NOT RUN statements describe the source-only closure above. Real
owned-peer failover/restore passed2.530s/2.652s with restart/rollback. Corrected
automatic static per-member SNAT passed54.253s; joined PR159 source passed53.404s
with1000 weighted UDP flows, exact2000 echoes, correct per-peer source addresses,
native counters, restart retention, selective dead-member cleanup and API PBR.
See `tasks/closeout-wan-final-packets.md` and retained raw/provenance evidence.
This clears only static default-VRF IPv4 UDP acceptance. DHCP/PPPoE gateway
handoff, nondefault probe VRFs/namespaces, TCP/IPv6, scale and browser acceptance
remain deferred; no broad deployment/upgrade claim is made.

### Seven ready tasks: PPPoE client wiring source closeout (2026-10-04)

PR165 integrates desired sessions, parent LCP tap selection, versioned sealed
password delivery, runtime dial/exit/failCount/lastError state, credential-only
redial, withdrawal/removal and idempotent address/default-route mirror cleanup.
Complete cumulative hosted quick
[37223559490](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37223559490)
passed at `38cef0cb113789f2576605c9737b96a1277695df`; merge
`12472a11ca21d9f0bc5ab330b72250069159c72e` followed independent review and
19 successful combined IGP/PPPoE delivery cases. Real ISP/kernel PPP dial,
VPP/LCP address/routes, peer DNS, exit/restart and rollback remain **NOT RUN**.
The Setup Wizard PPPoE option remains disabled. Dynamic Multi-WAN gateway
handoff and non-default namespace PPP are separate unsupported boundaries.

### Seven ready tasks: Management host source closeout (2026-10-04)

PR163 supplies an isolated real-API management acceptance driver with atomic
candidate ownership/revision guards, private temporary certificate/key material,
TLS rotation/version/fingerprint checks, mismatched-key validation, audit scrub,
listener process invariants and guarded cleanup that preserves ambiguous state.
Five driver regressions and complete cumulative hosted quick
[37224837911](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37224837911)
passed at `fc6d896c99514c6403248f7c2b648f60c8dce719`; reviewed merge
`d1aba878ca5f0f2420e3ca146ffaaa4873a64e1a` followed.
Actual deployed API/database TLS rotation and anti-lockout, real audit/log scrub,
rollback and en/fa browser acceptance remain **NOT RUN**. Driver unit fixtures
are not actual appliance or browser results.

### Seven ready tasks: Dataplane host source closeout (2026-10-04)

PR160 supplies an isolated real-API Dataplane acceptance driver with atomic
candidate ownership, owner/lock/revision checks, preview/diff/SHA/readback and
semantic-error pointer checks, plus process/startup no-restart invariants.
It accepts both supported Apply-availability outcomes and does not invoke the
separate privileged VPP apply/restart action. Three driver regressions and
complete cumulative hosted quick
[37226354347](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37226354347)
passed at `4eaaa17a676fbff11a05321a385e3160cb9008a3`; reviewed merge
`4069b365d1b762411ebdfc1a07c37461de04cb18` followed.
Actual deployed API/database preview/commit/revision behavior and en/fa browser
acceptance remain **NOT RUN**. Isolated driver tests are not real host proof.

### Seven ready tasks: Traffic composed source closeout (2026-10-04)

PR166 supplies the composed VLAN/bridge/BVI/VRF/ECMP/uRPF/PBR/ACL/NAT44-ED/EI
acceptance executor, canonical manager lease/lock, in-tree owned NAT fixture,
private captures, packet/counter/readback correlation, configuration ownership
and guarded cleanup. Source validation has 61 Python fixtures and three Go
cases/vet PASS. Complete cumulative hosted quick
[37227941852](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37227941852)
and all ten companion PR fixture checks passed at
`ab4ef8d548a2596bd356cd40f41ff4d3f0842b45`; reviewed merge
`1f094f631caa6e4cdbc03afdb5c2cee8dcc23301` followed.
All actual packet forwarding/drop, both-path/selected-PBR, NAT translation and
endpoint independence, rollback/residue, capture-loss and VPP identity cases
remain **NOT RUN**. No live whole_chain_proven result was generated. Use the
existing manager lease and an idle owned rig window for this single campaign.

## F-hardening-lite offline tooling acceptance (2026-10-05)

Implemented and independently fixture-tested10controls/signature/key-rotation cases; complete reviewed-source quick PASS36m12. Full signed package install/keyrotation, real daemon runtime under optional profiles and appliance boot smoke NOTRUN: signed /srv/ngfw-artifacts/apt pool and disposable full appliance absent. Do not activate profiles on sharedhost. Owner permits deferring only these actual laboratory executions; offline source/tests and final/hosted current-main integration gates remain required. Follow docs/install/hardening.md on disposable target, record real servicehealth and effective settings before claiming runtime acceptance.

## F-images target execution (2026-10-05)

Implemented qcow2/vmdk/OVA/VHDX/VHD/GCP conversion and offlineVM/cloudtargetprofiles with independent13fixture/format cases andcomplete corrected-source quick PASS22m44. Full signed applianceimage build, actualfirmwareVMboot andAWS/Azure/GCP import NOTRUN: /srv/ngfw-artifacts/apt signedpool/manifest absent andrequiredfullappliancebuildspace unavailable. Disposableformatroundtrips are not applianceboot proof. Ownerpermitsdeferralonly ofthese genuine lab executions; codeguards/current-main local+hosted gates remainrequired. Run docs/install/images.md on isolated provisioned targetandrecordactualboot/interfaces/noautomaticdataplaneNICclaims.
## F-ab-upgrade appliance execution (2026-10-05)

Signed preformat/staging/confirm/rollback tooling implemented;17independentfixtures, actualownedprivate-loopunsigned/tamperedrefusal,stageB/confirmB/healthfailuredefaultA passed; exactreviewedsourcequick PASS9m49. Actualfirmware one-shotboot/powerfailure,reboothealth onfullappliance,databasebackup/export/restore NOTRUN because disposabledual-slotfirmware/appliancetarget unavailable. Loopmount/GRUBenv fixtures are notfirmwarebootproof. Follow docs/install/ab-upgrade.md onprovisionedisolatedtarget; collectboot/rollback/health/backup evidence. Ownerpermitsonlythese genuine runtimeexecutionsdeferred; finalcurrent-main/hostedmandatory gates remainrequired.

## P11-host remaining appliance execution (2026-10-05)

Actual isolated native PSK responder/initiator production-agent forwarding, rekey, restart, peer loss/retry and authoritative rollback independently PASS; see P11-host-live-test-T3.md. Full appliance deployment acceptance is NOTRUN without a disposable appliance. Certificate peer acceptance belongs to the separate certificate campaign and is not inferred from this PSK evidence. Source and mandatory final integration gates are not deferred.

## F-bfd-redistribution remaining native peer execution (2026-10-05)

Native multihop actual two-peer packet/liveness/authentication and live FRR redistribution counters across forwarding VRFs are NOTRUN because the isolated owned peer rig has not been provisioned. Durable native ownership/compensation/recovery,1024-session indexing, bounded observation lifecycle, FRR parsing/timers/family matrix and authenticated API/browser fixtures were implemented and independently tested. Their source tests and complete current-main local/hosted quick are mandatory, never deferred. Provision the owned private peer rig following docs/user/routing/bfd-redistribution.md; do not use shared VPP or infer native packet PASS from mock race/API fixtures.
## F-ha-state-sync remaining two-node execution (2026-10-05)

Actual native NAT44-EI session/TCP continuity, dedicated-sync packet capture, VRRP convergence and owned failover/VM-fault execution are NOTRUN because the disposable second appliance ngfw-b has not been provisioned. The concrete acceptance.py driver leases only its own candidate, checks exact EI tuples/roles and a persistent TCP connection, safely restores priority/revision ownership and bounds private output; optional VM helper requires matching root-owned PID/boot/unit proof. Sixteen offline safety/timing/redirect tests, real API permission/failure-audit/restart scenarios and frontend browser cases were independently executed. Source/correctness/security/mandatory current-main local+hosted quick are not deferred. Follow test/topology/ha-state-sync/README.md on a provisioned isolated two-node rig. ED/ACL/IPsec-SA sequence/replay sync remains unsupported, not deferred implementation.

## Six final tasks — 2026-10-05 freeze campaign

Source completion of the six final rows is separate from release acceptance.
The obsolete P11-pkg strongSwan/kernel-vpp build is superseded by the owner's
DEC-ipsec-route-based decision; native product packages and installation remain
tracked by P10/F-vpp-debs/P11-host. No historical package install is required.

| Case | Current campaign outcome | Next acceptance |
|---|---|---|
| Cross-component offline freeze | PASS: contract build, reachability, commit-engine/service46 and auth11 checks | Rerun on the final merged source via test/acceptance/freeze/run.py |
| Disposable VPP smoke, slot31 | PASS: af_packet ping/counters and cleanup; two tests, no skips | Does not certify API/browser or full product packet chains |
| TEST-traffic-B | NOT RUN here | Owned native route-based IPsec/daemon campaign; record exact SHA and packet evidence |
| TEST-traffic-C | NOT RUN | Corrected driver and independent reviews already exist; execute in an owned quiet window for real MPLS/SRH/VRRP/QoS/riders and rollback |
| Final browser | NOT RUN | Owned API/agent stack, en/fa real candidate/commit/rollback and console checks |
| Appliance install/HA | NOT RUN | Clean target boot/install and authorized two-node failover; never restart shared VPP while handover is pending |

Full local and hosted quick CI remain mandatory merge gates. The first freeze
quick attempt failed timing checks during concurrent load; preserve that failure
and rerun serially with existing concurrency controls, without weakening tests.
STATUS-FINAL must retain every unresolved code or acceptance gap explicitly.

### Wizard interface discovery fix (2026-10-07, PR #198)

Host-independent acceptance: exact API preview/stage for discovered physical NICs,
rejection of host-owned/virtual/managed-orphan interfaces and live VRF drift, and
English/Persian picker loading/empty/retry/selection regressions. See
docs/status/tasks/wizard-interfaces-fix-20261007-wip.md and the independent reports.

Deferred laboratory acceptance: real appliance NIC inventory, browser screenshots
against an owned API/agent stack, and confirmed-commit DHCP/NAT/LAN management
packet checks. No appliance configuration or service was modified for this fix.
The unchanged complete local and hosted quick gates remain mandatory before merge.
## Automatic Interfaces host discovery — 2026-10-07

PR199 / interfaces-discovery-20261007 source is independently reviewed; unit checks use scripted API/agent and fake sysfs fixtures. Real appliance/browser acceptance has **not been executed**: confirm all agent-returned physical PCI functions (including management and userspace-bound devices) appear automatically on the target appliance, capture EN/FA browser screenshots, confirm host-only drawers have no configuration actions, and observe unavailable engine/inventory diagnostics without changing NIC ownership. No claim of real seven-NIC VMware inventory or host connectivity verification is made by unit evidence. Existing HostNics scope is one entry per PCI function; Linux-only virtual/USB inventory and ambiguous virtio engine identity are explicitly outside this change. This laboratory-only acceptance is deferred under the owner instruction; complete local and hosted quick gates remain mandatory before merge.
