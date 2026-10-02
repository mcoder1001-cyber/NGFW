# PENDING: product agent ownership of daemon configuration files

Status: pending owner decision; parks only appliance acceptance/runtime use of affected daemon renderers. Packaging, tests, reviews and unrelated features continue.

## Concrete mismatch

P10 prescribes the agent capability set `CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK`. The existing atomic file writer (`apps/agent/internal/renderers/helpers_files.go`) creates a new temporary file as the agent and calls `os.Lchown` to the renderer's fixed `File.Owner`. Existing product renderers require daemon UIDs, including FRR `frr:frr`, Kea `_kea:_kea`, and chrony `_chrony:_chrony`. Root with CAP_CHOWN removed cannot assign these different UIDs. Directory SGID or precreated destination files do not solve changing the new temporary file's UID.

No capability was added and no host service changed. Actual sandboxed appliance acceptance is NOT RUN. This is a concrete contract/security decision, not a lab-only waiver.

## Options

1. Recommended for review: explicitly add CAP_CHOWN to the product agent unit while retaining the current narrow writable paths and fixed renderer ownership. Small implementation/reversal cost; changes an agent privilege boundary and requires owner approval.
2. Introduce a privileged file-writing helper with a narrow authenticated request contract. Larger implementation, review and deployment scope.
3. Redesign renderer ownership and daemon read permissions so all generated files remain owned by the agent. Requires cross-renderer compatibility proof and changes the current product file contract.

Decision policy: `docs/decisions/decision-policy.md`, always-PENDING item 4 covers deviations to agent privileges. Existing task authority authorizes the three listed capabilities, not an additional one.

Decision: pending. Affected acceptance: P10 appliance daemon configuration installation/reconciliation; no whole-project stop. Record the resolution in the decision log before applying a privilege change.

## Additional strict-filesystem mismatch

System identity product paths include `/etc/hostname`, `/etc/localtime`, `/etc/issue`, `/etc/issue.net`, `/etc/motd` and a resolved drop-in. The shared atomic writer creates a temporary sibling in the parent directory before rename. Under ProtectSystem=strict, granting only a single file as writable does not make `/etc` writable for temporary creation/rename; a file bind mount can also prevent replacement. Existing approved writable daemon directories do not cover these global file parents.

CAP_CHOWN alone does not solve this second issue. Do not add broad writable `/etc` silently. A reviewed narrow privileged file-writer/explicit product-path architecture, or an owner-approved filesystem exception with an assessed security boundary, is required for installed system-identity mutations. Keep readback/banner code and unrelated development moving; only affected appliance acceptance depends on this decision.
