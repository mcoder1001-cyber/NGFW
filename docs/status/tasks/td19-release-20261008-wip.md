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

Focused integrity, installer and prior-generator regression tests are running.
No full CI invocation. Next: independent review, publish checkpoint, then manager
integration and combined final gate. See `deploy/lab-python/README.md` for exact
reproduction and truthful generation OS vs target OS boundaries.
