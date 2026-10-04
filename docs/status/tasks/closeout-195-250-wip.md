# Connected-host acceptance closeout, 2026-10-04

Owner requested testing hosts195/250 after connecting ports, then completion of tasks awaiting tests. Initial root branch codex/test-closeout-current-195-250; final integration branch codex/test-closeout-195-250-final; initial current-source base4c8d1b247. Root owns integration, board/evidence, tools/ci.sh generated guard and test/topology/test-closeout; delegated work uses isolated branches.

## Actual evidence

- Physical preflight: ens161/224/225 pair across195/250; ens193/256 share broadcast domain;195ens257 has carrier but no proven counterpart. Payload transfer over SSH passes in both directions.
- Temporary remote VPP AF_PACKET forwarding, isolatedVRF65001:50 UDP packets each direction, exact payload and TTL64→63; cleanup verified. This is AF_PACKET smoke, not DPDK/composed acceptance.
- Initial host batch source664544349: tunnel, SRv6+globals, WireGuard handshake, NIC/state and untagged restart pass; stale canonical agent expectations, stale LB boot-store lifetime and a genuine MPLS rollback label leak fail. Original manifest retained.
- Agent strict fixture corrections: four real race host tests pass, including LB garbage collection65seconds; no skipped tests.
- MPLS matching-binding normalization fixes repeated apply/restart reference leak; unchanged rollback assertion passes real VPP and full descriptor race suite. Matching API removes only the same prefix+label; no generic route deletion. Existing old leaks are not swept; reapply briefly withdraws/reinstalls a binding. Root independently reviewed API guards and regression/live evidence.
- OSPF test fixture aligns owned veths with9000-byte VPP/LCP MTU; Full neighbors,100 routes, restart recovery7.4seconds and rollback0 routes pass. Root reviewed only-owned-link changes and unchanged assertions. Root-netns VPP FIB proof remains owed.
- Native IPsec initiator with production agent/sealed-secret delivery on disposable patch0002: exact1048576-byte TCP, bidirectional ICMP, counters, foreign-SPI refusal, rekey, actual agent restart, peer loss/defaultDPD/recovery, route withdrawal, rollback and ESP/no plaintextIPIP all pass109.64seconds. Binary was built from664544349 product source. This is native proof, not full API/browser site-to-site acceptance.
- Real API management TLS cert rotation at samePID, TLSminimum1.3, negative cert/key pointers, secret leak scan, dataplane runtime/preview/validation and unchanged startup pass. Independent scoped review APPROVE.
- Real API rule expiry and global blocking forwarding+nft local-input, restart-after-deleting-owned-ACLs and rollback:2PASS/noSKIP68.24seconds. Independent scoped review APPROVE;200k scale, wider feed/IPv6/anti-lockout and browser cases not run.

## Failures and pending gate

Initial quick ran all suites successfully but final exit127: root edited the running ci.sh while adding missing YANG generation coverage, shifting Bash's source offset. This run is FAILED, not a complete gate pass. Root error; rerun unchanged frozen final integration tree. Generation also exposed missing committed OSPF/PKI YANG output; regenerated output and gate path were reviewed.

Initial full API integration:61files,291PASS/1FAIL883.30seconds. NAT46 test expected schema-invalid PATCH200; actual400/pointer was correct. Explicit-boundary fixture fix independently reviewed; focused4/4 pass. Complete final rerun pending.

NAT46 live legacy driver failed because it precreated unclaimed VPP ports; repaired handoff exposes reply-path failure (forward SYN and IPv6 SYN-ACK observed, no IPv4 reply). Historical bounded return-path projection recovered and independently reviewed; guarded real packet/restart/rollback tests PASS on3d001ee60. Arbitrary nonembeddedIPv6 server return remains unsupported; no general NAT46 claim.

TEST-traffic-A composed executors/lifecycle/capture provenance are missing; B/C orchestrators absent. Source fixtures cannot close TEST A/B/C or INTEGRATE-E2E. PPPoE live peer/dial prerequisite and multi-WAN full traffic/weights/failover remain unverified.

