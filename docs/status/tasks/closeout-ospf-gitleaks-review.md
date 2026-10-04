# Independent classification of final-gate public digest findings

Verdict: **APPROVE precise fingerprint-only exclusions for these two findings**. No scanner rule or path-wide exemption is justified.

Reviewed the final gate JSON report using only RuleID, File, StartLine, EndLine, Commit and Fingerprint metadata. Secret and Match fields were not printed. Both reports identify historical commit `2fd82320a9c431a92c76a47115d047de08268892`, `docs/status/tasks/TD-19-trust-material-20261004.md`, `generic-api-key`, beginning at lines 14 and 27. The source explicitly describes public key-bundle SHA256 checksums immediately before each value.

Independent read-only HTTPS downloads from the documented canonical endpoints reproduced the exact checksum values:

- FRR public key bundle, `https://deb.frrouting.org/frr/keys.gpg`: 16112 bytes, SHA256 `bf10935b9296e2ce7c5d9855fa29ef30c35810b0fc4b1f53005494a04a33554d`, exact document match.
- NodeSource public key bundle, `https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key`: 1717 bytes, SHA256 `b42e0321dabdc24e892115da705cf061167eac12a317f23d329862d0aa0a271d`, exact document match.

These are hashes of freely downloadable public material, not API credentials or private keys. This classification does not authorize these repository keys as installer trust pins; that separate owner review and fail-closed policy remains unchanged.

Only approved exclusion fingerprints:

```
2fd82320a9c431a92c76a47115d047de08268892:docs/status/tasks/TD-19-trust-material-20261004.md:generic-api-key:14
2fd82320a9c431a92c76a47115d047de08268892:docs/status/tasks/TD-19-trust-material-20261004.md:generic-api-key:27
```

Reviewer made no `.gitleaksignore`, scanner configuration or product code changes. Manager must run unchanged gitleaks/complete quick with the precise exclusions and record the actual result.
