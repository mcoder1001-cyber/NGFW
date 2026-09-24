# F-vpp-debs — independent review

Reviewer: review agent (did not write this code). Branch `task/F-vpp-debs` @ 90dbb81, base main. Reviewed on the host by
reading the code and the scratch clone (`deploy/vpp/.build/src/vpp`, read only), running `deploy/vpp/verify.sh` (static and
`--manifest/--sums` against the produced `out/26.06-release`) and `tools/ci.sh --base main`. No rebuild was run. Nothing
under `/root/vpp`, `/etc/vpp`, `/root/ngfw` or dpkg state was touched.

## Evidence from my own runs

```
$ deploy/vpp/verify.sh                                              → verify.sh: OK (exit 0)
$ deploy/vpp/verify.sh --manifest .build/out/26.06-release/manifest.json --sums .build/out/26.06-release/SHA256SUMS
ok   manifest: 11 packages 26.06-release, patches ['0001-DEMO-…-V16.patch'], SHA256SUMS consistent  → verify.sh: OK
$ ls deploy/vpp/.build/src/vpp/.git/objects/info/                   → packs     (no alternates file: --dissociate worked)
$ tools/ci.sh --base main
  run 1: CI GATE FAILED — apps/agent lint: "parallel golangci-lint is running" (lock held by another worker; environmental,
         this branch touches no Go)
  run 2 (02:42, HEAD 90dbb81): CI GATE PASSED (wall time 1m19s, logs /root/ngfw-wt/logs/ci/F-vpp-debs-20260924-024227-2154558)
$ # GNU patch 2.8 accepts fuzzy hunks with the exact flags build.sh uses:
$ patch -d t -p1 --forward --batch --dry-run --quiet < p.diff; echo rc=$?   → rc=0   ("Hunk #1 succeeded at 1 with fuzz 1.")
$ apt-get install -s -qq nonexistent-pkg-xyz 2>/dev/null | wc -l          → 0 (apt rc=100) → build.sh reports "all satisfied"
```
The contract/binapi paths are not touched (`git diff --name-only main...HEAD -- packages/schema packages/proto apps/agent/gen
packages/api-client/src/generated apps/agent/binapi tools` is empty). No binaries are tracked (`git ls-files` has no
.deb/.whl/.tar*/.so). The status file's pasted CI output matches what I got.

## What is good (checked, not assumed)

- The pin is real: `build.sh:106-111` compares the tag **object** against `VPP_TAG_OBJECT`, its peeled commit against
  `VPP_COMMIT`, requires an annotated tag and `HEAD == VPP_COMMIT` after the pristine checkout. It is not just `git checkout <tag>`.
  A tag that exists locally with a different value makes the fetch at `:102` fail rather than clobber it.
- `/root/vpp` is only read: `pwd -P`, `git clone --reference-if-able … --dissociate` (reads objects; no alternates left,
  checked), the fallback `git clone --no-hardlinks`, and `cp -n` out of `build/external/downloads`. If `/root/vpp` is gc'ed
  or removed, the scratch clone keeps working.
- External **tarballs** (DPDK 26.03, ipsec-mb, rdma-core, xdp-tools, quicly, libcbor, daq, octeon-roc) are sha256-checked by upstream
  `build/packages_common.mk:42-52` against the hashes in the pinned tree. `git clean -ffdx` removes the `.download.ok` stamps,
  so seeded tarballs are checked again on every build. VPP has no git submodules.
- The CPU cap holds: `--jobs` has to be 1..8, `MAKEFLAGS` is unset, and `taskset` plus `nice` wrap the whole make (this also covers
  ipsec-mb's unbounded `make -j` and LTO's parallel link). `SOURCE_DATE_EPOCH` is set to the commit time.
- `.gitignore` covers `deploy/vpp/.build/` and `*.deb`, and `verify.sh` checks this.
- The package names and versions evidence and the before/after snapshots of `/root/vpp` and dpkg are pasted. The tracedump/tracenode
  finding for V18 is correct and useful.

## Findings (ranked)

