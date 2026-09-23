# F-startup-gen — contract change (D-081): four additive `dataplane` fields

Branch `contract/F-startup-gen` (from main@1ff8f3b). Additive only, after `contracts-v1`; no field renamed or reshaped.

| JSON path (schema) | proto (`DataplaneConfig`) | shape / bounds |
|---|---|---|
| `dataplane.managementPci` | `repeated string management_pci = 8` | `pciAddress[]`, ≤ 4, default `[]` |
| `dataplane.devices` | `map<string, DataplaneDevice> devices = 9` | record keyed by `pciAddress`; `DataplaneDevice { optional string name = 1; optional uint32 rx_queues = 2; tx_queues = 3; rx_desc = 4; tx_desc = 5 }` — name `[a-z](?:[a-z0-9_-]{0,13}[a-z0-9])?`, queues 1–256, desc 64–16384 |
| `dataplane.buffersPerNuma` | `optional uint32 buffers_per_numa = 10` | 1024–4194304 |
| `dataplane.plugins` | `optional PluginSet plugins = 11`; `PluginSet { map<string, bool> switches = 1 }` (D-084) | optional `{ switches: record keyed by <name>_plugin.so (≤ 64 chars) → boolean }` |

New semantic validators (`packages/schema/src/semantic/dataplane.ts`, 100 % coverage kept): `dataplane.devices-pci-unique`
(case-insensitive), `dataplane.management-not-dpdk` (also duplicate management entries), `dataplane.logical-name-unique`,
`dataplane.descriptors-power-of-two`.

`plugins` semantics (D-084, amending the first version of this branch where `plugins` was a bare map): **present** →
the document is authoritative, exactly `switches` are rendered (the generator warns when `linux_cp`, `linux_nl` or
`npt66` — D-060 — are not listed); **absent** → the generator keeps the switches of the current start-up file. The wrapper
message gives the field presence in proto3 (a bare map cannot tell absent from empty).

Touched outside the schema/proto: `packages/schema/src/domains/group-a.test.ts` — one expectation (`DataplaneSchema.parse({})`
now also returns the three new defaults), forced by the additive change (same kind as D-067).
`docs/contracts/schema.md` — dataplane table.

Verification (pasted in `docs/status/tasks/F-startup-gen.md` → Review fixes): drift guard `876 scalar leaves and 192 messages
compared, 4 accepted difference(s), 0 finding(s)` — before the change it would report the four new schema leaves;
`packages/proto` vitest 68/68; schema vitest --coverage 1214/1214 with thresholds; `tools/ci.sh --base main` on this branch.
