# deploy/vpp — the VPP the product ships (WBS D0.2)

The appliance runs FD.io VPP built **from source by us**, never from the FD.io APT repository (D-001). This directory
makes that build reproducible from the repository: a pinned upstream tag, a patch series, one build script and a
manifest that the packaging (P10) and the installer ISO (P14) consume.

| file | purpose |
|---|---|
| `VERSION` | upstream URL, branch, tag, **tag object + commit hash**, expected Debian version, expected package set |
| `patches/series` | quilt-style series applied in order on top of the tag (`patch -p1`) |
| `patches/*.patch` | our patches; each has a `Subject/Track/Status/Upstream` header (Track = V-item in `docs/vpp-code-track.md`) |
| `patches/optional/*.patch` | patches only applied by a build option (`--trace-plugins core`) |
| `build.sh` | clone → verify pin → apply series → dependency report → `make pkg-deb` → `.deb` + `SHA256SUMS` + `manifest.json` |
| `verify.sh` | cheap consistency gate (no clone, no build, < 1 s); also validates a produced manifest |
| `.build/` | scratch: source tree, ccache, outputs — **git-ignored**, never committed (nor is any `*.deb`) |

## Build

```bash
deploy/vpp/build.sh                      # default: host-equivalent build + patches/series → .build/out/26.06-release/
deploy/vpp/build.sh --prepare-only       # clone/verify/patch/version/dependency check only (~1 min once cloned)
deploy/vpp/build.sh --trace-plugins core # tracedump + tracenode shipped in vpp-plugin-core (V18) → .build/out/26.06-release+trace-core/
nohup deploy/vpp/build.sh > /root/ngfw-wt/logs/<id>-vpp-build.log 2>&1 &   # a full build takes long — run detached, poll the log
```

What it does, in order (any mismatch aborts):

1. `verify.sh` static checks.
2. Source in `.build/src/vpp`: `git clone --reference-if-able /root/vpp --dissociate <VPP_UPSTREAM_URL>` (objects are
   borrowed from the local clone and then copied, so the result does not depend on `/root/vpp`); falls back to a
   read-only `git clone --no-hardlinks /root/vpp` when upstream is unreachable (`--offline` forces that).
   **Nothing is ever built, checked out or cleaned inside `/root/vpp`.**
3. The tag must resolve to `VPP_TAG_OBJECT` and peel to `VPP_COMMIT` (a moved/re-signed tag fails the build).
4. Pristine checkout of the commit (`git clean -ffdx`, keeping `build-root/.ccache` and the external download cache).
5. The series is applied **uncommitted** — VPP's `src/scripts/version` derives the Debian version from
   `git describe`; on the exact tag it yields `26.06-release`, which is checked *before* compiling.
6. Build dependencies: upstream's own `DEB_DEPENDS` list is read from the VPP Makefile and checked with
   `apt-get install -s` (a simulation — the same check as upstream's `.deps.ok`). Missing packages are reported (and
   written to `.build/missing-deps.txt`); `--strict-deps` makes that fatal. `make install-dep` is never run.
7. `make pkg-deb` with the flags the vrx-a host build used — upstream defaults (`CMAKE_BUILD_TYPE=release`, LTO, all
   plugins, no `VPP_EXCLUDED_PLUGINS`, no extra cmake args; read from `/root/vpp/build-root/build-vpp-native/vpp/CMakeCache.txt`).
   Parallelism is capped: `MAKE_PARALLEL_JOBS=JOBS=--jobs` (default and maximum 8, shared host) and the whole
   build runs under `taskset` on that many CPUs + `nice 10` (some external deps call `make -j` without a limit).
   `SOURCE_DATE_EPOCH` = commit time. External tarballs are seeded from `/root/vpp/build/external/downloads`
   (VPP re-verifies their checksums).
8. Output directory: every `.deb`, the `.buildinfo`/`.changes`, `SHA256SUMS`, `manifest.json`. The package set and
   every package version must equal `VPP_PACKAGES` / `VPP_DEB_VERSION`, then `verify.sh --manifest … --sums …` runs.

Byte-identical output is **not** claimed (LTO + build paths); names, versions and the source input are.

### Plugins (V18 / D-077)

26.06 builds every plugin, including `tracedump` and `tracenode`, but upstream packages those two (together with
`tracepath`, `unittest`, `perfmon`, `oddbuf`, `bufmon`, `dispatch_trace`) into **`vpp-plugin-devtools`**, which vrx-a does
not install. Two ways to make their binary API available on the appliance:

* default build + install `vpp-plugin-devtools` (also ships the unittest plugin — not wanted on a product), or
* `--trace-plugins core` → `patches/optional/trace-plugins-core.patch` moves only those two CMake components to
  `vpp-plugin-core` (build configuration, no C code). Output goes to `…/26.06-release+trace-core/` and the manifest
  records `options.trace_plugins = "core"`. Which one is the product default is an open question (F-vpp-debs-questions).

## Bump the tag (e.g. 26.06 → 26.06.1 or 26.10)

1. `git ls-remote https://gerrit.fd.io/r/vpp 'refs/tags/v26.06.1' 'refs/tags/v26.06.1^{}'` — first hash = `VPP_TAG_OBJECT`,
   the `^{}` hash = `VPP_COMMIT`.
2. Edit `VERSION`: `VPP_TAG`, `VPP_BRANCH` (`stable/YYMM`), both hashes, `VPP_DEB_VERSION` (`26.06.1-release`);
   adjust `VPP_PACKAGES` if upstream adds/removes a package.
