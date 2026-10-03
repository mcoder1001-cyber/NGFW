# PENDING: automatic network bootstrap

- raised: 2026-09-30 by network-defaults-web
- decision: **Awaiting the product owner's answers in chat**
- dependent work: boot-time NIC takeover, initial persistent interface configuration,
  automatic routing TAP activation and upgrade migration

## Requested behavior

The product owner requests that the web interface use product terminology instead of
FRR, VPP and strongSwan, hide manual Linux routing interface configuration, transfer
data NICs to the packet engine at boot, retain the management NIC in Linux, and create
routing TAPs automatically during initial configuration.

## Questions sent to the product owner

1. Select and persist the management NIC during installation/initial configuration
   (recommended), or infer it from the default route and refuse ambiguous discovery?
2. Apply the defaults to new installations with explicit migration of existing
   installations (recommended), or also activate them on existing installations at
   the next boot while preserving compatible explicit configuration?

Neither an unanswered question nor a preselected UI choice authorizes a particular
management identity or upgrade migration.

## Technical interpretation

- Ownership candidates are physical/virtual PCI Ethernet NICs, not loopback,
  bridges, VLANs, existing TAP/TUN devices or management member interfaces.
- A routing interface needs its own TAP pair; one shared TAP cannot represent
  independent OSPF/IS-IS links. Preserve explicitly configured pair names, types and
  namespaces. Logical subinterfaces need deliberate eligibility and mapping.
- Never claim management NICs or their shared hardware isolation group. Unsupported
  drivers, missing identity or unreadable inventory must stop takeover.
- Persist initial interface configuration through the existing datastore and agent
  desired state. Creating unmanaged TAPs in a shell script is not restart-safe.
- Order device preparation before the packet engine, persistent reconciliation after
  connection, and routing application after the required interfaces and daemon sockets
  are ready. Preserve the existing routing descriptor's independence from pair churn.

## Repository findings

`ngfw-startupgen` currently renders only explicit devices; an empty configuration emits
`no-pci`. Appliance installation/firstboot units are not implemented. A fresh agent has
empty desired state and replays only persisted domains. Existing validation requires
explicit `interfaces.<name>.lcp`; hiding its web controls does not implement automatic
pairing. The full requested feature must not be deployed until these dependent paths
and migration behavior are implemented and tested together.

## Work that can continue

Product terminology, removing manual pairing controls while preserving stored data,
read-only inventory safety primitives, regression tests and development-tool checks.
No live NIC rebinding or packet-engine restart is part of the cloud-host checks.
