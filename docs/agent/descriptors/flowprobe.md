# flowprobe plugin descriptors (DF-8, WBS D7.6)

Package `apps/agent/internal/descriptors/flowprobe` — per-packet IPFIX flow records: global parameters and flowprobe
on an interface. Message names only from `apps/agent/binapi/flowprobe`. `flowprobe.Register(registry, client, owner, opts...)`;
option `WithInterfaceKey`.

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `flowprobe.params` (singleton) | `flowprobe.params/global` | `flowprobe_set_params`; delete = record 0 + default timers (15/120) | `flowprobe_get_params` while a record flag is set | `ErrRecreate` (see below) | — |
| `flowprobe.interface` | `flowprobe.interface/<ifname>` | `flowprobe_interface_add_del` is_add=1 / 0 (which ip4\|ip6\|l2, direction rx\|tx\|both) | `flowprobe_interface_dump` (owned interfaces) | `ErrRecreate` | `interface/<ifname>`, `flowprobe.params/global` (mandatory — VPP refuses interfaces before record flags are set), `ipfix.default-exporter/global` optional |

Value fields: `Params{record_l2, record_l3, record_l4, active_timer, passive_timer}` (explicit seconds, passive ≥ active
unless 0 = off, at least one record flag); `Interface{interface, which, direction}`.

## Notes and limitations
- VPP accepts `flowprobe_set_params` only while **no interface of any owner** has flowprobe enabled (`UNSUPPORTED`,
  seen on the host). `flowprobe.params` returns `ErrRecreate` on every change so the scheduler takes the dependent
  `flowprobe.interface` objects down first; on the shared host another slot's enabled interface blocks it (the error
  says so).
- One variant per interface: a second different variant is `ENTRY_ALREADY_EXISTS` (error); an identical re-apply
  succeeds (compared with the dump). Legacy `flowprobe_tx_interface_add_del` / `flowprobe_params` are not used.
- Records go to IPFIX exporter 0 (`ipfix.default-exporter`), never to additional exporters.
- Ownership: interfaces by owner tag. The params singleton is reported only when set; the host test skips when
  someone else set it and resets it in Cleanup.
