# Interfaces — basics

**Screen:** *Interfaces → Interfaces* (`/interfaces`). **REST:** `/api/v1/state/interfaces` (live) and the generic
configuration routes under `/api/v1/config/interfaces`. **CLI:** `ngfw show interfaces`, `ngfw set interfaces …`
(`docs/user/cli/reference.md`).

The screen automatically shows every physical PCI network interface discovered by the agent on the host, including management NICs, alongside the **live interface table of the data plane**, as the agent reads it from VPP (not the configuration):
name, type, admin and link state, MTU, addresses, VRF, receive/transmit rates with a packet-rate trend, and errors
(receive/transmit errors plus drops). Each row also says whether the candidate changes it (*pending*). Rates come from
the live counter stream (`WS /api/v1/stream`, topic `iface.counters`, about one sample per second). The table polls every
3 s, so the admin/link state follows within a few seconds.

![Interfaces list](img/interfaces-list-en.png)

Status chips use the semantic status colours: **Up** (green), **Down** (red; administratively up but no link),
**Admin down** (grey; switched off in the configuration — this is not a fault).

## Automatic host discovery

Physical NICs appear automatically; no **Add interface** action is needed just to see them. Host interfaces not configured or present in the engine show a **host interface** label and a read-only drawer with Linux name, PCI address, driver, MAC and observed link state. A **management** label identifies NICs used for host management traffic. Listing or opening these rows never stages configuration, claims a NIC, or changes management connectivity.

NICs already configured by physical PCI address are correlated with their existing logical rows rather than added twice. Unique MAC matches also correlate DPDK and native vmxnet3 interfaces. Other engine device types with ambiguous hardware identity (such as virtio, also used by virtual TAP devices) remain separate unless their configured PCI or exact name identifies the NIC. Existing live engine interfaces remain editable. If the host inventory or engine observation is unavailable, the page explicitly warns that the list may be incomplete and keeps the available rows. A confirmed kernel carrier shows **up**; false or unavailable carrier shows **down or unknown**, including userspace-bound NICs without a Linux netdev. Host discovery uses the current agent's physical PCI NIC inventory (one entry per PCI function, including virtio-pci child netdevs); Linux-only virtual interfaces without PCI devices are not included unless exposed by the engine.

The same merged read-only inventory is available from `GET /api/v1/state/interfaces` (`hostInventory`, `inventoryOnly`, availability flags and `observationErrors`).

## Editing an interface

Click a row to open the detail drawer:

- **Live state:** type, `sw_if_index`, MAC, MTU (and the link MTU), addresses, VRF with its table id, RX mode, rates.
- **Configuration (candidate):** the form is generated from the configuration schema (`interfaces.<name>`): enabled,
  description, MTU, MAC, promiscuous, RX mode, IPv4/IPv6 addresses, VRF, IP unnumbered, DHCP client. **Save to candidate**
  sends only the fields you changed (a merge patch). Nothing reaches the data plane until you **Commit** in the bar at the
  top (review the diff there; the default commit auto-reverts unless you confirm it).
- **Sub-interfaces (802.1Q):** add, edit and remove single-tag VLAN sub-interfaces (`<parent>.<id>`, usually id = VLAN).
  QinQ (802.1ad / 802.1Q-in-802.1Q) sub-interfaces: [vlan-qinq.md](vlan-qinq.md).
- **Remove from configuration** deletes the interface from the candidate. The agent deletes interfaces it created (`host-…`,
  `loop…`) on commit; physical NICs stay and only lose their configuration.

![Interface drawer](img/interfaces-drawer-en.png)
![Sub-interface dialog](img/interfaces-sub-dialog-en.png)

Validation errors from the server appear on the matching field (RFC 9457 problem pointers), e.g. an MTU outside
68–9216 or an address that is not a prefix.

The screen is fully available in Persian (right-to-left); the drawer opens from the reading end:

![Interfaces list, Persian, dark](img/interfaces-list-fa-dark-rtl.png)
![Interface drawer, Persian](img/interfaces-drawer-fa-rtl.png)

## Interface names

| name | what the agent does |
|---|---|
| `host-<netdev>` | creates an af_packet interface on the Linux netdev `<netdev>` (lab/test path; `host-w1l0` on `w1l0`). Only a **veth** is accepted: naming an existing netdev of any other kind (the management NIC `ens192`, a bridge, …) fails validation (400 validation problem) at `/interfaces/host-<netdev>` (rule `interfaces.af-packet-veth`), and the agent checks again right before it creates the interface |
| `loop<N>` | creates loopback instance N |
| anything else (`GigabitEthernet0/8/0`, `TenGigabitEthernet…`) | an existing NIC; it is configured, never created or deleted |
| `<parent>.<id>` | an 802.1Q sub-interface (exact-match, routed), configured under `subinterfaces.<id>` of the parent |

