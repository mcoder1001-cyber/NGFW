# R1 combined integration review

Reviewed exact integration head: `6eda5e94acca007bd290e37ab74d53ba45157e30`.

Final source branch comparisons used LDP `7e897472bf85a4a4d5b98ce41399be1466d7ccc4`, PIM `6b7c82df784f24516c8e5dc5431d7f0dffdc8bb5`, and detectors `5302fc6d30e3d9effc90b6a9888869c4160626f7`. Independent feature reviews remain preserved in their respective task reports.

Compared every changed source/test file under `apps/`, `packages/` and `test/` against its final source branch: 13 LDP files, 11 PIM files and 13 detector files. Detector blobs match exactly. All individual LDP and PIM blobs match except the two shared files. `desired/bgp.go` is the exact additive union of both branches' FRRDoc projections; neither projection is lost or altered. The single manual conflict resolution in `subsystems.go` preserves `registerPim` and `registerMplsLdp` immediately after `registerP12`, with both errors propagated. No additional manual product-source changes were observed.

No unresolved correctness findings in the combined product tree. Scoped tests exercise both registrations/projections, final PIM outage/recovery and snapshot cap behavior, final LDP retrieved installed counts (including Retrieve failure and missing reader), and detector observation streaming. Both dynamic sources retain their own descriptor/cache namespaces.

Independent actual verification in this integration worktree:

```text
go test -race -count=1 ./internal/subsystems ./internal/agent -run 'Test(Pim|Ldp|AutoBlockObservation|JournalObservation)'
ok ngfw/agent/internal/subsystems 1.113s
ok ngfw/agent/internal/agent 1.122s

go test -race -count=1 ./internal/frrsync/ldp ./internal/frrsync/pim ./internal/detectors
ok ngfw/agent/internal/frrsync/ldp 1.234s
ok ngfw/agent/internal/frrsync/pim 1.106s
ok ngfw/agent/internal/detectors 1.029s
```

Complete local quick was not repeated: baseline and an independent earlier feature gate already reproduced platform Unix-socket `listen EPERM` and ownership `chown EINVAL` failures. No full quick pass, real FRR/VPP forwarding, or live topology acceptance is claimed. The complete hosted quick gate remains mandatory before integration/merge.

Verdict: APPROVE for R1 correctness at the exact stated integration head; merge remains conditional on the mandatory complete hosted quick gate.
