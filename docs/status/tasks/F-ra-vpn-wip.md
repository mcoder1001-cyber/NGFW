# F-ra-vpn WIP
Branch codex/ready-f-ra-vpn-20261005, base7b507db5, remote checkpoint pending.
Completed: read current native-only route-based decision and obsolete RA prompt; inspected pinned VPP source/generated APIs. Auth methods only RSA_SIG and SHARED_KEY_MIC, no EAP/profile pools/virtual-IP API; NGFW native projection only fixed peers. Existing remoteAccess branch merely emits unsupported warning, allowing silent no-op risk.
Remaining: enabled-profile failclosed schema+agent; authenticated honest capability API/UI; disabled draft handling; exact blocker/options PENDING; tests, unchanged quick gate, PR and independent review. Full RA transport/auth/pool/session functionality cannot be delivered on the approved engine today.
Owned: current ready envelope; no host state changes.
Actual tests: none for RA yet. Current failure: missing approved native EAP/address-assignment path; engineering/source blocker, not deferred laboratory acceptance.
Next command: implement semantic/ra-vpn.ts and desired/ra_vpn.go failclosed gates.

2026-10-05 owner decision: independent strongSwan RA engine authorized; DEC-independent-ra-vpn replaces pending proposal. Boundary: per-profile private namespace/charon/XFRM plus outer/inner VPP TAPs, explicit transit addresses and selected VRF/policy handoff; native S2S unaffected. Existing enabled-profile refusal remains until operational engine verified. Next command: commit additive transit contract and tests, then implement secure renderer/runtime descriptors and RPCs. RA is not delivered.

Contract checkpoint: RemoteAccessProfile.transport field17 explicit outer/inner point-to-point pairs plus optional IPv6 inner pair; generated Go/TS stubs. Schema validators reject wrong families/subnets/same endpoints, duplicate ownership, interface/client pool/transit overlaps, native listener collision, IPv6 pool without IPv6 handoff. Only explicitly routed transport skips historical interface-owned local-address rule. Runtime remains failclosed. Focused7 tests PASS5.75s; buf lint PASS; proto regeneration PASS. Next: implement standalone secure RA renderer+descriptor lifecycle, namespace and VPP TAP/routes, actual VICI readback/actions. API/UI drafts not complete.

Standalone private RA renderer implemented (not wired/operational): fresh
kernel-netlink plugin configuration, private VICI socket, route installation
explicitly off; IKEv2 EAP/user pools/DNS/split selectors/XFRM IDs/DPD/rekey;
strict EAP-TLS/pubkey revocation trust and bounded numeric RADIUS sources.
Sealed resolver values appear only in private file bytes; fmt/JSON serializers
redact all file content, resolver failure details never propagate. Three focused
Go renderer tests PASS0.081s. Existing S2S renderer untouched. Runtime ownership,
PKI snapshots/CRLs, namespace/TAP/routes/readback/restart/rollback/session actions,
packaging/API/UI and real packet acceptance remain to implement. Next command:
implement RA runtime descriptor and bounded VICI observation using private roots;
activation guard must remain until engine verified.

Bounded private VICI observation/action source added: exact singleton connection
ownership,1024-event buffer plus stable before/after stats to detect dropped
listings, pool VIP and XFRM ID validation, public-only identity/counters, exact
uint64 JSON strings, generation-derived opaque session IDs and owned termination
followed by readback. Unix dial checks root-only parent/socket mode and SO_PEERCRED
exact managed PID, with a deadline that bounds context-free subscription too.
Seven focused RA Go tests PASS (latest runtime listed in own disk log); includes
200-session listing beyond govici default buffer, incomplete/foreign/stale refusal
and real private Unix socket mode/PID checks. No operational engine wired yet.
Private privilege/route design documented: constrained unit uses no SYS_ADMIN;
IKE/ESP mark1 selects owned underlay routing table, decrypted traffic traverses
VPP inner VRF; full-tunnel routes cannot loop outer crypto into protected path.
Next: fixed namespace helper/unit, secure lifecycle/PKI staging, TAP/routes and
scheduler descriptor; then API/proto/UI and disposable interoperable acceptance.

Additional contract18: explicit protected TAP accessPolicy ingress/egress lists,
required for activation; missing/duplicate ACL refs and empty sides refused.
Reserved field allocation documented. This closes the default-permit/new TAP
policy gap before consumers. Schema9 focused tests PASS5.10s; proto regeneration
and buf lint PASS. Runtime network plan source is unfinished and not activated.
Next: publish contract18, then namespace helper/unit/firewall plus TAP dependency
wrappers and daemon descriptor, preserving generic S2S and DF-1 ownership.

Network-plan source checkpoint: internal/ra_vpn owns typed, bounded public helper
input, explicit namespace outer/inner routing with mark1/table100, XFRM pool
routes and RADIUS host routes preferring the dedicated public source address.
Private nft rules drop client→outer/lo bypass, require pool-source xfrm→inner and
pool-destination inner→xfrm, explicit crypto/RADIUS output and root-owned table
comment. Only fixed network sysctls are produced. Four plan tests PASS0.066s with
NGFW_INTEGRATION=1: real nft --check in an unnamed disposable network namespace,
nf_tables already loaded, no host table installation or global ruleset flush.
This verifies syntax/plan guards; it does not prove EAP or forwarding. Namespace
helper/lifecycle/PKI/VPP handoff and API/UI still incomplete. Next: implement fixed
root helper with namespace inode/capability checks, constrained unit, descriptor
ordering/readback and disposable real daemon acceptance. Current RA remote84050494
contract18; local plan checkpoint publication next.

