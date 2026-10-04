> Historical recovery record from merge `79fff64a` (2026-09-28). Current recovery verification is in `F-default-vpp-nics-wip.md`; historical completion and publication claims do not describe the current main.

# Contract change — F-default-vpp-nics (additive, D-164)

Manager review requested. All changes are additive; no existing field is renamed or reshaped.

## packages/schema
- `InterfaceSchema.physical?` — new optional sub-object `InterfacePhysicalSchema`
  (`packages/schema/src/domains/interfaces.ts`):
  - `pci`: `pciAddress`
  - `owner`: `z.enum(['dataplane','host']).default('dataplane')`
  - `builtIn`: `z.boolean().default(true)`

  Presence of `physical` marks the row as a physical NIC seeded from the host inventory. `x-ngfw-ui`
  group `general`, order 7.
- New semantic validators in `packages/schema/src/semantic/default-vpp-nics.ts` (`defaultVppNicsValidators`, one
  anchored import + spread in `semantic/index.ts`; `semantic/dataplane.ts` is unchanged):
  - `dataplane.owner-consistent` — a `physical.owner: 'dataplane'` NIC's PCI must be in `pciWhitelist`/`devices`; a
    `'host'` NIC's must not be. Pointer `/interfaces/<name>/physical/owner`.
  - `dataplane.physical-name-matches-device` — `dataplane.devices.<pci>.name` equals the interface key (D-069) for a
    dataplane-owned physical row, and one NIC is at most one physical row (`/interfaces/<name>/physical/pci`).

## packages/proto — `ngfw/v1/dataplane.proto`
Numbers taken as next-free (wave-BC; wave-A §2 marks `Interface` 20–29 for waves B+):
- `Interface.physical = 24` → new message `InterfacePhysical { pci=1, owner=2, built_in=3 }`
  (mirrors `interfaces.<name>.physical`; drift guard `contracttest/TestSchemaProtoDrift` green).
- New read-only RPC in `service Dataplane` under the `// wave-BC: F-default-vpp-nics` anchor:
  `rpc HostNics(HostNicsRequest) returns (HostNicsResponse)`.
- New message section `// ----- F-default-vpp-nics -----` at the end of the file:
  - `HostNicsRequest { owner=1 }`
  - `HostNicsResponse { nics=1 (repeated HostNic), owner=2, retrieved_at=3, management_notes=4 }`
  - `HostNic { netdev=1, pci=2, driver=3, mac=4, is_management=5, bound_to_dpdk=6, link_up=7 }` — `netdev` is empty
    for a NIC already bound to a DPDK driver (enumerated from `/sys/bus/pci/devices`); `bound_to_dpdk` comes from the
    PCI function's driver (`vfio-pci`/`uio_pci_generic`/`igb_uio`), or a live VPP `dpdk` interface with the NIC's MAC
    (bifurcated drivers). Semantics: `docs/contracts/proto.md` "F-default-vpp-nics: HostNics (+ Interface.physical 24)".

## Regenerated (never hand-edited)
`pnpm gen` regenerated: `apps/agent/gen/ngfw/v1/*`, `packages/proto/gen/ts/ngfw/v1/dataplane.ts`,
`packages/yang/generated/ngfw-interfaces.yang`, `packages/api-client/src/generated/schema.d.ts` (also after the
`/state/interfaces` view flags `physical`/`builtIn`/`awaitingDataplane` and the schema help-text changes).

## Verification
- `pnpm -F @ngfw/schema build` — OK
- `go test ./internal/contracttest/` — `ok` (schema↔proto drift green with the new field)
- `go build ./...` (apps/agent) — OK
