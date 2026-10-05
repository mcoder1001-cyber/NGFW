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