2026-10-05 helper boundary checkpoint: strict bounded network.json decoding,
unknown/trailing/foreign instance rejection, nonzero namespace identity;
O_NOFOLLOW root-owned parent/manifest walk, private0700 instance/0600 manifest,
regular single-link manifest, held NSFS binding matches helper namespace and
excludes PID1 host network namespace before any planned mutation. Two boundary
regressions and focused RA renderer/VICI tests PASS (packages0.027s/0.080s).
Removed nonexistent tls plugin (eap-tls links libtls). No helper execution,
daemon, sysctl or host mutation performed; lifecycle remains incomplete.
Official Debian security tracker currently lists bookworm5.9.8-5+deb12u5 vulnerable
to CVE-2026-78134 (EAP access control) and other issues. Upstream6.1.0 fixes these;
patched authenticated engine fixture/appliance packaging must precede acceptance.
Sources: https://security-tracker.debian.org/tracker/source-package/strongswan
https://www.strongswan.org/blog/2026/09/07/strongswan-6.1.0-released.html
Current parent remote d8afac81. Next command: implement fixed namespace executor
and capability guard, obtain authenticated6.1.0 fixture without host installation.

Capability checkpoint: helper refuses effective/permitted/bounding/ambient/
inheritable capabilities outside NET_ADMIN, NET_BIND_SERVICE, IPC_LOCK and
requires network administration+UDP low-port binding. Root/SYS_ADMIN caller is
refused before manifest read. Three boundary tests PASS0.024s via heavy semaphore.
Upstream6.1.0 source signature verified using release fingerprint
948F158A4E76A27BF3D07532DF42C170B34DBA77 (official download page keyid pinned).
Source SHA256 fe6c97481298767213cfc2e9a1da29fdd8018d481ff4cb9cf0283099654f20d4.
Authenticated source extracted ONLY /root/.cache/t19/strongswan-source;
configure finite process ongoing; no shared package/service installation.
Current remote0b352da0. Next: finish private source build then constrained executor.

Namespace executor checkpoint: only fixed ip/nft argv with10s timeout and1MiB
bounded private output; requires exact helper NSFS/capability check, two existing
transit links, refuses foreign links/XFRM identity, duplicate/foreign marked
priority100 rules and nft tables; nft syntax-check before private transaction.
Only fixed net/ sysctl paths permitted. Readback ownership regressions PASS,
whole ravpn package PASS0.026s. Not invoked against host or daemon; runtime
integration/readback of addresses/routes/ACLs still required before activation.
Current remote fa7d6613. Next: root helper executable/unit + descriptor lifecycle;
private authenticated6.1.0 build running finite heavy session32413 (no services).

Fixed helper/unit checkpoint: ngfw-ra-daemon takes only64hex instance, verifies
namespace/capabilities before fixed network setup, validates root-private daemon
config and execs fixed /opt/ngfw-ra/libexec/ipsec/charon with closed environment.
Dedicated template NetworkNamespacePath, bounding NET_ADMIN/BIND_SERVICE/IPC_LOCK,
no SYS_ADMIN, no namespace creation, root0700 runtime only and net namespace
sysctl exception; no automatic install activation. Focused ravpn+cmd compilation
PASS0.028s. systemd-analyze verify parsed unit but exit1 because appliance-only
/usr/lib/ngfw/ngfw-ra-daemon absent on shared host; no host install attempted.
Actual disposable systemd enforcement remains acceptance work.
Private authenticated6.1.0 make/install exited0; private charon --version reports
strongSwan6.1.0. Source/config/build/install logs owned /root/.cache/t19 only.
Current remote f6f4187d. Next: network/PKI lifecycle descriptors, real private daemon
startup/crypto/forwarding/ACL fixtures; runtime integration still incomplete.

Namespace lifecycle checkpoint: fixed /usr/bin/unshare --net child binds its
network namespace to exclusive root-private instance, persists observed inode;
agent process namespace never changes. Root-only guarded parent creation,
foreign inode and host namespace removal refused; held binding identity pinned
through owned-only detach/cleanup. Actual integration under shared lab lock PASS
0.049s, agent namespace inode unchanged, private mount+manifest removed.
Initial thread-based creation failed actual agent namespace identity assertion;
replaced with short-lived subprocess (never unshare a Go thread). Held namespace
fd initially made normal umount EBUSY; exact verified binding detach fixes cleanup.
Failed fixture mounts explicitly inode-verified and cleaned; no owned mounts remain.
No VPP, shared namespaces/services/sysctls touched. Descriptor restart/recovery,
PKI/VICI activation and packet acceptance remain incomplete. Current remote0919fcf0.
Next: persist/readback namespace descriptors then TAP/ACL dependency handoff.

Patched engine packaging checkpoint: additive deploy/ra-vpn/build-engine.sh pins
signed source6.1.0 SHA256 and public release key fingerprint; new offline root
only, heavy semaphore, build dependencies preflight, DESTDIR install beneath
/opt/ngfw-ra, no host ldconfig/unit activation. Build on appliance Debian12 ABI;
shared Ubuntu fixture binaries must never ship in appliance. shellcheck PASS.
Use charon-systemd instead of plain charon: plaincharon compiled globalpidfile
would collide across independent profile processes. Fixed daemon exec/config
namespace nowcharon-systemd, no inherited systemd environment. Focused Go tests
PASS ravpn0.034s/strongswan0.089s/cmdcompilation. Own fixture reconfigure needed
explicit private --with-systemdsystemunitdir; corrected builder accordingly.
Private source currently finishing systemd configure; retain signedsource receipt
and remove own build intermediates after final private install due disk pressure.
Current remote7bfaf4e2. Next: private systemddaemon startup fixture, descriptor
readback/dependency/PKI/VICI lifecycle; RA not operationally delivered yet.

Ownership contract checkpoint: public helper plan now records explicit bounded
owner/profile and must hash exactly to instance. This permits restart filtering
without adopting another owner's namespace. Native/session secrets remain absent.
Focused ravpn tests PASS0.018s; source namespace consumer tests underlablock PASS
0.061s; publish contract before descriptor consumer. Current remoted66634f5.
Next: publish namespace descriptor recovery and actual rollback evidence.

