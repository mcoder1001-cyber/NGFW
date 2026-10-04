# Eight review-task recovery — 2026-10-04

Recovered source from preserved local Git objects rather than replacing current main with historical history. Owner explicitly authorized recovery and final merge without complete CI. No workflow gates were weakened.

## Source recovery

- Default NICs: historical merge 79fff64a, adapted additive contract/agent/API/UI. Seeding remains default off; management NIC protection and physical-row ownership controls retained.
- Classify zero-fill and sentinel: historical merges ada90234 / 75d12c1f, adapted sanitizer/fixtures and owned global sentinel.
- det44: 55a18e0f..4092b9d5 plus current recovery safety correction. Leftover arc reconciliation and desired projection filter retained. Unsafe full DS-Lite topology acceptance is parked before host mutation.
- Alarm restart rebuild: 1aa6813b, current NGFW fixture and state recovery with removed/disabled-rule clearing.
- Missing task prompts: 4343ccd9; existing current prompts are preserved, missing historical prompts restored with NGFW paths. Historical split proposals are documentation only, not new product work.
- NAT46/OSPF host: restored evidence drivers, OSPF topology test and historical evidence. Added current failure assertions and fail-closed OSPF root-mode preconditions.

## Current validation

- Alarm restart + alarm engine: 12 tests PASS; API TypeScript check PASS.
- OSPF topology source compiles (integration opt-in disabled); NAT46 unit tests PASS.
- Restored shell driver syntax and ShellCheck error-level check PASS.
- OSPF historical evidence secret scan: no leaks.
- Further component results and independent review are recorded in individual recovery reports.

Complete CI is NOT RUN by owner instruction. Historical packet evidence is historical. Real hardware, FRR/VPP packet forwarding, restart and rollback acceptance is NOT RUN for the recovered integration tree unless explicitly recorded below.

## Integration review

Independent reviewer approved classify recovery and the combined product source. Det44 reviewer identified prohibited historical DS-Lite pool deletion; full unsafe topology test now skips before any host mutation, retaining pool-free arc tests. NIC reviewer identified PCI device-key casing mismatch; matching actual keys without case sensitivity and an uppercase-key release regression fix it.

Scoped real NAT46 descriptor command: `NGFW_INTEGRATION=1 go test -count=1 -v -timeout 90s -run '^TestNat46OnHost$' ./internal/descriptors/nat46` with `tools/lab env 17`: PASS, real VPP, create/retrieve/idempotent apply/delete/cleanup, 0.086s. An initial invocation without slot env failed closed before creating objects; corrected invocation passed. Packet/restart/API acceptance is not inferred from this result.

Six implementation/documentation rows record source integration; NAT46/OSPF host rows remain parked for remaining live acceptance.

Final combined API scope: 23 tests PASS (NIC seed/drift + alarm state/engine). Restored OSPF FRR live test was run on slot11 with `NGFW_INTEGRATION=1 go test -count=1 -v -timeout 90s -run '^TestOSPFLive$' ./internal/renderers/frr/ospf`: PASS, 41.597s, real FRR10.7.1, adjacency/state readers/50-prefix redistribution/withdrawal/neighbour-loss event/config removal. This FRR-only test does not prove VPP FIB or packet forwarding.
