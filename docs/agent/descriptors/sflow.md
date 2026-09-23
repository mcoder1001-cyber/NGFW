# sflow plugin descriptors (DF-8, WBS D7.6)

Package `apps/agent/internal/descriptors/sflow` — random packet sampling: global parameters and sFlow on an
interface. Export to a collector is hsflowd's job (out of scope). Message names only from `apps/agent/binapi/sflow`.
`sflow.Register(registry, client, owner, opts...)` (+ `RegisterGlobals` for `sflow.global`); option `WithInterfaceKey`.

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `sflow.global` (singleton) | `sflow.global/global` | `sflow_sampling_rate_set`, `sflow_polling_interval_set`, `sflow_header_bytes_set`, `sflow_direction_set`, `sflow_drop_monitoring_set`; delete = VPP defaults | the five `*_get`, reported while ≠ defaults (10000 / 20 s / 128 B / rx / off) | in place | — |
| `sflow.interface` | `sflow.interface/<ifname>` | `sflow_enable_disable` enable=1 / 0 | `sflow_interface_dump` + learned/probed mapping (below) | re-apply | `interface/<ifname>`, `sflow.global/global` optional |

Value fields: `Global{sampling_rate (0 = off), polling_interval, header_bytes (64..256, step 32 — VPP rounds other
values silently, so Validate rejects them), direction rx|tx|both, drop_monitoring}`; `Interface{interface}`. Meta:
`InterfaceMeta{SwIfIndex, HwIfIndex}`.

## The hw_if_index gap (VPP API)
`sflow_enable_disable` reads a **sw_if_index** from its field named `hw_if_index`, while `sflow_interface_details`
reports the real **hw_if_index**, and no VPP 26.06 API maps a hw_if_index back to a sw_if_index (only sflow, gtpu,
vxlan offload and flow messages carry hw indexes). The descriptor therefore:
1. learns the mapping at Create (dump before/after the enable);
2. in Retrieve, maps learned hw indexes directly; when enabled hw indexes remain that it has not learned (agent
   restart, or another owner's interfaces), it **probes the owned, non-sub interfaces** not yet known to be enabled
   with `sflow_enable_disable(enable=1)`: `VALUE_EXIST` = enabled (reported); success = it was disabled, and it is
   disabled again at once. Only this agent's own interfaces are probed, only while unlearned indexes exist; one
   unambiguous result is learned. All calls of the descriptor are serialised.
Proposed VPP fix (for `docs/vpp-code-track.md`): add `sw_if_index` to `sflow_interface_details`.

## Notes
- VPP answers a redundant enable/disable with `VALUE_EXIST`; Create and Delete treat it as success.
- Ownership: this owner's tagged interfaces or claimed untagged ones (below). The global singleton is VPP-global: the host test skips when not at defaults and
  restores the defaults in Cleanup.

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- `sflow.Register(...)` registers the per-owner object types (`sflow.interface`); `sflow.RegisterGlobals(...)` registers the
  VPP-global singletons (`sflow.global`) constructed as **globals owner** — P08 calls it only in the designated globals
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