## The same with REST

All calls need `Authorization: Bearer <access token>` (or an API key). Examples use the lab rig names.

```sh
# live table, one item per (sub-)interface: state (live, from VPP), config (what the agent retrieved from VPP),
# running (the committed configuration), counters, hasPendingChange (the candidate changes this interface)
curl -s -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/state/interfaces
# counters of one interface (absolute; 64-bit counters are decimal strings)
curl -s -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/state/interfaces/host-w1l0/counters

# configure two interfaces in the candidate (RFC 7386 merge patch on the interfaces node)
curl -s -X PATCH -H "authorization: Bearer $T" -H 'content-type: application/merge-patch+json' \
  http://127.0.0.1:3000/api/v1/config/interfaces \
  -d '{"host-w1l0":{"enabled":true,"ipv4":["10.1.1.1/24"]},"host-w1w0":{"enabled":true,"ipv4":["10.1.2.1/24"]}}'
# one field; a NIC name with "/" is one path segment written with ~1 (GigabitEthernet0~18~10)
curl -s -X PATCH -H "authorization: Bearer $T" -H 'content-type: application/merge-patch+json' \
  http://127.0.0.1:3000/api/v1/config/interfaces/host-w1w0 -d '{"mtu":1400}'
# a VLAN sub-interface
curl -s -X PUT -H "authorization: Bearer $T" -H 'content-type: application/json' \
  http://127.0.0.1:3000/api/v1/config/interfaces/host-w1w0/subinterfaces/100 -d '{"vlanId":100,"enabled":true,"ipv4":["10.1.100.1/24"]}'

curl -s -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/config/diff          # review
curl -s -X POST -H "authorization: Bearer $T" 'http://127.0.0.1:3000/api/v1/config/commit?comment=interfaces'
curl -s -X POST -H "authorization: Bearer $T" 'http://127.0.0.1:3000/api/v1/config/rollback/1?comment=undo'
```

## The same with the CLI

```
ngfw show interfaces
ngfw show interfaces host-w1l0
ngfw set interfaces host-w1l0 enabled true
ngfw set interfaces host-w1l0 ipv4 10.1.1.1/24          # appends to the address list
ngfw set interfaces host-w1w0 mtu 1400
ngfw merge interfaces host-w1w0 subinterfaces '{"100":{"vlanId":100,"enabled":true,"ipv4":["10.1.100.1/24"]}}'
ngfw show configuration diff
ngfw commit confirm 120 comment "interfaces"
ngfw confirm
ngfw rollback 1
```

## What happens on the data plane

After a commit the agent converges VPP to the configuration and verifies it (Retrieve). It also restores the interfaces by
itself: if the agent restarts, or objects disappear from VPP behind its back, the next reconcile recreates them without
any API call (the P08 restart-safety test measured 0.33 s from agent start to a converged data plane and a working ping
1.7 s after start). A rollback applies an older revision as a new one and removes what that revision does not contain
(for example an MTU or an address added later).

Not in this release: bonding, bridge domains, QinQ, LACP, LLDP (planned features), per-interface description in VPP
(the description is kept by the agent and shown from there).

See also: [Bond interfaces (link aggregation, LACP)](bonding.md).

See also: [Bridging](bridge-l2.md) — bridge domains, cross-connects, VLAN tag rewrite on L2 ports and the time-range MAC filter (`interfaces.<if>.l2`, `routing.l2`).
See also: [Loopbacks as BVI, GSO, port mirroring, LLDP and the delay simulator](loopback-bvi-gso-lldp-span.md) — `interfaces.<if>.gso`, `interfaces.<if>.mirror`, `services.lldp`, `services.nsim`.
See also: [Tunnels](../vpn/tunnels.md) — GRE, VXLAN and IPIP tunnel interfaces (`tunnels.*`).
See also: [Default data-plane NICs](default-dataplane-nics.md) — built-in physical NICs seeded on first boot, release to / reclaim from the host (`interfaces.<if>.physical`).

### IP unnumbered

Set **IP unnumbered** to another configured interface's logical name to borrow its
IPv4 and IPv6 addresses. The donor must be numbered and must use the same VRF as
the borrower. A borrower cannot configure its own addresses or DHCP client; donor
chains and cycles are rejected. Subinterfaces can borrow from a numbered interface
using the same field.

Saving to candidate, commit, retrieve and rollback use the normal interfaces
transaction. Clearing the field removes the VPP borrowing association; it does not
delete the donor or its addresses. The live association is retrieved from VPP and
reconciled after restart. An existing unclaimed association on a physical interface
is refused rather than silently adopted.

CLI equivalent: update the candidate `interfaces.<borrower>.unnumbered` field to
`<donor>` with the configuration commands, then commit. Clear that candidate field
and commit to revoke borrowing.
