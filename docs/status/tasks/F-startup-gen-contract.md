# F-startup-gen — contract change (D-081): four additive `dataplane` fields

Branch `contract/F-startup-gen` (from main@1ff8f3b). Additive only, after `contracts-v1`; no field renamed or reshaped.

| JSON path (schema) | proto (`DataplaneConfig`) | shape / bounds |
|---|---|---|
| `dataplane.managementPci` | `repeated string management_pci = 8` | `pciAddress[]`, ≤ 4, default `[]` |
| `dataplane.devices` | `map<string, DataplaneDevice> devices = 9` | record keyed by `pciAddress`; `DataplaneDevice { optional string name = 1; optional uint32 rx_queues = 2; tx_queues = 3; rx_desc = 4; tx_desc = 5 }` — name `[a-z](?:[a-z0-9_-]{0,13}[a-z0-9])?`, queues 1–256, desc 64–16384 |
| `dataplane.buffersPerNuma` | `optional uint32 buffers_per_numa = 10` | 1024–4194304 |
| `dataplane.plugins` | `map<string, bool> plugins = 11` | record keyed by `<name>_plugin.so` (≤ 64 chars), default `{}` |

New semantic validators (`packages/schema/src/semantic/dataplane.ts`, 100 % coverage kept): `dataplane.devices-pci-unique`
(case-insensitive), `dataplane.management-not-dpdk` (also duplicate management entries), `dataplane.logical-name-unique`,
`dataplane.descriptors-power-of-two`.

`plugins` semantics: a map has no presence in proto3, so "absent" and "empty" are the same. The generator therefore
**overlays** the document's switches on the switches of the current start-up file (unlisted plugins keep their switch;
to turn one off set it to `false`). A document without `plugins` can never drop the D-060 block.

Touched outside the schema/proto: `packages/schema/src/domains/group-a.test.ts` — one expectation (`DataplaneSchema.parse({})`
now also returns the three new defaults), forced by the additive change (same kind as D-067).
`docs/contracts/schema.md` — dataplane table.

Verification (pasted in `docs/status/tasks/F-startup-gen.md` → Review fixes): drift guard `876 scalar leaves and 192 messages
compared, 4 accepted difference(s), 0 finding(s)` — before the change it would report the four new schema leaves;
`packages/proto` vitest 68/68; schema vitest --coverage 1214/1214 with thresholds; `tools/ci.sh --base main` on this branch.
