# PENDING: automatic network bootstrap

- raised: 2026-09-30 by network-defaults-web
- decision: **Persistent NIC registration implemented (owner instruction 2026-10-10, D-249); hardware takeover and
  routing TAP policy remain pending**
- dependent work: boot-time NIC takeover, automatic routing TAP activation and hardware ownership migration

## Update: automatic interface registration (owner instruction 2026-10-10, ported to main 2026-10-17)

On 2026-10-10 the product owner reiterated that host interfaces must be registered automatically at boot, without
manual addition. The dev-host implemented this as commit `76484d79f` (on no branch or tag; reachable by SHA, see below);
this update is its port
onto the current `main` with the `ngfw` naming (D-249). The appliance API unit keeps `NGFW_SEED_DEFAULT_NICS=1` and the
registration service (`apps/api/src/features/default-vpp-nics/seed.service.ts`) now runs at **every** API start
(boot, service restart, package upgrade), not only on a database without revisions: the data NICs missing from the running configuration are added through one
system commit. Existing rows, logical names, interface settings, device tuning and host ownership (released NICs) are
preserved; NICs are matched by PCI address, so a NIC that lost its kernel name after binding is never added twice.
Management identity comes from the host inventory plus the management PCI addresses saved earlier. PCI-backed virtio
NICs are resolved consistently for inventory and management detection (agent side, already on `main`). Agent
reconnects, an unreachable inventory and an initially empty data inventory are retried; candidate edits or a pending
confirmation defer the system commit (the commit engine's `systemCommit` rule), so the setup wizard's staged document
is never overwritten and an unchanged inventory creates no revision. A commit the validators reject is final for that
boot and recorded once as the warning event `system.seed-defaults-failed` (with its pointers); other failures and an
empty data inventory are retried with exponential backoff (1 min → 1 h). NICs that cannot be registered — a reserved
or invalid name, a name already used by a bond, a PPPoE client, another physical row or another PCI's saved device
name, a kernel NIC whose saved device name differs from its kernel name (registered once the start-up configuration
binds it), a bound NIC without a saved device name, a management NIC the configuration still hands to the engine, more
than four management addresses or 64 whitelisted ones — are reported in the audit event and as the warning event
`system.seed-defaults-skipped`, never dropped silently; no row is ever registered under a name the engine does not have
(review R4 #2). Ethernet display labels are numbered over
the PCI-sorted hardware rows (configured physical rows and host-inventory rows); registration adds configuration rows
for PCI addresses the inventory already lists, so the labels do not move.

For interface *registration* this applies the owner's instruction to new and existing installations alike, preserving
compatible explicit configuration (question 2 below asked whether existing installations get the defaults at the next
boot; the owner's 2026-10-10 instruction, as recorded in the dev-host commit, says yes for registration — the owner's
go-ahead for this port is recorded on PR #238 when given). It does not add automatic hardware rebinding,
startup-configuration application or routing TAP activation; those boot operations, their validation and question 1
(persisting the management choice at installation) remain pending below. The original commit `76484d79f` is reachable
on GitHub by SHA only (no branch or tag references it); archiving the dev-host lineage is the owner's call.

Behaviour the port changes for operators: a rollback to a revision from before a NIC was registered drops its row, and
the next API start registers the NIC again as engine-owned — a release made after that revision is not restored (the
built-in rows used to be invariant across every revision). HA pairs must have the same PCI layout: cluster sync copies
`/interfaces` from the primary while `/dataplane` stays node-local, so any engine-owned row of the primary whose PCI the
backup does not whitelist makes the sync fail on the backup (`dataplane.owner-consistent`); rows are re-registered at
every start and lost at the next sync only when the primary carries none (tracked in `docs/tech-debt.md`). Review R4
also found a pre-existing agent defect this widens — block lists on *all interfaces* and automatic blocking still target
unbound and released physical rows (`global_blocking.go` `allIfs`) — tracked in `docs/tech-debt.md` with a proposed
separate agent fix; the user doc states the exception.

Validation (2026-10-17, port branch `claude/boot-nic-registration-port`; exact commands and outputs, SHAs and the
complete `tools/ci.sh quick` results are pasted in `docs/status/tasks/boot-nic-registration-port-wip.md`, the review
reports in `boot-nic-registration-port-review-R*.md`): `pnpm turbo run lint typecheck test --filter=@ngfw/api` (ESLint,
`tsc`, Vitest) and the complete `tools/ci.sh quick` gate passed on the first checkpoint `814532721`; the second
iteration (review findings R2/R7/R8) is gated the same way before the PR leaves draft. Unit coverage:
`seed.service.test.ts` (33 cases: registration over an existing revision without a repeat on restart, PCI matching
after binding or a kernel rename (upper-case rows included), released/disabled rows and device tuning preserved, bond /
PPPoE / second-row / reserved-name / invalid-name skips reported, a logical name never taken from another PCI's saved
device name whatever the PCI order, a kernel NIC under a differing saved name and a bound NIC without a saved name
skipped, management NIC that the configuration hands to the engine never written into `managementPci`, the four-address
and 64-address limits, saved management PCI protected, every planned document passing `validateConfig`, empty-inventory
backoff that grows and resets, a validator-rejected commit final for the API start with one failure event, 409/503
problems and transient errors retried with backoff and recorded once, a failure alternating with an unreachable
inventory recorded once with no peer address in the log, shutdown during an inventory request) and `commit.service.test.ts` (26 cases, two of them driving the system commit
through real gRPC against the fake agent: registration after an operator commit and after a release; deferral while a
candidate has edits and while a commit awaits confirmation, then registration after the confirm);
`deploy/debian/ngfw/tests/test_packaging.py` (17 cases) checks that the appliance unit and its hardening drop-in leave
`NGFW_SEED_DEFAULT_NICS` enabled. **Not run:** `apps/api/test/e2e/default-vpp-nics.e2e.test.ts` (PostgreSQL e2e, runs
only under `test:integration` / `tools/ci.sh full`), `tools/ci.sh full`, and a boot of a real appliance with an existing
revision (deferred to `docs/status/DEFERRED-ACCEPTANCE.md`, *Active feature acceptance*); no appliance, lab host or VPP
instance was touched.

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