Browser absent; official Playwright CDN403 prevents browser installation. No T4 screenshot PASS. Browser cases remain in deferred acceptance, not silently waived. System VPP observed MainPID1014/NRestarts0 throughout these executions; no system VPP restart or startup.conf edit.

Each delegated branch has a blob/tree-verified remote checkpoint; root publishes via authorized GitHub connector because CLI push403. Latest main1af871f0b was integrated, preserving current routing/WAN/HA/native-event changes; final complete quick/API/host reruns follow on frozen integration source. Rule-expiry host row is review-ready, not merged; broader rows remain open where criteria are unmet.


## Latest-source rerun and additional proof

Current integration base is main d5557440cd496ab5d0e0158fa6c61af4623ea0c3. Frozen host source d44d44eb6: all13 race host cases PASS/no SKIP with production/race binaries isolated per output directory. Production agent SHA2568658f0111afb301935815d462200debb359e75f5ffe7a6c71920c6758f5fce3a. Exact manifest and hashed receipts retained in closeout-195-250-evidence/latest-*.

Full API suite on isolated frozen worktree:289PASS/3SKIP (missing binary path). Those exact three real-agent tests were then executed against this binary and private VPP:3PASS/no SKIP; patch/diff/commit/readback/rollback and unconfirmed commit timeout passed. Initial skipped receipt remains preserved; this is combined292PASS, not a claim that the first run had no skips. Current production API/agent management TLS and expiry/global-block forwarding/nft/restart/rollback repeat PASS.

Current native certificate delivery/profile replay/shared-key rotation/revision revert/restart/removal PASS4.18s (certificate peer negotiation not exercised). Current native responder packet test PASS26.80s, including exact1MiB TCP, rekey, agent restart retained SPIs, partial-Apply guards, route/SA fail-closed and ESP capture. Earlier initiator/defaultDPD proof remains separately scoped.

Private OSPF root-for-FRR acceptance PASS59.16s:100 FRR+VPP routes, withdrawal50/reannounce100 before/after restart, pair-loss recovery7.6s, rollback/cleanup0. Whole execution uses private network/mount namespaces and AF_PACKET, not root-host physical/DPDK acceptance. Bounded daemon launcher file output fixes an inherited pipe hang without weakening lifecycle/FIB assertions. See closeout-ospf-fib-wip.md.

Two complete quick runs on latest base have failed and are retained. First: stale ISIS default expectation and developer branding/test knobs in tunnel help. Exact ISIS IPv4/IPv6 defaults and RIP version2 fixture expectations were aligned; user text retains real limitations and removes implementation/test details. Independent review APPROVE,31 focused checks PASS. Second: tunnel-page test still matched the former copy; its hardcoded text matcher was missed by the initial focused set. Correcting the matcher and executing the complete affected page suite precedes the next unchanged full gate.

The earlier API ENOSPC was /tmp inode exhaustion, not a product failure; retry used an owned disk TMPDIR. Historical gitleaks findings were two independently verified public certificate-bundle checksums; only their exact fingerprints were excluded and unchanged full scan was clean. No detector-wide exemptions or trust-policy approval were introduced.

Reviewable draft PR: https://github.com/mcoder1001-cyber/NGFW/pull/161. Exact remote checkpoint tree verified; archived reviewed prior head before rebase. No merged/all-Done claim while final complete gate and final bounded Multi-WAN review remain pending. Browser/T4,200k scale/full feed/IPv6 coverage, PPPoE peer, broader NAT46 server support and missing composed wave executors remain explicit outstanding scope.


Final bounded Multi-WAN source0b1343a4a/evidence da16135f2 PASS32.73s/no SKIP: real API/agent device-bound ICMP probes, installed default and selected-peer packet identities. Failover2.530s and full failback2.652s; preferred/failover/restore/restart3/3; applied rollback removes group after bounded1Hz snapshot refresh, defaultdrop/no forwarding next hop and0/3. Private peer arp_announce=2 fixes off-link source after neighbour flush; all failed diagnostics retained. Independent review APPROVE. Weighted balance, per-member NAT, ABF/PBR, IPv6 and dynamic gateways are still not proven by this bounded test.

