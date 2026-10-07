# Independent R4 BFD compensation

Exact SHA 0912488609545fec12a329f6f801d00a7cd77231, 2026-10-05. Reviewer branch codex/review-r4-seven-20261005 owns reports only. Envelope: independent R4/T3, no product edits, shared host/VPP mutations or real process kills. Exact archive copied apps/agent and proto dependencies into reviewer-owned .review-fixtures/bfd.

Inspected bfd.go: Create requires complete VPP identity; successful native add precedes durable recovery proof, which precedes interface/endpoint claims. Failed add never creates proof or compensates a foreign existing tuple. Later failure attempts exact cleanup and failed cleanup returns PartialCreate. Retrieve reconstructs persisted failed-claim orphans with one interface snapshot and at most one identity query. Delete validates metadata/record exact tuple, boot identity and logical interface index before authorizing native deletion. Expired boot/index proof does not authorize reused tuple removal. Total proof-storage outage explicitly remains degraded/in-process only, not falsely crash-durable.

Inspected subsystems/bfd.go and bfd_multihop.go: enable is supplied only for globals owner; endpoint claims use owner/boot-bound KeyedClaims, reject ambiguous tuples and preserve foreign claims. Generated binapi/bfd includes BfdUDPEnableMultihop with empty request; there is no invented boolean disable field. One-way activation is explicitly documented; no disable-on-rollback claim. Assigned ID range is passed through w.IDRange()/WithIDs. Diff of four BFD commits shows no binapi edits. No VPP C code edited. Shared-host multihop activation must remain manager-only opt-in under globals lock; this reviewer never activated it.

Actual reviewer commands from snapshot apps/agent (heavy wrapper path ../../../../tools/heavy.sh):
```
go test -count=1 ./internal/descriptors/bfd ./internal/desired -run 'BFD|Bfd|Session|Multihop|Recovery'
ok ngfw/agent/internal/descriptors/bfd 0.622s
ok ngfw/agent/internal/desired 0.144s

go test -count=1 ./internal/subsystems -run 'KeyedClaims|Bfd|BFD'
ok ngfw/agent/internal/subsystems 0.586s

go test -count=1 ./internal/agent -run 'TestBfd|TestFRRBfd'
ok ngfw/agent/internal/agent 0.484s
```
All exit0; NGFW_INTEGRATION was not enabled. Includes persisted reopen/new scheduler recovery, foreign-add refusal, same-PID boot/index reuse guards and multihop apply/state/restart/rollback tests. Initial command mistakenly named nonexistent internal/claims and internal/vpp/globals packages: setup FAIL for those two paths; descriptor passed0.127s. Corrected actual package runs above passed. That invocation error is not a product failure.

Findings: none in R4 scope. Verdict: APPROVE for ownership/recovery implementation. Actual private VPP multihop peer/runtime acceptance not executed; defer that execution explicitly, not implementation defects.
