# F-backup-restore independent R8 operability review
Source: e9bc1ae9c2137a89690b8553d42a3fd9e020cfb2. Reviewer authored no product code. Isolated branch codex/f-backup-operability-review-20261005, worktree /root/ngfw-wt/f-backup-operability-review-20261005. Owned only this report and review envelope/status.

## Findings
1. **BLOCKER** — deploy/debian/ngfw/debian/ngfw-agent.install:15 (missing entries), deploy/debian/ngfw/prepare.sh:41–42. Prepare stages ngfw-support-collect, ngfw-upgrade-dispatch and ngfw-upgrade@.service but no binary package .install manifest includes any of the three. Packaged appliance support RPC therefore lacks its executable and upgrade mutations lack their executor/unit. Add explicit agent-package install entries and a fixture checking manifest coverage, not just staging. No package installation was performed.
2. **MAJOR** — deploy/support-bundle/ngfw-support-collect:11; deploy/debian/ngfw/debian/control:12. Collector unconditionally invokes /usr/bin/lsb_release, but lsb-release is not a declared runtime dependency. Minimal appliance lacking this executable raises FileNotFoundError and fails support export. Declare runtime lsb-release and document it, or use existing /etc/os-release without an external dependency.
3. **MAJOR** — apps/api/src/features/backup-restore/schedule.ts:108–166 and docs/user/system/backup-restore.md. Document crash/full-disk behavior and migration rollback limits. Durable minute claims intentionally prevent retry: crash leaves result=running and no same-minute re-execution; local/SFTP write can leave a partial final archive which authenticated restore refuses. Audit finish failure can prevent terminal history update. State how administrator handles stale/partial outcomes and that later due minutes continue. Migration0010 is additive/backward compatible; old binaries may leave added table/nullable column/index in place. Document maintenance-only removal and loss of run history/restored candidate metadata rather than implying automatic downgrade SQL.

## Independent commands and actual output
`python3 .github/scripts/packaging-fixtures.py`
```
Ran 35 tests in 42.917s
OK
Packaging fixtures: 35 run, 0 failures, 0 errors, 0 skipped, 0 expected failures, 0 unexpected successes
```
This proves preparation/maintainer fixtures, not actual deb member inclusion; finding1 remains.
`python3 deploy/support-bundle/test_collect.py`
```
Ran 2 tests in 0.005s
OK
```
`rg -n 'support-collect|upgrade-dispatch|upgrade@' deploy/debian` returns prepare/test references only and no .install manifest.
`git diff f574 e9bc1ae9c -- tools/ci.sh .github/scripts/packaging-fixtures.py` produced no changes. Mandatory gate unchanged.
Journal ends with exactly idx10/tag0010_f_backup_restore; SQL adds table f_backup_run, nullable config_candidate.restore_secrets and f_backup_run_at_idx. No destructive rewrite or backfill. Prior nine migration rows remain.
Python dependency already declared. Agent sandbox unchanged; API ReadWritePaths includes /data; API child ownership provision fixture passes. Dedicated root oneshot is not automatically enabled/started: dh_installsystemd --no-enable --no-start. No packages installed, services started/restarted, VPP touched, actual upgrade performed or DB changed by this reviewer.

Verdict: **BLOCK** until findings1–3 addressed and independently rechecked.

## Focused closure on 2560e8511
Independent reviewer cherry-picked the developer fix into own worktree (local981764268). All three findings closed:
- Actual ngfw-agent.install now includes both executable helper paths and upgrade@ unit with correct destinations. Preparation test asserts tuple coverage and existing real byte/mode0755/0644 checks.
- Agent Depends directly declares lsb-release; docs/09-os-packages.md explains its collector purpose.
- User recovery documentation faithfully states at-most-once minute claims, no automatic retry/replay, stale running outcomes, audit-failure limits, partial final archive/full-disk handling, no atomic rename/fsync guarantee, manual retry, and conservative no-down-migration/downgrade maintenance/data-loss limits.
Independent command `python3 deploy/debian/ngfw/tests/test_prepare.py`:
```
Ran 3 tests in 1.697s
OK
```
No actual package installation or service execution performed. Initial35fixture run remains prior-source evidence; focused3 re-run proves changed fixture and install/dependency assertions. Actual boot/upgrades remain assigned laboratory acceptance. Other panel responsibilities and full unchanged quick remain manager gates.
Final R8 verdict: **APPROVE**. Zero unresolved BLOCKER/MAJOR/MINOR.

