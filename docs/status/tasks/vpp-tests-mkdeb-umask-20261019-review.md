# PR #245 — independent review (2026-10-20, reviewer agent, read-only)

Head `3031bddc1`, base `cb148aafb`. **Verdict: APPROVE.**

Questions: fix complete (mkdeb was the only umask-dependent fixture; SHA256SUMS/manifest/wheel/lock/output dirs are read back by the same user; version script mode is set explicitly in lib.sh:127; test 66 also uses mkdeb) — yes. Product code changed — no (`deploy/vpp/tests/run.sh` and the wip note only). Status note matches the diff — yes.

Results (scratch worktrees, removed): base under `umask 077`: 69 passed, 3 failed (61, 62, 69; 100× "control directory has bad permissions 700"). PR head: `umask 077` + `LC_ALL=C` 72/72; plain 72/72; `umask 027` 72/72; `verify.sh` under `umask 077` OK; shellcheck rc 0; `gh pr checks 245` all pass.

Findings:
1. MINOR (pre-existing in main, outside this PR): `deploy/vpp/tests/run.sh:170,174` — `dpkg-deb` exit code is not checked (suite runs without `set -e`); at base under umask 077 ten negative tests (63–68, 70–72) passed without testing anything. Fix: fail loudly in mkdeb or have mkout check the .deb count against `VPP_PACKAGES`. Follow-up row.
2. NIT: wip note gives summary lines rather than raw output; reproduced, fine to leave.
