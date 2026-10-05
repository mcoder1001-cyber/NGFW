# Upgrade narrow health delta R2 review
Frozen source: be1448118afd246f39b0d6a7db01d2f986448482. Reviewed only new health-probe delta from prior R2-approved eb9dbc46; earlier signing-key export blocker repair and approval preserved.
No new BLOCKER, MAJOR or MINOR security findings. deploy/upgrade/ngfw-upgrade-health:26 now catches OSError alongside failed/timeout probes inside the bounded trial loop. Missing/nonexecutable probe cannot confirm the trial; deadline reaches rollback and reboot. Unreadable initial status exits without mutating unknown boot state. Fixed argv and suppressed subprocess stderr retain existing privilege/injection boundaries. No secret/signature verifier/export path change.
Actual independent commands:
```sh
git archive be144811 deploy/upgrade | tar -x -C /root/.cache/review-r2-final/upgrade
TMPDIR=/root/.cache/review-r2-final/tmp PYTHONDONTWRITEBYTECODE=1 tools/heavy.sh python3 -m unittest discover -s /root/.cache/review-r2-final/upgrade/deploy/upgrade/tests -v
```
Output: Ran 17 tests in 11.631s; OK, no skips. Includes missing/nonexecutable probe, unreadable status, signing-key root/inode-alias exclusion, tamper/path/special-node refusal and actual fixture grubenv lifecycle. Health reboot commands are mocked; shared boot/services/config never changed. Batch secret delta scan: gitleaks exit0, no leaks.
[other: R4/R8] Actual appliance trial boot, watchdog/reboot, vfat EFI GRUB environment and PostgreSQL migration rollback compatibility: NOT RUN. Full quick not independently run by R2.
Verdict: APPROVE (R2 narrow delta, exact head).
