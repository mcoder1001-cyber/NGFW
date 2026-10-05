# F-hardening-lite: independent T1 verification

Exact frozen source: `726d92141402459505ee27fdc6fafc7aeef94295`, remote `4707bffb8c84a25b5fd332b1bd095866512ccf43`. Reviewer fixture `/root/ngfw-wt/r1-gate-hardening-20261005`. Started 2026-10-05 07:14:22 UTC, ended 07:50:35 UTC. Verdict: PASS; unchanged complete quick gate exit0, wall36m12s.

Command: `TURBO_ENV_MODE=loose TMPDIR=/root/.cache/review-r1-tmp GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_CI_TASK_CONCURRENCY=2 tools/heavy.sh tools/ci.sh quick --base origin/main`. Loose environment preserves disk temporary directory; checks unchanged. No host service/configuration mutation.

Observed API/web/schema suites PASS; TS gate 35/35 (9 cached) 11m53s; agent race/build PASS; topology hardening wrapper PASS1.386s (all ten Python cases). All deploy checks PASS149 passed/0 failed across four fake-host shards. Agent14m20s, CLI25s, test modules1m28s, deploy5m12s. Exact HEAD rechecked; real generation and tracked source remain clean. Complete raw evidence: `hardening-quick.log`; CI step logs `/root/ngfw-wt/logs/ci/r1-gate-hardening-20261005-20261005-071423-1830376`.

Actual live daemon compatibility and real signed APT installation remain laboratory-only acceptance; staged fixtures do not prove runtime compatibility.
