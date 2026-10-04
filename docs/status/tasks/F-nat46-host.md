# Recovery note — 2026-10-04

The historical report below preserves previous commands and outcomes. Its historical PASS/BLOCKED claims do not certify current main or current host readiness. This recovery adapts names to NGFW; current results and deferred acceptance are recorded in eight-review-recovery-20261004.md.

# F-nat46-host — host evidence owed by F-nat46 (slot 17, `w17`, agent level only)

Branch `task/F-nat46-host`, base `main@4741ff86`, worktree `/root/ngfw-wt/F-nat46-host`. Owed section: `docs/status/tasks/F-nat46.md`
"Not done / remains" (+ "Remains (host)" of part B). Slot 17 has no Valkey database → no API stack: the real `ngfw-agent` on the slot
socket driven with `ngfw-agentctl` (protobuf-JSON documents exactly as the API sends them) on the `tools/lab` af_packet rig.

## Status: BLOCKED on D-211 (INC-vpp-hang-20260928) — VPP hung since 2026-09-28 19:13:04; row parked until the owner restarts VPP

Manager confirmed 2026-09-29 08:17: the restart is an owner action (D-012); this row is parked and will be respawned with CONTINUE
after the restart. See `F-nat46-host-questions.md` Q1 (pasted facts). VPP's process is alive but neither `vppctl` nor the binary API answers
(`os_panic() called, aborting.` in the journal, no exit, NRestarts=2). Only the manager restarts VPP (D-012). Everything owed is on this
VPP, so no host evidence could be produced in this run. Delivered instead:

- `docs/status/tasks/F-nat46-host-evidence/host.sh` — the complete evidence driver (steps 0–7 below), syntax-checked, ready:
  `cd /root/ngfw-wt/F-nat46-host; eval "$(tools/lab env 17)"; docs/status/tasks/F-nat46-host-evidence/host.sh run` (~3 min; needs
  `/tmp/g-w17/bin/{ngfw-agent,ngfw-agentctl}` = `go build -C apps/agent -o /tmp/g-w17/bin/ ./cmd/ngfw-agent ./cmd/ngfw-agentctl`).
- questions Q1 (incident + ask), Q2 (API e2e / screenshot not possible on slot 17 → T4 on the main stack, D-175), Q3 (D-210 reading).

## Owed checklist → command → output

| # | owed item | driver step | result |
|---|---|---|---|
| 1 | `NGFW_INTEGRATION=1 go test -run TestNat46OnHost ./internal/descriptors/nat46` (26.06 accepts /32↔/128, ea_bits_len 0) | step 1 | not run — VPP hung |
| 2 | `vppctl show map domain` after a commit | step 1 (pause) + step 3 | not run — VPP hung |
| 3 | af_packet packet test IPv4 client → IPv6-only server (`path: af_packet`) | step 4 | not run — VPP hung |
| 4 | restart (≤ 30 s) evidence | step 5 | not run — VPP hung |
| 5 | rollback evidence | step 7 | not run — VPP hung |
| 6 | duplicate IPv4 service address → refusal with `pointer` (agent-level stand-in for the API 400) | step 6 | not run — VPP hung |
| 7 | API e2e on the host PostgreSQL, NAT46 tab screenshot | — | not possible on slot 17 (no Valkey db, no API stack); T4 after merge (D-175) |

NRestarts before (task start): `NRestarts=2`. NRestarts after: `NRestarts=2` (no restart happened; VPP still hung at the end of the run).

## Windows held
None. No VPP-global setting is needed by NAT46 (map params untouched); the driver holds only the shared lab lock (fd 9, closed in
every background child) — no globals lock, no exclusive window.

## Shared hunks
None — `files_owned` is `docs/status/tasks/F-nat46-host*` only; no product code touched.

## Out of scope / not done
Everything in the owed list above until VPP is restarted by the manager. Stateful NAT46, mapnat, schema/proto/API/UI unchanged.

## Decisions
- D-210 read as: the existing `TestNat46OnHost` run stays as owed host evidence (feature output); no new test code.
- Duplicate-IPv4 refusal evidence is produced at the agent level (`ngfw-agentctl dryrun` → `ValidationReport` with `pointer`) because the
  slot has no API stack; the API 400 problem+json path was verified by F-nat46 part B's unit/e2e code and stays owed to T4.

## CI gate
`plan/NO-TESTS` on main (D-210) → compile-only gate. Run after merging current main, at load < 30: see the tail below.

```
(pending)
```

## Open questions
Q1–Q3 in `F-nat46-host-questions.md`.
