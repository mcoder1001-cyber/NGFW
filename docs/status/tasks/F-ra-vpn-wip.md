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
