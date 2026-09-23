# deploy/vpp — the VPP the product ships (WBS D0.2)

The appliance runs FD.io VPP built **from source by us**, never from the FD.io APT repository (D-001). This directory
makes that build reproducible from the repository: a pinned upstream tag, a patch series, hash-locked build inputs,
one build script and a manifest that the packaging (P10) and the installer ISO (P14) consume. Decisions: D-089.

| file | purpose |
|---|---|
| `VERSION` | data (parsed, never sourced): upstream URL, branch, tag, **tag object + commit hash**, upstream Debian version, `VPP_LOCAL_REV`, the package set, what vrx-a runs, what the product ships |
| `patches/series`, `patches/*.patch` | product patch series (`Status: product`) — plus `Status: demo` patches that are applied only with `--demo` |
| `patches/optional/*.patch` | applied only by a build option (`--trace-plugins core`) |
| `build-patches/series`, `build-patches/*.patch` | build-infrastructure patches, always applied; may only touch `build/` (never product code), do not change the version |
| `pydeps.lock` | the Python packages the build runs (DPDK's meson venv): exact versions + PyPI sha256 + file + URL |
| `lib.sh` | shared helpers (VERSION parser, path guards, strict patch apply, apt check, pydeps) |
| `build.sh` | the build (below) |
| `verify.sh` | static gate (+ `tests/run.sh`); `--require-files <out>` validates a produced output; `--install-gate` for installs |
| `tests/run.sh` | unit-style tests of lib.sh, build.sh guards and verify.sh (no clone, no network, ~30 s) |
| `.build/` | scratch: source tree, ccache, wheelhouse, outputs — **git-ignored**, never committed (nor is any `*.deb`/`*.whl`) |

## Versions (D-089)

| build | patches applied | Debian version | output dir |
|---|---|---|---|
| `build.sh` | build-patches only (the product series is empty today) | `26.06-release` (= upstream, = vrx-a today) | `.build/out/26.06-release/` |
| `build.sh --demo` | + `Status: demo` patches | `26.06-release+vrx<N>` | `.build/out/26.06-release+vrx<N>-demo/` |
| `build.sh --trace-plugins core` | + optional V18 patch | `26.06-release+vrx<N>` | `.build/out/26.06-release+vrx<N>-trace-core/` |
| once `patches/series` holds a product patch | product patches | `26.06-release+vrx<N>` | `.build/out/26.06-release+vrx<N>/` |

`N` is `VPP_LOCAL_REV` in `VERSION`: **bump it whenever the product series changes**, so each patched build set has a
distinct, higher version (`dpkg --compare-versions 26.06-release+vrx1 gt 26.06-release` is true). Mechanism: after
patching, `build.sh` replaces `src/scripts/version` (upstream derives the version from `git describe`) with a script
that prints the local version; the library soname prefix (`26.06`) is unchanged. A demo build and a product build with
the same `N` would share a version — demo builds are never installed (install gate below), so that is harmless.

## Build

```bash
deploy/vpp/verify.sh                                  # static gate + tests (~30 s)
deploy/vpp/build.sh --prepare-only                    # clone/verify/patch/version/deps/pydeps only (~1 min once cloned)
nohup deploy/vpp/build.sh > /root/ngfw-wt/logs/<id>-vpp-build.log 2>&1 &   # full build (20–45 min, 8 jobs) — poll the log
deploy/vpp/build.sh --demo                            # the demo series (V16) → 26.06-release+vrx1, variant demo
deploy/vpp/build.sh --trace-plugins core              # V18 variant
deploy/vpp/build.sh --offline-reference               # no network: source from /root/vpp (read-only), wheels from the verified cache
```

In order (any mismatch aborts):

1. **Path guards first**: `--build-dir` must resolve (realpath) inside `deploy/vpp/.build`, `--out` inside the build dir;
   `/`, `$HOME`, `/root`, `/root/vpp`, `/etc/vpp`, `/usr`, `/root/ngfw` and their subtrees are always refused. Every
   `rm -rf` goes through `vrx_rm_rf`, which only deletes *strictly below* a root carrying the `.vrx-owned` marker; an
   existing `--out` is emptied only if it holds nothing but build.sh artefacts.
2. `verify.sh` static checks; `VERSION` parsed as data with a strict format per key.
3. Source in `.build/src/vpp`: `git clone --reference-if-able /root/vpp --dissociate <VPP_UPSTREAM_URL>` (objects
   borrowed from the local clone, then copied — no dependency on `/root/vpp` remains). **An unreachable upstream is
   fatal**; building from `/root/vpp` requires the explicit `--offline-reference`, and `manifest.build.source` records
   `upstream` or `reference`. Nothing is ever built, checked out or cleaned inside `/root/vpp`.
4. The tag must resolve to `VPP_TAG_OBJECT` and peel to `VPP_COMMIT`; pristine checkout (`git clean -ffdx`, keeping
   ccache and the external tarball cache); the pristine `src/scripts/version` must print `VPP_DEB_VERSION`.
5. Patches: `build-patches/series`, then `patches/series` (demo ones only with `--demo`), then options — each with
   `git apply --check` followed by `git apply`: exact context, **no fuzz** (GNU `patch` would apply a drifted hunk
   "with fuzz"; `tests/run.sh` and the status report show both).
6. D-089 version (above), verified before compiling.
7. Build dependencies: upstream's `DEB_DEPENDS` from the VPP Makefile, checked with `apt-get install -s` (simulation);
   an apt error is fatal (no "all satisfied" on failure), missing packages are reported (`--strict-deps` → fatal).
   `make install-dep` is never run.
8. Python build inputs (`pydeps.lock`): the wheelhouse `.build/pydeps/wheelhouse` ends up holding exactly the locked
   files, each sha256-checked; missing ones are downloaded from the locked `files.pythonhosted.org` URL and checked
   (never copied from `/root/vpp` or `~/Downloads`; with `--offline-reference` a missing file is fatal). A scratch venv
   proves `pip install --require-hashes --no-index --find-links <wheelhouse> -r pydeps.lock`. Upstream's `DL_CACHE_DIR`
   (`~/Downloads`) is pointed at an empty dir. External *source tarballs* (DPDK, ipsec-mb, rdma-core, …) may be seeded
   from `/root/vpp/build/external/downloads` — upstream re-verifies their sha256 on every build.
9. Free disk under the build dir ≥ 40 GB.
10. `make pkg-deb` with the flags the vrx-a host build used — upstream defaults (`CMAKE_BUILD_TYPE=release`, LTO, all
    plugins; read from `/root/vpp/build-root/build-vpp-native/vpp/CMakeCache.txt`). `build-patches/0001` makes the
    DPDK meson venv (inside the build tree, never system-wide) install **only** via
    `pip3 install --require-hashes --no-index --find-links $VRX_PYDEPS_DIR -r $VRX_PYDEPS_LOCK` (fails closed if unset).
    The same wheelhouse serves the PEP 517 build of `python3-vpp-api` (`setuptools>=61` in build isolation): the
    whole make runs with `PIP_NO_INDEX=1 PIP_FIND_LINKS=<wheelhouse> PIP_NO_CACHE_DIR=1`, so no pip in the build can
    reach PyPI, `~/.cache/pip` or `~/Downloads` (round 1 silently fetched an unpinned setuptools there).
    Parallelism: `MAKE_PARALLEL_JOBS=JOBS=--jobs` (≤ 8) and `taskset` on that many CPUs + `nice 10`.
    `SOURCE_DATE_EPOCH` = commit time. After the build, the DPDK venv's `pip list` must equal the lock.
11. Output: every `.deb`, `.buildinfo`/`.changes`, `SHA256SUMS`, `manifest.json`; then `verify.sh --require-files`.

Byte-identical output is **not** claimed (LTO + build paths); names, versions and all inputs are pinned.

### Plugins (V18 / D-077 / D-089)

26.06 builds `tracedump`/`tracenode`, but upstream ships them in `vpp-plugin-devtools` (with `unittest`, `perfmon`, …),
which the product does not ship. D-089: the `--trace-plugins core` variant (CMake-only optional patch moving the two
components to `vpp-plugin-core`) is used when F-capture-trace starts.

## Bump the tag (e.g. 26.06 → 26.06.1)

1. `git ls-remote https://gerrit.fd.io/r/vpp 'refs/tags/v26.06.1' 'refs/tags/v26.06.1^{}'` — first hash =
   `VPP_TAG_OBJECT`, the `^{}` hash = `VPP_COMMIT`.
2. Edit `VERSION`: `VPP_TAG`, `VPP_BRANCH`, both hashes, `VPP_DEB_VERSION` (`26.06.1-release`), reset `VPP_LOCAL_REV=1`,
   adjust the package lists if upstream changed them.
3. Check `build/external/packages/dpdk.mk` of the new tag: if the meson/pyelftools requirement changed, update
   `pydeps.lock` (version, PyPI sha256 from `https://pypi.org/pypi/<name>/<version>/json`, file, URL) and refresh
   `build-patches/0001` so it applies exactly.
4. `verify.sh`, `build.sh --prepare-only` — every patch must apply exactly; refresh or drop the ones that do not.
5. Full build; regenerate binapi from the new `.api.json` (P04 procedure); agent integration suite after install.

## Add a product patch

1. In a scratch tree (`.build/src/vpp` after `--prepare-only`): `git diff > …` from the VPP root (`a/`/`b/`, `-p1`).
2. Save as `patches/NNNN-<area>-<summary>-<Vitem>.patch` with the header
   `Subject:` / `Track: V<n>` / `Status: product` / `Upstream: <gerrit URL | not submitted>`
   (demo patches: `DEMO` in the name and `Status: demo`).
3. Append it to `patches/series`, **bump `VPP_LOCAL_REV`**, run `verify.sh` and `build.sh --prepare-only`.
4. Funding a real V-item is the product owner's decision (FAST MODE: no C code in VPP in the 21-day plan).

## Output and how P10 / P14 consume it

`manifest.json` (schema `vrx.vpp-debs.manifest/v2`):

```json
{
  "schema": "vrx.vpp-debs.manifest/v2",
  "upstream": { "url": "…", "branch": "stable/2606", "tag": "v26.06", "tag_object": "29c5…", "commit": "c320…", "describe": "…" },
  "version": "26.06-release+vrx1", "upstream_version": "26.06-release", "local_rev": 1, "variant": "demo",
  "patches": [ { "name": "patches/0001-DEMO-….patch", "sha256": "…", "kind": "demo" } ],
  "options": { "trace_plugins": "devtools", "demo": true },
  "build": { "source": "upstream", "builder_commit": "…", "builder_dirty": false,
             "build_patches": [ { "name": "build-patches/0001-….patch", "sha256": "…", "kind": "build" } ],
             "inputs": { "python": [ { "name": "meson", "version": "0.57.2", "sha256": "…", "file": "…", "url": "…" } ],
                         "dpdk_meson_venv": [ "meson==0.57.2", "…" ] },
             "jobs": 8, "seconds": 0, "source_date_epoch": 0, "os": "Ubuntu 26.04.1 LTS", "missing_build_deps": [] },
  "packages": [ { "package": "vpp", "version": "26.06-release+vrx1", "architecture": "amd64", "file": "vpp_…_amd64.deb",
                  "size": 0, "sha256": "…", "ship": true, "installed_on_vrx_a": true } ]
}
```

* **Always** run `deploy/vpp/verify.sh --require-files <dir>` on an output before consuming it: it fails on a missing
  or extra `.deb`, a manifest entry whose `Package`/`Version`/`Architecture` differ from the `.deb` control fields, a
  hash mismatch against `SHA256SUMS`/the manifest, a stale patch or lock, or a version that violates D-089.
* `SHA256SUMS` and `manifest.json` are **unsigned**: they detect corruption, not tampering by someone who can write the
  directory. Signing is P10's job (APT `Release` signing).
* **P10**: select packages by `package` with `ship: true` (D-089: the 7 runtime packages; never `vpp-dbg`, `vpp-dev`,
  `libvppinfra-dev`, `vpp-plugin-devtools`); pin `vrx-meta`'s `Depends: vpp (= <manifest.version>)`.
* **P14**: copies the same files into the ISO pool and records `upstream.commit`, `patches[].sha256` and
  `build.inputs` in the image build info.
* **Storage (D-089)**: artefacts never go into git. After a build the manager publishes the output directory as-is to
  `/srv/vrx-artifacts/vpp/<version>[-<variant>]/` (`cp -a`, then `verify.sh --require-files` on the copy); P10 decides
  the APT repository.

## Installing on vrx-a (manager only, only after `handover: done`)

Only a **patched product build** is ever installed: the unpatched `26.06-release` build is what vrx-a already runs, and a
demo build is never installed. `verify.sh --install-gate` enforces both (and refuses a build from an uncommitted builder).

```bash
exec 9>/run/lock/vrx-lab.lock; flock -x 9                               # barrier: no integration test is running
OUT=/srv/vrx-artifacts/vpp/26.06-release+vrx<N>                          # a product build, never "-demo", never unsuffixed
deploy/vpp/verify.sh --require-files "$OUT" --install-gate || exit 1
(cd "$OUT" && sha256sum -c SHA256SUMS) || exit 1
B=/var/backups/vrx-vpp-$(date +%Y%m%d-%H%M%S); mkdir -p "$B"
cp -a /etc/vpp "$B/etc-vpp"; dpkg -l | grep -i vpp > "$B/dpkg-before.txt"; vppctl show version > "$B/version-before.txt"
cp /root/vpp/build-root/{vpp,vpp-plugin-core,vpp-plugin-dpdk,vpp-drivers,vpp-crypto-engines,libvppinfra,python3-vpp-api}_26.06-release_amd64.deb "$B/"
dpkg -i $(python3 -c 'import json,sys; m=json.load(open(sys.argv[1]+"/manifest.json")); print(" ".join(sys.argv[1]+"/"+p["file"] for p in m["packages"] if p["ship"]))' "$OUT")
systemctl restart vpp && sleep 5
systemctl is-active vpp && vppctl show version && ls /run/vpp/api.sock      # expect the +vrx<N> version string
# then: agent reconcile check (restart vrx-agent, Retrieve == desired), tools/ci.sh full on main
flock -u 9
```

Rollback (any check fails): `dpkg -i "$B"/*.deb` (a downgrade from `+vrx<N>` to `26.06-release`; dpkg warns and
proceeds), `cp -a "$B/etc-vpp/." /etc/vpp/`, `systemctl restart vpp`, re-run the checks, record it in `docs/decisions/LOG.md`.
