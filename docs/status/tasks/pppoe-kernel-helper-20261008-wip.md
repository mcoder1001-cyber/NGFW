# PPP kernel carrier helper checkpoint

Branch: `codex/pppoe-kernel-helper-20261008`; initial base `db7eb9d4`.
Owned source: scripts/pppoe-kernel-carrier.py, its focused tests, inert assets in
scripts/pppoe-carrier-assets, packaging test loader, and this receipt. Parent owns
Go controller, renderer, VPP topology, readiness and package integration.

28 focused controls pass through the strict packaging discovery seam:
`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_pppoe_carrier.py -v`
(0.021 seconds). The controls use fake command executors and temporary files;
there are no skipped cases. No native namespace/service/route/packet test or CI
was run. These controls are source evidence, not laboratory acceptance.

## Namespace and forwarding contract

Token: `ngp-` plus first12 lowercasehex SHA256(UTF8(owner + NUL + logical)).
Raw/transit Linux names: `pw`/`pt` plus token's12hex; PPP remains namespace-local
ppp0. Root-only ledger under /run/ngfw-pppoe-carrier records full owner/logical,
boot, random32hex generation, namespace dev/inode and immutable spec. Spec fields
are exactly owner, logical, parent, mtu, host4, peer4, host6, peer6. Host addresses
carry /30 IPv4 or /126 IPv6 prefix; peer addresses are bare IPs. MTU128..1492; below1280 is strictly IPv4-only.
Known transit overlap and token collision are refused. Host VPP collision checks
and unique allocation remain parent responsibilities.

Root-controlled/no-symlink ancestors, generation/boot/dev/inode and NS_GET_NSTYPE
protect namespace entry. nsenter receives an inherited pinned FD. Foreign links,
wrong TAP kind/alias/index, wrong raw MAC and unexpected policy state fail closed.
Preparation binds aliases TOKEN:GEN:NAME, disables raw IPv6, removes raw IPv4,
and permits no plain-IP raw-WAN forwarding. Configure requires exact negotiated
PPP MTU agreement, installs separate transit defaults and ingress policy before
local-address routing. IPv6 control, transit ND and PMTU are explicitly preserved.
There is no Linux NAT and the forward chain permits only transit/PPP pair.

`verify TOKEN GEN` checks actual links, MTU, ready transit addresses, exact RPDB,
both default-route tables, namespace sysctls, absence of foreign firewall tables
and nft semantic fingerprint tied to current policy source. A persisted configured
flag alone never passes. Configure verifies before committing configured=true.
Failure after enabling forwarding closes it again. `configured` is not VPP or
joint forwarding readiness; parent must verify VPP and current PPP generation.

`configure TOKEN GEN --accept-default-route yes|no` owns accept_ra=2, autoconf=1
and accept_ra_defrtr choice. Carrier IPv6 hooks must skip their own sysctl writes;
PPP service keeps ProtectKernelTunables. Withdraw durably invalidates configured
before closing forwarding. Delete additionally refuses all links except loopback.
A crash between namespace creation and ledger save leaves an unadopted orphan.

## Fixed privileged broker and agent interface

Install helper at /usr/lib/ngfw/pppoe-carrier.py; always invoke python3 -I.
The agent does NOT gain capabilities or writable /etc from this source.
Requests/results are atomic root0600 files under /run/ngfw/pppoe-broker, inside
its existing allowed /run/ngfw subtree. Boot,32hex nonce,10-second monotonic expiry
and SHA256 bind every request and reply; expiry is rechecked after acquiring the
carrier lock before an operation. Unknown ops/fields and arbitrary paths/executables
are rejected. Broker service timeout12s and KillMode=control-group bound children.

1. `broker-queue TOKEN NONCE --request-json JSON` returns token, nonce, boot,
   expires (CLOCK_MONOTONIC seconds, not wall clock), request_sha256.
2. Start fixed `ngfw-pppoe-broker@TOKEN.service`.
3. `broker-result TOKEN NONCE` verifies expiry and identity, returns same envelope
   plus ok and result, or ok=false/error. Caller must compare queue hash and own
   context; a late/ambiguous reply never establishes readiness. Recover from actual
   inventory/verify because a timed-out operation can have partially applied.

Exact finite request bodies:

- provision: op, owner, logical, spec.
- list: op, owner. Its service token is `ngp-`+first12hex SHA256("inventory"+NUL+owner).
- prepare: op, token, generation, physical_mac, transit (local4,peer4,local6,peer6).
- configure: op, token, generation, accept_default_route (boolean).
- verify/inspect/delete/withdraw: op, token, generation.
- probe: op, token, generation, kind, target.

