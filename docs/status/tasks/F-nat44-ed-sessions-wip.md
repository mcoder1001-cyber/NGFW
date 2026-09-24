# F-nat44-ed-sessions — WIP log (slot 4, started 2026-09-24T17:27)

| time | state |
|---|---|
| 17:45 | contract(proto) committed: NatSessions/NatSummary RPCs, ActionRequest 5 nat_session_kill; next: adjacent-pool rule, desired/nat.go |
| 17:50 | contract(schema) adjacent-pool rule committed |
| 18:16 | agent: desired/nat.go builder + assembler, subsystems/nat44_ed.go, projection hooks, coretest nat44-ed model, actions pager, rpc_nat44_ed.go; unit tests green; W-seed@df67a8e merged (manager safety update) |
| 18:42 | API module + e2e (fake agent) green; api-client + CLI table regenerated (contract(api-client)) |
| 18:50 | UI delegated to a fork sub-worker (apps/web only) |
| 19:00–19:15 | topology runs 1–3 on slot 4: config/validation/Retrieve/vppctl OK; found the rig's netns tx-checksum-offload vs NAT issue (V-new, Q7) and the fixture's 1024-session limit; fixed test side |
| 19:25 | topology run 4 (full) running |
