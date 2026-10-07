# claude-td19 WIP: D-238 repository trust anchors (TD-19)

- branch: claude/td19-trust-20261006 (from origin/main 75434a3e5), worktree /root/ngfw-wt/claude-td19
- remote: NOT published (owner instruction: local commits only, no push)
- local commits: 04dbab69d (pins + fixtures, reviewed checkpoint of the interrupted agent work), d5ca9bb8a (D-238 decision record), c692655f7 (this file), then the security-review fix round commit (R2 APPROVE-WITH-NITS, /root/ngfw-wt/claude-td19-security-review.md)
- owned files: scripts/00-add-repos.sh, docs/status/tasks/TD-19-{run-fixtures,test-frr-selection,test-preflight,test-repo-keys,test-repository-trust}.py, docs/decisions/{DEC-238-td19-repository-trust.md,PENDING-TD19-repository-trust.md,LOG.md (D-238 row)}, this file

## Completed

- Owner chose option 2; recorded as D-238 (DEC-238-td19-repository-trust.md, LOG.md row; PENDING file is now a pointer).
- scripts/00-add-repos.sh: built-in `NGFW_FRR_AUTHORIZED_PRIMARIES` (4A56C773…3ED1, 3D9968AC…7FDA, BBC9ACA9…975B, A90FC36D9429409798E9C2D874DEED43AB194DBF) and `NGFW_NODESOURCE_AUTHORIZED_PRIMARIES` (6F71F525282841EEDAF851B42F59B5F99B1BE0B4). Unset administrator variables resolve to these; a set variable must equal the authorized set exactly (else `REFUSED:` before artifact preflight/network/APT). New `check_frr_raw_primaries` refuses a downloaded FRR bundle whose primary set differs (extra/missing/changed); duplicates are canonicalized by the existing private import/export. NodeSource uses the existing strict exact-set verifier. All existing validation unchanged.
- Review of the interrupted agent changes: fingerprints checked against TD-19-trust-material-20261004.md; logic correct; one duplicated test case replaced with `missing-first`.

## Security review fix round (R2 APPROVE-WITH-NITS)

- MINOR 1: NodeSource now uses the same path as FRR (`select_pinned_certificates`: private import, complete export of exactly the pin, exact verification); the raw dearmored download is no longer installed.
- MINOR 2: one shared parser `check_primary_set` (modes canonical/exact) for raw downloads and exports; `check_frr_raw_primaries` replaced by `check_raw_primaries`.
- NIT 3: all pin/identity refusals start `REFUSED:` and name D-238.
- NIT 4: end-to-end fixtures for an attacker subkey on a pinned primary (FRR and NodeSource: reaches the APT boundary, subkey absent from installed keyrings, gpgv rejects its signature) and revoked/expired pinned primaries (FRR, FRR duplicate-copy revocation, NodeSource: refused before APT).
- DEC-238 records the owner source (direct answer in the Claude Code chat session, 2026-10-06, verbatim quote).
- After the fix round: shellcheck PASS; TD-19-run-fixtures.py 44 tests OK, 0 failures/errors/skips; selector policy 5 OK; selector 13/13 accepted. Full quick gate not re-run (owner instruction); the last gate pass was before the fix round, on c692655f7.

## Test results (2026-10-07, this host, before fix round)

- shellcheck -x -P SCRIPTDIR scripts/00-add-repos.sh scripts/20-install-build.sh scripts/40-install-lab.sh tools/lab: PASS
- python3 docs/status/tasks/TD-19-run-fixtures.py: 42 tests, 0 failures/errors/skips (includes 6 new TD-19-test-repository-trust.py tests: pins accepted incl. duplicate FRR cert canonicalized and real gpgv VALIDSIG for fourth signer/Node primary; wrong/extra/missing/changed/duplicate/empty/lowercase overrides refused; extra/missing/changed FRR and Node downloaded identities refused before APT)
- TD-19-frr-selector-ci-policy.py: 5 OK; TD-19-run-frr-selection.py: 13/13 accepted
- deploy/debian/bundle/test_verify.py 23 OK, test_install.py 11 OK, test_export.py 18 OK
- TMPDIR=/root/.cache/ngfw-ci-host tools/ci.sh quick --base origin/main: CI GATE PASSED (14m08s, logs /root/ngfw-wt/logs/ci/claude-td19-20261007-052550-1888208)

## NOT RUN / remaining

- Real target installation/provisioning/boot: NOT RUN (no real machine provisioning permitted). Ubuntu 26.04 runtime compatibility of the NodeSource repository is not established.
- Board: plan/tasks.yaml TD-19 row still `parked` on PENDING-TD19-repository-trust; manager should unpark/update it and PROGRESS.md (not edited here: board is manager-owned).
- Independent security review (D-201: security-sensitive diff), publish branch/PR, merge.

## Next command

cd /root/ngfw-wt/claude-td19 && python3 docs/status/tasks/TD-19-run-fixtures.py
