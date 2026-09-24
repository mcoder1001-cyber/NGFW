# Descriptors — mactime plugin (F-bridge-l2)

Package `apps/agent/internal/descriptors/mactime`, built on `descriptors/dfkit` (D-077): values are dfkit structpb specs
(`Device`, `Enable`). `mactime.Register(r, client, owner, store)` — `store` is the owner's persisted D-076 BootStore
(`subsystems.Wiring.BootStore()`, never in memory in the product).

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `mactime.range` | `mactime.range/<name>` | — | `mactime_add_del_range` (add / del by MAC); Retrieve `mactime_dump` (raw stream: details, `mactime_dump_reply`, control-ping reply — V-new (F-bridge-l2) a) | delete + add (an add on an existing MAC **appends** ranges in VPP); MAC or name change = ErrRecreate | One device of the VPP-wide device table, keyed by MAC in VPP. Ownership = device name `<owner>:<name>` (D-071: only owner-prefixed devices are read or deleted). `drop` false = allow (inside the ranges when there are any, else always), true = drop. Ranges are seconds since Sunday 00:00 of VPP's mactime clock, sorted (Normalize). Create adopts nothing but VPP's own learned entry for the MAC (`mac-<mac>`, static allow, no ranges), which it replaces; a MAC under any other name → `dfkit.ErrNotOurs`. |
| `mactime.enable` | `mactime.enable/<interface>` | `interface/<name>` (D-065) — so the filter is disabled before the interface goes (V19 family) | `mactime_enable_disable`; readback `feature_is_enabled("device-input", "mactime", sw_if_index)` | none (the value is the interface) | Hardware interfaces only (VPP rejects sub-interfaces). VPP stacks the feature on every enable, so the enable is applied **once per VPP boot**: the applied-once record (D-076/D-080) holds `<sw_if_index>/<logical name>` under the boot identity. Retrieve reports an interface only when that record matches this boot, index and name **and** `feature_is_enabled` is true (the readback alone is unreliable, V23 a). A Create that finds the feature "on" without a record disables once, then enables (normalises a lost record, and is a no-op for an index the arc never reached). Delete re-resolves the name (indexes are reused), disables once, forgets the record, releases the claim on untagged interfaces. |

Projection (F-bridge-l2, `internal/desired/l2.go`): `routing.l2.macFilters.<name>` → `mactime.range/<name>` (one VPP range
per listed day: `day*86400 + HH:MM`), `interfaces.<if>.l2.macFilter: true` → `mactime.enable/<if>`. Retrieve groups the
per-day ranges back by time window (days in `mon … sun` order, groups by first day, then start, end).

Unit tests (`mactime_test.go`, fake VPP modelling the appending add, the stacking enable and the out-of-range readback):
`TestDevice`, `TestEnableAppliedOnce`. Host evidence: `TestMactimeOnHost` (`VRX_INTEGRATION=1`) and the topology test
`test/topology/bridge-l2` — `docs/status/tasks/F-bridge-l2.md`.
