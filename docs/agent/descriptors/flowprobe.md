# flowprobe plugin descriptors (DF-8, WBS D7.6)

Package `apps/agent/internal/descriptors/flowprobe` — per-packet IPFIX flow records: global parameters and flowprobe
on an interface. Message names only from `apps/agent/binapi/flowprobe`. `flowprobe.Register(registry, client, owner, opts...)` (+ `RegisterGlobals` for the params);
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
- Ownership: this owner's tagged interfaces or claimed untagged ones (below). The params singleton is reported only when set; the host test skips when
  someone else set it and resets it in Cleanup.

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- `flowprobe.Register(...)` registers the per-owner object types (`flowprobe.interface`); `flowprobe.RegisterGlobals(...)` registers the
  VPP-global singletons (`flowprobe.params`) constructed as **globals owner** — P08 calls it only in the designated globals
  owner's agent (D-071), before `Register`. A descriptor constructed without the role (`dfkit.GlobalsOwner(false)`)
  only *requires* the value: Create succeeds when VPP already has it (checked through the getter where one exists,
  otherwise `dfkit.ErrNotGlobalsOwner`), Delete is a no-op, Retrieve is write-only.
- Interfaces are named by their **logical name** and resolved with DF-1's `iface.ResolveName` (D-069): this owner's
  tag id first, then an untagged interface's VPP name; another owner's interface fails with
  `iface.ErrForeignInterface`, local0 never resolves. Objects on an **untagged** interface (a DPDK NIC) are recorded
  in the owner's ClaimStore (`iface.Claims`, shared with DF-1; P05/P08 install a persisted one) on Create, released on
  Delete, and reported by Retrieve only while claimed (D-071 claim rule).
- Deletes re-resolve the logical name right before acting by sw_if_index (never a Meta index — indexes are reused
  after a VPP restart) and first check that the object still exists (D-074); "already gone" is success.
- Retrieve never reports a key twice (`dfkit.Dedupe`).
- Restart simulation (fresh connection + fresh descriptors → empty plan; objects deleted via binapi → exactly their
  re-creation planned → empty plan again): `internal/descriptors/dfkit/restarttest`, output in `DF-8.md`.
