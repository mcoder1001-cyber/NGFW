# Current integration readiness

Root-owned codex/integrate-images-final-20261005 in /dev/shm/ngfw-integrate-images-final-20261005, pinned parent b8c6c5478858691ca28e790d12b5f9dd32cdf981. Reviewed guard6bd unchanged; hardening PR172 verified merged with identical tested tree. SelectedfreshR1/R2/R4/R7/R8 allAPPROVE; independent exact6bd fullquick PASS22m44,13imagecases+149actualfakehostchecks, T3guard13+5adversarialPASS. Sourceproductunchanged, thischeckpoint integrates review receipts/currentstate only. Priorbf9 and6bdreviewedhistory archivedlocally/remotely beforeD112finalrewrite. Current-main rebase/singlecommit finalexacttree/local+hosted gates remainrequired; no mergeclaimed. Fullsignedappliancebuild/boot/cloudimport remainsNOTRUN missingpackagepool/fulltargetbuildcapacity. Historicalchronology below.

# F-images current integration state

Source bf9fa4b2 is archived locally under refs/archive/F-images-reviewed-20261005
and remotely at codex/archive-F-images-reviewed-20261005 before D112 integration.
Manager-owned current-main worktree /root/ngfw-wt/integrate-images-20261005
is integrated on pinned main492c0156 (after external MPLS closeout PR177). The integration additionally fixes the R4 GRUB root/symlink guard; unrelated historical hardening
review stays in the archive, not the final image scope. Source is implemented,
PR173 is attached,11 safety/format tests passed, independent R1/R2/R7/R8 approve
and independent unchanged source quick passed27m57s. R4 found a GRUB host-root/alias-root BLOCKER on bf9. It is now fixed with a
common canonical non-host-root mutation guard plus GRUB path preflight;13 tests
pass, including preserved external boot/config fixtures. Affected R1/R2/R4/R8
verification and final integration T1 remain pending; no combined approval yet.
Final current-main exact-tree quick/hosted merge checks remain required. Full
appliance build, boot and cloud import are NOT RUN pending signed pool/producer
manifest and sufficient build headroom/disposable guests.

The following sections are chronological history, superseded by this summary.

# F-images recovery checkpoint
Branch: `codex/ready-images-20261005`; worktree: `/root/ngfw-wt/ready-images-20261005`; base: `7b507db5`; slot: 16.
Owned product files: `deploy/image/vm/**`, `deploy/image/cloud/**`, `docs/install/images.md`, `test/topology/images/**`.
P14 common is read-only at `deploy/image/iso/common/` (actual merged location).
Local/remote SHA: recorded in git branch history; initial envelope checkpoint has no product changes.
Completed: mandatory context, contribution, policy, worker/task/P14 prompts read; environment preflight.
Actual preflight: debootstrap, qemu-img, sfdisk, grub-install present; mkfs.vfat absent; `/srv/ngfw-artifacts/apt` absent.
Remaining: builder, formats/cloud profiles, offline validation, tests, quick gate, PR and independent review.
Current failure: full appliance image cannot be built without signed published NGFW/APT pool and VPP manifest. No host packages or services changed.
Next command: `python3 -m unittest discover -s test/topology/images -p 'test_*.py'` after source implementation.

## Source checkpoint
Completed: shared layout parser, image target configuration, explicit management
NIC/netplan/firewall inputs, P14-to-P10 completion marker bridge, BIOS+UEFI
bootloader pipeline, qcow2/VMDK/OVA/VHDX/cloud conversions, installed-root offline
inspection, package/provenance/checksum manifests, per-cloud overlays/import docs.
Actual tests: Python 10 tests PASS including real 16 MiB qemu-img roundtrips for
qcow2/streamOptimized VMDK/VHDX/fixed VHD; shellcheck -x PASS.
Full image build NOT RUN: signed NGFW pool/VPP manifest absent and `df -h /`
reports 29 GiB available, below required 40 GiB. Manager explicitly deferred full
mount/conversion builds. No image mount/loop/host boot/package/service changes.
Remaining: unchanged quick gate, independent review, PR; actual full image and
boot/import acceptance need provisioned inputs/build headroom/disposable guests.
Next command: `TMPDIR=/tmp/g-w16 tools/ci.sh --base origin/main`.
Publication: initial checkpoint remote `faaa8dfa7f2c10bfabc277ec3a0a327481055bbb`.
CLI push 403; authorized GitHub connector published the checkpoint successfully.

## Reviewable source state
PR 173 created as draft; source 446bcac7 published. Complete quick gate running.
Additional source review added read-only target sysfs during GRUB probing (only
image disk nodes are exposed in target /dev). Go module vet/test passed; integration
inspection skipped without prepared root. Next: record complete gate result,
publish final status, address manager's independent findings.

## Published recovery state and gate environment failure
Local/remote source SHA `f70d7c7a72ede981afac222d16021b1ed6f41ebe` successfully
published via connector. PR173 draft. Native PR attachment call hung and was
terminated; no attachment success claimed (manager informed).
First complete unchanged gate failed at Go lint because shared `/tmp/g-w16`
returned `no space left on device`. Actual log `/tmp/F-images-quick.log` and
`/root/ngfw-wt/logs/ci/ready-images-20261005-20261005-060153-1096102`.
This is not a passing gate; source checks and TS stages passed, Go gate unresolved.
Retried final exact published source with task-owned root-disk TMPDIR
`/root/.cache/g-w16`, log `/tmp/F-images-quick-final.log`.
Next command: `tail -30 /tmp/F-images-quick-final.log`; record final result before
merge. No gate weakened. Manager retained image review/merge ownership while
worker moved to explicitly assigned HA task in a separate worktree/branch.

## Independent image review fix
Root review found configure() unlinks/symlink creation bypassed put()'s escape
check through parent aliases. Added full affected-path preflight before any write,
unlink or symlink and canonical offline-root requirement. Regressions preserve
outside machine-id/SSH files and prevent any prior root write for aliased dbus,
SSH, ngfw marker and systemd wants paths. Python suite now 11 tests PASS;
shellcheck -x still PASS. Gate retry remains running and will discover this suite.
Next command: publish checkpoint, then manager independently re-run image tests.

Complete unchanged retry finished PASS16m18s, CI GATE PASSED, log
`/tmp/F-images-quick-final.log`; image module passed1.902s after the full-path
preflight fix. Source frozen at published0ca5d636; only evidence additions now.
Remaining: manager independent re-verification and exact final integration-tree
gate; full artifact/VM/cloud execution deferred for concrete missing inputs.
Next command: manager `python3 -m unittest discover -s test/topology/images -p 'test_*.py' -v` then review/integration gate.