The fixed broker deliberately shares the host mount namespace: persistent
/run/netns bind mounts must be visible to host VPP after its oneshot exits.
Do not add mount-namespace sandbox directives to that unit. Its root privileges
serve only the finite validated helper, with no arbitrary argv/path. It has
NET_ADMIN, NET_RAW, SYS_ADMIN and SETPCAP; PPP/probe leaf executors irrevocably drop
to NET_ADMIN|NET_RAW only before network-facing execution. The leaf independently
checks exact effective/permitted/bounding masks, empty inheritable/ambient sets,
NoNewPrivs=1 and actual namespace dev/inode plus boot/generation. The broker unit
and tmpfiles configuration are inert assets pending package integration/review.

## PPP fixed unit and private files

`ngfw-pppoe-carrier@TOKEN.service` binds per-session
/var/lib/ngfw/pppoe-carrier/TOKEN/ppp read-only at /etc/ppp. Other /etc and /var/lib
content is hidden by private tmpfs; only non-secret NSS/loader files are exposed.
/run is private. The ownership ledger and namespace handles are read-only;
only /run/ngfw/pppoe/TOKEN is writable and shared back to the host. Broker request
files are absent. Renderer/state observer must use that per-token StateDir.

The launcher reads the ledger under a read-only lock, refuses configured=true,
and requires a prior broker withdrawal. It cannot write the ledger through this
unit. Parent must withdraw before initial start and every autorestart repair.
The four fixed dispatchers install under /usr/lib/ngfw/pppoe-carrier-hooks and are
copied with peers/carrier, options, credentials and fragments into the private
PPP tree. No generated /etc/systemd/system writes and no broad writable /etc.

## Bounded health probes

WAN worker supplies /usr/lib/ngfw/ngfw-wan-probe with --kind icmp|dns|http,
--target literalIPv4, --timeout-ms3000, device fixed internally to ppp0. Helper
requires its root-owned, non-writable, no-symlink binary and .sha256 sidecar,
verifies digest and executes the pinned ELF FD. JSON has exactly sent, received,
latencyMs, unavailable. Hostnames/IPv6/link-local/multicast/loopback are explicitly
unavailable for this initial carrier probe path; no ambient resolver is used.

Supervisor timeout4s; exact target and actual PPP-local address nft sets expire
in5s. Only local OUTPUT-marked probe conntracks have input/return exceptions;
forwarded LAN connections to the same target do not match. No forward-chain
exception exists. Finally cleanup restores normal nft policy, deletes temporary
rules and verifies the original state. Cleanup ambiguity invalidates configured
and closes forwarding. Tests cover success, timeout and cleanup failure.

## Review and remaining integration

R4 findings addressed in source include transit ULA ND, PMTU, unique transit
allocation, capdrop before pppd, expiry after lock, immutable ledger mounts and
per-token writable state. Native acceptance and final source approval remain
separate. Parent must finish VPP rollback/readback, private renderer/hook switch,
fixed service packaging and nonce-bound lost-reply handling before claiming source
completion. No installed host mutation occurred and aggregate CI stays deferred.

## IPv4-only lower MTU compatibility

The source now supports the existing128..1492 public MTU range. Below1280 it
assigns/verifies only IPv4 transit, disables IPv6 on transit and PPP, sets private
IPv6 forwarding=0, and refuses an IPv6 default-route request. IPv6 addresses stay
reserved in immutable spec but are not configured. Parent must likewise omit
VPP IPv6 transit addresses and reject an enabled IPv6 PPP mode below1280. A
576-byte control passes real helper readback fixtures with no IPv6 route/address
commands; silent negotiation below the desired MTU still fails closed.

## VPP object-loss recovery receipt

Inventory/inspect now returns repair_required=true and configured=false when any
formerly bound TAP is absent but every surviving TAP still has the exact owned
alias/index/type/MAC and no foreign links exist. The pinned namespace, immutable
spec, boot and generation remain verified. This is a read-only diagnostic: no
replacement TAP is adopted, no binding/generation is changed, and verify continues
to fail until full dependency-safe teardown/recreation. Changed PPP identity,
foreign links and replaced/mismatched TAPs are refused. Parent's retrieved-only
repair marker must drive stop/withdraw, owned TAP removal and old namespace deletion
before a new generation is provisioned. Tests cover total loss, either partial
loss, a foreign link, changed surviving index and refusal of forwarding readiness.
The verify receipt also carries actual non-tentative PPP address CIDRs so parent
can reject stale hook addresses before mirroring them into VPP.

The fixed units also use DevicePolicy=closed. Only the PPP service additionally
allows /dev/ppp read/write; UID0 does not gain host block-device access merely
because filesystem mounts are read-only. Standard service API devices remain
available. This is source hardening, with no installed service/device mutation.
