#!/usr/bin/env python3
"""Package authenticated private engine for its explicit offline appliance ABI.

Never installs on the builder and never calls a daemon, ldconfig or systemctl.
"""
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

module = importlib.util.spec_from_file_location('ra_stage', Path(__file__).with_name('stage-engine.py'))
stage = importlib.util.module_from_spec(module)
module.loader.exec_module(stage)


def package(artifact, abi_root, output):
    output = Path(output).absolute()
    if output.exists() or output.is_symlink() or output.suffix != '.deb':
        raise stage.Refused('new .deb output required')
    prefix, _ = stage.validate(Path(artifact), Path(abi_root))
    build = (prefix / 'share/ngfw/engine-build.txt').read_text()
    expected = ('version=6.1.0\n'
                'source_s…5147 tokens truncated…rprivate mounts.
Current remoteef935936. Next: publish hostNSFS consumer/unitrunfs isolation.

HostNSFS consumer checkpoint: trusted agent binds parent NSFS hostnetns before
unsharechild creates distinctnetns; strict rootplan decoder requires distinctpair.
Lowcap helper opens BOTH held NSFS files, matchesrootmetadata/currentprivate;
never relies on masked Proc1stat. Actual bounded-cap child onhostrefused and
private-ns accepted; namespace restart/rollback/cancel +NFTparser integration
PASS0.244s, renderer/VICI PASS0.111s, helper PASS0.038s underlablock. No daemon.
Unit now hideshost /run,etc,varlib,varlog,opt using private mount overlays and
binds onlyownprofile readonly +VICI daemon child writable. PrivateDevices and
PrivatePIDs (systemd257+, actualUbuntu26.04 systemd259) hidehostblock/processes;
onlypublic NSS/ldso files rebound, hostdata/boot/backups inaccessible, debug/mount/
reboot/rawIO/swap syscalls denied. Helper additionally requiresPID1/privateproc,
NoNewPrivs and absenthostcontrol/credential/dev paths beforeexec. VICI moves
instance/daemon/vici.sock; config/PKI/metadata remain readonly insideunit.
Actual systemd mount/seccomp enforcement is not yet accepted; disposable fixture
and packagedVM verification required; activeprofiles remainfailclosed.
Source: https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml

Diskcritical ownership action: moved ONLY own t19 artifacts into executable
/dev/shm/w19-ra-20261005 (owner marker), source tar/binary/receipt hashes matched,
original /root/.cache/t19 prefix preserved via ownsymlink. New TMPDIR/GOTMPDIR/
GOCACHE must use ownshm subdirs; nosharedcaches touched. Current remote7f647612.
Next: guarded TAP/ACL/readback consumer and real private6.1 EAP lifecycle fixture.

### Explicit TAP endpoint contract (2026-10-05)

TransitTAPs now builds fixed outer0/inner0 TAP endpoints exclusively in the protected per-profile namespace binding, with explicitly reserved distinct IDs and bounded ring sizes. Full protobuf readback equality refuses moved namespaces, changed IDs/addresses, bridges and offload settings. No VPP mutations or daemon activation were performed.

Actual verification: unchanged finite heavy wrapper, owned shm TMPDIR/GOTMPDIR/GOCACHE; `go test ./internal/ra_vpn ./cmd/ngfw-ra-daemon` PASS (0.050s / cached). Additional daemon host-sandbox refusal regression passed. Remaining: guarded descriptor registration, VPP/VRF/ACL readback, PKI/VICI lifecycle and actual EAP packet acceptance; task remains incomplete. Exact next command: `sed -n '1,200p' apps/agent/internal/subsystems/wiring.go`. Authenticated fixture/source receipt retained under /dev/shm/w19-ra-20261005 via existing logical-prefix symlink; hashes checked equal before owned cache move.

### Ubuntu appliance ABI staging guard

The offline builder explicitly supports the verified P14 Ubuntu26.04 amd64 target and records exact libc6/libssl3t64/libsystemd0 package versions. stage-engine.py refuses a shared host root, existing destination, wrong distro/architecture, missing or differing target runtime packages, symlinked target ABI paths and artifact paths escaping the private prefix before creating the destination. Six disposable offline tests PASS0.035s; builder shellcheck PASS. No host install/service activation. Full /opt/ngfw-ra build attempt stopped after successful configure with exit127 because editing the interpreted shell file during execution changed its subsequent read offset; no make/install/daemon execution occurred. Freeze source before retry, output new owned shm directory. Exact next command: `TMPDIR=/dev/shm/w19-ra-20261005/tmp GOTMPDIR=/dev/shm/w19-ra-20261005/go-tmp deploy/ra-vpn/build-engine.sh /dev/shm/w19-ra-20261005/appliance-engine-root-v2 /etc/os-release`. RA lifecycle remains incomplete.

### Guarded TAP lifecycle and durable runtime claims

GuardedTAP now validates reserved IDs, protected endpoint shape, actual namespace pair and complete VPP boot identity before each mutation. Pending root-private receipts are written before creation; full fresh owner-filtered endpoint/index readback is required before completing a claim. Delete refuses namespace replacement, each boot identity change, moved endpoints and recycled indices before the underlying destructive call. A boot change during create remains an uncertain pending claim and refuses adoption/deletion. FileTAPReceipts persists bounded private single-link JSON with a held no-follow root directory, fsync/atomic rename, and refuses foreign-boot claim replacement and symlink/hardlink files. ReadAgentPlan verifies actual held NSFS bindings and requires the caller in the recorded host namespace. These descriptors are not registered or activated yet.

Actual finite heavy tests PASS: `go test ./internal/ra_vpn ./cmd/ngfw-ra-daemon` 0.037s/0.023s. Actual shared-lab-lock disposable namespace readback/rollback test PASS0.072s, including forged host-inode refusal and restored original manifest before cleanup. No daemon invoked and no VPP mutation. Proper /opt/ngfw-ra signed strongSwan6.1.0 offline build now completed exit0 entirely in owned shm; receipt `/dev/shm/w19-ra-20261005/appliance-engine-v2-receipt.json` includes executable hashes and exact Ubuntu26.04 library ABI. No host install. Earlier reported df unprivileged Avail0 does not imply root cannot write small source checkpoints; manager verified reserved blocks and publication continued.

Remaining: actual descriptor registration/projection, explicit outer+inner ACL/VRF/route readback, PKI/VICI daemon lifecycle, real EAP packet acceptance and UI/API integration. Exact next command: `sed -n '1,160p' apps/agent/internal/subsystems/vpn.go`.

### Verified credential snapshot contract

Credentials is excluded from JSON and generic text formatting. VerifyCredentials validates bounded exact PEM, private-key/certificate correspondence, server DNS/IP identity, validity time, signing key usage, modern key/signature algorithms and each supplied certificate-chain signature. Certificate-authenticated clients require an explicitly present valid signing CA and current CRL signed by that exact issuer; missing/foreign/expired/ambiguous material fails closed. This is not client session acceptance and is not yet wired to daemon activation. Generated disposable ECDSA certificate/CRL tests cover wrong identity/time/key/CA, absent CRL, trailing certificate bytes, duplicate private-key material and serialization redaction. Actual finite heavy `go test ./internal/ra_vpn` PASS0.052s. Next: private file snapshot and PKI resolver consumer, guarded descriptor registration, full existing ACL/VRF handoff, isolated daemon/VICI lifecycle and real EAP packet acceptance. Corrected exact next source read (previous vpn.go path does not exist): `sed -n '1,160p' apps/agent/internal/subsystems/ikev2.go`.

### Exclusive private profile files and canonical settings

WriteSnapshot creates one exclusive root-private generation after credential and actual namespace verification: root0600 config/secrets/certificate/key/CA/CRL files, directories0700, fsynced writes, rollback of only entries created by this invocation. Existing generations cannot be overwritten while a daemon could retain earlier trust. Actual shared-lock disposable snapshot test PASS0.083s, including unchanged content after refused overwrite; own namespace and profile fixture removed.

RA template output now passes the existing strict canonical settings round trip. Root0600 VICI restriction pins the exact socket using O_PATH, verifies the peer PID via the pinned inode, and uses fchmodat2/AT_EMPTY_PATH; wrong PID, socket hardlink and symlink-to-regular-file tests prove no unrelated chmod. Focused renderer TestRA suite PASS0.071s. New RAMaterial/LoadRA loader source is still awaiting actual private-daemon load and regression tests, so it is not included in this coherent checkpoint. A first test run exposed Unix socket path length with long GOTMPDIR; own marker-protected `/dev/shm/r19t` now supplies both TMPDIR and GOTMPDIR for socket fixtures (GOCACHE remains owned shm). No daemon started in this checkpoint. Remaining real EAP/VPP/ACL acceptance and runtime/API/UI wiring remain explicit.

## 2026-10-05 private engine startup checkpoint
Recorded only known, down, unaddressed Linux automatic fallback devices before TAP activation, bound to protected namespace identity. Foreign names/kinds/indices and activated devices remain refused; operator-supplied baselines rejected. Sanitized helper diagnostics expose fixed source phase only. Focused fallback regression plus actual disposable namespace nft parser PASS (0.110s). Actual private engine fixture progressed beyond sandbox/link checks and currently refuses namespace stage16 (nft check); daemon socket/load/EAP not yet proven. No host daemon invocations or shared network changes. Next command: inspect private fixture public NSS/protocol files and rerun TestIntegrationPrivateEngineLoadsProfileThroughVerifiedVICI with owned /dev/shm artifacts. Engine VICI loader and fixture remain unfinished uncommitted consumers.

## 2026-10-05 actual private daemon and verified VICI milestone
Actual pinned strongSwan6.1.0 /opt/ngfw-ra offline artifact executed solely in verified private network/mount/PID namespaces with constrained capabilities. Verified root PID socket restricted0600, server credential/EAP user/pool/profile loaded over VICI, actual version and empty-session readback PASS1.079s. Missing public protocols/services caused earlier nft parser refusal; fixture and service now bind those public files read-only. Loader refuses nonempty/malformed daemon readback, uses EAP secret type and exact provided certificate paths, discards remote errors. Focused renderer/security/helper tests PASS (strongswan0.118s, ravpn0.059s, helper0.036s). Packet EAP, real TAP+VPP ACL handoff, production descriptor/runtime/API wiring and restart rollback remain unfinished. Artifacts and bounded private logs are owned /dev/shm/w19-ra-20261005; fixture daemons and namespace bindings cleaned. Next command: extend private fixture with separately owned client namespace and real EAP negotiation, then existing ACL enforcement through private VPP.

## 2026-10-05 10:44 actual EAP development checkpoint (unfinished)
Pinned authenticated engine v3 build exit0 after actual two-namespace packet fixture discovered omitted --enable-ikev2 under --disable-defaults. Private VICI loading milestone remains valid; earlier v2 artifact cannot negotiate IKE. New v3 two-namespace fixture creates veth directly in server namespace, moves peer directly to client protected NSFS binding, launches two guarded private PID/mount/network daemon fixtures, initiates actual EAP. Current packet failure: initiator emits IKE_SA_INIT/retransmit, responder sees no packet; full negotiation NOT passed. All own failed fixture processes/mounts cleaned. Added namespace-only fixed per-device reverse-path/redirect sysctls for existing outer0/inner0/xfrm0; helper rebuild session20962 queued behind heavy limiter, not yet retested. Focused transport wiring test session56071 queued. Exact next command after helperbuild: TMPDIR=/dev/shm/r19t GOTMPDIR=/dev/shm/r19t GOCACHE=/dev/shm/w19-ra-20261005/go-cache NGFW_INTEGRATION=1 NGFW_TEST_SLOT=19 NGFW_RA_ENGINE_ROOT=/dev/shm/w19-ra-20261005/appliance-engine-root-v3/opt/ngfw-ra NGFW_RA_HELPER=/dev/shm/w19-ra-20261005/ngfw-ra-daemon NGFW_RA_EVIDENCE_ROOT=/dev/shm/w19-ra-20261005/evidence ../../tools/heavy.sh flock -s /run/lock/ngfw-lab.lock go test ./internal/ra_vpn -run TestIntegrationPrivateEAPNegotiationAndObservedDisconnect -count=1 -v (cwd apps/agent). API/UI worker owns contract+consumer files on separate branch; outerPolicy19/RPC contracts published e495dada. Full RA lifecycle and VPP ACL packet policy remain unfinished.

## Guarded transport registration checkpoint
VPN domain now registers protected namespace and guarded TAP descriptors. TAP claims use concrete lazy file persistence: every actual load/save/remove validates root-owned no-follow parents before VPP mutation. Missing numeric scope grants no IDs; real product startup separately validates required scope. Protected namespace metadata declares persisted ownership, and handoff requires positive PID as well as complete boot tuple. Inventory now classifies guarded TAP as wired. Focused guarded-TAP/claims/lazy-unsafe-parent/handoff/Register/persistence tests PASS: ravpn0.066s, subsystems0.298s. Production profile projection/VRF/ACL/daemon descriptor remain unfinished, so enabledprofile failclosed guard still retained.

## Short protected NSFS alias checkpoint
Actual disposable VPP/TAP attempt independently proved VPP PID belongs to private coordinator namespace, then failed because fixed API host_namespace string[64] truncated full instance path. Added /run/ngfw/ra/n/<32hex> NSFS alias (<64bytes), with protected root parents, exclusive collision refusal, held NSFS inode match to full network.json and full-instance mapping rejecting ambiguity. Full64 identity/helper/unit paths remain unchanged. Namespace dependency keys use alias consistently; actual alias remove verifies root namespace inode before detach. Guarded TAP/namespace/agent-plan/transit/snapshot focused tests PASS0.481s including disposable namespace execution. Actual private VPP EAP+ACL packet fixture remains under development, latest command isolated-vpp.py session43494 running; no claim of packet-policy completion. Next command: inspect final private fixture result and correct real failure, then publish accepted evidence. New files isolated-vpp.py/vpp_integration_test.go own executable RAM fixtures, no shared services/VPP/NICs.

## Actual private VPP policy and encrypted packet milestone
Previous session43494 failed at TAP scope/API limits; subsequent session84006 negotiated EAP but encrypted ICMP reached VPP with unknown ICMP type. Disposable runner omitted ping_plugin ICMP responder registration. Added that fixture dependency, explicit selected-table next-hop interface routes (default recursive lookup previously produced drop), and reject TAP IDs above VPP limit8192 without remapping. Actual finite shared-lock private campaign PASS6.792s: EAP-MSCHAPv2 VIP/session, encrypted protected request/reply through owned TAP and innerVRF19001 with explicitly read-back outer/inner ACL bindings, denied protected address unreachable, observed disconnect. Command isolated-vpp.py session20848 exited0; own daemons/VPP/namespaces cleaned. Kernel diagnostics read bounded bytes in memory and whitelist public JSON fields; non-JSON XFRM output is discarded without printing or persisting keys. Focused guarded TAP/receipt/transit regressions PASS0.056s before packet run.
Production controller/projection/RPC still absent and enabled profiles still refused; separate controller worker assigned exclusive new lifecycle files. Existing helper/credentials/snapshot/VICI/packaging/packet security remain owned here. Next command: inspect snapshot.go and implement exact verified generation cleanup and sealed PKI/EAP/RADIUS preparation interface paired with controller EngineSpec; merge published API/UI8ec352cf after this checkpoint. No shared host mutation; closed historical host-daemon incident remains documented. Local checkpoint parent f0651aae; publication receipt reported to manager after successful connector update.

## Verified snapshot cleanup checkpoint
Credential snapshots now persist root0600 public inode-only generation receipt, bound to full namespace identity. Cleanup refuses changed credential inodes, symlink/hardlink/mode violations, unexpected child entries and live VICI listeners; inactive socket ownership is rechecked before unlink. Only exact snapshot files/directories are removed, preserving network.json and namespace ownership. Actual owned namespace integration PASS0.160s: replacement credential refused, live listener refused, stopped exact generation removed, new generation recreated and removed. Focused credential/TAP package checks PASS0.077s. Contract callback source consumed from independently published controller08ced; API/UI final source merged b121. Sealed preparation adapter is drafted but not yet tested/published; Recover and actual read-only ABI/helper/unit readiness remain unfinished. Next command: test sealed immutable fingerprint preparation against controller hmac contract fix, then implement receipt-backed recovery and actual EAP-TLS good/revoked/foreign certificates.

## Sealed preparation and verified restart recovery
Concrete SealedPreparation resolves only immutable hmac:<64hex> fingerprints from scheduler spec, never current literal cache values. Authenticated certificate/private-key identity, client CA and fresh signed cert/<CA>.crl are verified before private snapshot; EAP and RADIUS render through the same immutable map. Prepared Load keeps private bytes in redacted one-shot closure, clears keys/passwords/daemon secrets after use or cleanup. Recover reproduces all snapshot bytes from sealed generation and verifies recorded root inode/namespace ownership without rewriting active files. Corrected paired contract from worker0dea to preserve actual hmac prefix. Actual owned namespace integration PASS0.261s for immutable resolution, generation overwrite refusal, restart recovery, same-inode config tamper refusal and exact cleanup. No daemon started by these source tests. Preflight actual ABI/helper/unit/cache checks and EAP-TLS positive/revoked/foreign remain unfinished; production controller worker owns activation wiring/RPC. Next command: implement read-only fixed artifact/unit/ABI readiness and then private EAP-TLS certificate campaign.

## Actual EAP-TLS trust acceptance
Finite guarded two-private-namespace authenticated strongSwan6.1.0 suite PASS5.930s: internal-CA client certificate negotiates VIP and actual observed session/disconnect; signed CRL revocation refuses client with engine certificate-was-revoked marker and zero sessions; foreign client issuer refuses with no-trusted marker and zero sessions. Disposable CA/key/client material generated in memory; bounded public marker output only, raw engine logs remain private fixture. Initial negative assertion incorrectly treated govici failed-response error as transport failure; fixed to require explicit success=no and certificate evidence. Updated existing MSCHAPv2 fixture rerun PASS1.871s. All own daemons/namespace mounts cleaned. TLS acceptance no longer deferred; production lifecycle/read-only preflight/packaging/full independent security review remain unfinished. Exact next command: implement SealedPreparation.Preflight read-only actual engine ABI/source receipt/helper/unit/cache checks and package stage integration.

## Actual read-only installation readiness
SealedPreparation.Preflight now requires a real opened-cache Readiness proof and verifies root-protected fixed installation: pinned authenticated strongSwan source receipt, Ubuntu26.04 amd64 ABI/runtime dependency exact match, exact guarded unit digest, executable helper with packaged SHA receipt, engine ELF binaries and required EAP/PKI/kernel/VICI plugins. It reads files only; no daemon exec, unit activation, network mutation or plaintext resolution. Actual owned artifact readiness plus absent-runtime/missing-cache/foreign-unit/foreign-helper negatives PASS0.495s. Source-unit hash drift is refused. Minimal P14 prepare hunks compile/stage helper+SHA and template unit without activation, with agent install manifest; actual staging fixture3testsPASS1.608s. Renderer now clears resolved EAP/RADIUS byte copies (focused TestRA PASS0.077s). Separate offline engine package driver drafted, not yet accepted, and main integration must combine hardening/upgrade prepare/test additions sequentially. Next command: guarded stale VPP receipt retirement with complete old-process-death and all-owner TAP absence proofs, then offline artifact package test and bounded packet/restart/rollback security campaign.

## Guarded VPP boot repair
Existing root TAP receipt may be retired only after complete old boot identity is positively dead/reused or kernel rebooted, exact namespace pair/endpoint matches, all-owner raw TAP dump proves numeric ID/namespace-link/old TAP index absent, and complete current boot/namespace remain unchanged. Owner-filtered Retrieve alone is insufficient. Recovery removes root receipt only, never deletes another VPP interface; same-boot/pending/unknown/crash ambiguity stays refused. Registration supplies raw typed API absence proof and root proc identity proof. Meaningful guarded lifecycle/all-owner collision/recycled-index/live-process/unknown identity/boot-race/namespace-race tests PASS ravpn0.102s, subsystems registration0.302s. Production controller separately owns stop-before-repair; actual controller restart/rollback packet campaign remains pending worker integration. Next command: bounded ESP-only capture and bad-password negative in private packet fixture, plus offline engine package validation.

Recovery checkpoint: full production Service.Apply fixture is now executable in a private VPP/network/mount/PID rig. First attempts fail at normal foundation Retrieve before RA activation because the bounded private VPP fixture omitted af_packet then dhcp API plugins. Added explicit private fixture plugin dependencies; no product/host service changes. Full lifecycle, EAP via production projection, restart under30s and rollback conn/pool readback remain unfinished. New offline package-engine.py is a draft and has no packaging PASS yet. Guarded TAP/transit maximum is conservatively8191, aligned with manager/private VPP boundary; controller worker owns corresponding EngineSpec limit. Current remote before checkpoint05101e06d291ceec41b9c094dd08054936ff62c5. Exact next command: run NGFW_RA_TEST_MODE=production isolated-vpp.py under tools/heavy.sh and shared lab flock with existing v3 artifact and owned RAM cache; merge paired controller be606 before subsequent lifecycle validation.
