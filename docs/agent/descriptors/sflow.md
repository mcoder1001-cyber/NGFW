# sflow plugin descriptors (DF-8, WBS D7.6)

Package `apps/agent/internal/descriptors/sflow` — random packet sampling: global parameters and sFlow on an
interface. Export to a collector is hsflowd's job (out of scope). Message names only from `apps/agent/binapi/sflow`.
`sflow.Register(registry, client, owner, opts...)`; option `WithInterfaceKey`.

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
- Ownership: interfaces by owner tag. The global singleton is VPP-global: the host test skips when not at defaults and
  restores the defaults in Cleanup.
