# R2 security — bounded portable archive checksum slice

Reviewer: /root; independent of P10 developer.
Reviewed product SHA: local `71cee90b`, base `19aa88a5`.
Scope: deploy/debian/bundle exporter/tests, bundle user instructions and unique task status/envelope.

No BLOCKER or MAJOR security finding in this delta. Saved transport checksum is computed through the already-open, exclusive, non-following private temporary descriptor after flush/fsync; output remains mode0600, destination path stays pinned and no-replace publication/refusal rules remain. Hashing uses streaming hashlib.file_digest, without allocating the archive. The existing payload byte count is retained, new archive_bytes covers the serialized artifact. Private snapshot size/inventory limits and cleanup remain unchanged. No installation/start/restart, auth boundary, capability, shell invocation or dependency was added.

Documentation correctly requires an independently trusted report and manifest and does not treat co-delivered hashes as publisher authentication. Both English/Persian say to reject mismatch before extraction. Fixtures rebuild a valid changed-payload archive and separately reject malformed archives, preserving failure behavior rather than weakening verification. Test values are synthetic package names/payloads; no secret material added.

Independent checks:
```text
git diff --check 19aa88a5..71cee90b
(exit0, no output)
python3 --version
Python 3.14.4
Added-line scan of reviewed diff:
PRIVATE KEY not added
password= not added
sh -c not added
shell=True not added
CAP_CHOWN not added
```

Full fixture run in progress is T1 evidence separately, not claimed PASS here. No full P10, release signing, real artifacts or clean-device acceptance is certified. Applicable R1/R7/R8/R6 reviews and exact-tree unchanged complete gate remain requirements.

Verdict: APPROVE
