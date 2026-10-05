# T1 — complete unchanged quick gate

Source: `dfa9934053047738ecb6f649f9d38455c3a66c5a`; tree `53ad32352e21300e6e1a7a018c113e3e83427a34`; base `d6e1646cdfe57168dcb0e4ebf66c87026bc58863`. Independent worktree `/root/ngfw-wt/vpn-warning-r1-t1-20261005`, branch `codex/vpn-warning-r1-t1-20261005`. Date 2026-10-05. No product edits, host service mutation or packet claims.

Command:

```sh
GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_CI_TASK_CONCURRENCY=2 NGFW_CI_APPLY_SHARDS=2 tools/heavy.sh tools/ci.sh quick --base d6e1646cdfe57168dcb0e4ebf66c87026bc58863
```

Exit 0; wall time 30m15s; source remained clean through completion. Complete output is in `BUG-vpn-capability-warning-test-T1-output.log`. Step logs: `/root/ngfw-wt/logs/ci/vpn-warning-r1-t1-20261005-20261005-183255-430660`. Generation clean, all guards/board212/slot collision checks passed; Turbo35/35 successful (28 cached); agent vet/lint/race/build and CLI vet/lint/race/build passed; every listed topology module unit gate passed; shellcheck passed. Fresh startup fake-host harness: shard1 59 passed/0 failed, shard2 90 passed/0 failed, total149/149. No cache or optional bypass replaced the fresh harness. Unit-only topology integration skips are prescribed by quick mode and not packet acceptance.

Actual final output:

```text
  mode quick · wall time 30m15s · logs /root/ngfw-wt/logs/ci/vpn-warning-r1-t1-20261005-20261005-183255-430660

CI GATE PASSED
```

Verdict: PASS.
