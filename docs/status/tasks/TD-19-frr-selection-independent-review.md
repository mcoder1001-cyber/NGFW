# TD19 FRR certificate transformation — independent R1/R2/R4/R5

2026-10-02. **APPROVE source-only transformation** exact
763ce40ba29c675e6f6b4505abc2f5a3afb441ef. Own isolated
NGFW-TD19-frr-selector-review/task/td19-frr-selector-review; report only authored.
No product/test/CI authorship. No findings in this narrow source scope.

R1/R2: explicit authorized unique upperhex40 fingerprint set1..8, no default or
caller-text evaluation. Owned private parent and one NOFOLLOW/NONBLOCK/CLOEXEC
regular owned1MiB bounded FD; same fstat identity/size/mtime/ctime before/after
read; all downstream raw GPG reads private0600 snapshot, not caller path. Full
raw secret sanity/identity checks happen before import/discard; any nonzero GPG
packet/show/import/export aborts. Private no-options homes/no auto retrieval,
complete exports retain certificate revocations/subkeys; unchanged exact-set
raw verifier rejects missing/unexpected selected keys. Publication exclusive0600
in owned0700 caller staging parent; preexisting output preserved. Both FRR/Node
validate before APT/global keyring writes. Raw verifier body byte-identical344.
R4: no VPP API/unit/cap/sharedhost change; Node exact gate untouched. Selector
needs explicit ADMIN-authorized set, not downloaded bundle-derived trust. Published
three versus undocumented current signer A90 authority gap remains unresolved;
no fourth default/unattended/current repository authentication claim approved.
R5: raw/snapshot/output1MiB bounds and <=8 selector pins/private cleanup; kernel
metadata consistency prevents path-reopen substitution but is not hostile same-user
or parent-directory cryptographic authority. Actual import/export processing cost
is delegated to existing GPG private operations; no performance claims.

Actual independent execution: initially unittest discover selected zero tests;
that is NOTPASS. Correct explicit ControlledSelection loader verified exact6 and
ran6 PASS2.039s, zero skips/xfails/xpasses. Includes actual GPG malformed negative,
five controlled semantics/refusal cases, import2/export2 abort, path replacement,
secret/packet errors, FIFO/symlink/size/pins/private parent and fake entry FRR or
Node failure before any APT/install/mv. Additional controlled success with caller
path replacement/private0600 output PASS (not crypto proof); extracted actual
snapshot owner and fstat-mutation rejection before publication both PASS.
Bash syntax PASS. Seven real-certificate positives deliberately NOT RUN here;
local prior gpg-agent generation/setup failure remains attributed and preserved.
Full hosted strict13 actual completed/zero non-success mandatory before merge;
no repetitive blocked generation, false positive crypto/provenance claim, or
invalidation of prior actual test results. Original36/raw9 unchanged by scope.

No real APT/network/global keyring/install/VPP/nft/SSH/host operations or heavy
build. Current main782 Go-module delta still needs preservation in composition;
R7/R8 separate review, new strict13 CI and unchanged hosted full quick remain
required. This is not wholeTD19DONE/current signer authority/target lab acceptance.