INCIDENT 2026-10-05T08:27Z (host privilege boundary, disclosed to manager):
I invoked `/root/.cache/t19/ra-engine/sbin/charon-systemd --version` assuming
plaincharon version CLI. charon-systemd ignored argument and started on current
host network namespace. No profiles/secrets supplied; process startup is still
an unauthorized shared-host daemon invocation. Identified owned PID2211170,
PPID2211149, exact cmdline checked in /proc before SIGTERM. PID stopped, no private
engine descendants remaining. Journal confirms startup6.1.0 and SIGTERMshutdown.
Readonly afterstate: rootns UDP500/4500 listener count0; host XFRM state/policy
EMPTY (captured in memory, no key material printed); VPP MainPID1014/NRestarts0.
/run/charon.vici socket root:root660 inode15570 remains, ctime1791188837547932031ns;
ownfirstjournal1791188837496142us (51ms earlier). Likely incident-created but no
before-baseline; no deletion made because ownership not yet independently proven.
Manager checking readonly metadata; no foreign service/state cleanup authorized.
Do not use any daemon CLI flags for version probing; use pinned source/build
manifest or actual startup only inside verified dedicated disposable namespace.
Private install exited0. Removed ONLY own extracted146MB build-only source tree
using exact path/symlink/receipt guards; signedsource tar/key/signature, prefix
and buildlogs retained. No shared cache cleared. Current remote301de89b ownership
contract. Next: add namespace-equals-host/refused-start/noexec regressions and
publish descriptor recovery; never invoke daemon on current host namespace.

Post-incident safety checkpoint: pure binding identity regression explicitly
refuses binding=current=host, expected private but current=host (unshare failed),
and foreign inode. Helper startup control test proves boundary refusal prevents
config access/daemonexec; --version is rejected as non-instance before any step.
Production helper still uses closed configure/validate/exec functions and fixed
engine path; test injection is local only. Actual focused packages PASS0.030s
ravpn/0.026s cmd under heavy semaphore. Namespace consumer actual restart recovery,
foreignowner filtering and ownedrollback tests PASS0.082s underlablock, consumer
publication next. Current remote49093612 incident report; daemon remains stopped.
Next: publish namespace descriptor; private fixture startup only after NSidentity
verified and dropped capabilities, no daemon CLI version probe.

Namespace descriptor checkpoint: explicit owner-filtered StageVPP public plan
object, runtime inode in Meta only, actual NSFS binding dump on fresh restart,
recreate semantics for changed plan, owned reverse cleanup. Unknown/foreign
inputs refused; uncertain partial creation retains inode/PartialCreate+uncertain
marker rather than claim rollback. Actual namespace create→freshRetrieve→Delete
and foreign-owner isolation PASS; cancelled namespace child creation leaves no
binding/rootdir. Full focused integration underlablock PASS ravpn0.081s and helper
noexec0.030s, PID1/agent namespace unchanged, no private mounts left. Descriptor
not registered/activated until TAP/ACL/daemon lifecycle ordering is implemented.
Own verified-build-receipt.json in /root/.cache/t19/strongswan-source records signed
source6.1.0, systemd install exit0, fixture ABI and forbidden daemonflagprobe.
Current remote6f53e73d. Next: guarded TAP wrapper + public projection dependencies,
PKI/VICI lifecycle and full API/UI/session/packet acceptance still incomplete.

Incident residue closure (manager independent verification): root confirmed
socketinode15570/root660/ctime1791188837547932031, ownPID2211170 VICI plugin journal
1791188837549941us (~2ms after socketcreation), no live UNIXlistener. Manager
rechecked unchanged inode/ctime and unlinked ONLY proven incident-created inactive
/run/charon.vici. It is now absent. Root independently verified hostXFRMstate0/
policy0, UDP500/4500listener0, incidentPID+parent absent, VPP1014/NRestarts0active.
Incident closed with actual disclosed host invocation; never claim it never ran.

ABI correction: actual P14 deploy/image/iso/common/packages.list and current
install{iso,bare-metal} docs specify Ubuntu26.04 resolute amd64. Earlier Debian12
assumption was wrong. Builder now requires explicit targetOSrelease, refuses
ID/VERSION_ID/amd64 mismatch before creating output, emits compiler/libc/OpenSSL/
systemd ABI JSON manifest and upstream COPYING. shellcheck PASS; build/source
signature receipts remain valid fixture evidence, not appliance artifact acceptance.
Installation consumer must compare target ABI manifest before staging artifact.

Handoff contract checkpoint: root-private observed metadata records kernelNSinode
and complete D080 VPP bootID/mainPID/startTime plus both fresh owner-tagged TAP
indices/names. Refuses every boottriple change, incomplete identity, namespace
replacement, recycledindex, foreignlink. Focused actual TestHandoff PASS0.026s.
This contract will precede TAP/ACL/daemon consumers; actual ACL readback still
mandatory before activation. Current remote160b00d9 namespace descriptor; next:
publish handoff contract then guarded TAP wrapper and artifact ABI stage verifier.

Host binding contract checkpoint: runtime plan/handoff additionally records
trusted-agent hostNamespaceInode; handoff requires private!=host and exactpair.
Concrete lowcap metadata probe exposed /proc/1/ns/net returning masked non-NSFS
identity (mode040511/dev26/inode14523) instead of rootfullNSFS (dev5/inode4026531833).
Never use a low-cap Proc1stat as authoritative host identity. Upcoming consumer
captures root-owned NSFS hostnetns binding before creating distinctprivateNS,
helper validates both heldNSFS identities plus current=private; namespace handling
never calls setns/unshare insideGo. Pure handoff regression PASS0.040s;
consumer actual lowcap hostrefusal/privateacceptance +namespace restart/cleanup
underlablock PASS0.249s (not yet this contract-only remote tree). Contract publishes
first; consumer follows. No daemon invocation, no leftoverprivate mounts.
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

Actual first production activation milestone PASS3.098s: full Service.Apply normal foundation then RA desired projection, real scoped Wiring/TAP/routes/ACL, sealed preparation, private daemon, real empty VICI session readback. Explicit fixture plugins supply all normal-domain APIs. Full EAP packet/restart/rollback still pending. Connector publication b311 accidentally included tool-output truncation in package-engine.py; exact archive source restored immediately, no packaging execution or PASS was claimed. Per-file publication and tree equality checks required for all following checkpoints.
The same publication integrity audit found earlier8b2 wire_capture_test.go contained truncation text. Restored exact reviewed/tested archive/ra-local-e71406db4 source; remote lint was legitimately failed by publication damage. Source unfrozen; require exact local/remote Git tree equality before ref updates, not merely successful connector API calls.

