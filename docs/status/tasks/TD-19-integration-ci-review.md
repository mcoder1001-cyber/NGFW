# TD-19 integration and hosted fixture workflow review

**R1/R2/R5/R7: APPROVE bounded integration source** exact `def88309646f36109c0c5d6a0778d98e34a0ac60`, compared to base main `c193386a163d8ed35b1d23a1dcbe4c6434dca569`, independently reviewed 2026-10-02. A fresh composition onto the latest main and successful hosted full quick plus new fixture gates remain prerequisites to merge; this snapshot is not final published-head acceptance.

## Independent identity verification

Recursive tree comparison confirms all **4308 other existing main entries** have identical Git object/mode identities. The five changed existing entries are exactly the four reviewed provisioning scripts/tool and the central deferred-acceptance append. No existing quick workflow/tool, package manifest, board, other product feature or unrelated main file was overwritten. Approved production scripts, original VPP validator and strict fixture runner are byte-identical to `9f1f70196aebee2ea465e02b4eb38c3f89cfe3d7`. Manager delta adds only the dedicated provisioning workflow and central campaign appendix; review artifacts retain earlier findings and superseding bounded approvals.

## Workflow and acceptance audit

The new workflow uses Ubuntu 24.04 solely for offline redirected fixtures, explicitly executes the strict fixed-file runner, and cannot accept generic zero-test discovery. It has read-only contents permissions, the existing pinned checkout action revision, disabled persisted credentials, bounded runtime and per-ref concurrency. Tool availability checks install no packages. Pull-request/push path filters cover all five fixture scripts and runner, their three installer sources, tools/lab, original VPP validator/version inputs, agent Go module and existing CI generator/toolchain pin inputs. Fixtures use temporary data and stub package/remote commands; no job secrets, target services or production package installation are introduced. Existing full quick gate is untouched.

The central single campaign correctly labels all genuine downloads, exact seven-package remote install/readback, service suppression/handover and Ubuntu 26.04 boot/package acceptance **NOT RUN**. It explicitly distinguishes missing authoritative repository-key pins and other implementation reproducibility gaps from lab-only deferrals. No target success or full TD-19 completion is inferred from offline fixtures.

Checks: production/runner identity and full main-preservation tree assertions PASS; `git diff --check` PASS. No additional product tests repeated because this delta has no product/runner changes. Independent runner acceptance remains the exact unchanged 9f1 checkpoint's **23 PASS in 5.031 s**, zero skips, plus genuine seven-outcome and loader-refusal checks in `TD-19-fixture-runner-review.md`. Parent's current 23-test execution is separate corroborating evidence, not an independent hosted pass claimed here.

No MAJOR finding in this manager delta. No live package/service/SSH/lab operations performed. ShellCheck remains NOT RUN locally. Recheck final current-main composition and require actual fresh hosted gates at the published commit before merging; source checkpoint approval does not mark TD-19 DONE.