Tunnel page/locale/product-copy focused suite11PASS after matching the reviewed help. Root independently approved Multi-WAN source isolation/real route-and-peer proof and OSPF timeout/file output/private network guard. Final unchanged complete local/hosted gate follows this integrated tree; no merge before green.


Final integrated quick8e62ebbf passed all35 JS tasks and agent vet, then FAILED pinnedGo lint:50 issues (42 exported/unused comments,4 unchecked cleanup closes including our OSPF launch file,3 potentially overflowing casts,1 embedded selector simplification). No lint exemption added. Three isolated author branches fix disjoint native, routing and WAN/VRRP source; root fixes its OSPF cleanup. Independent reviews and next unchanged complete gate are mandatory. Hosted run37216682408 still observed in progress; no success claim.


Pinned Go lint fixes are independently reviewed: exact exported comments, cleanup close handling, equivalent RSA selector/size variable, protocol-bound VRID/observed-priority/WAN path weight guards with positive/negative race tests. ZeroGo lint issues on integrated source. Full standalone race package run found only missing test-only RIPng binary documentation; exact existing harness path added, production binary allowlist unchanged. TestAllowlistDocumented and production no-trampoline contract then PASS. Remaining full gate follows this frozen integration tree. Rule-expiry host row closes atomically with approved green PR161 integration; other incomplete rows are unchanged.


Latest production/race binaries built from frozen51625a523 (after allGo fixes): agent SHA256bb12c410e4b34382308887a130d479d01999b64e859bd68b4097fdf79a0ca3ae. All13 live host cases again PASS/no SKIP. Native certificate production + responder packet proof again PASS31.042s total; real Multi-WAN again PASS31.623s including restart/rollback/cleanup. Hashed repeat receipts and latest manifest retained; prior d44 manifest preserved separately. Shared VPP MainPID1014/NRestarts0 unchanged.

Complete quick51625a523 passed35JS tasks, fullagent lint/race/build, then FAILED CLI exactoperation-table/OpenAPI consistency. Official CLI generator restores exactly9 additive existingAPI operations, with fullCLI lint/race/buildPASS and independently regenerated identical API OpenAPI input. Root addsCLIgen to the existing generation step and its output/module consistency guard; no check removed. BashsyntaxPASS, ShellCheck baseline14/new14/no new diagnostics (pre-existing style findings retained). Next frozen complete local/hosted gate must pass before integration.


Frozen df7f58732 unchanged complete local quick PASS10m37s. Main then advanced to f8fcd6c2 via reviewed PR162 dataplane approval/apply flow. Integration preserves all new API/UI/executor/packaging functionality and current main lint fixes; equivalent comment/native cleanup/UI conflicts resolved to main. Existing inline priority guard is extracted into reviewed boundary helper, preserving both main VRID regressions and additional0/255/max/observed-priority tests. Duplicate RIP expectation introduced by automatic merge removed. Real management preview now explicitly expects applyAvailable=true (new API capability contract); no apply/restart request is sent and shared startup remains protected. Prior false-capability proof retains its original source scope. New complete gates/API rerun required on current integration.

Final independent fixture review repaired only test staging: same production confirm-revert callback, existing pending/degraded/failed-Apply/retry/baseline assertions retained;50 repeated race cases including actual confirm timer PASS. Source6441d1e3 complete unchanged local quick PASS12m30; source64fffb9d host13/13PASS/noSKIP on separately built provenance-checked binaries, sharedVPP1014/NRestarts0 unchanged. Fresh64 full API289PASS plus exact3 real-agent/privateVPP rerun PASS, current management/TLS/preview PASS; complete artifact trees checked unchanged. Prior source receipts/failures remain separately scoped. Main then advanced tobee8ed4b with reviewed WAN address-family PBR resolution and board closures. Integration preserves all main source and both main AF regression and our width-bound regression; current-source complete gates remain required. WAN scoped-pool and certificate capability corrections are independent follow-ons, not claimed integrated by this PR.