## Bounded wire encryption and negative password proof
Actual private VPP campaign PASS12.246s: header-only AF_PACKET capture opens only after exact owned network inode proof, records ESP/NAT-T ESP both directions RX2/TX4 with zero plaintext ICMP during protected allow-reply and ACL-denied traffic. No packet payload bytes printed/persisted; only bounded public counters root0600 evidence. Real incorrect MSCHAPv2 password refused with exact EAP-method-failed engine marker and zero sessions PASS1.867s. Longer capture exposed fixture reuse of deliberately10s-bounded VICI connection; after immutable private PID/start/NS verification fixture now redials fresh before disconnect, matching production per-operation connection contract. No shared capture/network/daemon; all private processes cleaned.
Production controller68cad merged and published d720; sealed read-only Validate9ca proves valid credentials/renderer before namespace exists and invalid private key rejected without creating namespace/files (actual0.151s). Full productionService.Apply fixture ownership now exclusive NEW agent/ra_vpn_controller_integration_test.go to exercise unexported actualprojectOwned; controllerworker product files remain separate. Actual full desired/Wiring/normal preservation/restart/rollback integration remains next and cannot be lab-deferred. Offline package-engine.py draft still uncommitted/untested. Exact next command: merge controller validator4388172, then construct private real Service/Wiring/supervisor fixture using authenticated owned artifact and cache.

Full checkpoint-tree audit and staging restoration are recorded in F-ra-vpn-publication-integrity.md. Three damaged product paths restored exactly; current cumulative prepare/test preserve all MAIN hardening/A/B assertions plus original RA additions. Initial cumulative test3errors because this old branch lacks deploy/hardening dependency source; manager authorizes actual MAIN merge next, no assertions waived. Added typed PreparedEngine.Unload adapter: exact private conn/pool ownership, no foreign/malformed mutation, actual empty readback after unload. Mock negative regressions PASS0.067s; real two-namespace EAP/VIP/disconnect/connection+pool empty rollback PASS1.904s, no shared daemon. Added Ubuntu26.04 ABI/authenticated package and materialized CA CRL prerequisites to user guide. Remaining: MAIN dependency merge, full production packet/restart/rollback, packaging and full owned lint/security gates. Exact next command: merge actual MAIN bb785c4 after published checkpoint, resolve only owned cumulative prepare/test, and rerun3 staging tests.

Current MAINbb785 union: normal pnpm gen PASS3m00 and CLI normal generation PASS; all generated conflicts regenerated, product registration conflict retained both RA and MAIN hooks. Cumulative hardening/A/B/RA staging3testsPASS1.873s. Offline package regression16testsPASS0.189s (includes reused ABI staging cases; synthetic executable payload tests package control/contents only). Real runtime artifact packaging still next. Cross-mount production fixture now models a distinct agent private mount namespace: actual RA Apply fails TAP creation (-13) PASS exposing deployment bug, no daemon activation, cleanup completed. Same-mount3.098s activation milestone remains accurately scoped. Fixed global NSFS broker design is authorized; must expose held namespace descriptors only in verified manager AND VPP mount namespaces, no capability/hardening/propagation weakening. Full source remains unfinished.

Public handoff/mutation contract published a936883cb6ab6872af97fd4d45fe386e7f0c95da after MAINunion7b292 exact-tree publication. Helper namespace/TAP callbacks now guard before mutations; namespace exports verify at Create/Retrieve and remove before local bindings only after quiesce. Guarded TAP failed-stop regression retains rows and root receipts PASS0.056s; existing guarded/namespace unit regressionsPASS0.055s. Actual broker not yet implemented; callbacks alone do not prove production handoff. Next: fixed FD-only namespace broker and persistent multi-target export receipts, then paired controller648 source and cross-mount real lifecycle campaign.

Broker development checkpoint (UNFINISHED): actual exact-agent-cap private Setns failsEPERM despite successful FD-open; CLONE_FS was unshared. Exact brokerSYS_ADMIN|SYS_CHROOT cannot reopen sourceagent3caps /proc/PID/fd/N (permission refused). These invalidate both inherited-agent execution and proc-FD manager draft; neither is operational/accepted. Executable test_broker_caps.py PASS0.537s instead proves bounded root peer SCM_RIGHTS of exactly3 typed/inodechecked NSFS descriptors, deniedprocFD access, private-target actualSetns with CLONE_FS unshared, exactagent3/broker2 caps and NoNewPrivs, with complete own cleanup. Current managerprocFD draft is explicitly unfinished and must be replaced by fixed root600 SOCK_SEQPACKET socket activation receiving authenticatedSCM_RIGHTS. No agent/daemon bounds increased. Draft broker library compiles (helper no tests, guarded regressionPASS0.048s), but this does NOT prove operational manager handoff. Remaining all original fullproduction packet/restart/rollback/lint/security work; sixotheroriginal tasks nowactuallymerged by manager. Exact next command: replace namespace_manager.go transport with socket-activated fixed helper, add protected service/socket artifact/read-only readiness, then run actual broker and cross-mount fullService fixture.

2026-10-05 recovery/SCM consumer checkpoint: rejected whole-controller9517
merge627d2dc06 archived (archive/ra-rejected-controller-merge-627d2dc06), never
published: stale controller broader tree inferred deletions of MAIN/BFD files.
Own source reset to exact published8ac; paired consumer integration will use
owned paths and original assembly baseline1495 only. FD contract published
db0aee83 (local5b0eed1a4, exact connector tree equality proved).
Replaced invalid proc-FD reopening manager transport with bounded root-authenticated
SOCK_SEQPACKET SCM_RIGHTS. Exactly three NSFS descriptors are closed on all parsed
refusal paths, including oversized/truncated messages. Added fixed socket-activated
broker unit with only SYS_ADMIN/SYS_CHROOT and no agent/daemon cap increases.
Actual Go targeted guard+malformed-rights/leak regressions PASS0.087s; preceding
exact-cap Python SCM/setns fixture PASS0.537s. These are boundary/component evidence,
NOT full product broker cross-mount/packet/recovery proof. Source unfinished.
Remaining: actual Go broker under exact caps in private manager/VPP roles, durable
partial-export/recovery/removal semantics, stop-first explicit handoff repair,
scoped latest controller/APIecc2 integration, full Service.Apply cross-mount
packet/restart/rollback, packaging/install readiness, owned lint/full gates.
Next command: compile broker into owned RAM; extend private PID/mount fixture to
exercise socket-activated Go helper and exact role readback, without host activation.

