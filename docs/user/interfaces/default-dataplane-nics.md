# Default data-plane NICs

Out of the box, **every physical NIC except the management interface belongs to the engine**. At every start of the
management API (boot, service restart, package upgrade) the appliance reads the host's NIC inventory and registers the
NICs still missing from the configuration, so that each non-management NIC is a data-plane interface. You see these
interfaces already present on the **Interfaces** screen, marked **built-in**; they can be edited and disabled but never
deleted.

The screen labels physical ports **Ethernet1 … EthernetN**, numbered by PCI address with the management NIC included
(see [Interface names](basics.md#interface-names)); the configuration, the events and the CLI configuration commands
use the logical name (`ens161`), and the row's drawer shows the PCI address. A NIC that is not registered yet (or was
skipped, below) is listed as a read-only **host interface** row.

## What is registered

At every API start the management API asks the agent for the host NIC inventory (`HostNics`) and saves the missing
NICs in one system revision (revision 1 on a fresh appliance; on an existing installation only the missing NICs are
appended to the lists below and the other settings stay as they are) with:

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

Registration also runs on existing installations and picks up NICs installed later at the next API start. Existing
interface settings, logical names (`dataplane.devices.<pci>.name`), device tuning and explicit releases to the host are
preserved; a row that already exists under the NIC's name is adopted (its settings kept, the `physical` marker added);
it then is a built-in row: no longer deletable, only releasable. NICs are matched by PCI address, so a NIC that loses its
kernel name after binding is not added twice. An unchanged inventory creates no revision.

Registration is audited as the system event `system.seed-defaults`, whose payload lists the registered NICs
(`dataplane`), the NICs taken as management (`management`) and any NIC that was **skipped** with a reason code. When
nothing could be registered but NICs were skipped, the warning event `system.seed-defaults-skipped` names them instead,
and does so again at every API start until the cause is resolved. System events appear on the **Dashboard** under
*Recent events* (message only); the event code and payload are available from `GET /api/v1/state/events`.

| skip reason | meaning | what to do |
|---|---|---|
| `invalid-name` | the kernel name (or the saved `dataplane.devices.<pci>.name`) is not a valid logical name (1–15 characters `a-z0-9_-`, starting with a letter) | give the NIC a logical name: `set dataplane devices <pci> name <name>`, commit, restart the management API |
| `reserved-name` | the name is one the engine creates itself (`local0`, `loopN`, `tapN`, `bondN`, `pppoeN`, …) | same: give the NIC another logical name |
| `name-in-use` | an interface of that name already is a bond, a PPPoE client or another physical NIC | rename the conflicting row, or give the NIC another logical name |
| `management-engine-owned` | the host now reaches the box through a NIC the configuration hands to the engine | **Release to host**: the engine then does not take it over at the next start-up configuration apply, and a later registration saves it in `dataplane.managementPci` |
| `management-limit` | `dataplane.managementPci` already holds four addresses (the maximum) | remove a stale management address |

If the registration commit fails, the warning event `system.seed-defaults-failed` is recorded once per distinct reason;
its message says whether it is retried. A rejection by the validators or a missing licence (`data.final: true`) is not
retried before the next API start: fix the configuration the listed pointers name, then restart the management API
(`ngfw-api.service`) or reboot. Any other failure (`data.final: false`, for example the agent or the database not
answering yet) is retried with increasing delays, up to an hour, and whenever the agent reconnects.

After an **upgrade of an existing installation**, review the Interfaces screen before the next start-up configuration
apply: every non-management NIC the saved configuration did not carry is now an engine-owned row, including NICs the
host still uses for other traffic (a storage link, an HA heartbeat without the default route). **Release to host** the
ones that must stay with the host. A site can switch registration off after firstboot with `NGFW_SEED_DEFAULT_NICS=0`
in `/etc/ngfw/api.env` (see [Package installation](../../install/bare-metal.md)).

Registration is controlled by `NGFW_SEED_DEFAULT_NICS` and is **off by default** (fail-closed): only the appliance's
own API service sets `NGFW_SEED_DEFAULT_NICS=1`. Development stacks, lab slots and tests leave it unset, so they never
seed the host's real NICs.

## Why the management interface is excluded

The management interface is the NIC the appliance is reached through: the NIC that carries the default route, the NIC
an SSH session arrives on, or a NIC named explicitly (`NGFW_MGMT_IF` / `NGFW_MGMT_PCI` for the agent, `--mgmt-if` /
`--mgmt-pci` for the start-up generator). Handing it to the engine would cut off management access, so it is always
excluded (`dataplane.managementPci`) and stays a normal host interface.

**If no management NIC can be identified** (no default route and no SSH session yet — for example the first boot
before DHCP has answered) the appliance does **not** register anything: it records the warning event
`system.seed-defaults-deferred` and tries again every minute and whenever the agent reconnects. Bring the management
network up, or name the management NIC with `NGFW_MGMT_IF` / `NGFW_MGMT_PCI`. Management PCI addresses saved by an
earlier boot (`dataplane.managementPci`) stay protected even while the management route is still coming up.
Uncommitted candidate edits and a commit awaiting confirmation (the setup wizard's included) defer registration; it
retries every minute and whenever the agent reconnects, so it lands once those edits are committed or discarded. Saving
other settings never disables automatic registration, and an initially empty data inventory is retried as well (with
increasing delays, up to an hour). Whatever was left undone is tried again at the next API start — a restart of the
management API service (`ngfw-api.service`) or a reboot.

## Which NICs are inventoried

Every network NIC with a **PCI address**, PCI-backed virtio devices included: NICs the host kernel drives (with their
kernel name, driver, MAC and link state) and NICs already bound to a user-space driver (`vfio-pci`, `uio_pci_generic`,
`igb_uio` — reported as bound).
NICs **without a PCI address** — virtio-mmio devices, USB network adapters — are never inventoried and never become
data-plane interfaces.

## "Awaiting engine"

A registered NIC is only really taken over once the engine's start-up configuration is regenerated from the data-plane
settings and the engine restarts (a first-boot / maintenance step; registration itself binds nothing). Until then the
NIC is still a host interface: the
Interfaces screen shows the row as **awaiting engine**, and the agent reports a warning (`agent.nic-not-bound`), not an
error — the commit succeeds and nothing is taken over early.

## Releasing a NIC to the host, and reclaiming it

A built-in NIC cannot be **deleted** from the configuration: the Remove button is hidden and the API refuses removal
with `403 interfaces.physical-nic-not-deletable`. The `physical` marker itself is read-only — only boot registration
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

A **rollback** restores an earlier revision as a whole; every revision since a NIC was registered carries its built-in
row, so a rollback can change the owner or settings but never removes it. A rollback to a revision from **before** a
NIC was registered drops its row, and the next API start registers the NIC again as engine-owned; if it had been
released to the host, release it again.

**HA pairs** (configuration sync) must have the same NIC layout (PCI addresses): the sync copies the interfaces from
the primary while the data-plane device lists stay per node. A primary NIC the backup does not have makes the sync
fail on the backup (`dataplane.owner-consistent`: an engine-owned row missing from the backup's whitelist); a backup
with extra NICs registers them at every API start and loses them at the next sync.

## Restoring a backup, replacement hardware, removed NICs

The `physical` markers describe the NICs of *this* box, so a configuration import (restore) treats them like any other
edit:

- **Importing a backup onto a box that has not registered its NICs yet** (registration off, or deferred): the
  backup's physical rows are new markers there and the import is refused with `403 interfaces.physical-marker-readonly`,
  pointer `/interfaces/<name>/physical`. Remove the `physical` members from the backup file (the rows then import as
  ordinary interfaces), or let the box register its own NICs first and import the rest of the configuration.
- **Importing onto a replacement box** that registered different NICs: a row whose PCI address differs is refused
  (`…/physical/pci`), and a registered row the backup does not contain is refused as a removal
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
