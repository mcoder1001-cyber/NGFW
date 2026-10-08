# Appliance file ownership and identity migration — independent R2 review

Verdict: APPROVE corrected source/security scope at local `4b39ad1f528fbcafb68ab64c0b4315857c077e7c`, tree `7b6454cf391c3c58f0788909f82aa134178e1af3`; author-confirmed remote `81b3a2cf474b7d782087b69b7c0b80ccb1ed2ae9` (PR213). Reviewed the complete branch diff against `417e8fcd`, then the correction against `11dda30e`. No further actionable release-blocking source finding identified within this review. This is not whole-product, installed-service or full-CI approval.

## Findings corrected

- R2-M1: observed identity readback previously read private managed targets without checking public provisioning. State now returns unavailable/provisioning error and no private-target facts when the public link/directory guard fails. The new regression preserves and inspects the private file while confirming it is not falsely reported as installed host identity.
- R2-M2: newly created managed directory entries previously lacked parent fsync before public links could reference them. Every successful mkdir now fsyncs its parent immediately, then opens/verifies the child, normalizes the newly-created directory mode and syncs it. The ordering regression observes real mkdir/fsync/replace operations.
- Added process-boundary coverage confirms the fixed systemctl argv, active/error refusal, inactive/not-found acceptance, and no service command during an already-managed reconfiguration. This helper never starts or stops services.

## Source/security assessment

The one-shot package helper accepts no runtime path argument. Parent traversal uses directory descriptors and NOFOLLOW, enforcing root ownership and no group/world writes. Preflight checks all public/private inputs before public replacement; unexpected symlinks, hardlinks, nonregular or oversized files and conflicting recovery targets are refused. Content is written and synced before public-link replacement, allowing interrupted migration replay. Root-owned public identity state is separate from private agent state. Public links remain outside the agent writable namespace, and Apply, Drift and State all validate provisioning.

CAP_CHOWN plus CAP_DAC_OVERRIDE is an explicit powerful privilege expansion documented by the manager decision. It preserves fixed renderer ownership/private modes and enables atomic daemon snapshot/replacement/restoration. ProtectSystem=strict and the enumerated writable directories remain unchanged except the dedicated identity state and resolved drop-in directories; no broad /etc write allowance or runtime privileged IPC helper was added. These capabilities are not a per-file authorization mechanism, and the review does not claim they are.

## Actual independent checks

- Corrected source: ten selected real filesystem migration/security/process-guard fixtures PASS in 0.017s, no skips.
- Corrected source: complete sysident Go package with race instrumentation PASS in 1.038s, no skips.
- Diff whitespace check PASS.
- Earlier complete nine-fixture run on `11dda30e`: seven pass, two ERROR before their assertions because chown to UID65534 returns EINVAL in this restricted UID mapping. Foreign-owned-parent and real capability snapshot/restore fixtures remain enabled; neither is certified PASS here. Those two cases require the final hosted environment.

No CI, package installation, host identity change or live service action was executed. Actual package configure/upgrade, strict-systemd-sandbox daemon mutations/readback/rollback, public identity consumers and reboot persistence remain laboratory acceptance. Manager-owned final combined CI remains deferred under the owner's instruction.
