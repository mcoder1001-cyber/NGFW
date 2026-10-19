# A3 independent ruling: final fake-host harness resource dispute

Date: 2026-10-07. Same frozen product: `9d1a0291d7e3d342a5b56047e68e039bec82845b`; comparison main `e74c33ebd2c083ef45d733b8d4494a78eb67edd9`. New disputed occurrence, separate from the earlier App timeout ruling. Arbiter owns reports only; no product source, assertions, deadlines, test configuration, scripts, live services or real VPP touched.

## Preserved failed-gate evidence

Read root whole-gate log `/root/ngfw-wt/logs/interfaces-final-integration-quick-retry.log` and individual `13-apply-startup-shard*.log` under `/root/ngfw-wt/logs/ci/interfaces-integration-20261007-20261007-112235-2901409/`. The original gate is FAILED, not waived. It passed all 35 Turbo tasks before the final fake-host harness. Failed assertions: shard 4 scenario 4 handover-done expected acceptance, actual exit 3; shard 2 scenario 6 reviewed-rendering refusal expected, actual locks-not-held refusal; shard 3 scenario 11 expected commit, actual rollback because neighbour state NONE. Shard 1 stopped at scenario 29 without a completion summary. Manager reports 113 other harness assertions passed. No successful subset is claimed as a successful whole gate.

`git diff --exit-code e74c33ebd2c083ef45d733b8d4494a78eb67edd9 9d1a0291d7e3d342a5b56047e68e039bec82845b -- deploy/vpp` exited 0: deploy scripts and fake-host harness unchanged. Independently queried hosted run 37611599559: completed SUCCESS on exact product SHA, including the mandatory complete quick gate.

## Resource observation and limits of causal evidence

Before this follow-up, this arbiter independently observed /tmp at 1,048,576 used inodes, zero free. Manager reports freeing 5000 regular regenerable files solely under node-compile-cache. Independently observed /tmp now 1,043,576 used, 5000 free, and root-backed /iac with millions free. Arbiter deleted nothing.

Fake systemd-run in the unchanged harness starts units using `setsid env -i` with only explicit setenv values; inherited TMPDIR is not automatically retained. This establishes that parent TMPDIR does not universally isolate child tools from default /tmp. The retained failed shard summaries do not contain direct ENOSPC output for these failed assertions. Lock acquisition and neighbour checks have bounded command deadlines; scheduling/process resource delays are also possible. Therefore inode exhaustion is a verified environment problem and plausible contributor, but **the exact original failure mechanism is unproven**. No claim is made that all three assertion failures were directly caused by ENOSPC.

## Own bounded reproduction after environment relief

Use unchanged own-worktree harness and the manager-built exact-final-tree generator binary, no rebuild or script edits:

```sh
TMPDIR=/iac NGFW_TEST_ONLY='4 6 11 29' bash deploy/vpp/test-apply-startup.sh /root/ngfw-wt/logs/ci/interfaces-integration-20261007-20261007-112235-2901409/ngfw-startupgen
```

Two sequential executions. This is a diagnostic selection, explicitly not the complete harness or quick gate.
Rerun 1: exit 0; 18 assertions passed, 0 failed; scenario 29 bounded rollback completed in 6 s. Log `/root/ngfw-wt/logs/interface-navigation-arbiter-deploy-rerun-1.log`.
Rerun 2: exit 0; 18 assertions passed, 0 failed; scenario 29 bounded rollback completed in 6 s. Log `/root/ngfw-wt/logs/interface-navigation-arbiter-deploy-rerun-2.log`.

## Ruling and required next action

**ENVIRONMENT / requeue mandatory gate; no reproducible product defect established.** Both own sequential unchanged selected-harness executions passed all 18 assertions after verified environment relief. Resource exhaustion is independently observed; assigning these precise original failures to that resource has only inferential support. Classify the gate execution as environment-sensitive/nonreproduced rather than declaring a proven code defect or claiming proven ENOSPC causation. Owner: integration manager for CI filesystem recovery/requeue; CI test maintainers / integration manager for stability diagnosis if recurrence. Follow-up due 2026-10-08. This task-specific ruling introduces no global CI relaxation.

Own `TMPDIR=/iac tools/ci.sh check --base origin/main` passed unchanged (17 s); not a whole quick gate. Apply ARBITER-PROMPT disputed test environment procedure, owner AGENTS real-code-failure/mandatory-CI requirement, and contributing complete unchanged quick gate requirement. Root owns environment recovery and exact-tree whole-gate rerun. Never weaken or select checks for the merge gate. Preserve failure evidence, run the complete unchanged gate, require `CI GATE PASSED`, required hosted checks green and expected heads current before merge. If failures recur, investigate retained per-case raw logs rather than dismissing them merely because scripts are unchanged.

Suggested manager-owned arbitration-log row:

| 2026-10-07 | A3 | interfaces-discovery final integration | tester environment dispute | Final unchanged fake-host shards failed assertions / one interrupted shard | ENVIRONMENT: requeue after verified resource recovery; exact cause unproven; two own diagnostic reruns 18/18 PASS each | ARBITER-PROMPT tester dispute procedure; contributing mandatory gate; owner AGENTS | Integration manager owns filesystem recovery and unchanged whole quick; CI test maintainers stability investigation if recurrence, due 2026-10-08 |
