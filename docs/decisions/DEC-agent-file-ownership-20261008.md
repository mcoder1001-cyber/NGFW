# Product daemon ownership and atomic identity writes

Decision: source implementation selected on 2026-10-08 by the manager under the owner's explicit instruction to complete the disclosed non-laboratory work and merge. This is not permission to modify or activate services on a live appliance. Independent review and final hosted tests are required before integration; actual installed-service acceptance remains unexecuted.

## Chosen boundary

Keep `ProtectSystem=strict`, `ProtectHome=yes`, `NoNewPrivileges=yes`, the existing address-family restrictions and explicit daemon writable directories. Add only `CAP_CHOWN` and `CAP_DAC_OVERRIDE` to the original `CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK` set.

`CAP_CHOWN` is required by the shared atomic writer when assigning its new inode to `frr:frr`, `_kea:_kea` and `_chrony:_chrony`. It is not sufficient alone: chrony keys are `_chrony:_chrony` mode0600 and snapshots/readback must read them; daemon-owned directory permissions must also allow atomic replacement. `CAP_DAC_OVERRIDE` enables those operations without changing daemon ownership, private-file modes or group membership. The executable fixture has negative controls for each missing capability and exercises private foreign-owned file read, new inode creation, chown, rename and exact snapshot restoration.

These are powerful capabilities: DAC restrictions on files visible to the agent are bypassed. The change does not claim a per-file authorization broker. The existing mount sandbox and explicit writable paths remain necessary; no broad writable `/etc`, no new socket and no runtime privilege escalation helper are introduced. Compromise of the privileged agent remains a serious host-security event.

Alternatives considered: CAP_CHOWN alone (incorrect for private readback); CAP_DAC_READ_SEARCH alone plus CHOWN (does not permit atomic writes into daemon-owned directories); changing daemon ownership/modes (breaks existing private-file/daemon contracts); a fixed privileged broker (larger cross-renderer snapshot/write/remove/restore IPC trust boundary). The selected two-capability change preserves existing renderer contracts and is directly reversible in the unit.

## Strict filesystem identity

Public `/etc/hostname`, `/etc/localtime`, `/etc/issue`, `/etc/issue.net` and `/etc/motd` become fixed package-provisioned links into root-owned mode0755 `/var/lib/ngfw-system-identity`. Their existing content is copied before atomic link replacement and survives interrupted migration/reconfiguration. This separate public directory avoids exposing the private mode0750 `/var/lib/ngfw` parent. Resolver writes receive only `/etc/systemd/resolved.conf.d` as a writable directory. Other `/etc` paths remain outside this added write allowance.

The one-shot package helper uses descriptor-relative no-follow traversal, trusted root ownership and non-writable parents; rejects unmanaged links, hardlinks, directories, oversized source files and conflicting recovery targets; and fsyncs writes and directories. It accepts no runtime path requests. Initial migration refuses an active or unverifiable old agent on systemd hosts; stop the service in the administrator's package-maintenance procedure before configuring this upgrade. It does not stop, start or reload services itself. Package scripts still use `dh_installsystemd --no-enable --no-start`.

The renderer operates on the managed targets. Its write and drift checks require the public links and protected directory ownership to be intact; a missing package migration cannot silently appear successful. Isolated test slots retain their previous private paths and never change public host identity.

## Acceptance

Local: ten real isolated Python migration/security fixtures pass; sysident race tests pass including provisioning guard controls. Real foreign-UID and capability fixtures cannot run in this execution environment because its UID map contains only UID0; attempts to use UID65534 return EINVAL. Both tests remain enabled, without skip or expected-failure, in packaging fixtures and the Debian build test target for the final hosted validation.

Laboratory: package configure/upgrade, public identity consumers, actual strict-systemd-sandbox daemon configuration snapshot/reconcile/rollback and reboot persistence are NOT RUN. Prior P10 development installation evidence remains valid and is not reclassified as failed. Product license authority remains a separate unresolved decision.

## Exact remote-access compatibility

Remote-access source authentication must attest the same five-capability product agent. Its canonical source mask and peer process validation share one constant; exact effective, permitted and bounding equality is retained, with no ambient privileges and only the previously bounded pre-normalization SYS_ADMIN inheritance. The optional hardening drop-in preserves this same mask and dedicated identity write paths. Both exact fragment SHA256 attestations track the reviewed source units, with a regression deriving the masks and digests from those packaged files. Every single missing/extra bit in each of the three capability sets is refused. Broker SYS_ADMIN/SYS_CHROOT and profile-daemon capabilities remain separate and unchanged; source privilege sets are not accepted as broker identity.

Changing the agent fragment or optional hardening fragment requires updating the paired RA installation attestation and passing this consistency test. A unit-only packaging edit cannot be declared compatible with RA from isolated sysident tests.