2026-10-05 static activation contract correction: removed historical blanket
semantic rejection of every enabled RA profile. Supported static profiles now
reach the agent's authoritative readonly credential/handoff/installation preflight;
this does not bypass runtime guards or advertise installed readiness. Enabled
EAP-TLS/pubkey clientCa names must be <=59 so cert/<name>.crl fits existing63-char
sealed reference grammar; disabled drafts retain63-char editability. Actual schema
10tests PASS5.76s, including59/60/63 boundaries and password-profile nonrefusal.
Root requests actual API400 pre-mutation probe after final source integration.
Current broker failure remains concrete: exact cap2 Go target-current typed inode
check fails. Expanded agent cap3 /proc/1/ns/mnt open returns non-NSFS FD (NS_GET_NSTYPE
ENOTTY); nonzero/open success was insufficient earlier. No false actual broker
PASS. Manager pre-cap-drop FD source required, no capability increase. Unpublished
owned partial-cleanup fixes/finite actual regression remain under development.
Next command: implement fixed socket service OpenFile manager namespace FD and
bounded authenticated manager-target response, with typed NSFS proof and explicit
unsupported custom VPP mount namespace readiness refusal; commit contract first.

2026-10-05 numeric manager-opened target draft checkpoint: fixed root-only
ngfw-ra-targets@PID socket/service, manager OpenFile=/proc/%i/ns/mnt and
/proc/1/ns/mnt before cap drop. SystemdNamespaceTargets.ExpectedVPP callback
must be actual connected VPP boot identity; Acquire only connects an already
provisioned socket, no mkdir/systemctl start. It verifies exact installed template
hashes/actual manager configuration, root socket peer, canonical VPP unit and
full boot, two held typed NSFS FDs, provider exact cap2/NNP/full identity and
manager-attested MainPID before ACK. One-request helper has finite deadlines.
Actual compile/guard/malformed-rights tests PASS0.085. Updated standalone exact
cap regression PASS0.601 with E/Prm/Bnd exact3/2, Inh/Amb0 andNNP1; formerly
inherited/ambient3 proof was bounded prerequisite, NOT exact installed-unit proof.
Proc namespace observations can return non-NSFS identities/FDs rather than errno;
new regression proves nonzero/open success must not be accepted. Actual Go
partial-removal/cross-root test currently FAIL at target-current (expected typed
proc failure), not positive; consumer must switch to fresh manager-attested proof.
Partial remover now idempotently skips only root0600empty single-link placeholders,
uses MNT_DETACH only for exact owned NSFS, and verifies no NSFS remains. Original
placeholder identity/replacement protection and durable role recovery remain.
Source still unfinished; no broker/RA operational or overall acceptance claim.
Next command: wire FixedNamespaceHandoff.Provider=SystemdNamespaceTargets with
held-FD acquisition before/after each export; replace broker current-target proc
check with independently authenticated fresh manager FD snapshots; exercise real
private numeric provider and Go broker under exact caps before packaging/ready.

Checkpoint: scoped paired controller consumers through local acde0a11a; preserved MAIN/BFD assembly hunks via three-way RA-only patches. Integrated independently approved renderer a73 delta (R1/R2 receipt d0fcf811). Whole ravpn tests PASS 0.219s and, after typed manager constructor, PASS 0.247s. Added pure held-provider constructor contract only; consumers still unfinished and fail closed. No full private Service.Apply/cross-mount readiness claim. Current provider rejects broker cap2 by design; a fixed-unit authenticated broker client role and fresh manager role snapshots must replace proc namespace checks before positive acceptance. Next command: implement held-provider pre/post dispatch observations and fixed-role authenticated supplier proof, then replay private broker partial removal fixture.

Broker role checkpoint: explicit internal agent/broker observation request role. Broker clients require exact cap2/NNP plus a bounded fixed broker cgroup/unit, actual systemd MainPID/ControlGroup, installed fixed fragment/argv, no dropins, and full process boot recheck. No general root/cap2 admission. This is source authentication only; actual positive supplier/role fixture and authoritative current-MNT proof remain unfinished. Whole ravpn tests PASS 0.289s; placeholder Fstat/Close failures now retain uncertain alias and prevent mount. Current durable prior remote c043d61a9d692a830a8d29b9b9d356b357582936 exact local d8c0a1439/tree7ff5f8ac. Next command: replace broker current-MNT proc check with fresh authenticated manager-held target snapshots and replay private partial removal fixture.

Contract-first checkpoint: NamespaceBrokerAttestedFDDispatch adds exactly FOUR typed roles target MNT/host NET/private NET/source self MNT. Source acquired by agent internally, never arbitrary PID/path; missing/extra/repeated/wrong typed FDs refused. Consumers not yet switched; legacy three-FD protocol remains archived in source and fail-closed readiness retained. Parent explicitly authorized bounded correction and fresh R2/R4/R8 review. Root owns NEW observer supplier/templates; author owns agent-side fixed activation/bootstrap and packaging. Daemon /run isolation prevents daemon-side management; bootstrap proposal corrected to agent-side explicit ObserverActivation after canonical unit Start, before first observation, not Preflight. Next command: implement four-FD consumers with manager-fresh pre/post target proof, then actual private broker/manager fixture.

