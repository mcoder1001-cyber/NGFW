# PENDING: automatic network bootstrap

- raised: 2026-09-30 by network-defaults-web
- decision: **Persistent NIC registration implemented (owner instruction 2026-10-10, D-249); hardware takeover and
  routing TAP policy remain pending**
- dependent work: boot-time NIC takeover, automatic routing TAP activation and hardware ownership migration

## Update: automatic interface registration (owner instruction 2026-10-10, ported to main 2026-10-17)

On 2026-10-10 the product owner reiterated that host interfaces must be registered automatically at boot, without
manual addition. The dev-host implemented this as local commit `76484d79f` (never published); this update is its port
onto the current `main` with the `ngfw` naming (D-249). The appliance API unit keeps `NGFW_SEED_DEFAULT_NICS=1` and the
registration service (`apps/api/src/features/default-vpp-nics/seed.service.ts`) now runs at **every** API boot, not
only on a database without revisions: the data NICs missing from the running configuration are added through one
system commit. Existing rows, logical names, interface settings, device tuning and host ownership (released NICs) are
preserved; NICs are matched by PCI address, so a NIC that lost its kernel name after binding is never added twice.
Management identity comes from the host inventory plus the management PCI addresses saved earlier. PCI-backed virtio
NICs are resolved consistently for inventory and management detection (agent side, already on `main`). Agent
reconnects, an unreachable inventory and an initially empty data inventory are retried; candidate edits or a pending
confirmation defer the system commit (the commit engine's `systemCommit` rule), so the setup wizard's staged document
is never overwritten and an unchanged inventory creates no revision. Ethernet display labels are numbered over the
PCI-sorted host inventory, which registration does not change, so labels do not move.

This answers question 2 for interface *registration*: it applies to new and existing installations alike, preserving
compatible explicit configuration. It does not add automatic hardware rebinding, startup-configuration application or
routing TAP activation; those boot operations, their validation and question 1 (persisting the management choice at
installation) remain pending below.

Validation (2026-10-17, port branch `claude/boot-nic-registration-port`; exact outputs in
`docs/status/tasks/boot-nic-registration-port-wip.md`): `pnpm turbo run lint typecheck test --filter=@ngfw/api` —
10/10 tasks successful: ESLint, `tsc` and Vitest with 113 files / 806 tests passed, among them the 19
`seed.service.test.ts` cases (registration over an existing revision without a repeat on restart, PCI matching after
binding or a kernel rename, released/disabled rows and device tuning preserved, bond / second-row / reserved-name
refusal, saved management PCI protected, empty-inventory retry, shutdown during an inventory request) and the two
`commit.service.test.ts` cases that drive the system commit through real gRPC against the fake agent (registration after
an operator commit and after a release; deferral while a candidate has edits). `test_packaging.py`: 17 tests OK,
including the new check that the appliance unit keeps `NGFW_SEED_DEFAULT_NICS=1` ahead of the site environment file.
The complete `tools/ci.sh quick` result is recorded in the task status file and on the PR. **Not run:** a boot of a real
appliance with an existing revision (deferred to `docs/status/DEFERRED-ACCEPTANCE.md`, *Active feature acceptance*);
no appliance, lab host or VPP instance was touched.

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
