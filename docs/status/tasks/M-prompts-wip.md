# M-prompts — WIP

started: 2026-10-01 16:55 UTC (time box 3h → stop by 19:55 UTC)
branch: task/M-prompts  worktree: /root/ngfw-wt/M-prompts

## Plan
16 rows (prompt ops/M-prompts.md). For each: read board row + cited review/questions/decision files + existing code on main, write the prompt.

## Done
- 17:12 written: prompts/fixes/S-capture-retention-stop.md, prompts/features/F-capture-trace-host.md, prompts/features/F-tunnels-host.md
- drafting in parallel (forks, same worktree, disjoint files): routing/HA (F-igp-followups split, F-pim-frrsync, S-vrrp-product-fixes), agent wiring (F-multiwan-wiring, F-pppoe-client-wiring, F-bruteforce-detectors), system/ops (F-dataplane-apply-flow, TD-19, LAB-vpp-per-slot, S-cli-ipsec), tests (TEST-traffic-A/B/C)

- 17:41 all 26 prompt files committed (a9590382); reviewed every fork file, fixed: multiwan-a hours, bruteforce-a proto field ref,
  LAB-a mem-canary (D-224), LAB-b open-row exclusions, TEST-C capture rule, rules blocks (D-128/heavy.sh/Files you own)
- 17:55 status file docs/status/tasks/M-prompts.md written (table, splits, board changes, overlap check, citations)

- 17:59 `tools/ci-slot.sh --base main` on c3b8d31d: CI GATE PASSED (11m49s); tail in the status file
- 18:01 `/tmp/g-w4` cleanup refused by the permission layer (final) → M-prompts-questions.md

## Next
- nothing: row finished (status file docs/status/tasks/M-prompts.md)