Source-authentication contract checkpoint: fixed OpenFile role source-agent-exe, reference /run/ngfw/ra/source-agent-exe and record /run/ngfw/ra/source-agent.json. Strict held-record fstat/private-mode/single-link/root-owner checks and exact source /proc/PID/exe reference plus full boot binding; manager-held EXE must independently match fixed protected /usr/sbin/ngfw-agent inode. Shared verifyFixedAgentPeer authenticates canonical root:ngfw unit, exact cap3/NNP/group credentials, installed agent fragment and exact known hardening drop-in, MainPID/cgroup/argv/full boot; unit metadata is explicitly NOT executable attestation. Provisioning/bootstrap and manager role consumers still unfinished. Real agent gid uncovered prior root:root assumptions: root-owned mode0600 placeholders now accept inaccessible nonzero group without CAP_CHOWN, meaningful 0640 refusal test. Reference test initially rejected /dev/shm writable ancestor then incorrectly reused per-instance validator; corrected private /root fixture + same held-file checks for global reference, no production guard removed. Whole ravpn tests recorded after correction; full actual source role/manager/Service.Apply proof remains mandatory. Next command: publish contract, extend source-agent-exe manager role consumers + four-FD broker current-target snapshots.

Snapshot group correction checkpoint: protected root-owned snapshot files/receipts accept nonzero group only with exact private 0600/0700 permissions, supporting actual root:ngfw agent without CAP_CHOWN. Daemon VICI socket root:root checks remain. Meaningful private root fixture proves gid65534 accepted and 0640/0710 rejected. Whole ravpn tests PASS0.223s. Full manager/source-generation/Service.Apply acceptance remains unfinished. Parent owns lint-only plan.go, boundary.go, installation.go, sealed_preparation.go, wire_capture_test.go from local31879a22e until delta returned. P11 owns private guest infrastructure; no duplicate builder or host activation. Next command: implement atomic source reference generation reader/provisioning, then four-FD fresh target consumers.

Atomic source reference reader contract: fixed aliases resolve through source-agent-current to a full-identity SHA256 generation with root0700 directory, root0600 versioned ownership record and exact proc executable link. Reader captures one immutable generation and rechecks current pointer; generic links and nonprivate members refused. Bootstrap writer remains unfinished, so no deployment compatibility or operational claim. Targeted reference regression PASS0.062s; fixed alias/reference paths unchanged for observer OpenFile. Next command: implement serialized generation publisher/bootstrap and fresh target snapshot consumers.

Source generation publisher checkpoint: bounded context-aware exclusive lock, protected immutable versioned generation, exact owned aliases, fsynced identity and atomic current pointer; existing dead generation authenticated before replacement, foreign pointers/preexisting unreferenced nodes refused. Targeted initial publish/idempotence/foreign preservation +reference tests PASS0.071s. Interrupted unpublished generation recovery remains failclosed and unfinished. Actual guest systemd259 revealed OpenFile %i literal ENOENT202/FDS: production target/observer templates require fixed canonical numeric configuration, not waived. P11 preserves failure and owns guest mechanism proof; author owns targets/bootstrap, root observer owns its corresponding correction. Next command: implement fixed authenticated startup bootstrap and bounded numeric OpenFile dropin provisioning; wire four-FD fresh provider consumers.

Contract-first numeric OpenFile configuration: closed Targets/Observer kinds and publisher accepting only kind, fixed instance and complete target boot identity, no caller path/unit/content. Parent confirms exact numeric dropin correction; root owns observer deterministic renderer/validator/templates, author generic writer/targets/bootstrap. Actual guest %i failure remains unresolved until consumers+replay. Next command: implement protected deterministic numeric writer with matched reuse and known-gone/inactive refresh only, then guest replay.

Targets numeric OpenFile renderer contract: exact canonical UUID/PID/start ownership header, reset directive then literal verified VPP mount path, fixed manager mount and source-agent EXE role. Deterministic parse/validate rejects appended commands, altered roles and noncanonical PID/header injection. Targeted tests PASS0.054s. Template, bootstrap and supplier consumers not changed yet, actual guest production failure remains open. Next command: implement closed-kind protected runtime dropin publisher and provision canonical target listener before readonly preflight.

Scoped paired lint-only delta consumed from controller local01a629a052fd8b12464352a38ec9457266381307 / remote099186a26afc4ee732c90a803a937b08c946d4f9: ONLY agent_plan.go,tap_store.go,tap_store_test.go; no stale whole-tree merge. Bounded held nofollow plan read and receipt/lazy loads propagate close failure, construction refuses parent-close failure, temporary cleanup errors retained except expected renamed ENOENT. No suppression. Own cumulative whole ravpn tests PASS0.331s. Remaining helper lint103 at paired source, root has separate5file batch; bootstrap/manager full acceptance unfinished. Next command: implement numeric configuration writer using root observer render contract and target renderer; consume fresh target/observer role proofs before private guest replay.

Numeric writer checkpoint: FixedNumericOpenFilePublisher implements closed Targets/Observer kind mapping to exact deterministic renderers (root observer pure contract08d49 scoped-consumed). Canonical fixed target unit/fullboot verified before and after; first install uses absent-destination hardlink to avoid foreign overwrite, fsync and single-link final verification; identical reuse preserves inode, unknown content/private-mode/link drift refused. Targeted config/writer tests PASS0.619s. Known-gone/inactive old-generation refresh and executable bootstrap remain unfinished failclosed; no actual production VM claim. Scoped root5file lint delta f226 consumed, whole ravpn previously PASS0.224s; source generation lock now finite5s even caller Background. Next command: implement known-gone/inactive deterministic refresh, fixed canonical startup/observation bootstrap and numeric supplier readback consumers, then guest positive full path.

Numeric refresh implementation: parses only deterministic owned old record, requires same derived numeric path and old full process generation proven gone, canonical current target recheck, actual fixed supplier MainPID/ControlPID zero inactive/failed and empty hierarchical cgroup.events before replacement. Private old file contents/inode/stat rechecked, no O_TRUNC; atomic rename/fsync. Filesystem replacement/stale expected contents/nonprivate preservation tests PASS0.078s; actual systemd old-generation refresh acceptance still pending guest. No default callback or host activation. Next command: wire fixed startup/observation helper bootstrap and strict numeric supplier template/dropin consumers.

