# P10 packaging resume — independent R8 operability review

Reviewed evidence HEAD `ee8c7ef9`; frozen product `db721ff49a747f8e7453ad1b4f4e11d085237d95`, including previously unreviewed unit-hardening checkpoint `aa76368a49f55e1dfa6e3198c02fdc9509ffc58a`. Independent reviewer owns this report only. Read R8/shared review instructions, P10 recovery envelope, historical checkpoint/final/installer reviews and firstboot final ruling. Historical findings and bounded corrections are preserved, not replaced with blanket appliance approval.

## Findings and scope

No new BLOCKER, MAJOR or MINOR in the bounded storage/hardening delta.

- API postinst provisions the three required durable storage roots under `/data` with 0750 and vrx ownership. Configure is repeatable, intentionally reapplies directory modes, and does not recurse into backups/support/update content. User docs accurately distinguish root-directory permissions from preservation of files inside. Independent redirected-script fixture confirms existing backup bytes and file metadata survive reconfigure. Existing `ReadWritePaths=/data` makes these roots available to the API sandbox. No new runtime dependency or service start is added.
- Unit hardening preserves the prescribed three agent capabilities and zero API capabilities. Agent network/netlink and non-cgroup namespaces remain available; no PrivateDevices, ProtectHostname or ProtectClock was imposed on the agent. Node API does not gain a JIT-breaking MemoryDenyWriteExecute policy. Existing restart policy, firstboot condition and service ordering are retained. Offline scores are reproduced below; they are not installed-sandbox compatibility evidence.
- Interrupted provisioning fails visibly under `set -eu`; a later configure can finish missing storage roots. Existing operator data is retained. No new remove/purge deletion or reboot action is introduced. The preparation warning correctly names unresolved release gates instead of obsolete firstboot scaffolding claims.
- Earlier installer no-start correction is still present; its isolated regression tests pass. Firstboot arbitration and APT signing-path/pin fixes retain their historical reports. Worker signing fixture is explicitly distinguished from actual repository publication.

Known runtime/release constraints remain explicit: dynamic punt synchronization, missing licensing/copyright, daemon file ownership requiring a pending CAP_CHOWN/helper decision, and atomic system-identity writes needing writable parents under strict filesystem protection. These are real unfinished product/security decisions, NOT merely unavailable lab tests. This R8 approval does not approve changing capabilities, making `/etc` broadly writable, or declaring P10 release-ready. The PENDING document continues to park affected runtime use/acceptance.

## Independent verification actually executed

Worktree `/workspace/scratch/de92de7d9874/NGFW-review-packaging-r8`:

```text
python3 deploy/debian/vrx/tests/test_packaging.py
Ran 9 tests in 1.038s
OK
python3 deploy/debian/vrx/tests/test_runtime_profile.py
Ran 2 tests in 1.477s
OK
systemd-analyze --offline=yes security deploy/systemd/vrx-api.service
exit 0; Overall exposure level for vrx-api.service: 3.0 OK
systemd-analyze --offline=yes security deploy/systemd/vrx-agent.service
exit 0; Overall exposure level for vrx-agent.service: 5.0 MEDIUM
bash -n deploy/debian/vrx/prepare.sh
exit 0
sh -n deploy/debian/vrx/debian/vrx-api.postinst
exit 0
git diff --check 53a43ce5 HEAD
(no output; exit 0)
```

Shellcheck is unavailable; no shellcheck PASS claimed. No host package installation, service start, live namespace/network/firewall operation, complete quick run, actual .deb lifecycle, installed boot or appliance acceptance was executed by this reviewer. Fixture tests and offline scores cannot establish these results.

**R8 verdict: APPROVE bounded storage and base-unit-hardening checkpoint.** Whole P10 remains partial, with privilege decisions still pending. Required complete CI, other applicable reviews, licensed release artifacts and actual appliance acceptance retain their separate gates.