### H1 — PyPI wheels used by the DPDK build are unpinned, unverified and seeded from a tree someone else owns
`build.sh:168` copies `/root/vpp/build/external/downloads/*`, including `meson-0.57.2`, `setuptools-84.0.0`,
`wheel-0.48.0`, `pyelftools-0.33` and `packaging-26.3` `.whl`, into the scratch tree. `build.sh:115` then keeps that directory
across every `git clean`. Upstream `build/external/packages/dpdk.mk:205-206` runs `pip3 download … meson==0.57.2 setuptools
wheel pyelftools` only when no `meson*` file is present. The command has no version pins except meson, no `--require-hashes`,
and `-f $(DL_CACHE_DIR)`, which is `$HOME/Downloads`. It then runs `pip3 install --no-index --find-links=downloads/`, and that
install takes **whatever wheels sit in that directory without any hash check**, as root. The wheels run at build time (meson
configures DPDK), so they can change the DPDK objects that go into `vpp-plugin-dpdk`.
- Failure scenarios: (a) the bring-up agent, or anyone with write access to `/root/vpp/build/external/downloads` or
  `~/Downloads`, drops in a modified or newer `pyelftools`/`meson` wheel, and our "reproducible, pinned" build uses it without
  noticing. (b) The cache fills up over time (a new `.whl` next to an old one), so `--find-links` picks the highest version.
  Two builds from the same `VERSION` then differ, and `manifest.json` does not show it (the wheels are not recorded at all).
- Fix: add `deploy/vpp/pydeps.lock`, a `requirements`-style file with exact versions and `--hash=sha256:` for meson, setuptools,
  wheel, pyelftools and packaging. Before `make`, empty the wheel files out of `$SRC/build/external/downloads`, populate them
  from a verified source (`pip download --require-hashes -r pydeps.lock`, or a sha256-checked copy), and export
  `DL_CACHE_DIR` to a directory under `.build` so `~/Downloads` is never read. Then record the wheel hashes in
  `manifest.json.build.inputs`. Do **not** seed wheels from `/root/vpp`. Tarballs may still be seeded, because they are re-hashed.

