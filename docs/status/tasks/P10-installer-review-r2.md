# Independent P10 installer fix verification, round 2

Frozen local `2d192cfe81be521116b4605f33c0f3454c785e60`, remote `cd7345358eb2f938ddee82efc61edfadfd2bb2fa`. Bounded installer scope; prior BLOCK at `9a0bff63` preserved. Reviewer changed only this report.

Original service-activation MAJOR resolved: verified artifact preflight remains before installation; serialized no-start policy returning 101 now covers all mutating APT calls. Existing operator policy regular file or symlink is backed up and restored on normal success and failure. Cleanup refuses to overwrite a policy changed independently. Management-network purges, autoremove, IRQ stop and all immediate --now stops are removed. Future-boot disabling now includes VPP; product firstboot activation remains separate.

**MINOR durability limitation:** backup/guard lives in /run and moves to /usr/sbin, commonly a different filesystem. That replacement is not an atomic rename; EXIT cleanup is armed after it completes. Power loss or a failed/interrupted cross-filesystem replacement is not established as recoverable, and /run backup may disappear at reboot. Avoid crash-safe claims; consider destination-sibling staging with atomic rename and explicit state handling/persistent recovery. Existing test demonstrates process-level APT failure recovery only. No release or crash acceptance approval follows from this checkpoint.

R1/R2/R7/R8: APPROVE reviewed installer checkpoint with stated MINOR. Whole P10 remains partial, including capability security decision, dynamic LCP synchronization and actual packaging/boot acceptance.

Actual independent checks:

```text
python3 deploy/debian/vrx/tests/test_runtime_profile.py
Ran 2 tests in 1.689s
OK
bash -n scripts/10-install-runtime.sh
exit 0
tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~118714 bytes (118.71 KB) in 858ms no leaks found
check PASSED (0m09s)
```

Fixture test covers six combinations of absent/regular/symlink policy and APT success/failure, queries shipped temporary policy to verify service-start denial 101 and compares restored content/mode/uid/gid or symlink target. Test-local root/path adaptation permits an unprivileged private fixture; it does not bypass shipped production EUID checks. Artifact verifier is a fixture here; prior original review separately exercised real missing-manifest rejection. No actual APT installation, systemctl service changes, host policy replacement or appliance acceptance was performed. Exact final hosted full quick remains required before merge.
