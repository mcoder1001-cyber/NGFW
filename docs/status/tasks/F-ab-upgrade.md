# Current final integration readiness

Root-owned codex/integrate-ab-upgrade-20261005 /dev/shm/ngfw-integrate-ab-upgrade-20261005, pinned parent e5605d9c123e4c90a300e15ac0236fb49c4d62d5 after verified images173 merge. Frozen reviewedbe144source unchanged, allselectedR1/R2/R4/R7/R8 APPROVE; independent exactquick PASS9m49, actualprivate-loopstage/confirm/rollbackPASS. docs09 nowlistsactualruntimepackage deps; unrelatedcross-taskreports preservedinreviewedsourcearchive andexcludedfromfinalscope. Own tasksourcearchivebe144local/remote existsbeforeD112. Hardening and upgrade packaging assertions preserved together after three-way integration; finalexactcurrent-main/local+hosted gates pending, no mergeclaimed. Runtimefirmware/backup/reboothealthgenuineapplianceNOTRUN. Historicalchronology below.

# F-ab-upgrade source delivery

Branch: codex/ready-ab-20261005; isolated worktree /root/ngfw-wt/ready-ab-20261005. Last published checkpoint before the security fix:72a21c72a78cb787cf63d3da8c0d501d0ba2f00d. Publication uses the authorized GitHub connector because CLI push403 was observed. PR:https://github.com/mcoder1001-cyber/NGFW/pull/174 (draft pending complete gate and independent review).

Implemented: dedicated Ed25519 signed bundle builder/verifier, bounded compression/archive validation, ext4 inactive staging and persistent mounts/config/identity, real GRUB one-shot/confirm/rollback, offline shared-EFI provisioning, pre-migration PostgreSQL dump service, real agent Health RPC/API probe and timeout rollback service. CLI argv/status JSON/exit codes are documented in docs/install/ab-upgrade.md. No new privileged Action endpoint or agent sandbox change; consumer uses the existing reviewed executor boundary.

Security correction authorized by manager: /data is root-owned0750 with the API's existing group, while only backups/updates/support are API-owned. This prevents replacement of root upgrade/shared state by renaming its writable parent. Existing child contents and descriptor-relative no-follow protection remain. Upgrade checks also require secure data parent, root shared-state bind identity, root0700 state directory, root0600 state file and no-follow lock. Real UID test demonstrates API cannot replace upgrade state and can write all three owned children.

## Actual verification

```text
deploy/upgrade/tests/run.sh
Ran13 tests in3.857s
OK
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py -v
Ran14 tests in1.985s
OK
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
Ran3 tests in3.494s
OK
cd test/topology/ab-upgrade && tools/heavy.sh go test -race -count=1 ./...
ok ngfw/test/topology/ab-upgrade26.961s
shellcheck deploy/upgrade/tests/run.sh deploy/debian/ngfw/prepare.sh
exit0
NGFW_INTEGRATION=1 python3 deploy/upgrade/tests/loop_image.py
PASS: unsigned and one-byte-tampered bundles refused; inactive device unchanged
PASS: stage/fstab/identity/manifest -> one-shot B -> confirm B; failed health -> default A
PASS: host lsblk/grubenv unchanged; no owned loop devices remain
```

The complete actual unit/storage/loop output is under F-ab-upgrade-evidence. Host grubenv SHA before/after:f64122858064885ef0733e42c6a3d2d3fd642671f714db0d974b880c0f087430; host lsblk JSON identical before/after. Loop image was sparse96GiB with P14 partition sizes, tiny hand-made root and ext4 EFI only for non-firmware env-file tests. No host reboot, boot edits, service start/package installs or VPP changes. Every owned mount/loop/file and throwaway key cleaned. Initial quick failed the old baseline rsyslog convergence test. Integrated origin/main7b507db5 (already contains the reviewed fixture wait fix) without changing the gate or test. Final complete quick rerun is running at /tmp/w15-ab-quick-current.log; not yet claimed green. Directory UID/GID restoration and hardlink-export regressions now pass14 upgrade tests. The topology wrapper includes the upgrade Python and Go probe tests in unchanged quick CI.

## Shared hunks

- deploy/debian/ngfw/prepare.sh: fixed staging-only upgrade block after existing agent builds; no provisioning/action in packaging.
- deploy/debian/ngfw/debian/ngfw-agent.install: upgrade CLI,2 Python helpers, Go health probe and2 units.
- deploy/debian/ngfw/debian/control: direct runtime dependencies openssl,zstd,grub2-common,e2fsprogs,mount,util-linux (python3 already present).
- deploy/debian/ngfw/tests/test_prepare.py: copy upgrade fixture and assert staged files only. Manager must combine with hardening fixture copy/assertions.
- deploy/debian/ngfw/assets/provision-api-storage.py and tests/test_packaging.py: manager-authorized root-parent ownership correction and regression tests.
- .github/workflows/ci.yml: manager-authorized hosted runner fixture dependencies grub2-common/zstd only. No gate step removed/skipped/changed.

## Remaining acceptance / scope

Source-complete pending independent boot/security/migration review, complete quick gate and hosted gate. Laboratory-only acceptance is explicitly deferred: real VMware vfat shared-GRUB one-shot consumption and A/B boot in both directions; actual database dump preceding production API migration and previous-binary schema compatibility; encrypted shared-volume unlock/initramfs; platform watchdog behavior when kernel hangs. Exact VM plan is in docs/install/ab-upgrade.md. Manager should add these to docs/status/DEFERRED-ACCEPTANCE.md; worker does not edit shared ledger.

No Secure Boot, air-gapped installation product, UI/fleet rollout, VPP package build, host upgrade or generic privileged executor. Writable ext4 and dedicated Ed25519 follow task defaults. Shared /var/lib/ngfw binds /data/ngfw because P14 has no dedicated state partition. SQL compatibility is a publisher/reviewer obligation; automatic root rollback never overwrites a live database. Initial provisioning is explicit offline maintenance and is not run by postinst; existing deployed appliances require their offline provisioning acceptance before using this CLI.