### H2 — The default build output is demo-patched but carries the host's exact version, and the README installs that directory
The default `build.sh` applies the whole series, which today is only the DEMO patch. It asserts `26.06-release` (`VERSION:10`,
`build.sh:140-143`) and writes to `out/26.06-release/`. `README.md:122-129` (manager install procedure) then runs `dpkg -i` from
exactly that directory. A manager who follows the README after handover would install the **demo** V16 patch on vrx-a, and
`dpkg -l` could not tell it apart from the upstream build. That contradicts the scope ("demonstration, not applied to the
host") and D-089, which says patched builds get the local suffix `+vrx<N>`. The rollback note in `README.md:136`
("same version → dpkg -i reinstalls") exists only because of this ambiguity.
- Fix: (1) implement D-089. Any build with a non-empty applied series gets version `26.06-release+vrx<N>`, with `N` from
  `VERSION` (`VPP_LOCAL_REV`). Upstream `src/scripts/version` reads `src/scripts/.version` when present, or you can set
  `debian/changelog` with `dch`. Update `VPP_DEB_VERSION`/verify.sh and the output dir name to match. (2) Leave `Status: demo`
  patches out of the default build unless `--with-demo` is given, and put `demo` in the output dir name and the manifest when
  it is. (3) In the README install block, refuse a manifest whose `patches[].kind` contains `demo`.

### M1 — "Applies cleanly" is not enforced: fuzz and offsets are accepted
`build.sh:125-126` uses `patch --forward --batch [--dry-run]` with GNU patch's default `--fuzz=2`. I reproduced it: a hunk whose
context no longer matches applies with `rc=0` ("succeeded … with fuzz 1"). The status file says "aborts on any fuzz failure",
which is wrong. After a tag bump, a patch can land in the wrong place, and `--prepare-only` still reports success.
- Fix: use `git -C "$SRC" apply --check -pN` followed by `git apply` (strict by default: no fuzz, whitespace errors reported),
  or `patch --fuzz=0` and fail when the output contains `offset|fuzz`. Correct the claim in the status file.

### M2 — `rm -rf` on user-computed paths with incomplete guards
- `build.sh:84` runs `rm -rf "$SRC"` (`$BUILD_DIR/src/vpp`) whenever `$SRC/.git` is missing. The guard at `:74` protects only
  the *current* `--reference`. Example: `--build-dir /root/vpp --reference /some/other/clone`, or `--build-dir` pointing at
  any VPP checkout. `SRC` is then `<checkout>/src/vpp`, which is VPP's own source directory with no `.git`, and it gets deleted.
  With `/root/vpp`, that breaks D-012.
- `build.sh:183` runs `rm -rf "$OUT_DIR"` on an arbitrary `--out` with no check. `--out /`, `--out ~`, `--out .` or
  `--out /root/vpp/build-root` wipe that directory.
- Fix: always refuse (a) `/root/vpp`, whatever `--reference` says, (b) any path whose realpath is `/`, `$HOME`, the repo
  toplevel or inside a git work tree other than `$SRC`, and (c) an existing `$SRC` that is not a git repo created by us (use a
  marker file such as `.build/.vrx-owned`). Before emptying `OUT_DIR`, require that it is new or contains only our artefact
  names plus `SHA256SUMS`/`manifest.json`.

### M3 — The build-dependency check reports "all satisfied" when apt fails
`build.sh:153` ends with `apt-get install -s … 2>/dev/null | awk … || true`. If apt errors (an unknown package name in a future
`DEB_DEPENDS`, a held lock, a broken sources list), stdout is empty, rc 100 is swallowed, and `:158` logs "all N DEB_DEPENDS
entries satisfied". I reproduced the empty output with rc=100. `--strict-deps` never triggers in that case.
- Fix: capture apt's exit status separately, and on non-zero print its stderr and `die`, or at least warn loudly and treat it
  as missing when `--strict-deps` is set. A cheaper alternative is `dpkg-query -W -f='${Status}'` per package.

### L1 — `verify.sh --sums` can be satisfied with fewer or different files than it claims
`verify.sh:151` silently skips entries whose `.deb` is missing, and extra `*.deb` files in the directory are never detected. It
also never checks the actual `.deb` control fields: the manifest can say `package: vpp` for `file: vpp-plugin-dpdk_…deb` and still
pass, and P10 is told to select by `package`. SHA256SUMS and the manifest are generated together and unsigned, so they protect
only against corruption, not against tampering (signing is P10's job; say so in the README).
- Fix: add a `--require-files` mode (used by the README install block and by P10) that fails on missing or extra `.deb` files and
  compares `dpkg-deb -f Package,Version,Architecture` with each manifest entry. Reject `file` values that contain `/`.

### L2 — `VERSION` is executed as shell
`build.sh:65` and `verify.sh:38` both run `source <(grep '^VPP_…=' VERSION)`. The format regex in verify.sh (`[^[:space:]"]*`)
accepts `VPP_TAG=$(cmd)` and backticks, and verify.sh sources the file even after reporting a format failure. VERSION is
repo-controlled, so the risk is low, but a supply-chain pipeline should parse data as data.
- Fix: parse with `while IFS='=' read -r k v` plus a strict per-key regex (hex, URL, `vYY.MM`), and stop on the first format
  error before using any value.

### L3 — The 40 GB disk budget is not enforced
The budget is honoured in practice (3.4 GB used), but `build.sh` has no `df` pre-check and no size guard. Keeping
`build/external/downloads` and ccache forever lets `.build` grow with every bump.
- Fix: a pre-flight check (`df --output=avail "$BUILD_DIR"` ≥ 15 GB), and print `du -sh "$BUILD_DIR"` at the end.

### L4 — Not yet aligned with D-089 on artefact location and the shipped set
The default `--out` is still under the worktree (`build.sh:69`), and `README.md:113-114` still describes the old location.
D-089 decided `/srv/vrx-artifacts`. README P10 guidance (`:106-110`) still calls the shipped set "P10's call", but D-089 now
says neither `vpp-dbg` nor `vpp-dev` ships. Update the README, and optionally add `"ship": bool` per package in the manifest,
derived from a `VPP_PACKAGES_SHIP` list in VERSION. Q1–Q4 in the questions file are answered by D-089 and should be marked
answered.

### L5 — A silent fallback changes where the source comes from
`build.sh:89` falls back to cloning `/root/vpp` when the upstream clone fails, even without `--offline`. The hash pin still
protects content integrity, but `manifest.json` does not record which remote the objects came from. Record
`build.source: upstream|reference`, and make the fallback require `--offline` explicitly (fail closed on network errors in normal mode).

### Info
- The whole pipeline, including curl/pip downloads and `dpkg-buildpackage`, runs as root on a shared host. This is acceptable in
  the current lab. A later task could run it as an unprivileged user (fakeroot is already used by `pkg-deb`).
- Hooking `verify.sh` into `tools/ci.sh` (Q5) is a P09 decision. It is cheap (<1 s) and should be added.
- Out-of-scope check: nothing extra was built. The `--trace-plugins core` optional patch is explicitly requested by the prompt
  (V18 option flag).

## Verdict

H1 (unverified build-time Python inputs) and H2 (a demo-patched build versioned exactly like the host, next to an install
recipe for it) must be fixed before P10/P14 consume this output. M1–M3 are small and should go in the same change. There
is no reason to block: the pin, the read-only handling of `/root/vpp`, the tarball verification, the resource caps and the
evidence are all sound.

**APPROVE WITH CHANGES**
