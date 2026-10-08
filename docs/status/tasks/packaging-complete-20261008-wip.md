# P10 identity/ownership completion checkpoint

Branch: `codex/packaging-complete-20261008`, based on main `417e8fcd`.
Owner completion direction: 2026-10-08, complete source work and merge; run CI only once at the end. Manager selected narrow managed identity storage and CAP_CHOWN source repair after the previously disclosed mismatch. No live host changes are authorized by this source checkpoint or performed here.

Owned paths: sysident paths; agent systemd unit; Debian agent install/postinst/rules; fixed identity provisioning helper and its fixtures; this task record. Board is manager-owned.

Implemented: atomic, fsynced, descriptor-relative migration of five public identity files into root-owned 0755 `/var/lib/ngfw-system-identity`, with fixed `/etc` symlinks. This separate parent preserves `/var/lib/ngfw` confidentiality while making public identity readable. Resolver gets only its dedicated writable drop-in directory. ProtectSystem=strict remains; broad writable `/etc` is not added. Product reconciliation refuses absent/redirected public links instead of silently writing disconnected state. Existing content is preserved; unmanaged symlinks, hardlinks, unsafe parents, oversized files and conflicting recovery data fail closed. Initial migration requires the old agent inactive on systemd hosts.

Verification: 7 focused Python migration/security tests passed. `go test -race -count=1 ./internal/renderers/sysident` passed (1.041s); repeated after provisioning readback guard + negative controls passed (1.055s). Foreign-UID ownership fixture cannot execute in this environment: only UID/GID0 is mapped; chown to UID65534 returns EINVAL. This real test remains in the suite without skip for the final hosted run. Aggregate CI not run by user direction.

Remaining source: CAP_CHOWN repairs ownership assignment but daemon-owned 0600 secret readback and potentially directory access also require a privilege choice or broker. Do not claim full ownership closure from CAP_CHOWN alone. Manager is considering the precise boundary. No authoritative NGFW product license found in LICENSE/COPYING/copyright paths or package.json metadata; no license invented.

Next: finalize ownership boundary with manager, add executable capability/readback regression, independent security review, publish checkpoint, include final source in one combined CI; actual package provisioning/sandboxed daemon acceptance remains laboratory-only.
