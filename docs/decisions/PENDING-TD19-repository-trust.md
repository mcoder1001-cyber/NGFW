# PENDING: repository bootstrap trust for TD-19

- raised: 2026-10-04 by TD-19
- decision: pending owner answer; retain administrator-supplied fingerprint refusal
- parked tasks: TD-19

## Context and exact evidence

See docs/status/tasks/TD-19-trust-material-20261004.md for official endpoints, certificate identities, verified repository signatures, index/package checksums and provenance limits. FRR resolute is signed by A90FC36D9429409798E9C2D874DEED43AB194DBF, present in its canonical HTTPS certificate bundle but absent from the separately published three-certificate list. NodeSource's canonical public certificate primary is 6F71F525282841EEDAF851B42F59B5F99B1BE0B4; no independent official fingerprint publication was located. Byte/signature consistency alone does not authorize bootstrap trust.

## Options and recommendation

1. Keep current strict administrator-supplied identity requirements. This is the current recommended default until a trust authority is explicitly selected; unattended provisioning remains refused.
2. Owner authorizes the recorded official HTTPS endpoints as initial trust anchors for exactly the observed identities. Canonicalize duplicate FRR certificates, pin the exact four-primary set and Node primary, and keep strict set/signature/digest validation. Never accept changing downloaded identities automatically. Target-runtime compatibility still needs its own tests.

No default trust anchor has changed. The security-boundary choice is always-PENDING item4 in decision-policy.md. Source tests and isolated product builds continue; provisioning/installation has not occurred.
