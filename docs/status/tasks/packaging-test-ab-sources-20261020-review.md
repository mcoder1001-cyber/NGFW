# PR #253 — independent review (2026-10-20, reviewer agent, read-only)

Head `b2949d5ad`, base `3fc333cde`. **Verdict: APPROVE** (two optional MINORs).

Checked: the two-file diff (`prepare.sh:43-45`, `maintainer_scripts.py:160-168`); `tests/test_packaging.py` OK and `test_prepare.py` OK at the repository root; in the replica of the failed package tree `test_packaging.py` OK and `test_base_policy.py` OK. Staging is safe: `debian/*.install|links|dirs` reference only `stage/...`; nothing packs `tests/ab-upgrade-sources`; files are 0644. Snapshot-first order is right (ROOT is meaningless in the prepared tree; the snapshot comes from the same clean commit). Other ROOT uses: `locate_unit`, `test_packaging.py:179,214`, `test_base_policy.py:37` fall back correctly; `test_packaging.py:191,291` are guarded and skip silently; `test_runtime_policy_boundaries.py`, `test_runtime_profile.py`, `test_publish_apt.py`, `test_deploy_hosts.py` are not run by `dh_auto_test` (rules runs packaging, system_identity, firstboot, base_policy, pppoe_assets only — confirmed by the manager).

Findings:
1. MINOR (pre-existing): `test_packaging.py:218` globs `ROOT/deploy/hardening/systemd/ngfw-api.service.d/*.conf`, empty in the package tree, so the D-192/D-252 effective-environment check runs without the drop-ins under dh_auto_test. Follow-up: stage or fail when missing.
2. MINOR (optional): `test_prepare.py` could assert `output/tests/ab-upgrade-sources/{provision,ngfw-upgrade}` exist with mode 0644 (a regression today surfaces as a prepare failure under `set -e`, not a silent skip).
