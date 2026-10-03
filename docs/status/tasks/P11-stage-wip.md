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