3. `deploy/vpp/verify.sh`, then `deploy/vpp/build.sh --prepare-only` — every patch must still apply; refresh or drop
   (when upstreamed: remove the line, keep a note in the commit) those that do not.
4. Full build, regenerate binapi from the new `.api.json` (P04 procedure, `tools/binapi-gen.sh` — owned by P04),
   run the agent integration suite against the new VPP after the manager installs it (below).

## Add a patch

1. Make the change in a scratch tree (`.build/src/vpp` after `--prepare-only` is fine), `git diff > …` from the VPP
   root (`a/` `b/` prefixes, `-p1`).
2. Save as `patches/NNNN-<area>-<summary>-<Vitem>.patch` with the header:
   ```
   Subject: [PATCH] <area>: <summary>
   Track: V<n> (docs/vpp-code-track.md)
   Status: product | demo
   Upstream: <gerrit change URL> | not submitted
   ```
   Demo patches carry `DEMO` in the file name and `Status: demo`.
3. Append the file name to `patches/series` (order matters), run `verify.sh` and `build.sh --prepare-only`.
4. Funding a real V-item is the product owner's decision (FAST MODE: no C code in VPP in the 21-day plan) — the
   one patch in the series today (V16) is a **demonstration** of the mechanism and is not installed on vrx-a.

## How P10 / P14 consume the output

`manifest.json` (schema `vrx.vpp-debs.manifest/v1`):

```json
{
  "schema": "vrx.vpp-debs.manifest/v1",
  "upstream": { "url": "…", "branch": "stable/2606", "tag": "v26.06", "tag_object": "29c5…", "commit": "c320…", "describe": "v26.06-0-gc3200b88d" },
  "version": "26.06-release",
  "patches": [ { "name": "0001-DEMO-….patch", "sha256": "…", "kind": "demo" } ],
  "options": { "trace_plugins": "devtools" },
  "build": { "builder_commit": "…", "jobs": 8, "seconds": 0, "source_date_epoch": 0, "os": "Ubuntu …", "missing_build_deps": [] },
  "packages": [ { "package": "vpp", "version": "26.06-release", "architecture": "amd64", "file": "vpp_26.06-release_amd64.deb",
                  "size": 0, "sha256": "…", "installed_on_vrx_a": true } ]
}
```

* **P10 (APT repo / `vrx-meta`)**: take `packages[]` (select by `package`, not by glob), check each file against
  `SHA256SUMS` (`sha256sum -c SHA256SUMS`) before `reprepro includedeb`; pin `vrx-meta`'s
  `Depends: vpp (= <version>)` to `manifest.version`; refuse a manifest where `verify.sh --manifest` fails.
  Which packages the product ships is P10's call — `installed_on_vrx_a` marks the set vrx-a runs today
  (vpp, vpp-plugin-core, vpp-plugin-dpdk, vpp-drivers, vpp-crypto-engines, libvppinfra, python3-vpp-api).
* **P14 (ISO)**: copies the same files into the ISO pool and records `upstream.commit` + `patches[].sha256` in the
  image's build info, so an installed appliance can be traced back to the exact source.
* Artefacts are never committed. Long-term storage is an open question (F-vpp-debs-questions); until decided they
  live in `deploy/vpp/.build/out/<version>/` of the worktree that built them.

## Installing on vrx-a (manager only, only after `handover: done`)

Not before `docs/lab/host-vrx-a.md` says `handover: done` (D-012), and only by the manager:

```bash
exec 9>/run/lock/vrx-lab.lock; flock -x 9                    # barrier: no integration test is running
OUT=/root/ngfw-wt/<id>/deploy/vpp/.build/out/26.06-release
(cd "$OUT" && sha256sum -c SHA256SUMS) && deploy/vpp/verify.sh --manifest "$OUT/manifest.json" --sums "$OUT/SHA256SUMS"
B=/var/backups/vrx-vpp-$(date +%Y%m%d-%H%M%S); mkdir -p "$B"
cp -a /etc/vpp "$B/etc-vpp"; dpkg -l | grep -i vpp > "$B/dpkg-before.txt"
cp /root/vpp/build-root/*.deb "$B/"                           # the currently installed build = rollback set
vppctl show version > "$B/version-before.txt"
dpkg -i $(for p in vpp vpp-plugin-core vpp-plugin-dpdk vpp-drivers vpp-crypto-engines libvppinfra python3-vpp-api; do
            ls "$OUT/${p}_"*.deb; done)                       # same set as installed today; add others deliberately
systemctl restart vpp && sleep 5
systemctl is-active vpp && vppctl show version && vppctl show plugins | head && ls /run/vpp/api.sock
# then: agent reconcile check (restart vrx-agent, Retrieve == desired), tools/ci.sh full on main
flock -u 9
```

Rollback (any check fails): `dpkg -i "$B"/*.deb` (same version → `dpkg -i` reinstalls the old files; use
`--force-downgrade` only if versions differ), `cp -a "$B/etc-vpp/." /etc/vpp/`, `systemctl restart vpp`, re-run the
checks, record the event in `docs/decisions/LOG.md`.

Note: the default build keeps the upstream version string `26.06-release` even with patches applied, so a patched
package is not distinguishable by `dpkg -l` alone — `manifest.json` (patch hashes) is the record. A local version
suffix is an open question for P10 (F-vpp-debs-questions).
