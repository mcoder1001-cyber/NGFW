# TD-19 repository key gate security verification round

Frozen `95516283df181587ed9f18a3c96ade03406f9647`, isolated task/TD19-repo-key-recheck. Initial53f69939 BLOCK report20251d33 preserved. No author product changes by reviewer.

**APPROVE this corrected R1/R2/R5 key-gate scope. Previous MAJOR closed.** Primary validity now explicitly permits only ordinary supported -,o,q,m,f,u; invalid/disabled/revoked/expired/not-valid/special/empty/unknown statuses fail before dearmor. Structured pub fields require valid length, positive decimal bits/algorithm/creation timestamp, full key ID, supported expiry syntax and signing capabilities without disabledD/unknown capability values. Existing trusted exact primary set, secret rejection and both-keys-before-global-write guards remain unchanged. Curl now has10s connection/60s total bound in addition to1MiB size, resolving previous liveness MINOR.

Independently checked GnuPG primary documentation `https://github.com/gpg/gnupg/blob/master/doc/DETAILS`, Field2 and Field12 meanings. The document identifies invalid/disabled/not-valid distinctly from unknown/undefined ordinary statuses. Conservative unsupported status rejection is appropriate; this parser does not create provenance from trust-level labels.

Personally executed full strict fixture runner:

```
python3 docs/status/tasks/TD-19-run-fixtures.py
Ran32 tests in8.638s — OK
failures0 errors0 skipped0
bash -n scripts/00-add-repos.sh
exit0
```

Nine key fixtures exercise the actual extracted function through controlled GPG arguments/output: former i/d/? bypasses now refuse, r/e/n/special/unknown/empty refuse; malformed numerical/keyID/time/capability fields, disabledD and nonsigning records refuse before output; supported ordinary states still succeed. These are parser/argument tests, not cryptographic signature proof.

Also personally executed actual installed GPG binary against a COPY of the existing local public-only Ubuntu archive keyring, all outputs in a0700 temporary home. Exact verify_repo_key function succeeded, dearmored temporary output created, three primary records accepted. Expected fixture fingerprints were derived locally ONLY to check mechanism compatibility; they are not used or claimed as authoritative FRR/NodeSource production pins. No network key retrieval, secret/private key material, global keyring/APT/system file writes or repository installation occurred.

Per-file atomic keyring publication remains distinct from cross-file rollback. Trusted private download parent/root keyring directory assumptions remain explicit. Authoritative published production fingerprints are unresolved; full unattended TD19 bootstrap and host acceptance remain incomplete. Hosted quick must pass on final integration head before merge; this security delta is separate from foundation PR fixture scope.
