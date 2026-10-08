# PPP DHCPv6 delegated LAN integration

Branch `codex/pppoe-pd-20261008`, base `db7eb9d`.

Contract checkpoint: additive explicit `delegationTargets` with LAN interface and
uint32 subnet ID; DHCPv6-only, enabled LAN, VRF, static IPv6/RA and duplicate owner
validation. Generated Go/TS use the repository generator with pinned CI versions.
A numeric uint64 design failed the JSON roundtrip test (ts-proto correctly emits
uint64 strings); uint32 preserves the existing numeric configuration contract.
Schema-driven interface form exposes targets, with nested English/Persian labels.

Focused schema tests: 10 PASS. Wire roundtrip test being rerun after uint32 fix.
No aggregate CI or host operation. This contract is not operational PD completion.

Remaining: enforce kernel carrier parent topology semantic rules; authoritative
lease generation/deadline parsing and tests; dynamic desired projection; durable
separate descriptor ownership/static exclusion; live readiness callback integration;
withdraw/renewal/rollback/restart regression tests; independent review.

DHCPv6 export names verified from upstream maintainer example:
https://github.com/NetworkConfiguration/dhcpcd/discussions/309
`new_dhcp6_ia_pd1_prefix1_pltime` and `_vltime` are used with existing new-event prefix.
Missing/invalid metadata must remove old PD, not extend an old lease. The existing
IPv6 admission token is 64 hex characters. The refresh generation alone does not
identify the configured carrier generation.

## Reviewed recovery and implementation checkpoint

Contract remote `0b68e0c30043cbe64a68dd1ca1cad9fd9b610d05`, tree
`7e71dfcd87ab18d79de257c50704c6c08072a5b7`, preserves the published carrier
foundation's extra review receipts. A resumed-turn history gap was resolved by
read-only preservation and explicit code review; current sole-writer branch is
`codex/pppoe-pd-recovered-20261008`. No unknown changes were discarded.

Implemented: kernel logical/raw-parent semantic validation; stable LAN desired keys;
current admission and DHCP lease deadline parsing; rejection/withdrawal of invalid
renewals and old admission events; pure desired projection; complete assignment
validation against static IPv6/RA and other live WAN assignments; dedicated dynamic
address/prefix/RA descriptor instances using existing core/ip6nd operations; durable
ownership partition and static Retrieve/Create exclusion; dependency ordering;
verified deletion; cached polling source and fail-closed readiness integration seam.
RA claims are recorded before mutation; errors with observed/uncertain partial state
provide rollback handles. Ownership file and parent directory are synchronized.

RA lifetimes round down to 30-second budgets, so they never exceed the DHCP lease.
Assignments conservatively withdraw up to 29 seconds before preferred expiry,
rather than using the static RA descriptor's zero-means-default lifetime behavior.
The snapshot Run loop polls each second; Desired performs no daemon/VPP I/O.

Focused results to this point:
- Schema semantic: 12 PASS; protobuf/schema JSON+wire roundtrip: 1 PASS.
- Interface form model: 7 PASS, including editable target array and Persian nested labels.
- Renderer admission/event/expiry/golden: 4 tests PASS under race, 1.387 s.
- Real scheduler with stateful fake VPP: product lifecycle and static-adoption refusal
  PASS under race, 1.147 s. Covers address+RA application, ownership-separated static
  retrieval, fresh wiring restart, injected renewal failure/rollback and withdrawal.
- Existing RA configuration/prefix lifecycle plus PD tests after claim-first changes:
  PASS under race (subsystems 1.121 s, ip6_nd 1.016 s).

Integration seam: parent carrier registers
`w.registerPppoeDelegation(reg, rt.DelegationSnapshot)` and implements
`CarrierDelegationReady(logical, admission string) bool`. That method must require a
supported current Carrier session and verified forwarding readback for this exact
admission. Until it exists, DelegationSnapshot returns no assignments. The parent
projection must call `desired.PppoeDelegationTargets(fullDoc, logical)` and return
its errors before applying a public plan. This branch does not own carrier wiring.

Still required: final claim-failure/overlap tests, final type checks, independent
review and parent integration. A VPP rejection during withdrawal can retain an
owned object under the existing dynamic-source retry/quarantine policy; failures
are reported and retried, not claimed as successful withdrawal. No aggregate CI,
native DHCP/RA packet acceptance or complete PPP carrier claim is made.
