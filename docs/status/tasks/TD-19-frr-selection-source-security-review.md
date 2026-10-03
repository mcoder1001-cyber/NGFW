# TD19 selector source R7/R8 security/authority review

Frozen source `763ce40ba29c675e6f6b4505abc2f5a3afb441ef`, base `344c9190`. Independent isolated `review/TD19-selection-source-20261002`; reviewer authored design/authority reports but neither source nor fixtures. No product/board/PR/main edits, APT installation, host services or global GPG access.

**R7/R8 conditional APPROVE bounded source; no new BLOCKER/MAJOR identified.** Merge still requires complete hosted unchanged quick and exact13 certificate fixtures with zero failures/skips/setup errors. Seven real-certificate positive/crypto-negative cases have NOT RUN locally due known GPG agent prerequisite failure, which reviewer did not repeatedly invoke. This report does not waive them.

Source satisfies reviewed installed-trust constraints: administrator-provided full40 FRR pins validated before bootstrap activity; no default/derived A90 trust; Node pin/validator unchanged. Caller input uses one O_NOFOLLOW/O_NONBLOCK/CLOEXEC opened FD, regular/owned/bounded fstat and metadata stability checks; downstream consumers read a private exclusive0600 snapshot. Caller/output parents must be owned0700 directories. Entire raw snapshot is packet/identity inspected for secrets before import. Private fresh import/verify homes, no user options/automatic retrieval, complete explicitly named public certificate export, no clean/minimal filters, nonzero import/export abort through set-e. Unchanged exact primary validity/signing validator checks selected output; direct function byte comparison against base is true. Missing/revoked selected identities cannot be made successful by exporting fewer identities because exact set remains mandatory. Output is exclusive0600 in private caller staging, fsynced, deleted on publication exceptions; temporary homes cleaned by EXIT trap. Same-user/root malicious concurrency is not a new security boundary this helper claims to contain.

Both FRR selection and Node validation finish before global keyring/APT mutations. Original raw duplicate/unmatched rejection cases remain unchanged. Selected unknown primaries cannot leak into installed export. GPG itself parses/canonicalizes imported public certificate material; complete import exit0 and hosted real revocation/duplicate/missing-key tests are therefore material acceptance, not optional coverage.

Actual independent verification:
- ControlledSelection six tests incl actual-GPG malformed refusal: 2.099s, OK; no positive key generation invoked.
- Original TD-19-test-repo-keys.py nine tests: 3.834s, OK.
- Source AST inventory: ControlledSelection6 + RealCertificates7 =13.
- bash -n scripts/00-add-repos.sh and git diff --check exit0.
- Initial unittest discover command reported0 tests (filename naming); direct original runner above then executed all9. No zero-test command is counted as acceptance.
- ShellCheck unavailable locally; NOT RUN. Full quick and hosted13 NOT RUN by reviewer.

Authority limits remain binding from research8c639171 and clarificationda68c73d: current resolute InRelease uses undocumented A90 signing AND primary fingerprint, not one of the published three. Selecting those three cannot authenticate that metadata. This source is only explicit administrator-authorized certificate transformation, not an unattended current FRR bootstrap repair. Do not auto-authorize A90 from bundle/signature success. Node extracted fingerprint is not independent authority, and nodistro is not proof of Ubuntu26.04 support. Legacy source comment saying tested on26.04/fallback unpublished remains historical, not evidence; report/release text must keep target provisioning and boot NOT RUN.

Manager next: retain authority gap and partial TD19 scope, publish/review source checkpoint, require hosted exact13 real certificate outcomes and unchanged full quick on current integration tree, then merge only coherent approved code scope. Repository signature authorization and actual target installation remain separate unmet acceptance, never converted into PASS or silently bypassed.
