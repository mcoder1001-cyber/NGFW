# Owner-authorized merge of all open PRs — 2026-10-20

Owner instruction (2026-10-20): "همه کدها را مرج کن. همچنین 110.211 آخرین نسخه باشد" — merge every
open pull request and bring `172.30.110.211` to the resulting `main`. Done sequentially by the
root manager with an independent read-only review per PR (reviewer agents; hand-backs saved as
`docs/status/tasks/<task>-review.md`), the unchanged hosted gates, D-112 archive refs and
expected-head squash merges (`gh pr merge --squash --match-head-commit`).

| Order | PR | Scope | Review | Reviewed head | Merge commit | Hosted gates on the head |
|---|---|---|---|---|---|---|
| 1 | #245 | `deploy/vpp/tests/run.sh` mkdeb fixture umask-safe (test only) | APPROVE | `3031bddc1` | `457431dc6` | quick 39226762416, artifact 39226762424, packaging 39226762499 |
| 2 | #246 | `dh_installsystemd --no-stop-on-upgrade` (D-253); packaging-fixture workflow installs debhelper | APPROVE WITH CHANGES → verify round APPROVE | `cb1991a30` (product head `ad0bc5027`) | `921f82388` | quick 39372355032 / 39370580383, packaging 39372356725 / 39370580418 |
| 3 | #244 | deploy-hosts.sh hides APT repositories instead of `--no-download`; receipts for the 2026-10-19 upgrades of hosts 37 and 211 | APPROVE WITH CHANGES (applied, rebased onto main after #246) | `af5c5a61c` | `e7377927f` | quick 39372836816, artifact 39372832568, packaging 39372832671 / 39372836768 |
| 4 | #248 | firstboot/upgrade-prepare `Wants=` + own checks incl. `pg_isready` (D-255, renumbered from D-254) | APPROVE WITH CHANGES (applied, rebased onto main after #246) | `9017df479` | `5593e5ab2` | quick 39374120967, packaging 39374120991 |
| 5 | #247 | deploy-hosts.sh unit-state records before/after APT, fails on a VPP/nftables/socket change (D-254) | APPROVE (four minors applied, rebased onto main after #248) | `a94c109a7` | `3fc333cde` | quick 39378808375, artifact 39378804670, packaging 39378804624 / 39378808404 |
| 6 | #253 | `prepare.sh` stages the A/B upgrade sources so `dh_auto_test` passes inside the package tree (the `main` `3fc333cde` build failed on `test_upgrade_never_stops_a_unit_vpp_requires`) | APPROVE | `b2949d5ad` | `a094258ea` | quick, artifact and packaging all pass |

Reviewed histories: `refs/heads/archive/{vpp-tests-mkdeb-umask-20261019-3031bddc1,
pkg-no-stop-on-upgrade-20261019-cb1991a30, deploy-host-37-20261019-fdc7fc470,
deploy-host-37-20261019-af5c5a61c, firstboot-wants-deps-20261019-9017df479,
deploy-hosts-vpp-restart-check-20261019-a94c109a7, packaging-test-ab-sources-20261020-b2949d5ad}`.

Main CI after each merge: `457431dc6` CI gate 39369652569 SUCCESS (+ packaging 39369652516,
provisioning 39369652523); `921f82388` CI gate 39372695645 SUCCESS (+ packaging 39372695683);
`e7377927f` CI gate 39374880793 CANCELLED by the next push (packaging 39374880771 SUCCESS) and
covered by `5593e5ab2` CI gate 39376212024 SUCCESS (+ packaging 39376212020); `3fc333cde`
CI gate 39380738314 CANCELLED by the owner's docs merge #252 (`7442c6a5f`, same concurrency group;
packaging 39380738149 SUCCESS); `a094258ea` (after #253): RESULT_A09.

Fixes during the queue: #246's new maintainer-script fixture ran `dh_installdeb` as root only
(hosted runner EPERM) → fixture honours `Rules-Requires-Root: no`; #247 and #248 both claimed
D-254 → #248 renumbered to D-255; #244 and #247 were rebased onto the moving main (doc and
LOG conflicts resolved, content of both sides kept). Review follow-ups not fixed in this queue:
`deploy/vpp/tests/run.sh` does not check `dpkg-deb`'s exit code (ten negative tests pass
vacuously when no .deb is built); `deploy/debian/bundle/install.py:173` still passes
`--no-download` (probably broken on apt 3); `test_packaging.py:218` globs the hardening drop-ins through
ROOT and runs the D-192/D-252 environment check without them under `dh_auto_test`; automation for stopping `apply-executor.service`
when idle and a safe base-firewall re-render after upgrades (D-253 consequences).

Deployment: the first build from `main` `3fc333cde` failed in `dh_auto_test` (fixed by #253, above);
packages built from `main` `a094258ea` (`deploy/deploy-hosts.sh -b -r main`, verified VPP
`26.06-release+ngfw3` output, no `NGFW_VPP_IN_TESTS` needed after #245);
host 172.30.110.211: DEPLOY_211_RESULT. Host 172.30.126.37 stays on `0.1.0~dev+git695.cb148aafb288`
(not requested).

No installed service outside the two lab appliances, no startup configuration and no privilege
or security boundary was changed by this operation.
