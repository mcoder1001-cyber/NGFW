# TD-7 — WIP (apply-startup.sh follow-ups from the TD-6 review, D-116)

Started 2026-09-24 16:09 (+0330), time box 2 h. Slot 6. Base: task/TD-6@621d1e2 (speculative, D-114).
Never `--apply` for real; fake-host harness only. Scope: F1 and F2 of `TD-6-review.md`.

| item | state |
|---|---|
| F1 per-apply rollback lock + cancel the armed dead-man first + refusal points at `systemctl start <dead-man>.service` | done |
| F1 scenarios 42/43 + refusal check in 32: fail on TD-6's script (6 FAIL), pass on the branch | done (16:54–16:57, continue) |
| F2 scenario 40: count the fake's `ip neigh` reads: main's script 6 reads (FAIL), branch 1 read | done |
| full harness (4 shards; 2 load misses at load 168 green on serial rerun), `tools/ci.sh --base main` (fresh cache dir): CI GATE PASSED | done |

Host load at start: 182 (1-min), other agents' CI.

Continued 2026-09-24 16:53 after the usage-limit stop (CONTINUE-quota.md). The 16:35 comparison logs of the stopped worker ran
scenarios 1–31 only (misconfigured); redone with the new harness + `VRX_TEST_APPLY_SCRIPT`, logs in /tmp/g-td7/runs.
Manager scope addition (vppstartup.md scenario-40 sentence; two pre-existing tech-debt items noted in TD-7.md): done.
Finished 17:45 — see TD-7.md.
