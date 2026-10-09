# pppoe descriptors (DF-6, WBS D6.6)

The client uses a private kernel PPP carrier with a separate VPP plain-IP transit.
VPP's native PPPoE session/CP descriptors below retain their server-side semantics;
the client does not repurpose the global CP setter or require disabling the plugin.
Live packet and restart acceptance remains deferred.

Package `apps/agent/internal/descriptors/pppoe`. Messages only from `apps/agent/binapi/pppoe`. Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| PPPoE session | `pppoe.session` · `pppoe.session/<client_mac>/<session_id>` | `pppoe_add_del_session` + owner tag | `pppoe_session_dump` + tag | `ErrRecreate` | `vrf/<decap_vrf_id>` |
| PPPoE control-plane interface (**VPP-global**, globals owner only) | `pppoe.cp` · `pppoe.cp/global` | `pppoe_add_del_cp` once per VPP boot (the enable stacks the `pppoe-input` feature) | **write-only** | set | `interface/<interface>` |

Model `pppoe.Session`: `session_id` (1–65535), `client_ip`, `client_mac`, `decap_vrf_id`.

Limitation: VPP creates a session only for a client MAC that `pppoe-input` has learned from PPPoE discovery packets;
otherwise `pppoe_add_del_session` returns INVALID_SW_IF_INDEX, mapped to `pppoe.ErrClientNotLearned`. The host has no
PPPoE clients and DF-6 sends no packets, so the host test verifies that typed error and skips the create/retrieve part
(Q3). PPPoE client/server daemons are out of scope.

`pppoe_add_del_cp` sets VPP's single `cp_if_index` (review M2), so it is a global singleton under D-071; its host test is
opt-in (`NGFW_DF6_PPPOE_CP_HOST=1`) and never runs on the shared VPP.

## Client carrier graph and hook lifetime

`pppoe.client.config/ngfw` is a `StageDaemon` singleton containing reference-only
logical PPP settings, selected parent metadata and normalized Multi-WAN membership.
The descriptor resolves version-pinned password references only for rendering; its
remembered session registry clears plaintext passwords. Validation stages private
files and invokes the structural checker once with redacted errors.

`pppoe.carrier.namespace/<token>` owns an immutable owner/logical/parent/MTU/transit
specification. The broker returns verified boot, namespace inode and generation
identity. Namespace creation checks live parent ownership, LCP/L2/bond/unnumbered
conflicts, MTU and transit/TAP-ID collisions before any namespace mutation.
`pppoe.carrier.tap` owns the raw and plain-IP TAPs separately from remote-access
TAP guards. It provides canonical `tapv2.tap` creator keys for the interface index
resolver. TAPs depend on the namespace; raw xconnects and interface aliases depend
on the TAPs; the daemon depends on both transport directions and, for VLAN parents,
the owned POP rewrite. Reverse deletion stops the daemon and withdraws forwarding
before removing transport and namespace resources.

Explicit VLAN parents retain configured root/sub-ID/outer/inner/802.1ad metadata in
the client manifest. Readiness compares actual classification to those exact values
and checks POP1/POP2, zero push arguments and an untagged raw TAP. VPP derives the
inverse egress rewrite from the sub-interface classification. A merely valid but
wrong live VLAN cannot authorize the session. Missing owned TAPs cause retrieved
namespace repair drift and complete consumer/namespace recreation with a fresh
generation; they are not silently rebound to an existing generation.

Production uses fixed packaged units and finite broker operations. Agent unit
capability and writable-path contracts remain unchanged. The broker supplies the
host-visible persistent namespace mount and bounded child privileges; it does not
accept arbitrary executables or arguments. Peer files are under the agent's existing
writable `/var/lib/ngfw/agent` tree. The PPP unit sees a private read-only `/etc/ppp`,
except its per-token resolver output file. It cannot rewrite host `/etc`.

`ClientConfig.Retrieve` compares the exact carrier-rendered peer/hook files and
remembers recovered sessions before returning file drift. Apply withdraws readiness,
mirrors and kernel forwarding, fences IPv6 hooks, stops the old unit, then replaces
files and rotates admission before starting the replacement. Persistent transition
markers retain retry/rollback evidence after partial failures. Non-supervising slot
agents retain their isolated renderer paths and never invoke systemctl.

The one-second watcher runs under the agent transaction fence. Forwarding admission
requires live namespace/link/MTU/PPP-address readback, VPP transit and cross-connect
readback and the active unit InvocationID. The epoch also includes hook admission
and NCP session generations, so persistent redials invalidate earlier probes even
if addresses or the process stay unchanged. WAN readiness expires after three
seconds; bounded probes recheck generation before and after execution.

Mirrored addresses remain /32 or /128 on the logical PPP interface. Default-route
paths use the owned Linux transit peer. Multi-WAN membership suppresses the PPP
daemon's automatic default path without changing the operator's stored setting;
join/leave reprojects the client manifest transactionally. Route withdrawal remains
single-path and multipath-safe. NAT/ACL consumers use the logical interface index.

DHCPv6-PD targets are explicit additive configuration. The delegated LAN reconciler
requires current carrier and lease admission generations, checks overlap and target
ownership, and manages runtime LAN /64 addresses and bounded RA lifetimes. Lease
renewal, expiry, carrier loss, disable and rollback reconcile withdrawals. Static
address/RA reconciliation excludes only the owned dynamic targets.

The fixed pppd/plugin is trusted. Kernel filtering blocks ordinary host IP bypass;
NET_RAW is not containment against malicious raw Ethernet injection. No unsupported
VPP classifier readback or stronger isolation claim is made. Combined source review,
final aggregate CI and native discovery/transit/NAT/ACL/PD/restart evidence remain
separate acceptance requirements.
