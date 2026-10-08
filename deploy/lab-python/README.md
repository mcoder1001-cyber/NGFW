# Reviewed lab Python release inputs — CPython 3.14 / Linux x86_64

`requirements.lock` pins all 22 runtime distributions and their selected wheel
SHA256 values. `direct.txt` records the six required automation packages. Exact
versions were selected from stable, non-yanked PyPI releases on 2026-10-08.
`upstream.json` records release JSON URLs, metadata digests, official artifact
URLs, upload times, sizes and published SHA256 values. All 24 original artifacts
(runtime/build wheels plus SSHLibrary source) were downloaded and independently
matched against PyPI's HTTPS release JSON; none had a vulnerability entry there
at retrieval. This is registry provenance, not a publisher-signature or an
independent vulnerability-audit claim.

SSHLibrary 3.8.0 publishes only a source archive. Its declarative `setup.py` was
reviewed and built using `build.lock` (pip, setuptools, wheel and packaging),
without build isolation downloads or dependency resolution. The derived wheel
was reproduced with identical bytes from separate builds using the recorded
`SOURCE_DATE_EPOCH` and `PYTHONHASHSEED`; its digest is distinct from its upstream
source digest. The wheel is not falsely represented as an upstream publication.
Build code runs in a disposable venv, which is not a security sandbox. No wheels
or source archives are committed to this repository.

## Prepare and install

Run preparation on CPython 3.14, Linux x86_64. The output directory must not exist:

```sh
python3.14 -I scripts/lab-python-release.py --output /path/to/lab-wheels
```

Every downloaded file must match the committed upstream size and hash. The
script builds SSHLibrary offline with the exact build-tool closure, refuses a
changed derived wheel, and emits only the 22 selected runtime wheels. An offline
machine can instead use `--artifact-cache /path/to/upstream-artifacts`; that
folder must contain all original filenames in `upstream.json`, including build
tools and the source archive. Cached files receive the same hash checks.

Copy the wheelhouse and this repository to the Ubuntu 26.04 target, then use the
existing explicitly selected lock interface:

```sh
sudo env \
  NGFW_LAB_REQUIREMENTS="$PWD/deploy/lab-python/requirements.lock" \
  NGFW_LAB_WHEELHOUSE=/path/to/lab-wheels \
  bash scripts/40-install-lab.sh --dry-run
# Review the plan; execute the same command without --dry-run to apply.
```

With `NGFW_LAB_WHEELHOUSE`, pip uses `--no-index --only-binary=:all:` and the
complete `--require-hashes` lock. There is no source build, online fallback, or
floating dependency installation on the lab target. Native APT tools are still
installed through the existing installer. Existing callers may explicitly
supply their own independently reviewed lock; no lock is selected implicitly.

## Verification boundaries

`resolution.json` is the unmodified generator receipt. It intentionally records
**Ubuntu 24.04 / CPython 3.14.8** as the actual generation environment, rather than
claiming execution on Ubuntu 26.04. Native wheels target CPython 3.14 or stable
ABI and x86_64 manylinux (maximum glibc floor 2.34). The exact lock successfully
installed from the materialized wheelhouse in a fresh CPython 3.14 venv, with
`pip check` reporting no broken requirements. Robot, SSHLibrary, base Scapy,
pytest, requests and paramiko imported; SSHLibrary constructed successfully.
Full `scapy.all` import attempted netlink interface enumeration and was denied
by this execution environment (`EPERM`); packet/host functionality is not
certified by these checks.

Actual Ubuntu 26.04 native APT installation, service/boot behavior, real SSH and
packet-tool acceptance remain laboratory checks. This release input closes the
missing pins, provenance and reproducible Python dependency-closure work; it
does not claim those laboratory outcomes.
