# TD-3 WIP — V19 guard (D-095)

Slot 2 (w2). Branch task/TD-3. Fix round 2; continue2 stopped at 13:4x by coordinator (manager handover).

| part | state |
|---|---|
| merge main (DF-5, F-startup-apply, V24) | done 6e42c16 |
| H1 ipsec.itf + wireguard.interface through Acquire/BeforeDelete + guard test | done cd21806 |
| M1 re-read on stolen holes, cap 16, fail closed (ErrCapped), capped metric | done cd21806 |
| M2 holder instance 16383↓16000, docs, schema CONTRACT question | done cd21806/e73db90 |
| L5 ci.sh after-failed-tests pre-flight, all FAIL lines | done e73db90 |
| host runs (ifsanitize, ipsec, wireguard, slot 2) | done 13:18–13:27, all PASS, NRestarts 0→0; pre-flight ok; no w2/loop16xxx/ipsec/wg objects left; no w2 veth up (checked after stop: NRestarts=0) |
| "Fix round 2" section in TD-3.md | done c15b16d |
| tools/ci.sh --base main | **NOT green yet**: run 13:29 (logs /root/ngfw-wt/logs/ci/TD-3-20260924-132925-26092) passed contract guard, generate, forbidden patterns, turbo lint/typecheck/test/build, then FAILED in `apps/agent: make lint` with `Error: parallel golangci-lint is running` (another worktree's CI linting at the same time — environmental, not a code finding). The failure printout names TD-2's log path (/root/ngfw-wt/logs/ci/TD-2-20260924-131851-10708) — looks like a ci.sh log-path mix-up while two runs share the host; worth a look |

Left: re-run CI when no other golangci-lint is running, paste its tail into TD-3.md "Fix round 2" (add a `### CI` subsection), commit.
Next command: `cd /root/ngfw-wt/TD-3 && pgrep -a golangci-lint; tools/ci.sh --base main`
