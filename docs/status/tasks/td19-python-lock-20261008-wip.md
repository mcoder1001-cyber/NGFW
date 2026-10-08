# TD19 offline Python lock WIP

Branch: codex/td19-python-lock-20261008. Owned files: scripts/lab-python-lock.py, scripts/tests/lab-python-lock.py, and td19-python-lock-20261008 task documents. Resolve containing commit with git rev-parse HEAD and published branch with GitHub branch lookup.

Implemented: optional offline wheel-only lock candidate builder. Six direct packages match the installer. Exact target environment assertions, bounded regular inputs, snapshotted wheel hashes, metadata URL/traversal refusal, isolated no-index dry-run resolution, and independent require-hashes closure validation precede output. Existing installer/default fail-closed behavior unchanged. No package installation or service changes performed. Hashes establish byte consistency only; output explicitly requires upstream provenance review.

Actual verification: python3 -I scripts/tests/lab-python-lock.py: 9 tests PASS, 2.634s, no skips. Includes transitive closure/digests and actual installer dry-run syntax interoperability, missing transitive package, incompatible wheel, runtime mismatch, metadata URL, traversal/symlink, malformed pins, pip configuration injection and existing-output preservation. python3 -m py_compile scripts/lab-python-lock.py scripts/tests/lab-python-lock.py PASS. git diff --check PASS. tools/ci.sh check --base main PASS, 3s; gitleaks unavailable, this is the bounded source check, not the full merge gate.

Remaining: upstream-reviewed exact direct versions and wheel provenance, Ubuntu 26.04 target generation and real installation acceptance. No production lock supplied and TD19 remains incomplete. Full unchanged hosted quick gate and independent review required. Next: publish source checkpoint, open draft PR and arrange review/hosted gate.
