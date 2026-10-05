# BFD recovery repair checkpoint

Branch: codex/bfd-compensation-20261005. Worktree: /root/ngfw-wt/bfd-compensation-20261005.
Base: 686846c50a03315a4df87c214779aa884554e612, remote equivalent a37d57a7a72f5dce1220da2df3bc83980f7f42e0. Previous published checkpoint: 46b16da229646fb2fa58d515a2ac59059702aa34 (local d79597a1b). Current source belongs to this commit; new remote SHA follows successful publication. Owned files: see envelope.

Implemented: complete boot identity before native add; exact successful-add evidence written to existing persisted BootStore only after successful add and before claims; post-add cleanup uses boot/index-bound metadata; cleanup failure returns scheduler.PartialCreate; Retrieve reconstructs failed-claim orphans from durable proof after agent restart using one interface snapshot and at most one identity query per dump; Delete refuses boot/index reuse; FRR ms intervals reject uint32 microsecond overflow. Existing seven revive lint findings corrected without suppressions. Navigation tests still check every root link's visible accessible label and exact root href; obsolete BFD unavailable assertion replaced with actual available /routing/bfd expectation.

Actual checks:
- Initial focused test: descriptor 0.128s, agent 1.163s PASS.
- Final focused race: descriptor 1.361s, agent 2.264s PASS (.bfd-repair-focused-final.log); actual scheduler DEGRADED followed by persisted-store reopen/new descriptor/new scheduler cleanup tested for interface/endpoint claim and flag failures plus native deletion failure. EEXIST adoption and same-PID boot/index reuse refused; proof-write failure retains PartialCreate and no false durable claim.
- go vet descriptor/agent PASS (.bfd-repair-vet.log).
- First UI collection failed because fresh worktree package outputs were absent. Actual dependency build completed 12/12 tasks in 1m39s; no generated source drift remained.
- Final UI: 3 files, 19 tests PASS in 117.48s (.bfd-repair-web-final.log).
- Final focused golangci-lint PASS: `0 issues.` (.bfd-repair-lint-final.log). Initial lint exposed seven existing revive findings; all were corrected without suppression. No shared host native VPP or service modified.

Storage limit: if even the successful-add proof cannot be durably written, failed native compensation remains loudly partial/degraded with exact current-process metadata. A total storage outage cannot provide crash durability; no crash-durable pass is claimed for that case. Ordinary claim-write failure is restart-recoverable through the independent successful-add record.
Remaining merge prerequisites: fresh independent reviews of authored repair; unchanged complete quick on final integration tree and hosted gate. Prior R8 review applies to old frozen source only and is not approval of this authored repair.
No commands remain running. Next action: root integrates published repair into PR175 after archival source refs, fresh independent review and final CI.
