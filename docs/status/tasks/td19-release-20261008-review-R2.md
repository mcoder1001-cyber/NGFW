# TD19 release inputs — independent R2 source/security review

Verdict: APPROVE source/security scope at local `4f1187981365823c82e456897e174d7cd11fe7b1`; manager-supplied remote `bfe917b6`. Reviewed all twelve changed paths against `417e8fcd`. No actionable source blocker found. No CI or target installation executed.

Independent verification:

- Release integrity/refusal suite: 5 tests PASS, 0.004s.
- Installer safe-root suite: 13 tests PASS, 2.734s.
- All 24 cached upstream artifacts match the committed digest and size receipts; all 22 selected wheels match resolution hashes. This checks artifact/receipt consistency, not a fresh independent retrieval of registry metadata.
- Inspected every wheel's Requires-Python/Requires-Dist metadata and the source archive's declarative SSHLibrary setup.py. Selected runtime closure is consistent with Linux CPython 3.14 without optional extras.
- Independently executed the complete offline materializer under actual CPython 3.14. It installed all four hash-locked build tools, rebuilt SSHLibrary without online dependency resolution, reproduced SHA256 `f265f7a410d6c7b8ebbcccc156f60088bd13bcc631ee59d22541ea26ffaf9870`, and emitted all 22 verified runtime wheels.

The downloader bounds bytes and verifies both digest and size. Cached symlinks are refused. Publication uses exclusive creation and anchored directory descriptors; existing output is not overwritten. Build execution occurs in a disposable environment, explicitly not a security sandbox. The installer preserves complete hash enforcement and adds no-index/wheel-only behavior only for an explicitly selected wheelhouse. No privilege, lock validation or root-path guard was weakened.

Provenance is accurately described as PyPI HTTPS metadata/digests, not publisher signatures. Generation on Ubuntu 24.04 is not misreported as target execution. Actual Ubuntu 26.04 installation, boot, real SSH and packet acceptance remain NOT RUN; full combined CI remains manager-owned and deferred by the owner's instruction.

One initial reviewer invocation through an obsolete relocated debug venv failed during Python bootstrap because its standard-library prefix was absent. Rerunning through the real CPython installation passed the complete materializer. That environment failure did not execute the product script and is not hidden as a passing invocation.
