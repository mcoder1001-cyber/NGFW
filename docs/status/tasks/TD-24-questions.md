# TD-24 — open questions

## Q1 (M, follow-up for dhcp/**): a VRF change on a DHCP-client interface with a bound lease is refused by VPP

- **What happens now.** Assigning, changing or removing the `vrf` of an interface or sub-interface that runs `dhcpClient`
  while its lease is bound makes VPP refuse `sw_interface_set_table` with `ADDRESS_FOUND_FOR_INTERFACE` (-85,
  `ip_table_bind`, `vnet/interface_api.c:584-590`). The lease is an address on the interface. The transaction rolls back
  cleanly and the lease stays.
  - Probe on the fake, product wiring: apply a sub-interface with `dhcpClient`, bind a lease, then set `vrf: blue`.
    Result: `APPLY_STATUS_ROLLED_BACK`, `create interface-ip.table/host-w1w0.100: sw_interface_set_table … table 1001:
    VPPApiError … (-85)`, the lease is intact.
- **Before TD-24 this was worse but silent.** The reconcile deleted the lease as an undesired address, and then the
  rebind worked. VPP's client still has `installed == learned`, so it never re-installs the address on renewal
  (`dhcp_client_addr_callback`, `client.c:219`). The interface stayed without its address, and the default route stayed
  in the old table.
- **Fix, one line, in `dhcp/client.go`.** `ClientDescriptor.Dependencies` should also return
  `{Key: interface-ip.table/<if>, Optional: true}`.
  - The executor's `around` then re-creates the client around every Create, Update and Delete of the binding: the client
    is deleted, VPP releases the lease and its default route, the table is rebound, and the client is re-created and
    discovers in the new VRF.
  - I did not make this change here because dhcp/** belongs to F-kea (D-133).
- **Proposal:** F-kea's fix round or a new TD row, with the probe above as its test.

## Q2 (M, new finding, not TD-24's scope): `dhcpClient: {}` without a hostname fails at apply

- The schema says hostname "absent = the system hostname" (`packages/schema/src/domains/interfaces.ts:61-65`).
- The projection (`desired/interfaces.go:242`) passes `""`, and DF-8's `Client.Validate` refuses it with `invalid
  spec: hostname is empty`. The whole commit is rolled back.
- Found while writing `internal/agent/dhcplease_test.go` (the first version used `dhcpClient: {}`). The F-kea
  topology test always sets a hostname, so it never hit this.
- **Fix:** the projection fills the system hostname (or `vrx`) when the hostname is absent. The assembler must then
  leave it out again when it equals that default, or Retrieve will show drift.
- **Owner:** the P08 successor or F-kea.

## Q3 (L, gap, proposed V26): IPv6 addresses that VPP installs itself have no dump in 26.06

- These all add addresses with `ip6_add_del_interface_address`, so they appear in `ip_address_dump`, but no message
  lists them as VPP-owned:
  - SLAAC: `rd_cp` `ip6_nd_address_autoconfig`;
  - the DHCPv6 IA_NA client: `dhcp6_client_enable_disable`;
  - addresses built from a delegated prefix: `ip6_add_del_address_using_prefix`.
- The binapi of `rd_cp`, `dhcp6_ia_na_client_cp` and `dhcp6_pd_client_cp` has enable/disable messages only. No product
  path enables any of them today:
  - `subsystems` registers only `dhcp.client` from DF-8;
  - no task branch uses `rd_cp` (checked the other 30 `task/*` branches; F-neighbors-ra only generates it).
- **Risk.** The day a feature enables one of these on an interface of ours, `interface-ip` would delete those addresses
  the same way it deleted the DHCPv4 lease.
- **Proposal.**
  - Add a V-item to `docs/vpp-code-track.md` (the manager allocates the number; V26 is the next free one on every
    branch): "no dump of the IPv6 addresses VPP installs itself (rd_cp autoconfig, DHCPv6 IA_NA, prefix-derived);
    upstream: an `ip6_nd_address_autoconfig_dump` / per-client address dump, or a flag in `ip_address_details`".
  - Rule for the feature that enables any of them: it must not ship until `interface-ip` can tell those addresses apart.
    The configuration-only fallback is that `interface-ip` skips non-desired global IPv6 addresses on an interface where
    that feature's object exists.
- I do not own `docs/vpp-code-track.md`.

## Q3b (L, known race, documented per review 50fb951d; the code stays as it is)

- **The race.** Retrieve reads `ip_address_dump` first and then, lazily, `dhcp_client_dump`. The two calls are not
  atomic. A renewal to a new address can land between them (`dhcp_client_addr_callback`: release, then acquire,
  `client.c:219-225`). One Retrieve then reports the superseded address as ours.
- **The effect.** The reconcile's delete of that address fails, because VPP already removed it
  (`ADDRESS_NOT_FOUND_FOR_INTERFACE`). That one transaction rolls back. Nothing is wrongly deleted, and the next
  Retrieve (retry or resync) is consistent.
- **Why the code stays.** Dumping the leases first, unconditionally, would break D-132's "no dump without an address
  of ours".

## Q4 (info): the schema rule "no static IPv4 next to dhcpClient" is F-kea's, reused and not duplicated

- `interfaces.kea-dhcp-relay-dhcp-client-no-static` is on `task/F-kea-dhcp-relay`, not on main.
- Until F-kea merges, a static IPv4 equal to the lease on the same interface fails at apply with VPP
  `DUPLICATE_IF_ADDRESS`. That is a loud rollback, not a silent delete.
- A static IPv4 that differs from the lease works, but it is exactly what F-kea's rule will forbid.
- The known-issue line that F-kea's review asked for (L7, `docs/user/services/kea-dhcp-relay.md`) can be dropped once
  TD-24 is merged. That file is F-kea's.

## Q5 (info): files outside the envelope's list, and merge notes

- The envelope names `docs/agent/descriptors/core*.md`. No such file exists. The core descriptors are documented in
  `apps/agent/internal/descriptors/core/README.md`, so the paragraph went there (a table cell plus one new section).
- Test support outside the list:
  - `core/coretest/dhcp.go` (new, the DHCPv4 client model);
  - `core/coretest/fakevpp.go` (+2 lines: field `Iface.DHCP`);
  - `core/coretest/ifext.go` (the empty `dhcp_client_dump` handler is replaced by the model, and the header comment is
    updated);
  - `internal/agent/dhcplease_test.go` (new);
  - `test/topology/interfaces/dhcplease_test.go` (new, the host proof).
- **Merge notes:**
  - TD-23 edits `fakevpp.go` in the `VPP` struct and in `New()`. My hunk is in the `Iface` struct, so the two do not
    overlap.
  - F-kea and TD-23 do not touch `ifext.go:192`.
  - No other branch touches `core/ifaddr.go` or `core/README.md` (checked against the merge-base with TD-11c).

## Q6: the host proof is pending TD-25

- `test/topology/interfaces/dhcplease_test.go` is committed but has not been run. Per the manager, af_packet creates on
  the shared VPP fail closed ("placeholder cap reached … (VPP V19 quarantine)") until TD-25 lands.
- One run on slot 7 is to follow under `tools/lab lock`, with NRestarts before and after (`systemctl show vpp -p NRestarts` = 2 at 09:2x on 2026-09-25).
