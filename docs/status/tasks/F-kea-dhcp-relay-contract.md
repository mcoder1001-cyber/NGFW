# F-kea-dhcp-relay — contract changes (for the manager's review)

Committed on `task/F-kea-dhcp-relay` as a separate `contract(schema,proto): …` commit (envelope: workers do not
create `contract/` branches). Additive only; nothing renamed or reshaped.

## proto (`packages/proto/vrx/v1/dataplane.proto`)
- `rpc DhcpLeases(DhcpLeasesRequest) returns (DhcpLeasesResponse)` under the `// wave-A: F-kea-dhcp-relay` anchor of
  `service Dataplane` (framed by blank lines, the anchor stays a detached comment).
- New messages in the `// ----- F-kea-dhcp-relay -----` section: `DhcpLeasesRequest`, `DhcpLease`, `DhcpSubnetUsage`,
  `DhcpServerStatus`, `DhcpClientLease`, `DhcpLeasesResponse` (all `Dhcp*`, the feature's noun — hotspots §0 rule 5).
- Numbers: only the allocation of `docs/status/wave-BC-numbers.md` "Batch-2 follow-ons" (rpc `DhcpLeases` + its own
  messages). **`DhcpRelay` 9–10 are not used**: option-82 / remote-id is not built (no config gap for this task).
  No `ActionRequest` member, no `EventKind`.
- `buf lint` clean; generated Go (`apps/agent/gen`) and TS (`packages/proto/gen/ts`) regenerated with `pnpm gen`.
- `docs/contracts/proto.md` §11 "F-kea-dhcp-relay: DhcpLeases" documents the semantics.

## schema (`packages/schema`)
- No domain/leaf change (the `services.dhcp` model of P02c is complete for this task; the drift guard is unaffected).
- New rule file `src/semantic/kea-dhcp-relay.ts` (`keaDhcpRelayValidators`), one import + one spread under the anchors
  in `src/semantic/index.ts` (C2):
  - `services.kea-dhcp-relay-reservation-outside-pools` — a reserved address must lie outside every pool
  - `services.kea-dhcp-relay-one-vrf-per-family` — the enabled servers of one family share one VRF (one Kea process
    per family, RF-3)
  - `services.kea-dhcp-relay-relay-source-per-vrf` — relays of one family on one client VRF share one source address
    and never list a server twice (VPP keeps one src per rx VRF/family; proxy key rx VRF + server VRF + address)
  - `interfaces.kea-dhcp-relay-dhcp-client-no-static` — a (sub-)interface with `dhcpClient` has no static IPv4
- Tests: `src/semantic/kea-dhcp-relay.test.ts` (the four rules + the existing P02c DHCP rules the feature relies on).

## api (P5, typecheck)
- `apps/api/src/testing/fake-agent.ts`: one `dhcpLeases` handler line under the anchor (UNIMPLEMENTED stub in the
  contract commit; later the same line points at `features/kea-dhcp-relay/fake.ts`).
