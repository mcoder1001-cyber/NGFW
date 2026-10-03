R1 independent fresh correctness/tests review: APPROVE

Reviewed full diff 19aa88a5..71cee90b2a28421480dfd67a6d64ca7f47f37200 (six files).
Local HEAD: 71cee90b2a28421480dfd67a6d64ca7f47f37200
Local tree: 538878ff020cae5e4752bb81d1d1c6d8ed95cc87
Published PR98 SHA from manager evidence: e53bc20bda899ebe5b12d12233e75086fb212edb; manager separately verified same tree.
Worktree: /root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/NGFW-p10-resume

Read AGENTS.md, prompts/00-CONTEXT.md, docs/contributing.md, decision-policy.md, prompts/REVIEW-PROMPT.md, R1-correctness-tests.md, bounded envelope/wip and manager review-plan/T1 evidence. Manager documents were read from NGFW-manager/docs/status/tasks, because they are not in the product worktree.

Findings: none (0 BLOCKER, 0 MAJOR, 0 MINOR).

Acceptance mapping:
- Valid changed .deb: fixture builds real Debian ar archives twice with identical control metadata and newly added payload, asserting same parsed fields and different SHA256. Metadata inspection still uses real dpkg-deb. Separate truncated archive explicitly asserts InvalidBundle. Both run successfully under dpkg-deb 1.23.7.
- Saved transport checksum and size: output temporary descriptor is opened O_RDWR and wrapped w+b; after complete tar closure, flush/fsync, saved stream is rewound and hashlib.file_digest reads the full serialized file. Size is checked against fstat. No pathname reopen is introduced. Existing atomic no-replace publication and cleanup remain in place.
- Independent published-byte proof: deterministic roundtrip fixture compares archive_bytes with saved output stat size and sha256 with hashlib.sha256(output.read_bytes()); tar member inventory includes retained vpp-dbg plus manifest/SHA256SUMS; restored verifier plan equals original. This exercises tar headers/padding as well as payload bytes. Existing bytes expression remains unchanged payload sum.
- Documentation correctly explains the independent trusted report channel and pre-extraction comparison, without claiming publisher authentication or whole-P10 completion.

Actual commands run from product worktree (TMPDIR scratch and PYTHONDONTWRITEBYTECODE prevent source writes):
TMPDIR=/root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/review-p10-r1 PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/debian/bundle -p 'test_verify.py' -v
Result: Ran 23 tests in 54.837s; OK; exit 0. No skips. Includes changed real_deb_metadata_hash_and_architecture and new malformed_archive_rejected, each printed ok.

TMPDIR=/root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/review-p10-r1 PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/debian/bundle -p 'test_export.py' -k deterministic -v
Result: test_deterministic_regular_members_and_verifier_roundtrip ... ok; Ran 1 test in 9.572s; OK; exit 0.

git diff --check 19aa88a5..HEAD
Result: no output, exit 0.
git status --porcelain
Result: no output, exit 0.
python3 --version: Python 3.14.4
dpkg-deb --version: 1.23.7 (amd64).

Per explicit review envelope, this read-only reviewer corroborated focused tests rather than rerunning a source-writing full quick gate. Manager T1 separately records 44 bundle tests PASS and exact unchanged complete hosted quick 37110162496 PASS for the published equal tree. Those are read evidence, not commands independently run here. No installs, service changes, privileges, live VPP acceptance, signing/provenance or complete appliance claims are made. Current-main rebase/tree changes require manager integration validation.

Verdict: APPROVE for this bounded candidate correctness/tests slice.
