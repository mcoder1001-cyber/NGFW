# PPPoE kernel carrier implementation checkpoint

## Scope and decision

The owner's 2026-10-08 instruction to complete non-laboratory implementation after
the audit includes the reported PPPoE datapath gap. The manager selected option C
from the published design at `a0134e49d1dbfd08a4198232a8e2d54d79167553`:
kernel PPP encapsulation, exclusive per-parent Ethernet transport, and a separate
plain-IP transit path through VPP. VPP continues to own firewall/NAT decisions.
No VPP C, plugin disabling, Linux NAT, installed-host change, or global writable
`/etc` allowance follows from this source decision.

This work extends the independently reviewed lifecycle source `a673d34` in the
existing PPPoE task worktree. Review receipts already present in that worktree are
not rewritten by this checkpoint. The code and this receipt are owned by the PPPoE
worker; packaging/systemd changes belong to the packaging worker; dynamic WAN
routing belongs to its separate worker. Aggregate CI remains deferred by the owner.

## Implemented source in this checkpoint

`AllocateDelegation` provides the internal LAN prefix-assignment contract and
implementation. Each explicitly named LAN has a stable subnet ID within the
delegated prefix. The allocator returns disjoint /64 networks and router addresses,
sorts by interface name, rejects duplicate LAN/subnet selections and out-of-range
IDs, and refuses absent, invalid, link-local, multicast, mapped-IPv4 or unsupported
prefix lengths. It validates the complete assignment before returning anything.
Lease renewal changes the network portion without changing the selected subnet.

This component is not yet a public configuration field or a live assignment
controller. No existing interface is selected automatically, no RA is advertised,
and no LAN support-completion claim follows from allocator tests.

Focused validation with Go 1.26.0:

```
go test -race -count=1 -run '^TestAllocateDelegation' ./internal/renderers/pppoe
ok  ngfw/agent/internal/renderers/pppoe  1.028s
```

The three tests cover every supported prefix-length boundary, overflow, duplicate
and invalid plans, deterministic ordering, renewal and a single-subnet /64 lease.
`git diff --check` passed. No complete quick gate or native acceptance is claimed.

## Required carrier security and routing contract

1. A per-session private network namespace must be created/verified by an owned
   provisioner with durable generation identity. Never enter an arbitrary namespace
   solely because a configured pathname exists. Only this namespace may receive
   kernel forwarding changes; the host's forwarding state remains untouched.
2. The raw WAN-to-Linux path is exclusive bidirectional L2 transport. Admission
   must reject an existing bridge/xconnect/server attachment or foreign owner before
   changes. It must preserve/recover the previous owned state on every failed step.
   EtherType registration and the global PPPoE CP singleton are not overridden.
3. A separate VPP transit interface carries plain IP. VPP firewall, NAT outside
   role, interface-derived pools, ACL/Global Blocking, uRPF and counters must follow
   this effective interface. Merely changing the default route is insufficient.
4. Kernel ingress from PPP needs explicit policy routing back to VPP before local
   address delivery: the negotiated WAN address is also local on the PPP device.
   Otherwise NAT replies terminate in Linux. Preserve IPv6 link-local/RA/DHCPv6
   control delivery and prevent routes from leaking between WAN namespaces/VRFs.
   Every data return path must pass through VPP; Linux NAT is forbidden.
5. Kernel forwarding policy permits only the owned transit/PPP pair. Raw WAN must
   not carry plain IP, and Linux must not expose a direct LAN path bypassing VPP.
   A failed or departed session withdraws forwarding readiness before cleanup.
6. Fixed packaged units consume a private PPP directory and the verified owned
   namespace. The private directory must include the pppd hook dispatchers, not only
   hook fragments. Renderer changes and packaging must land together. No generated
   unit writes to `/etc/systemd/system`, broad `/etc` write permission, implicit NIC
   detachment or host sysctl change is authorized by this design.
7. WAN routing consumes a future verified forwarding snapshot: logical interface,
   effective VPP interface, transit next hop, negotiated local address and
   boot/session generation. Hook `phase=up` alone is not forwarding readiness.
   The current implementation has no such verified carrier snapshot.
8. Public PD configuration needs additive schema/proto fields for explicit target
   LAN and subnet ID. The controller must validate target ownership and overlap
   with static addresses/RA prefixes, apply through existing owned descriptors,
   and remove/rebuild assignments on lease loss, renewal, rollback and restart.

## Remaining implementation

Namespace provisioning and inode/generation ownership; VPP L2 transport and transit
reconciliation with rollback/readback; kernel forwarding/rule/filter lifecycle;
NAT/firewall effective-interface mapping; fixed-unit/private-hook packaging;
controller-backed forwarding snapshot; public PD model/projection and live
address/RA reconciliation remain unimplemented. The allocator is a coherent source
foundation, not an implementation of all those components.

Original product discovery/encapsulation failures remain unresolved. Lab packet
acceptance must follow completion and independent review of this source. No native
service, namespace, route, packet test or aggregate CI is run by this checkpoint.

## Namespace and admission checkpoint

Added an immutable carrier specification with distinct logical/raw identities,
per-session IPv4/IPv6 transit derivation and overlap rejection. The namespace
descriptor validates exact spec/owner/boot/inode/generation receipts, preserves
parent dependency, refuses foreign deletion and recreates consumers on spec changes.
Live VPP admission refuses an existing logical interface, raw-parent addresses,
LCP pair, bridge/cross-connect or PPP server/control attachment and rejects observed
transit-address overlaps before calling namespace provision. The helper adapter uses
only a fixed packaged Python path and fixed subcommands, bounds and parses JSON
receipts, redacts process errors, and requires actual verification rather than a
stored configured flag. Its runtime registration/projection are not integrated yet.

Focused Go 1.26.0 race checks for `^TestCarrier(Spec|Namespace)`:
descriptor package PASS 1.024s; renderer PASS 1.016s. Subsystems compiled with the
same command (no matching tests). The real VPP admission function compiled but has
not been executed against VPP. Full CI remains deferred. The initial helper and its
subsequent actual verification/session-name fixes are preserved as separate source
commits. These checks do not complete carrier runtime integration or packet proof.

### Source checkpoint: projection and broker adapter

Added the logical transit/raw transport projection, immutable TAP ID candidates,
namespace TAP dependency/admission, explicit daemon session carrier specification,
private single-peer renderer without generated units, and a finite fixed-service
broker adapter. Focused race checks passed for carrier projection/spec/namespace
and renderer, plus agent compile. Aggregate CI remains deferred by owner.
This checkpoint is not operationally complete: product Apply/poll/teardown,
forwarding readiness/probe methods, helper broker packaging and live TAP ID
collision admission still require implementation and review. No target service
or network namespace was activated.

### Runtime integration checkpoint (review in progress)

The product runtime now uses fixed namespace units, per-token private configuration
and state trees, finite lifecycle broker requests, carrier-aware manifest readback,
stop-before-replace recovery, helper verification, VPP transit/MTU/address/xconnect
readback, expiring WAN readiness, bounded probes and delegated LAN registration.
WAN epochs include systemd InvocationID, namespace generation and transition
admission. PD schema/UI/runtime commits and independent PD R1 receipt are integrated.
Focused carrier recovery/broker and delegated scheduler tests passed under race.

Not final: independent full carrier reviews, exhaustive forwarding/failure tests,
packaging integration, and remaining readback/admission findings are pending. The
raw Ethernet path trusts the fixed packaged pppd/plugin: namespace IP filtering
blocks normal host IP routing but does not contain a malicious NET_RAW daemon
injecting arbitrary Ethernet frames. No such stronger claim is made.
