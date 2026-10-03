# P11 resumed HOME blocker fix — current checkpoint

Branch/worktree unchanged: `codex/p11-stage-20261003`, `NGFW-p11-stage`.
Previous materialised-stage implementation is durable: observed local HEAD
`9565735f7de97208786aba43edc0bc18cd7189df`; manager reports remote publication
`295b8ce42b600e21784e04f7dbf53b3535be2d0f`. Developer cannot independently read remote.
Current fix remains local pending coordinator commit/publication because shared Git metadata is read-only.

Original independent FAIL is preserved: manager/root T1 observed `VERIFY.run`
raise `InvalidBundle: bash failed`; direct `verify.sh` exited 1 with
`FAIL tests/run.sh failed`, and direct unchanged tests stopped after 19 passes at
`deploy/vpp/tests/run.sh: line 60: HOME: unbound variable`.
Evidence: `../NGFW-manager/docs/status/tasks/P11-stage-test-T1-root.md` and final
HOME blocker in `P11-stage-manager-feedback.md`. Prior 21 stage/9 intake fixture
passes did not resolve or supersede this real verifier defect.

Exact changed files for this resumed fix:
- `deploy/strongswan/verify_inputs.py`: set deterministic `HOME=/nonexistent`
  inside `verified_snapshot`'s existing shared sanitized environment; do not
  change shared helper, privileges, caller restoration or full gate flags.
- `deploy/strongswan/test_verify_inputs.py`: regression for exact sanitized
  environment and full caller restoration after success, invalid digest and
  consumer error; run real unchanged VPP 66-test script and static `verify.sh`
  through the original production `VERIFY.run` inside the snapshot environment.
  Synthetic intake provenance remains explicitly stubbed; real static checks do not.
- `docs/status/tasks/P11-stage-wip.md`: this recovery and actual verification record.

Final frozen verification (no skipped tests):
```
python3 deploy/strongswan/test_verify_inputs.py
Ran 11 tests in 94.568s
OK
python3 deploy/strongswan/test_prepare_stage.py
Ran 23 tests in 137.742s
OK
tools/ci.sh check --base HEAD
check PASSED (0m08s)
python3 -m py_compile deploy/strongswan/verify_inputs.py deploy/strongswan/test_verify_inputs.py
exit 0
git diff --check
exit 0
```
Both suites execute the unchanged real `deploy/vpp/tests/run.sh` via the original
production `VERIFY.run`, under the fixed three-variable environment in the actual
P11 snapshot scope. Direct real script output: `66 passed, 0 failed` (66 `ok` lines).
Both also execute the unchanged real static `deploy/vpp/verify.sh` without
`--no-tests`, which itself reruns the full 66-test suite. Actual static output:
```
ok   VERSION: v26.06 c3200b88dc46bd380f00a49ca3392a102cc1980b → 26.06-release (patched: 26.06-release+vrx1), 11 packages, ship 7
ok   series: patches 1 · build-patches 1 (build/ only) · optional optional/trace-plugins-core.patch
ok   pydeps.lock: meson==0.57.2 pyelftools==0.33 setuptools==84.0.0 wheel==0.48.0 packaging==26.3 (sha256-pinned)
ok   scripts parse + shellcheck; no .deb/.whl/.build in git; .gitignore covers deploy/vpp/.build and *.deb
ok   tests/run.sh: 66 passed, 0 failed
verify.sh: OK
```
Logs: `/tmp/p11-home-intake-tests-final.log`, `/tmp/p11-home-stage-tests.log`,
`/tmp/p11-home-check.log`. The first new intake regression run genuinely executed
both real commands successfully but failed because the test expected the wrong
success label (`verify.sh: all checks passed` rather than actual `verify.sh: OK`).
That assertion was corrected and the full final intake suite rerun passed; initial
log preserved at `/tmp/p11-home-intake-tests.log`. This test-assertion error is
separate from the original independently reproduced missing-HOME product FAIL,
which is preserved above. Positive synthetic provenance remains stubbed; these
real static successes prove the environment prerequisite, not a real product
build/source authenticity, ABI, release or lab acceptance.

No VPP test/verifier scripts, shared installer, privileges or other task code
modified. Ancestor/overlap/rollback fixtures retained and passing. Only own
untracked `deploy/strongswan/__pycache__` cleaned before handoff. Current fix is
ready for coordinator commit/publication; read-only shared Git metadata remains
the known blocker, no retries of unavailable connector approval or auth changes.
Independent root R2 and unchanged full hosted quick remain mandatory/pending.
Builder, release/security/licensing and lab acceptance remain unfinished.

Exact next commands for the coordinator in this same worktree:
```
git add deploy/strongswan/verify_inputs.py deploy/strongswan/test_verify_inputs.py docs/status/tasks/P11-stage-wip.md
git commit -m 'fix(packaging): provide deterministic HOME for P11 verification'
git push origin HEAD
```
Record actual new local/remote SHAs; obtain independent root R2 and unchanged
hosted quick results. Developer does not self-review, merge or wait for hosted CI.

