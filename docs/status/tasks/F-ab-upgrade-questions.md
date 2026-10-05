# F-ab-upgrade implementation decisions / integration
- Writable ext4 root slots chosen as task default; no squashfs/overlay dependency.
- Dedicated Ed25519 key chosen as task default; private key remains outside source and images. Key rotation remains an administrator operation.
- P14 has per-slot /boot and a shared EFI filesystem. Provision offline only: install stable EFI GRUB config/grubenv, trampoline from slot A firmware GRUB, and preserve old grub.cfg. Stage reproduces the trampoline in every replacement slot. No host boot mutation.
- Shared /var/lib/ngfw is a bind mount of /data/ngfw; existing P14 layout has no separate partition for it. Offline provisioning copies initial data once; bundles exclude all shared data.
- Database migrations must preserve previous binary compatibility. A root-owned pre-start service dumps database ngfw before API startup; existing API entrypoint executes migrations. Automatic rollback switches root; backup restoration is an explicit administrator recovery operation with services stopped.
- Fixed argv CLI is the consumer contract. No new agent privileges or new root action socket added. F-backup-restore must use the existing reviewed executor boundary; a template unit accepting caller-controlled operation/bundle would broaden it.
- Manager CI hook request: run deploy/upgrade/tests/run.sh and the Go health probe tests in quick CI. Worker does not edit tools/ci.sh. A test/topology Go wrapper also invokes both so unchanged quick CI covers them.
- Minimal package prepare/install hooks needed because current P10 has no historical upgrade anchor. Coordinate with root's hardening hunks.
