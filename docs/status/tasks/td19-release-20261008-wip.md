# TD19 release dependency closure

Branch: `codex/td19-release-20261008`. Base: `417e8fcd`.
Owned files: `deploy/lab-python/`, release materialization helper and its tests,
`40-install-lab.sh` wheelhouse support and task documentation. No board edits.

Official PyPI release JSON was retrieved for all six direct packages on 2026-10-08.
Exact direct versions selected in `deploy/lab-python/direct.txt` (latest stable as
observed); no dependency version is inferred from a hash. SSHLibrary 3.8.0 has only
an upstream sdist, requiring a reproducible local wheel build and separate source
provenance. CPython 3.14.8 is available for target-language resolution. The actual
build environment is Ubuntu 24.04, not the target Ubuntu 26.04.

In progress: fetch artifacts, verify against independently fetched official PyPI
JSON, lock build tools, reproduce wheel bytes, generate complete closure, and
exercise offline installation/imports. No CI has been invoked. Target Ubuntu
26.04 installation/boot remains unexecuted lab acceptance.

## Completed release inputs

22 runtime pins and hashes generated; all 24 upstream source/build/runtime
artifacts independently matched official PyPI digests. Reproducible SSHLibrary
wheel built from source (three identical builds); offline wheel materialization
and fresh CPython3.14 installation succeeded. `pip check` found no broken
requirements. Six module imports (base Scapy) and SSHLibrary construction passed.
`scapy.all` was attempted and failed on netlink socket creation with EPERM;
full host/packet functionality remains laboratory acceptance, not PASS.

Focused integrity, installer and prior-generator regression tests all passed.
No full CI invocation. Next: independent review, publish checkpoint, then manager
integration and combined final gate. See `deploy/lab-python/README.md` for exact
reproduction and truthful generation OS vs target OS boundaries.

## Exact evidence and publication

Source checkpoint local `79c3db6b38da04b493651890c51f1a1f98c6b8ba` and remote
`ba1cf9f94146fc3b7c6d617b8c5716f9facd5a88` have identical tree
`6247552fe46e6b2f173c140b09459497d653c339`. Remote publication succeeded on
`codex/td19-release-20261008` using `[skip ci]`; complete CI is deferred by owner.

Actual focused results on that source:

```text
python3 -I scripts/tests/lab-python-release.py
Ran 5 tests in 0.003s — OK
python3 -I scripts/tests/td19-safe-root.py
Ran 13 tests in 2.531s — OK
python3 -I scripts/tests/lab-python-lock.py
Ran 14 tests in 7.051s — OK
bash -n scripts/40-install-lab.sh — exit 0
git diff --check — exit 0
```

Final materializer used the offline artifact cache with all independently
verified original upstream bytes; it force-reinstalled all four build tools
from `build.lock`, built SSHLibrary without index/dependencies/build-isolation,
and verified/emitted 22 runtime wheels. Separate builds reproduced SHA256
`f265f7a410d6c7b8ebbcccc156f60088bd13bcc631ee59d22541ea26ffaf9870`.

A disposable CPython 3.14 venv installed every runtime distribution from those
wheels using `--force-reinstall --no-index --only-binary=:all: --require-hashes`;
exit 0. `pip check`: `No broken requirements found.` Real installer dry-run
with release lock and wheelhouse: exit 0, supplied version/hash lock validated.
No native APT/apply/system-service action occurred. Ubuntu's official package
metadata confirms the target language series:
https://packages.ubuntu.com/resolute/python3 (Python 3.14).

Open work: independent source review, integration/final combined CI (manager),
and actual Ubuntu 26.04 lab installation/boot/SSH/packet acceptance. The old
missing production pins/provenance/closure inputs are supplied by this branch.
