# Installer artifact preflight envelope

Date: 2026-10-07. Worker: bounded read-only installer/artifact preflight.
Branch: `codex/test-host-install-preflight-20261007`.
Worktree: `/root/ngfw-wt/test-host-install-preflight-20261007`.
Base: `origin/main` = `f6ae6e555ecb1edf5397ff1a8115076ebfc11d38`, verified with git ls-remote.
Owned files: only `docs/status/tasks/test-host-install-preflight-20261007*`.

Inspect local artifacts, manifests, builder process evidence and installer source.
No product changes, downloads, package changes, host/target mutation or access to
the excluded license environment file. Manager owns network/SSH persistence and
reboots on 172.30.110.211 and 172.30.126.37. Root install/testing/reboot authority
already exists for those targets but this worker does not exercise it.
No SSH inventory until manager confirms reboot completion; local work is independent.

Deliver exact conditional install command, prerequisites, missing artifacts and
unresolved security approvals; commit and publish branch, report actual remote SHA.