Trusted inventory contract checkpoint: NamespaceDescriptor.Inventory uses paired NamespacePlanInventory(ctx,owner) only as explicitly injected Go fixture seam. Nil preserves all protected production filesystem/held NSFS checks; mutations unchanged. Callback observations bounded1024, own instance/plan/inode pair and duplicates checked, input cloned, optional handoff Verify still required. Exact empty mock inventory avoids reading shared host. Meaningful isolation/alias/duplicate tests recorded; paired controller contract.go consumed by owned path only, no stale assembly tree. Full helper/cmd compile tests PASS0.289s before new specific test. Source bootstrap draft now compiled but agent ProtectSystem=strict blocks runtime dropin writes outside/run/ngfw; manager-owned bounded provisioning needed, no agent hardening relaxation/host activation. Next command: publish inventory seam then coordinate manager provisioning and complete numeric supplier consumers.

Contract-first manager publisher: pure NewSystemdNumericOpenFilePublisher() NumericOpenFilePublisher with existing closed-kind API; explicit failclosed stub until authenticated manager transport complete. Canonical agent MainPID caller only, manager-held actual source EXE/fullboot/caps, zero arbitrary path/content, bounded manager-owned runtime config writes and fixed supplier socket activation. Parent approves correcting agent ProtectSystem EROFS without weakening agent caps/write allowances. NamespaceHandoffInitialization/NamespaceTargetInitialization optional Initialize(ctx) contracts explicit startup after global inactive barrier; Verify/Preflight/Acquire remain readonly. Parent root/controller own consumers after contract publication. Next command: implement manager publisher templates/server/client and source Initialize, then actual guest proof.

Restart contract correction: source-only InitializeSource(ctx) interfaces precede owned live-unit observations/global stop barrier; postbarrier Initialize remains supplier config/socket provisioning only. Source-only phase must prove own internally opened actual executable/canonical unit/fullboot, monotonically clear own inheritable/ambient on ALLGoOSthreads without altering effective/permitted/bounding, then publish immutable current reference so manager can independently capture current source EXE. Concrete all-thread normalization still pending. Actual original agent ELF+unit guest boot12 CONFIRMS InhSYSADMIN, othersoriginal3caps/Amb0/NNP1; strict validator remains. Correction to earlier static EROFS inference: guestboot9 first/run/systemd/system fixture write SUCCEEDEDEXIT22 and boot12/runrw/rootro; actualEROFS NOT established. Manager publisher remains approved boundedauthority architecture, not a claimed reproducedEROFSfix. Next command: implement all-thread source normalization+source phase, then concrete manager publisher transport and actual guest tests.

Source-only implementation checkpoint: canonical own unit/full boot/internally opened installed ELF precede all-thread monotonic Capset and ambient clearing; exact permitted/effective/bounding stay original3. AllThreadsSyscall fails closed in CGO builds, no thread-local fallback; all surviving task capability sets checked before/after. Source generation publication precedes live-unit observations; supplier provisioning remains separate and failclosed publisher stub. Pure capability parser regressions and cumulative helper/cmd tests PASS0.318s; actual original-unit guest normalization is NOT yet proven. Scoped root observer ee2c and paired unit supervisor9c consumed; --observe-unit fixed numeric mode wired. Targets supplier ownership handed to root branch codex/ra-target-supplier-20261005 at local6c1c3aff8/remote8ad4; do not edit target files until returned. Remaining manager publisher transport/service, broker four-FD export consumers, exact private guest full lifecycle and whole lint mandatory. Next command: implement fixed authenticated numeric publisher server/client/templates; supply compiled source normalization to private guest reviewer.

Manager publisher transport contract: finite probe/publish phases, canonical complete source/target/previous-server identity, closed numeric kind and instance, no paths/commands/content. Probe cannot carry target or rights; publish requires previous fresh source-executable capture. New fixed cap0/root:ngfw manager unit supplies source-agent-exe OpenFile plus fixed seqpacket listener; only /run/systemd/system writable, finite64starts/triggers. Contract refusal regressions PASS0.050s. Templates/server entry point not wired yet; transport remains failclosed. Next command: implement two-capture source attestation, exact installed manager/unit/socket proof and bounded publish server; then actual guest test.

Scoped source consumer split: namespace_bootstrap.go now contains source-only initialization and canonical self proof; postbarrier VPP supplier methods moved namespace_supplier_bootstrap.go so paired controller can consume source phase without stale/root-owned target dependency updates. Cumulative helper/cmd compile/test PASS0.445s before split (method code unchanged). Independent algorithm-only R1/R2 APPROVE d00395e onb716; actual original-unit allthread normalization NOTRUN. Next command: complete fixed publisher transport and compile/tests, then publish and guest replay.

Concrete manager publication transport: canonical-agent root/fullboot/exactcaps/private current source authenticated BEFORE accepting rights; probe captures manager-opened installed source EXE, waits exact server exit/inactive, publish captures fresh source EXE and compares prior held descriptor before deterministic writes. Root-authenticated PID1 socket, protected installed helper and exact publisher templates, actual service MainPID/fullboot/caps0/fixed cgroup/ELF verified; finite5s total. Manager invokes only deterministic closed-kind writer, fixed daemon-reload and derived supplier socket start with target pre/post. Fixed --publish-openfile entry point wired. Whole cumulative helper/cmd PASS0.332s; real private manager two-activation/first-start/publication acceptance NOT yet executed, whole lint pending. Missing installation fails closed; this is source progress, not operational RA completion. Next command: scoped review/actual guest publisher replay, wire read-only publisher installation prerequisites in paired suppliers, complete broker four-FD/exports and actual full production lifecycle.

Scoped paired snapshot lint delta consumed ONLY snapshot.go,snapshot_cleanup.go,snapshot_test.go,credentials.go from root localb4fb8508539e3312607b7e58441759958cb439bd/remoteb81d37d2; no stale tree merge. Root:ngfw/private UID/mode/nlink/inode/namespace/foreign guards preserved. Root wholeRA race1.946 and independent APP321b6da9/race2.031, changed paths zero lint; actual private VICI/snapshot fixture explicitly NOTRUN. Own cumulative replay follows new broker receiver checkpoint. Next command: finish four-role authenticated broker source proof/provider-held export consumer; run cumulative race/lint and private guest replay.

