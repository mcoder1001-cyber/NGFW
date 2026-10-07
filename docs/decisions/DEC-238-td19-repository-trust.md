# D-238: TD-19 repository bootstrap trust anchors

- raised: 2026-10-04 by TD-19 (was PENDING-TD19-repository-trust)
- decision: product owner chose **option 2**: the recorded official HTTPS key endpoints are the initial trust anchors for exactly the observed identities.
- source: Owner answer given directly in the Claude Code chat session on 2026-10-06 (verbatim: 'به‌عنوان مرجع اعتماد بپذیر (گزینه ۲)').
- affected tasks: TD-19 (unparked for implementation; real target installation remains NOT RUN).

## Context

Evidence: docs/status/tasks/TD-19-trust-material-20261004.md (official endpoints, observed primaries, verified InRelease signatures, signed index and package digests, provenance limits). Before this answer, scripts/00-add-repos.sh refused unattended repository setup unless an administrator supplied `NGFW_FRR_KEY_FINGERPRINTS` and `NGFW_NODESOURCE_KEY_FINGERPRINTS`.

## Decision

Authorized identities (primary fingerprints, exact sets):

| repository | endpoint | authorized primaries |
|---|---|---|
| FRR | https://deb.frrouting.org/frr/keys.gpg | 4A56C7738BB3F81595A805D2A832769908F13ED1, 3D9968AC9AE7BE1169288DDB1FD5839895F57FDA, BBC9ACA9D13025A2C186FF7F741E92A1F6E3975B, A90FC36D9429409798E9C2D874DEED43AB194DBF |
| NodeSource | https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | 6F71F525282841EEDAF851B42F59B5F99B1BE0B4 |

Rules implemented in scripts/00-add-repos.sh:

1. The sets are reviewed source constants (`NGFW_FRR_AUTHORIZED_PRIMARIES`, `NGFW_NODESOURCE_AUTHORIZED_PRIMARIES`). An unset administrator variable resolves to them; a set variable must equal the authorized set exactly (order-insensitive), otherwise setup refuses before artifact preflight, network or APT.
2. The downloaded FRR bundle must contain exactly the four authorized primaries. Repeated copies of an authorized certificate (the endpoint repeats BBC9…975B) are canonicalized by the private import/export; any extra, missing or replaced primary is refused.
3. The NodeSource download must contain exactly the one authorized primary; a duplicate is refused.
4. Both repositories take the same path: raw primary-set gate, private GnuPG import, complete export of exactly the pinned primaries, exact verification of the export. Only GnuPG-validated material reaches the installed keyrings (an attacker subkey without a valid binding to a pinned primary is dropped; a revoked or expired pinned primary is refused). One shared parser (`check_primary_set`) applies identical validity/capability/fingerprint rules to raw downloads and exports; all refusals name D-238.
5. Existing strict validation stays: secret-material refusal, validity/capability checks, private GnuPG homes, bounded files, exact product VPP artifact manifest/digest preflight, both key sets validated before any global mutation.
6. A changed downloaded identity is never accepted automatically. Any new set (rotation, added signer) requires a new recorded owner decision and a reviewed source change.

## Options considered

1. Keep administrator-supplied fingerprints only; unattended provisioning refused (previous default).
2. Authorize the observed official-endpoint identities as initial anchors with exact pinning (chosen).

## Limits

The authorization is provenance by owner choice, not independent fingerprint publication: FRR publishes only three of the four fingerprints separately and NodeSource publishes none. Ubuntu 26.04 target-runtime compatibility (NodeSource table ends at 24.04) and real installation/boot are not established by this decision and remain NOT RUN. This answer satisfies decision-policy item 4 for this bounded trust boundary only.
