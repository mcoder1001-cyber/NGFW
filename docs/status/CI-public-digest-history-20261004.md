# Public checksum history findings

Post-merge main CI run 37218891891 stopped at the complete history scan, before
product tests. Its two findings predate PR162: commit
`2fd82320a9c431a92c76a47115d047de08268892`, task document
`TD-19-trust-material-20261004.md`, generic-api-key findings at lines14 and27.
Both matches comprise the label "Raw key SHA256" and a64hex checksum of a
downloaded public repository-signing key. Neither is a credential or private key.
The task document's trust approval remains unresolved and is not changed here.

The `.gitleaksignore` entries identify only those two immutable findings by full
commit, file, rule and line. They do not ignore a path, a rule, a value pattern or
any future commit. `.github/gitleaks.toml`, scanner invocation, history depth and
all product tests remain unchanged. Main history is not rewritten.

Independent security review APPROVE: the reviewer independently downloaded both
official public key artifacts and verified their recorded SHA256 values. Redacted
complete history scan PASS:346commits,43.84MB,zero findings. Existing fixture
fingerprints are retained verbatim. Unchanged complete hosted quick remains pending. PR162's product tree already passed full
hosted quick 37217602615; this repair addresses the preexisting history scan only.