Four-role broker receiver implementation: archived production RunFDs(3) now refuses; new RunAttestedFDs(4) and receiver require exact typed targetMNT/hostNET/privateNET/sourceSELF_MNT. Canonical source authenticated before receiving rights, manager-opened fresh source-agent-exe independently validates installed caller image, full unit/caps/source reference rechecked before/after mutation. Broker root:ngfw cap2 fixed unit clears own inheritable/ambient on ALLGoOSthreads only after canonical accepted-unit/cgroup/MainPID/installed ELF proof; sourcecap3 boundary remains separate. Authenticated held-target operation replaces unavailable foreign proc-MNT stat only on this path; archived direct helper retains prior check. Central bounded SCM_RIGHTS receiver closes every unexpected right. Pure cgroup/capability refusal +cumulative helper/cmd PASS0.326s. Agent provider-held pre/post export consumer still unfinished, old caller3 fails closed; actual manager/caps/four-role guest acceptance NOTRUN. Next command: consume root target producer, update FixedHandoff to retain fresh manager snapshots across four-role dispatch/post-observation and safe partial receipts, then private guest lifecycle.

Scoped root target producer consumed ONLY eight published product/template paths +new namespace_targets_kernel_test.go from local2f994a5311b12dc92dca5f8dd30ae2276074d1b8/remoteb72016d8; root observer one-line publisher prerequisite included. Root keeps target ownership for guest fixes. Cap0/GIDngfw, manager-held MNT/VPP/source EXE roles, exact literal dropin, two fresh exited/distinct server captures; root pure/kernel races1.319/1.245 and independent APP26b7ef09. No actual target service activation claim. Scoped root four helper lint paths consumed from570ac92078b5640361b599686e0dbad3191262b0/ffc3 only; whole unchanged root race3.558/scopedlint0, no writable/error/ownership semantics weakened. Own cumulative helper/cmd PASS0.446s before final four-path copy. Manager reports independent original-unit actual SourceVM boot3 PASS: CGO0 agent with real VPP, before1thread InhSYSADMIN; after9OSthreads ALL Inh/Amb0 with original3 E/P/B and NNP1, immutable source published, QEMU EXIT0; raw log SHA74ff6de7a4f5cca367a0f9c7d922c5f8be82434a833f19fadecf3f038d857432. This is source-normalization acceptance only, not full RA completion. Next command: publish reviewed paired deltas, finish held export/partial recovery and actual manager/broker/profile campaign.

Held export and broker source-view checkpoint: FixedHandoff retains validated fresh manager target/source/image descriptors across exact four-role dispatch, captures fresh post-state and compares full identities before any later TAP/admin-up. Export mismatch compensation uses original held target, never new foreign target; pending ownership retained on failure. Broker restores held source mount namespace and proves typed source inode before canonical source postchecks, including operation failure. Exact direct OR canonical templated broker cgroup accepted; foreign nesting refused. Publisher PID/GID conversion bounds checked and fixed service resource limits pinned. Whole cumulative ravpn race PASS2.558s and broker command compiles. Actual private manager/broker cross-root replay NOTRUN; per-target partial progress/placeholder birth identity, old-target retirement and explicit stopped ExportExistingRepair remain unfinished. Local parent2db4aaa46 / remote5cd6050d tree d4512d87; next command: implement durable partial export recovery and stopped repair, compile immutable guest helper for independent manager replay.

Scoped helper lint progress: checked namespace inventory read close before accepting ownership, represented four rights as explicit typed records with duplicate descriptor refusal, removed unused archived manager wrapper, documented descriptor/export contracts. No ownership/capability/readback weakening. Whole ravpn PASS1.954s after finite heavy queue; prior exact ee8 lint FAIL53 preserved in /root/w19-ra-lint-ee8ceb35e.log, fresh lint still required. CGO0 fixed helper ee8 build EXIT0 /root/w19-ra-broker-ee8ceb35e SHAfa6e3975c1208925bf2dbae38b31632bf19e2e451c4dcb0642ea4762d21b3d9b. Next command: durable birth-inode receipt contract and stopped export repair, actual immutable publisher/target guest replay.

Actual independent guest OLD6e boot6 exposed never-started publisher service inactive/dead/MainPID0/empty ControlGroup with installed exact template and active listening socket (bounded raw log SHA84b148b33849d54935ef398e76a6ee05e7a7d65e1574ac5b326d9b5dbb6262df). Narrow first-activation preflight correction permits empty cgroup ONLY without attested live identity and MainPID=ControlPID=0/inactive/dead; active process retains exact canonical cgroup/fullboot/image/caps proof. Meaningful regression refuses live identity, either nonzero PID, active/running or foreign cgroup; PASS0.070s. Whole lint attempt EXIT3 shared parallel-lint lock, not source PASS; no rule/config changes. Actual new-generation publisher activation still pending independent guest. Next command: parent coherent MAIN/controller/helper union CGO0 guest build and publisher replay; fixed birth-inode receipt contract for partial exports.

Placeholder birth ownership checkpoint: exclusive hostnetns/netns/alias regular-file Dev/Ino persisted in fixed private instance receipt BEFORE each bind obscures file identity. Receipt strict version/instance/closed role parsing, root private single-link read, fsynced exclusive staging/atomic rename. Broker accepts ordinary placeholders for export/remove retry only if exact original birth matches; foreign root600 replacements, hardlinks and symlinks refused. Source cleanup handles manager-already-detached shared bindings only by authenticated recorded birth, checks post-detach original before unlink, accepts already-absent role only with receipt. Whole ravpn race PASS2.319s before final private receipt read guard; exact final replacement/hardlink/symlink regression PASS0.063s. Actual private broker partial/remove campaign remains NOTRUN; unavailable old-target retirement and explicit stopped ExportExistingRepair still unfinished, no completion claim. Next command: immutable helper compile/actual private manager partial replay and stop-gated old export repair.
