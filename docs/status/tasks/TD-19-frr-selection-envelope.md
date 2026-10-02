# TD-19 FRR explicit-authority certificate selection

Own branch `task/TD19-frr-certificate-selection-20261002`, isolated
`NGFW-TD19-frr-selection`, actual main base
`344c919083f9e5ae7ab177efcdcbda8f1a6b702f`. Owned only narrow
`scripts/00-add-repos.sh` FRR preprocessing, new selection fixtures and phase
metadata. Node gate, raw verify_repo_key, old raw-validator tests and provisioning36
remain unchanged. No main/PR88/board/global host writes.

Use only existing explicit trusted-administrator full40 FRR fingerprint set;
no default pins, download-derived authority, automatic Node pin or extra installed
primary. Preserve every raw bound/nonregular/secret rejection before import.
Fresh private transient GPG homes, no user config/key retrieval/global keyring,
strict abort any import/export error; complete public certificate export without
clean/minimal stripping merges duplicate certificates/revocation material.
Unchanged exact-set/validity verifier runs in another private fresh context on
selected output. Both FRR and Node gates complete before APT/installed-key changes.

Alternatives reviewed: upstream canonical corrected bundle vs explicit constrained
selection. Manager assigned the latter under conditional design769e1709; this
implements the supplied policy, not a new pin/authority/privilege decision.
Actual raw FRR contains unpinned fourth/duplicate and original validator must
still reject it. Revoked/missing/invalid authorized primaries cannot be omitted to
obtain PASS. No anti-rollback or missing upstream revocation recovery claim.

Meaningful actual GPG and fake-host tests required. Local gpg-agent socket errors
must be reported NOT RUN/failed rather than bypassed; positive full workflow
requires actual hosted CI. New independent source/security/docs/operability review
and unchanged full hosted quick remain mandatory. No whole TD19 DONE/target
installation or Ubuntu/Node support claim. All lab acceptance remains NOT RUN.
