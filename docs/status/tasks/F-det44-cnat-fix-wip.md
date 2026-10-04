# F-det44-cnat-fix — WIP (slot 16, w16)

Started 2026-10-01 20:25 (+0330). Time box 4 h.

## State
- [x] root cause (a): VPP 26.06 det44_interface_add_del() enables the node on its delete path (questions Q1)
- [x] suspect (b) not needed: CNAT 12/12 without any cnat interface feature (questions Q4)
- [x] fix in descriptors/det44 (arc repair via binapi/feature) + fake-VPP unit tests (PASS)
- [x] host: TestDet44ArcSplitOnHost before (FAIL, CNAT 0/12) / after (PASS); TestDet44MapDsliteCnatOnHost PASS
      (CNAT 12/12 rev 4a + rev 4, rollback/cleanup leave nothing, NRestarts=0)
- [x] status file + questions file
- [x] CI gate `/root/NGFW/tools/ci-slot.sh --base main` → CI GATE PASSED (21:39, 7m48s)
- [ ] `/tmp/g-w16` not deleted: the permission layer refused `rm -rf /tmp/g-w16` (questions Q6)

## Recovery checkpoint — 2026-10-04

- Branch: `codex/recover-det44-20261004`, isolated worktree `/tmp/ngfw-recover-det44`.
- Base: `980d54be5` (`origin/main`); historical delta `55a18e0f..4092b9d5`. Original evidence and review above describe the historical run, not new lab execution.
- Owned: det44 descriptor/tests, desired det44 assembly/test, descriptor documentation, topology/det44 and task evidence/status.
- Recovered full reviewed fix and missing topology suite; migrated historical VRX paths/environment to NGFW.
- Current focused verification: `go test ./internal/descriptors/det44 ./internal/desired` PASS; `go test ./...` in topology/det44 PASS with host tests skipped because `NGFW_INTEGRATION` unset.
- No new full CI or host/VPP execution, per owner request. Remaining laboratory acceptance is explicitly deferred; historical evidence preserved.
- Review findings 1, 2 and 4 implemented in source. Known limitation: two feature reads per eligible owned interface and ambiguous both-node answers intentionally left untouched.
- Remote publication delegated to manager via authorized connector; local SHA is the enclosing commit, remote SHA pending actual publication.
- Next command: manager independently review current diff, publish this checkpoint, integrate sequentially.

### Independent recovery review correction

Reviewer found the historical full topology scenario deletes DS-Lite pools in restart/cleanup, prohibited by D-211. `TestDet44MapDsliteCnatOnHost` now unconditionally skips before any host operation until a verified VPP fix and reviewed safe harness exist. The pool-free `TestDet44ArcSplitOnHost` remains available. Full DS-Lite acceptance is deferred; historical success is not current authorization to rerun.

Focused topology `go test ./...` and `go vet ./...` pass after the guard.
