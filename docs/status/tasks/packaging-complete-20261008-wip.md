# P10 identity/ownership completion checkpoint

Branch: `codex/packaging-complete-20261008`, based on main `417e8fcd`.
Owner completion direction: 2026-10-08, complete source work and merge; run CI only once at the end. Manager selected narrow managed identity storage and CAP_CHOWN + CAP_DAC_OVERRIDE source repair after the previously disclosed mismatch. No live host changes are authorized by this source checkpoint or performed here.

Owned paths: sysident paths; agent systemd unit; Debian agent install/postinst/rules; fixed identity provisioning helper and its fixtures; this task record. Board is manager-owned.

Implemented: atomic, fsynced, descriptor-relative migration of five public identity files into root-owned 0755 `/var/lib/ngfw-system-identity`, with fixed `/etc` symlinks. This separate parent preserves `/var/lib/ngfw` confidentiality while making public identity readable. Resolver gets only its dedicated writable drop-in directory. ProtectSystem=strict remains; broad writable `/etc` is not added. Product reconciliation refuses absent/redirected public links instead of silently writing disconnected state. Existing content is preserved; unmanaged symlinks, hardlinks, unsafe parents, oversized files and conflicting recovery data fail closed. Initial migration requires the old agent inactive on systemd hosts.

Verification: 10 focused Python migration/security tests passed. `go test -race -count=1 ./internal/renderers/sysident` passed (1.041s); repeated after provisioning readback guard + negative controls passed (1.055s). Foreign-UID ownership fixture cannot execute in this environment: only UID/GID0 is mapped; chown to UID65534 returns EINVAL. This real test remains in the suite without skip for the final hosted run. Aggregate CI not run by user direction.

Ownership source: manager selected CAP_CHOWN plus CAP_DAC_OVERRIDE for private daemon snapshot/readback and atomic replacement while preserving existing renderer modes/owners and strict writable paths. Decision and risk recorded in DEC-agent-file-ownership-20261008.md. Real foreign-UID and capability tests remain mandatory in final hosted packaging fixtures, not a lab-only waiver. No authoritative NGFW product license found in LICENSE/COPYING/copyright paths or package.json metadata; no license invented.

Remote checkpoints: 347c5d9e matches local ee82ad0; 4b20e452 matches29d6b1b; bb7da90e matches8689506 (tree10a29e5709f9163258f4103bde80df787aea6c1d). All published with [skip ci].

Next: independent security review, publish final checkpoint, include final source in one combined CI plus packaging fixtures; actual package provisioning/sandboxed daemon acceptance remains laboratory-only.

Independent review round: R2-M1 State() disconnected-target facts and R2-M2 mkdir parent durability fixed. State now reports provisioning unavailable without private-target host facts; each new ancestor is persisted before linking. Added active/error/inactive fixed-systemctl guard, command-free reconfigure and fsync ordering fixtures. Ten non-UID Python tests PASS (0.027s); sysident race PASS (1.050s). Two real UID/capability fixtures remain enabled and require final hosted execution. Draft PR213 holds reviewable checkpoints; R2 APPROVE corrected source at remote81b3a2cf/local4b39ad1f; receipt in packaging-complete-20261008-review-R2.md. Hosted capability/ownership fixtures and final combined CI remain outstanding.
