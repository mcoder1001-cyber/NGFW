# Default data-plane NICs

Out of the box, **every physical NIC except the management interface belongs to the engine**. On first boot the
appliance reads the host's NIC inventory and seeds the configuration so that each non-management NIC is a data-plane
interface. You see these interfaces already present on the **Interfaces** screen, marked **built-in**; they can be
edited and disabled but never deleted.

## What is seeded

On the very first boot (no saved configuration yet) the management API asks the agent for the host NIC inventory
(`HostNics`) and commits revision 1 with:

- `dataplane.managementPci` — the PCI address of the management NIC(s);
- `dataplane.pciWhitelist` — every non-management NIC's PCI address (the NICs the engine takes over);
- `dataplane.devices.<pci>.name` — the logical interface name: the NIC's kernel name (`ens161`), or, for a NIC that is
  already bound to a user-space driver and has no kernel name, the predictable name of its PCI address
  (`0000:1d:00.0` → `enp29s0f0`);
- `interfaces.<name>` — one entry per non-management NIC, `enabled: true`, with a `physical` marker
  `{ pci, owner: "dataplane", builtIn: true }`.

Worked example — the reference host (management NIC `ens192`, six data NICs); two data NICs shown:

```json
{
  "dataplane": {
    "managementPci": ["0000:0b:00.0"],
    "pciWhitelist": ["0000:04:00.0", "0000:0c:00.0"],
    "devices": {
      "0000:04:00.0": { "name": "ens161" },
      "0000:0c:00.0": { "name": "ens193" }
    }
  },
  "interfaces": {
    "ens161": { "enabled": true, "physical": { "pci": "0000:04:00.0", "owner": "dataplane", "builtIn": true } },
    "ens193": { "enabled": true, "physical": { "pci": "0000:0c:00.0", "owner": "dataplane", "builtIn": true } }
  }
}
```

Seeding runs once, and only on a configuration nobody has touched yet. A later boot, or a boot after an operator has
committed, never re-seeds. It is audited as the system event `system.seed-defaults`.

Seeding is controlled by `NGFW_SEED_DEFAULT_NICS` and is **off by default** (fail-closed): only the appliance's own API
service / first boot sets `NGFW_SEED_DEFAULT_NICS=1`. Development stacks, lab slots and tests leave it unset, so they
never seed the host's real NICs.

## Why the management interface is excluded

The management interface is the NIC the appliance is reached through: the NIC that carries the default route, the NIC
an SSH session arrives on, or a NIC named explicitly (`NGFW_MGMT_IF` / `NGFW_MGMT_PCI` for the agent, `--mgmt-if` /
`--mgmt-pci` for the start-up generator). Handing it to the engine would cut off management access, so it is always
excluded (`dataplane.managementPci`) and stays a normal host interface.

**If no management NIC can be identified** (no default route and no SSH session yet — for example the first boot
before DHCP has answered) the appliance does **not** seed: it records the warning event
`system.seed-defaults-deferred` and tries again every minute and whenever the agent reconnects. Bring the management
network up, or name the management NIC with `NGFW_MGMT_IF` / `NGFW_MGMT_PCI`. If an operator commits a configuration
before a retry succeeds, the seed is abandoned (it only ever seeds an untouched configuration); the NICs can then be
added by hand.

## Which NICs are inventoried

Every network NIC with a **PCI address**: NICs the host kernel drives (with their kernel name, driver, MAC and link
state) and NICs already bound to a user-space driver (`vfio-pci`, `uio_pci_generic`, `igb_uio` — reported as bound).
NICs **without a PCI address** — virtio-mmio devices, USB network adapters — are never inventoried and never become
data-plane interfaces.

## "Awaiting engine"

A seeded NIC is only really taken over once the engine's start-up configuration is regenerated from the data-plane
settings and the engine restarts (a first-boot / maintenance step). Until then the NIC is still a host interface: the
Interfaces screen shows the row as **awaiting engine**, and the agent reports a warning (`agent.nic-not-bound`), not an
error — the commit succeeds and nothing is taken over early.

## Releasing a NIC to the host, and reclaiming it

A built-in NIC cannot be **deleted** from the configuration: the Remove button is hidden and the API refuses removal
with `403 interfaces.physical-nic-not-deletable`. The `physical` marker itself is read-only — only the first-boot seed
creates it; an edit that adds it to another interface or changes its `pci` / `builtIn` is refused with
`403 interfaces.physical-marker-readonly`. The supported way to give a NIC back to the host is to **release** it:

- **Release to host** sets `physical.owner = "host"` and removes the NIC from `dataplane.pciWhitelist` and
  `dataplane.devices`: the engine no longer takes it over.
- **Reclaim for the engine** sets `physical.owner = "dataplane"` and adds it back.

The web UI does both halves in one step. The rule `dataplane.owner-consistent` rejects a commit where they disagree
(a released NIC still whitelisted, or an engine-owned NIC missing from the whitelist), and
`dataplane.physical-name-matches-device` keeps `devices.<pci>.name` equal to the interface name. The change takes
effect with the next start-up configuration apply. Releasing a NIC does not configure it on the host side (no address,
no netplan).

A **rollback** restores an earlier revision as a whole; every revision since the seed carries the same built-in rows,
so a rollback can change their owner or settings but never removes them.

## Restoring a backup, replacement hardware, removed NICs

The `physical` markers describe the NICs of *this* box, so a configuration import (restore) treats them like any other
edit:

- **Importing a backup onto a box that has not seeded** (seeding off, or seeding deferred): the backup's physical rows
  are new markers there and the import is refused with `403 interfaces.physical-marker-readonly`, pointer
  `/interfaces/<name>/physical`. Remove the `physical` members from the backup file (the rows then import as ordinary
  interfaces), or let the box seed its own NICs first and import the rest of the configuration.
- **Importing onto a replacement box** that seeded different NICs: a row whose PCI address differs is refused
  (`…/physical/pci`), and a seeded row the backup does not contain is refused as a removal
  (`403 interfaces.physical-nic-not-deletable`). Keep the replacement box's own `physical` rows and `dataplane` NIC
  lists, and carry over only the other settings (addresses, VRFs, descriptions …).
- **A NIC that was physically removed** keeps its built-in row — it cannot be deleted. Release it to the host
  (`physical.owner = host`): a released row is never configured on the data plane, so the missing NIC does not make
  commits fail. The row stays in the configuration as a record of the NIC.

## CLI equivalent

Release `ens161` (PCI `0000:04:00.0`) to the host:

```
ngfw configure
set interfaces ens161 physical owner host
delete dataplane pciWhitelist 0000:04:00.0
delete dataplane devices 0000:04:00.0
commit comment "release ens161 to the host"
```

Reclaim it:

```
ngfw configure
set interfaces ens161 physical owner dataplane
set dataplane pciWhitelist 0000:04:00.0
set dataplane devices 0000:04:00.0 name ens161
commit comment "reclaim ens161"
```

`set` on the `pciWhitelist` list appends one item; `delete <list> <value>` removes it.

See also: [Interface basics](./basics.md).
