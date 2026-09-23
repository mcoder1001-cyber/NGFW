# F-startup-gen — questions for the manager / product owner

## Q1 — contract: four dataplane fields are D-055 stand-ins (needs a `contract/…` branch or P03b)
The Zod `DataplaneSchema` is a `strictObject` and the proto `DataplaneConfig` has only workers / corelist / mainCore /
rxQueues / txQueues / hugepagesGb / pciWhitelist. The generator also needs, and today reads from the JSON document only:

| JSON path | type | why |
|---|---|---|
| `dataplane.managementPci` | `pciAddress[]` (max ~4) | the management NIC must always be blacklisted and never a `dev` |
| `dataplane.devices` | record keyed by PCI → `{ name?, rxQueues?, txQueues?, rxDesc?, txDesc? }` | D-069 logical names (`dev <pci> { name lan }`), per-NIC RSS |
| `dataplane.buffersPerNuma` | int ≥ 1 | NUMA-aware buffers (`buffers { buffers-per-numa }`) |
| `dataplane.plugins` | record `<file>_plugin.so` → boolean | D-060 plugin enable/disable list |

Until the schema carries them, the API rejects a document containing them (strictObject), so the generator can only be
fed from a hand-made JSON file. Proposal: additive fields with these exact names/shapes (records keyed by natural key,
D-045/D-053); the schema-level checks mirroring `BuildModel` (logical-name regex, mgmt ∉ devices, PCI canonical
uniqueness) belong in `semantic/dataplane.ts`. `pciWhitelist` could later be deprecated in favour of `devices`.

## Q2 — NIC → port-group mapping on vrx-a (product owner)
Still unknown. The six-NIC golden uses a clearly marked SAMPLE: `0000:04:00.0 wan`, `0000:0c:00.0 lan`,
`0000:13:00.0 dmz`, `0000:14:00.0 p2p`, `0000:1b:00.0 lan2`, `0000:1c:00.0 sync`. The real mapping only changes the
input document, never the code.

## Q3 — who calls the Renderer?
`vppstartup.Renderer` implements `renderers.Renderer` so the commit engine can Render+Validate in a dry run and report
"restart required" / validation errors; `Apply` always returns `ErrManagerStep`. P05/P08 must therefore **not** put it in
the apply list (or must treat `ErrManagerStep` as "restart pending", not as a commit failure). Please confirm, or say if
the generator should stay CLI-only.

## Q4 — `tools/lab provision` has its own shell template for remote VMs
`render_startup_conf()` in `tools/lab` (P04) renders startup.conf for remote vrx VMs by hand (no plugins block, one mgmt
blacklist). Suggest a follow-up on its owner to call `vrx-startupgen` instead, so there is one generator (not in my file set).

## Q5 — isolcpus rule interpretation (decided, see F-startup-gen.md D-SG-3 — overturn if wrong)
The prompt says "workers + main core … disjoint from isolcpus rules". Implemented: when the host isolates CPUs, worker
cores must be **inside** the isolated set and the main core **outside** it (housekeeping). The literal reading (all VPP
cores disjoint from isolcpus) would forbid the usual layout of pinning workers onto isolated cores.
