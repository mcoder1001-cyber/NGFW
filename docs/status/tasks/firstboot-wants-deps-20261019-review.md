# PR #248 — independent review (2026-10-20, reviewer agent, read-only)

Reviewed `9cabd7f47..a99580bad` (own commits only; the branch later moved to `cf9ad2e58` = D-254→D-255 renumber + status commit, not reviewed). **Verdict: APPROVE WITH CHANGES** (no BLOCKER/MAJOR; findings 1–3 recommended before merge).

Checks: full packaging suite in a scratch worktree `Ran 139 tests in 717.191s — OK`; mutation checks (restoring `Requires=` in firstboot: 2 failures; single `is-active a b c` call: 4 failures; restoring `Requires=` in upgrade-prepare: 2 failures); `bash -n`/shellcheck clean on firstboot.sh and firstboot-propagation.sh; `systemd-analyze verify` only "binary missing" warnings.

Focus answers: first boot still fails closed (After= waits for the jobs; Valkey Type=notify; nftables RemainAfterExit; gate runs before any credential/key work); the restart cascade is broken (no Requires/Requisite/BindsTo/PartOf links firstboot to the daemons; firstboot is stopped only by its own stop/restart, ngfw-upgrade-prepare on A/B images, prerm on remove, shutdown; an nftables restart still restarts agent and API, not VPP); A/B path equivalent (pg_dump check=True only when a snapshot is due); tests adequate.

Findings:
1. MINOR — `firstboot.sh:16-18`, LOG.md:292, DEFERRED-ACCEPTANCE.md:462-469, `firstboot-propagation.sh:136,215`: on Ubuntu `postgresql.service` is an `ExecStart=/bin/true` wrapper; the cluster runs in `postgresql@<v>-main`, so `is-active postgresql.service` passes when the cluster failed; fail-closed holds only via the first `psql` (ON_ERROR_STOP, set -e). Add `runuser -u postgres -- pg_isready -q` to the gate or state that psql is the real gate; add an acceptance case "cluster fails to start".
2. MINOR — `deploy/debian/README.md:12-14`: "their explicit restart does not propagate to VPP, the agent or the API" is false for nftables (the agent's Requires= restarts agent and API). Limit to PostgreSQL/Valkey and add the nftables exception.
3. MINOR — `docs/install/upgrade.md:48-50`, LOG.md:292: "VPP and the agent no longer wait for PostgreSQL or Valkey" is wrong (still ordered after them via firstboot's After=; no longer *withheld*); and on a provisioned appliance VPP now starts even when nftables fails (before, firstboot's Requires=nftables withheld it) — undocumented behaviour change; reword and state it.
4. NIT — upgrade.md:41: the API requires the agent, not nftables.
5. NIT — `ngfw-upgrade-prepare.service:4-6`: "fails on its own when PostgreSQL is down" only when a snapshot is due.
6. NIT — LOG.md:292, DEFERRED-ACCEPTANCE.md:441: the observation ran on root's persistent `--user` manager on the shared dev host with throwaway units under /run/user/0, not "a disposable user-manager instance".
7. NIT — `firstboot-propagation.sh:176,217`: `sleep 1` as synchronisation; systemctl start/restart already wait for the job.
8. NIT — `test_packaging.py:387-398`: stopped-units check uses the non-enabled closure (upgrade-prepare not guarded there; covered by `test_upgrade_paths_never_stop_start_or_restart_a_unit`).
No secrets in the diff. Hosted packaging failure inherited from #246, not counted.