Frozen tested product-file SHA-256 fingerprints:
`deploy/strongswan/verify_inputs.py`: `3299b289c382c4bafdc4eaf9b519d8af255e0539705ed81cfaa9d07802a14f6d`

`deploy/strongswan/test_verify_inputs.py`: `d17d5f32bb16ff0d59a3ec9e586083aa1e350c1cb5a27afa7a9aa650eb9d79af`


---

The following is the preserved historical record before manager committed/published
`9565735f` / `295b8ce4`; its then-pending publication statements are historical.

# P11 stage WIP — tested checkpoint ready for manager publication

Branch: `codex/p11-stage-20261003`; base/frozen PR101 `2dbdff2405e845da47aa149bb72cdedbce60649c`.
Observed local envelope checkpoint: `a5b4daf355199557a75505207c5b6dcd61fa74c7`. Manager feedback reports envelope publication at remote `7d22ed7874b7ac7003938d82124cd8c54b8b54b0`; developer cannot independently verify remote. Product changes below remain uncommitted/unpublished pending manager checkpoint.

Owned product files (all final):
- `deploy/strongswan/verify_inputs.py`: scoped verified snapshot lifetime, existing read-only API preserved; full VPP gate unchanged.
- `deploy/strongswan/prepare_stage.py`: real bounded materialisation from those exact private snapshots.
- `deploy/strongswan/test_prepare_stage.py`: actual source/header bytes, trust refusal, original-input tampering, budgets, late failure cleanup, real linked Debian payload refusal, member hazards, no overwrite, atomic race, symlink ancestors, input overlap and parent replacement rollback.
- `deploy/strongswan/test_verify_inputs.py`: fixture-only temporary-root redirection for sandbox; production fixed `/var/tmp` policy unchanged.
- `deploy/strongswan/README.md`: API/CLI contract, layout/bounds and unfinished work.
Owned developer docs: `P11-stage-envelope.md`, this WIP, `P11-stage-pr.md`. `P11-stage-manager-feedback.md` was written by the manager, read and addressed; developer did not edit it.

Completed code: mandatory trusted digest at public API; unchanged complete intake (quartet, metadata/hash, full verified VPP install gate and tests); source and two development payloads extracted without executing anything. Per-file 128 MiB, aggregate payload and decoded tar 1 GiB, per-tar decoded 1 GiB, 50,000 member limits; links/traversal/duplicates/special files/collisions rejected; directories 0700, files 0600/0700. Every destination ancestor pinned without symlink following, owned non-writable parent, no input overlap, Linux atomic no-replace publication. Publication occurs after verified-snapshot cleanup; parent identity checked before/after with rollback of our owned inode on error. Partial tree cleanup proven.

Actual final verification:
```
python3 deploy/strongswan/test_verify_inputs.py
Ran 9 tests in 10.823s
OK
python3 deploy/strongswan/test_prepare_stage.py
Ran 21 tests in 25.836s
OK
tools/ci.sh check --base HEAD
check PASSED (0m09s)
python3 -m py_compile deploy/strongswan/verify_inputs.py deploy/strongswan/prepare_stage.py deploy/strongswan/test_verify_inputs.py deploy/strongswan/test_prepare_stage.py
exit 0
git diff --check
exit 0
```
Logs: `/tmp/p11-intake-tests.log`, `/tmp/p11-stage-tests.log`, `/tmp/p11-stage-check.log`. No skips. Positive provenance explicitly stubbed; real complete VPP gate rejects synthetic builds. No real authenticated source/product VPP build or lab acceptance claimed.

Current blocker: local `git add`/`commit` cannot create `/root/NGFW/.git/worktrees/NGFW-p11-stage/index.lock` (read-only). CLI push fails socket operation not permitted. GitHub create_commit connector rejects: `MCP tool call requires approval, but approval policy is never`. No auth/key changes or sandbox broadening attempted. Manager already recovered/published initial envelope; manager can commit/publish this tested coherent checkpoint.

Remaining explicit: actual builder/compiler/ABI validation, ID allocation, plugin/agent wiring, old-source security/licensing/release approval, real build/installation/IKE/ESP/reboot acceptance. Stage prerequisite implemented; P11 overall unfinished. Independent R2 verdict, reviewable PR and unchanged complete hosted quick gate pending; no self-review/merge. Full local quick NOT RUN because it regenerates files outside developer ownership; hosted quick mandatory, no waiver or weakened gate.

Exact next command for manager in this worktree:
```
git add deploy/strongswan/README.md deploy/strongswan/verify_inputs.py deploy/strongswan/test_verify_inputs.py deploy/strongswan/prepare_stage.py deploy/strongswan/test_prepare_stage.py docs/status/tasks/P11-stage-wip.md docs/status/tasks/P11-stage-pr.md
git commit -m 'feat(packaging): materialise verified P11 stage prerequisites'
git push origin HEAD
```
If connector publishes a different commit SHA, record actual remote SHA. Then open PR using `P11-stage-pr.md`, obtain R2 independent review and unchanged hosted quick result before manager integration. Preserve reviewed history and use D112 only after review; do not rewrite main or frozen PR101.
