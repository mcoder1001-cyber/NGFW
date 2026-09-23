# F-vpp-debs — reproducible VPP 26.06 package build (pinned source + patch series), WBS D0.2

> **Review round (APPROVE WITH CHANGES, afb833a) — see "Review fixes" at the end.** Round-1 sections below are kept as
> history. Important: the round-1 output `out/26.06-release/` was **demo-patched but versioned exactly like the host**
> (H2); it has been deleted. The default build is now unpatched (`26.06-release`) and the demo series builds only with
> `--demo` as `26.06-release+vrx1`.

Branch `task/F-vpp-debs`, worktree `/root/ngfw-wt/F-vpp-debs`, base main@87f84b2. Worker ran directly on the host.

## What was built

| path | what |
|---|---|
| `deploy/vpp/VERSION` | upstream `https://gerrit.fd.io/r/vpp`, `stable/2606`, tag `v26.06` = tag object `29c51fb8…` → commit `c3200b88dc46bd380f00a49ca3392a102cc1980b`; expected version `26.06-release`; expected 11-package set; the 7 installed on vrx-a |
| `deploy/vpp/patches/series` + `0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch` | quilt-style series, one **demonstration** patch (V16, the two-line `REPLY_MSG_ID_BASE` fix in `flow_api.c`) — marked `Status: demo`, not installed anywhere |
| `deploy/vpp/patches/optional/trace-plugins-core.patch` | V18 option: moves `tracedump`/`tracenode` CMake components from `vpp-plugin-devtools` to `vpp-plugin-core` (CMake only, no C); applied only with `--trace-plugins core` |
| `deploy/vpp/build.sh` | verify.sh → clone (`--reference-if-able /root/vpp --dissociate`, fallback read-only clone of /root/vpp) into git-ignored `deploy/vpp/.build/` → verify tag object + commit → pristine checkout → apply series uncommitted → check the tree version is `26.06-release` *before* compiling → build-dep report (upstream `DEB_DEPENDS` via `apt-get -s`, never installs) → `make pkg-deb` (upstream defaults = host flags) with `MAKE_PARALLEL_JOBS=JOBS=8`, `taskset` on 8 CPUs, `nice 10` → `.deb` + `SHA256SUMS` + `manifest.json` → package set/version check → `verify.sh --manifest` |
| `deploy/vpp/verify.sh` | < 1 s static gate: VERSION format/coherence, series ↔ files (no orphans/dups, headers, unified diff, DEMO marking), shellcheck, no `.deb`/`.build` in git + ignore rules; with `--manifest/--sums` validates a produced manifest against VERSION + series and the `.deb` hashes on disk |
| `deploy/vpp/README.md` | bump the tag, add a patch, rebuild, manifest schema and how P10/P14 consume it, the manager's post-handover install/rollback procedure (flock -x, backup, dpkg -i, restart, verify, rollback) |
| `.gitignore` | `deploy/vpp/.build/`, `*.deb` |

Host build flags were read (not guessed) from `/root/vpp/build-root/build-vpp-native/vpp/CMakeCache.txt`:
`CMAKE_BUILD_TYPE=release`, `VPP_USE_LTO=ON`, `VPP_USE_CCACHE=ON`, `VPP_EXCLUDED_PLUGINS=` (empty), `VPP_PLUGINS=` (all) —
i.e. plain upstream `make pkg-deb`; `build.sh` uses exactly that.

**Finding (V18/D-077):** tracedump and tracenode *are* built by 26.06; upstream ships them in `vpp-plugin-devtools`
(`dpkg-deb -c` shows `tracedump_plugin.so`, `tracenode_plugin.so`, `tracepath_plugin.so`, `unittest_plugin.so`, …),
which vrx-a does not install. So V18 is a packaging choice, not a missing build flag → Q2.

## How it was verified

### Before (recorded 2026-09-24T01:42:45+03:30, before any work)
```
$ git -C /root/vpp status
On branch stable/2606
Your branch is up to date with 'origin/stable/2606'.

nothing to commit, working tree clean
$ git -C /root/vpp log -1
commit c3200b88dc46bd380f00a49ca3392a102cc1980b
Author: Andrew Yourtchenko <ayourtch@gmail.com>
Date:   Sun Jun 21 15:53:33 2026 +0200

    misc: VPP 26.06 Release Notes
    
    Type: docs
    Change-Id: Ib0bb99d0ab36d29d5b109ffd26b8e67c11a9169a
    Signed-off-by: Andrew Yourtchenko <ayourtch@gmail.com>
$ dpkg -l | grep -i vpp
ii  libvppinfra                             26.06-release                               amd64        Vector Packet Processing--runtime libraries
ii  python3-vpp-api                         26.06-release                               amd64        VPP Python3 API bindings
ii  vpp                                     26.06-release                               amd64        Vector Packet Processing--executables
ii  vpp-crypto-engines                      26.06-release                               amd64        Vector Packet Processing--runtime crypto engines
ii  vpp-drivers                             26.06-release                               amd64        Vector Packet Processing--runtime device drivers
ii  vpp-plugin-core                         26.06-release                               amd64        Vector Packet Processing--runtime core plugins
ii  vpp-plugin-dpdk                         26.06-release                               amd64        Vector Packet Processing--runtime dpdk plugin
```

### Full build (`nohup deploy/vpp/build.sh > /root/ngfw-wt/logs/F-vpp-debs-build.log 2>&1 &`, 01:49 → 02:34)
```
[build.sh 01:49:05] pinned: https://gerrit.fd.io/r/vpp v26.06 (c3200b88dc46bd380f00a49ca3392a102cc1980b) → expect libvppinfra libvppinfra-dev python3-vpp-api vpp vpp-crypto-engines vpp-dbg vpp-dev vpp-drivers vpp-plugin-core vpp-plugin-devtools vpp-plugin-dpdk version 26.06-release
[build.sh 01:49:05] build dir: /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build  out: /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/out/26.06-release  jobs: 8  trace-plugins: devtools  reference: /root/vpp
[build.sh 01:49:05] verified: v26.06 = tag object 29c51fb8b95a92d0d325cff2e502549e652aad9f → commit c3200b88dc46bd380f00a49ca3392a102cc1980b
[build.sh 01:49:06] checked out v26.06-0-gc3200b88d (pristine)
[build.sh 01:49:06] applied 0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch (-p1)
     M src/vnet/ipfix-export/flow_api.c
[build.sh 01:49:06] tree version: 26.06-release (git describe v26.06-0-gc3200b88d)
[build.sh 01:49:09] build dependencies: all 46 DEB_DEPENDS entries satisfied
[build.sh 01:49:09] make -C /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/src/vpp pkg-deb MAKE_PARALLEL_JOBS=8 JOBS=8 (taskset -c 24-31, nice 10, SOURCE_DATE_EPOCH=1782050576)
...
[982/2922] Building C object CMakeFiles/vnet/CMakeFiles/vnet_objs.dir/ipfix-export/flow_api.c.o
...
dpkg-buildpackage: info: binary-only upload (no source included)
[build.sh 02:34:10] make pkg-deb finished in 45m1s
[build.sh 02:34:20] OK: 11 packages, version 26.06-release, patches: 0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch
```
Disk: `deploy/vpp/.build` = 3.4 GB (source + build tree + ccache + out), budget 40 GB.

### Package names and versions — built vs host (side by side)
```
PACKAGE                BUILT (out/)     HOST dpkg -l     HOST build-root    ON HOST
libvppinfra            26.06-release    26.06-release    26.06-release      installed
libvppinfra-dev        26.06-release    -                26.06-release      built-only
python3-vpp-api        26.06-release    26.06-release    26.06-release      installed
vpp                    26.06-release    26.06-release    26.06-release      installed
vpp-crypto-engines     26.06-release    26.06-release    26.06-release      installed
vpp-dbg                26.06-release    -                26.06-release      built-only
vpp-dev                26.06-release    -                26.06-release      built-only
vpp-drivers            26.06-release    26.06-release    26.06-release      installed
vpp-plugin-core        26.06-release    26.06-release    26.06-release      installed
vpp-plugin-devtools    26.06-release    -                26.06-release      built-only
vpp-plugin-dpdk        26.06-release    26.06-release    26.06-release      installed
```
Extra check (not required): the *file lists* (`dpkg-deb -c`) of 10/11 packages are identical to the host's
`/root/vpp/build-root/*.deb`; `vpp-dbg` differs only in 776 `/usr/lib/debug/.build-id/…` entries (0 other differences) —
expected, the binaries differ (build path, patched `flow_api.c`). Byte identity is not claimed.

### SHA256SUMS (`deploy/vpp/.build/out/26.06-release/SHA256SUMS`)
```
430f9b29167e1a23cecc1ae187f74727b9d19685ca9d331f16e3c87dd3ed8571  libvppinfra_26.06-release_amd64.deb
3b0e50b192f483d459601fc2c2a40eabba14df66bc0a0ba5ade76d8af736d94b  libvppinfra-dev_26.06-release_amd64.deb
43a43fc227acdcbbc06c2a007eff2459cfeca14edf5ecaf065188c190b0c25ec  python3-vpp-api_26.06-release_amd64.deb
06d4015ef23d80ef38190ff7e16eea0c1c67042a3808da5723a8b11e76227d4b  vpp_26.06-release_amd64.deb
505d3fc33b373af4ac82a6ec5d67ff61bb73b9f01813b85237af19e24d07beab  vpp-crypto-engines_26.06-release_amd64.deb
8ce43edf2afeaa836e09d28609c36c0124179bdb5a088cd7c471dba49f3d28e9  vpp-dbg_26.06-release_amd64.deb
58d16f1880438c0c7e142877a0d0ec6bbfa5cfcf8e9276609738ca201f4886ce  vpp-dev_26.06-release_amd64.deb
9c239507b4da16b980e460855cb024f53a85dbb07580039982d98fa2dc2bff49  vpp-drivers_26.06-release_amd64.deb
6078fd4cb32471907b6e1c34eb194ad7b43cd62b4642cf10dd9e7437802b7118  vpp-plugin-core_26.06-release_amd64.deb
38d1bbfee7592fb15c9c88af2b8b65a8e1a832e99a41de3b0dba5c9fb350acd2  vpp-plugin-devtools_26.06-release_amd64.deb
db3ee4c6bae50442479e0c171ad0f4385e5b6bf1a109bb1051610f16b8c9f36b  vpp-plugin-dpdk_26.06-release_amd64.deb
```

### manifest.json (`deploy/vpp/.build/out/26.06-release/manifest.json`; `packages[]` shortened to 2 of 11 here)
```json
{
  "schema": "vrx.vpp-debs.manifest/v1",
  "upstream": {
    "url": "https://gerrit.fd.io/r/vpp",
    "branch": "stable/2606",
    "tag": "v26.06",
    "tag_object": "29c51fb8b95a92d0d325cff2e502549e652aad9f",
    "commit": "c3200b88dc46bd380f00a49ca3392a102cc1980b",
    "describe": "v26.06-0-gc3200b88d"
  },
  "version": "26.06-release",
  "patches": [
    {
      "name": "0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch",
      "sha256": "00bef2354f263bf91971186c9830a0122b4c78b60ed47342552db17c83205df6",
      "kind": "demo"
    }
  ],
  "options": {
    "trace_plugins": "devtools"
  },
  "build": {
    "builder": "deploy/vpp/build.sh",
    "builder_commit": "b7b10eb8f599d075bd32b6a7ddde39e8f3112a5a",
    "builder_dirty": false,
    "command": "make pkg-deb (upstream defaults: CMAKE_BUILD_TYPE=release, LTO, all plugins)",
    "jobs": 8,
    "seconds": 2701,
    "source_date_epoch": 1782050576,
    "os": "Ubuntu 26.04.1 LTS",
    "arch": "x86_64",
    "finished": "2026-09-23T23:04:16+00:00",
    "missing_build_deps": []
  },
  "packages": [
    {
      "package": "vpp",
      "version": "26.06-release",
      "architecture": "amd64",
      "file": "vpp_26.06-release_amd64.deb",
      "size": 4748764,
      "sha256": "06d4015ef23d80ef38190ff7e16eea0c1c67042a3808da5723a8b11e76227d4b",
      "installed_on_vrx_a": true
    },
    {
      "package": "vpp-dbg",
      "version": "26.06-release",
      "architecture": "amd64",
      "file": "vpp-dbg_26.06-release_amd64.deb",
      "size": 92169196,
      "sha256": "8ce43edf2afeaa836e09d28609c36c0124179bdb5a088cd7c471dba49f3d28e9",
      "installed_on_vrx_a": false
    }
  ]
}
```
(`builder_commit` b7b10eb is the WIP commit on this branch that contained the final build.sh/verify.sh/VERSION/series.)

### Demo patch applied cleanly and is listed
```
$ git -C deploy/vpp/.build/src/vpp describe --long --dirty; git status --short; git diff --stat
v26.06-0-gc3200b88d-dirty
 M src/vnet/ipfix-export/flow_api.c
 src/vnet/ipfix-export/flow_api.c | 6 ++++--
 1 file changed, 4 insertions(+), 2 deletions(-)
```
**[Corrected in the review round, M1]** round 1 used `patch --dry-run --forward --batch`, which *accepts* fuzz (the claim
"aborts on any fuzz failure" was wrong); patches now go through `git apply --check` + `git apply` — see Review fixes.
Round 1:
the patch is listed in `manifest.json` `patches[]` with its sha256 and `kind: demo`.
Optional V18 patch, prepare-only run (`build.sh --prepare-only --trace-plugins core`):
```
[build.sh 01:48:57] applied 0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch (-p1)
[build.sh 01:48:57] applied optional/trace-plugins-core.patch (-p1)
     M src/plugins/tracedump/CMakeLists.txt
     M src/plugins/tracenode/CMakeLists.txt
     M src/vnet/ipfix-export/flow_api.c
[build.sh 01:48:57] tree version: 26.06-release (git describe v26.06-0-gc3200b88d)
[build.sh 01:49:00] build dependencies: all 46 DEB_DEPENDS entries satisfied
[build.sh 01:49:00] --prepare-only: stopping before the compile
```
(the `core` variant was not fully compiled — same compile, only the package split differs.)

### verify.sh (static + against the produced manifest), and a negative test
```
$ deploy/vpp/verify.sh --manifest .build/out/26.06-release/manifest.json --sums .build/out/26.06-release/SHA256SUMS
ok   VERSION: v26.06 c3200b88dc46bd380f00a49ca3392a102cc1980b → 26.06-release, 11 packages
ok   series: 1 patch(es) [0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch], optional: optional/trace-plugins-core.patch
ok   scripts parse; no .deb/.build in git; .gitignore covers deploy/vpp/.build and *.deb
ok   manifest: 11 packages 26.06-release, patches ['0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch'], SHA256SUMS consistent
verify.sh: OK
$ sha256sum -c SHA256SUMS | tail -3
vpp-plugin-core_26.06-release_amd64.deb: OK
vpp-plugin-devtools_26.06-release_amd64.deb: OK
vpp-plugin-dpdk_26.06-release_amd64.deb: OK
$ # tampered copy: "version": "26.06-rc1"
FAIL manifest: vpp-plugin-dpdk: version '26.06-rc1'
FAIL manifest: vpp: version '26.06-rc1'
verify.sh: 12 finding(s)
```

### After (recorded 2026-09-24T02:35:09+03:30) — identical to Before
```
$ git -C /root/vpp status
On branch stable/2606
Your branch is up to date with 'origin/stable/2606'.

nothing to commit, working tree clean
$ git -C /root/vpp log -1
commit c3200b88dc46bd380f00a49ca3392a102cc1980b
Author: Andrew Yourtchenko <ayourtch@gmail.com>
Date:   Sun Jun 21 15:53:33 2026 +0200

    misc: VPP 26.06 Release Notes
    
    Type: docs
    Change-Id: Ib0bb99d0ab36d29d5b109ffd26b8e67c11a9169a
    Signed-off-by: Andrew Yourtchenko <ayourtch@gmail.com>
$ dpkg -l | grep -i vpp
ii  libvppinfra                             26.06-release                               amd64        Vector Packet Processing--runtime libraries
ii  python3-vpp-api                         26.06-release                               amd64        VPP Python3 API bindings
ii  vpp                                     26.06-release                               amd64        Vector Packet Processing--executables
ii  vpp-crypto-engines                      26.06-release                               amd64        Vector Packet Processing--runtime crypto engines
ii  vpp-drivers                             26.06-release                               amd64        Vector Packet Processing--runtime device drivers
ii  vpp-plugin-core                         26.06-release                               amd64        Vector Packet Processing--runtime core plugins
ii  vpp-plugin-dpdk                         26.06-release                               amd64        Vector Packet Processing--runtime dpdk plugin
$ ls -ld --time-style=full-iso /root/vpp/build-root/*.deb /etc/vpp /usr/lib/x86_64-linux-gnu/vpp_plugins  (mtimes)
2026-09-23 11:49:12 /root/vpp/build-root/libvppinfra-dev_26.06-release_amd64.deb   (… all host .deb 2026-09-23 11:49)
2026-09-24 00:21:56 /etc/vpp                                   (before this task started)
2026-09-23 11:54:02 /usr/lib/x86_64-linux-gnu/vpp_plugins
```
The clone has no `objects/info/alternates` (`--dissociate`), so it does not depend on `/root/vpp`.

**Observation, not caused by this task:** the running VPP aborted (SIGABRT, backtrace in libvlib) at 02:23:03 and
systemd restarted it (`restart counter is at 4`, new MainPID 1995428). This task never talks to VPP (no API, no vppctl,
no service commands); the build only compiled in `deploy/vpp/.build`. Reported for the manager (journal `-u vpp` at 02:23).

### CI gate
```
$ tools/ci.sh --base main      # final run, 2026-09-24 02:36, HEAD 74fc56d (only this report changed after it)
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   0m27s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   0m26s
  apps/agent: make lint test build                   0m13s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 1m15s · logs /root/ngfw-wt/logs/ci/F-vpp-debs-20260924-023628-2103994

CI GATE PASSED
```

## Out of scope / not done
Installing the packages anywhere (install procedure only documented for the manager after handover); APT repo /
signing (P10); real V7–V18 fixes (only the V16 demo patch); cross-compilation; ISO; a full compile of the
`--trace-plugins core` variant; hooking `verify.sh` into `tools/ci.sh` (P09-owned file → Q5, documented as a manual gate).

## Decisions made (for the LOG)
- F-vpp-debs-1: patches are applied **uncommitted** on the exact tag so `src/scripts/version` yields `26.06-release`
  (same version as the host, per acceptance); the manifest (patch sha256) is the record of what is inside (see Q4).
- F-vpp-debs-2: source is cloned from upstream with `--reference-if-able /root/vpp --dissociate` into a git-ignored
  scratch dir; both the annotated tag object and the commit hash are pinned and verified.
- F-vpp-debs-3: parallelism cap enforced by `taskset` on 8 CPUs + `MAKE_PARALLEL_JOBS=JOBS=8` (ipsec-mb's `make -j` is unbounded upstream).
- F-vpp-debs-4: V18 is a packaging question (plugins built, shipped in `vpp-plugin-devtools`); offered as
  `--trace-plugins core` (CMake-only optional patch), default stays upstream packaging.
- F-vpp-debs-5: build dependencies are *reported* via upstream's own `DEB_DEPENDS` + `apt-get -s`; `--strict-deps` makes it fatal.

## Open questions
See `docs/status/tasks/F-vpp-debs-questions.md`: Q1 ship vpp-dbg/vpp-dev? · Q2 trace plugins default (V18) ·
Q3 long-term artefact storage · Q4 local version suffix for patched builds · Q5 add verify.sh to tools/ci.sh (P09).



---

## Review fixes (round 2, 2026-09-24) — review afb833a, D-089

Commits: `a595956` (fixes + tests), `554a40d` (every build-time pip from the wheelhouse), plus this report.
Re-verified: one full `--demo` build (11m18s, warm ccache, 8 jobs) to prove H1 and H2 on the real pipeline; everything
else by `--prepare-only` and `deploy/vpp/tests/run.sh` (64 unit-style tests, no clone/network/build).

| finding | fix |
|---|---|
| **H1** unpinned/unverified Python wheels | `deploy/vpp/pydeps.lock`: meson 0.57.2 (sdist), pyelftools 0.33, setuptools 84.0.0, wheel 0.48.0, packaging 26.3 — exact versions + **PyPI-published sha256** + file + URL. `build.sh` builds `.build/pydeps/wheelhouse` holding exactly those files (sha256-checked; missing → https download from the locked URL + hash check; stale/foreign files removed; **never copied from /root/vpp or ~/Downloads**; `--offline-reference` + missing = fatal), proves `pip install --require-hashes --no-index --find-links <wheelhouse> -r pydeps.lock` in a scratch venv under `.build`, and points `DL_CACHE_DIR` at an empty dir under `.build`. `build-patches/0001` (build infrastructure only, `build/` only — enforced by verify.sh) makes upstream's DPDK meson venv install **only** via `pip3 install --require-hashes --no-index --find-links $VRX_PYDEPS_DIR -r $VRX_PYDEPS_LOCK` (no download step; fails closed if unset). The whole make runs with `PIP_NO_INDEX=1 PIP_FIND_LINKS=<wheelhouse> PIP_NO_CACHE_DIR=1` — this also caught a second unpinned input the review did not list: python3-vpp-api's PEP 517 build isolation fetched `setuptools>=61` from PyPI in round 1; it now resolves to the locked setuptools 84.0.0. After the build the DPDK venv's `pip list` must equal the lock; lock and venv are recorded in `manifest.build.inputs`. (Why a build patch and not env vars: pip ignores constraint-file hashes for command-line requirements — tested, questions Q6.) |
| **H2** demo-patched build versioned like the host | D-089: any build with an applied product/demo/optional patch is `26.06-release+vrx<N>` (`VPP_LOCAL_REV=1` in VERSION; `build.sh` replaces `src/scripts/version` after patching and checks the value before compiling). `Status: demo` patches are applied **only with `--demo`**; output dir and manifest carry `variant: demo`. Default `build.sh` = unpatched = `26.06-release`. Round-1 `out/26.06-release/` (demo-patched, unsuffixed) **deleted**. README install block requires `verify.sh --require-files <out> --install-gate`, which refuses an unsuffixed build, any demo patch and a dirty builder. |
| **M1** fuzz accepted | `vrx_apply_patch` = `git apply --check` then `git apply` (exact context). Tested with a fuzzed patch (below; tests 39–44). |
| **M2** `rm -rf` on computed paths | path guards run **first**: `--build-dir` must realpath-resolve inside `deploy/vpp/.build`, `--out` inside the build dir; `/`, `$HOME`, `/root`, `/root/vpp`, `/etc/vpp`, `/usr`, `/var/lib/dpkg`, `/boot`, `/root/ngfw` (and subtrees) always refused. Every `rm -rf` goes through `vrx_rm_rf` (strictly below a root carrying the `.vrx-owned` marker); `--out` is emptied only if it holds nothing but build.sh artefacts. |
| **M3** apt failure reported as "all satisfied" | `vrx_apt_missing` captures apt's rc; non-zero → message + fatal. |
| **L1** verify.sh gaps | `verify.sh --require-files <out>`: missing/extra `.deb`, `dpkg-deb -f Package/Version/Architecture` vs each manifest entry, sha256 on disk vs manifest vs SHA256SUMS (set and hashes), `/` in file names, stale patch/lock hashes, the D-089 version rule, `ship` flags. README states SHA256SUMS/manifest are unsigned (integrity only; signing = P10). |
| **L2** VERSION sourced | `vrx_parse_version`: line by line, a strict regex for every field; unknown or repeated names are rejected; assignment via `printf -v`; used by build.sh and verify.sh; no `source`. |
| **L3** disk budget | build.sh refuses to compile with < 40 GB free under the build dir; prints `du -sh` at the end (3.4 GB). |
| **L4** D-089 alignment | `VPP_PACKAGES_SHIP` (7 runtime packages; no vpp-dbg/vpp-dev/libvppinfra-dev/devtools) → `ship` per package; README: publish to `/srv/vrx-artifacts/vpp/<version>[-variant]/`; questions Q1–Q4 marked answered. |
| **L5** silent fallback to /root/vpp | an unreachable upstream is fatal; `--offline-reference` is the explicit opt-in; `manifest.build.source` = `upstream`/`reference`. |

### Unit-style tests (`deploy/vpp/tests/run.sh`, also run by `verify.sh`)
```
ok 1 - real VERSION parses
ok 2 - VERSION with $(cmd) is rejected
ok 3 - ... and the command never ran
ok 4 - VERSION with backticks is rejected
ok 5 - unknown key is rejected
ok 6 - duplicate key is rejected
ok 7 - tag/version mismatch is rejected
ok 8 - short commit hash is rejected
ok 9 - ship list outside VPP_PACKAGES is rejected
ok 10 - no patches → upstream version
ok 11 - patches → +vrx<N>
ok 12 - +vrx sorts above the unpatched version (dpkg)
ok 13 - generated src/scripts/version prints the local version
ok 14 - ... and the upstream lib version prefix is unchanged
ok 15 - guard: path inside root accepted
ok 16 - guard: root itself accepted as build dir
ok 17 - guard: ../ escape refused
ok 18 - guard: /root/vpp refused
ok 19 - guard: / refused
ok 20 - guard: $HOME refused
ok 21 - guard: /root (ancestor of /root/vpp) refused
ok 22 - rm_rf: strictly below marked root works
ok 23 - ... and removed it
ok 24 - rm_rf: the root itself refused
ok 25 - rm_rf: outside refused
ok 26 - ... outside still exists
ok 27 - rm_rf: root without ownership marker refused
ok 28 - out dir with only artefacts is ours
ok 29 - out dir with a foreign file is refused
ok 30 - build.sh --build-dir /root/vpp refuses before doing anything
ok 31 - build.sh --build-dir /root/vpp/build-root refuses before doing anything
ok 32 - build.sh --build-dir /tmp refuses before doing anything
ok 33 - build.sh --build-dir /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/../.. refuses before doing anything
ok 34 - build.sh --out / refuses before doing anything
ok 35 - build.sh --out /root refuses before doing anything
ok 36 - build.sh --out /root/vpp/build-root refuses before doing anything
ok 37 - build.sh --out /root/ngfw-wt/F-vpp-debs/deploy/vpp refuses before doing anything
ok 38 - build.sh --out /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/../x refuses before doing anything
ok 39 - fuzzed patch: GNU patch would accept it (the old behaviour)
ok 40 - fuzzed patch: vrx_apply_patch refuses it
ok 41 - ... and the tree is unchanged
ok 42 - exact patch applies
ok 43 - ... with the expected result
ok 44 - already-applied patch is refused
ok 45 - apt-get -s failure → rc 2
ok 46 - installed package → nothing missing
ok 47 - pydeps.lock parses (5 entries)
ok 48 - unhashed lock entry is rejected
ok 49 - prepare (offline): cached verified file kept
ok 50 - ... foreign/stale file removed
ok 51 - verify: wheelhouse matches lock
ok 52 - verify: tampered file detected
ok 53 - prepare (offline): tampered file removed and not replaced → fails
ok 54 - ... tampered file is gone
ok 55 - unpatched output passes --require-files
ok 56 - unpatched output fails the install gate
ok 57 - missing .deb detected
ok 58 - extra .deb detected
ok 59 - manifest package name not matching the .deb control field detected
ok 60 - demo-patched build with the unsuffixed version rejected (D-089)
ok 61 - demo-patched build with +vrx passes --require-files
ok 62 - ... but never passes the install gate
ok 63 - demo patch without demo variant rejected
ok 64 - SHA256SUMS with an extra entry rejected
64 passed, 0 failed
```

### M2 / M3 / L2 on the real script
```
$ deploy/vpp/build.sh --build-dir /root/vpp --prepare-only
--build-dir: /root/vpp is a protected path — refusing
[build.sh] ERROR: --build-dir must resolve inside /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build
rc=1
$ deploy/vpp/build.sh --build-dir /root/vpp --reference /some/other/clone --prepare-only
--build-dir: /root/vpp is a protected path — refusing
[build.sh] ERROR: --build-dir must resolve inside /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build
rc=1
$ deploy/vpp/build.sh --out / --prepare-only
--out: / is a protected path — refusing
[build.sh] ERROR: --out must resolve inside the build dir /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build
rc=1
$ deploy/vpp/build.sh --out /root/vpp/build-root --prepare-only
--out: /root/vpp/build-root is a protected path — refusing
[build.sh] ERROR: --out must resolve inside the build dir /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build
rc=1
$ deploy/vpp/build.sh --build-dir /tmp/elsewhere --prepare-only
--build-dir: /tmp/elsewhere is outside /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build — refusing
[build.sh] ERROR: --build-dir must resolve inside /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build
rc=1
$ vrx_apt_missing vrx-nonexistent-pkg-xyz   (M3)
apt-get install -s failed (rc=100): E: Unable to locate package vrx-nonexistent-pkg-xyz 
rc=2
$ VERSION with VPP_TAG=$(touch /tmp/pwned) (L2)
VERSION:19: VPP_TAG='$(touch /tmp/vrx-pwned)' does not match ^v[0-9]{2}\.[0-9]{2}(\.[0-9]+)?$
rc=1
ls: cannot access '/tmp/vrx-pwned': No such file or directory
```

### M1 — a fuzzed copy of the demo patch on the real VPP tree
```
$ diff patches/0001-DEMO-*.patch fuzzed-demo.patch
21c21
<    rmp->src_port = htons (fcm->src_port);
---
>    rmp->src_port = htons (fcm->src_port); /* drifted context */
$ patch -p1 --forward --batch --dry-run   (old build.sh behaviour)
checking file src/vnet/ipfix-export/flow_api.c
Hunk #1 succeeded at 332 with fuzz 1.
rc=0
$ vrx_apply_patch = git apply --check && git apply   (new)
error: patch failed: src/vnet/ipfix-export/flow_api.c:332
error: src/vnet/ipfix-export/flow_api.c: patch does not apply
patch fuzzed-demo.patch does not apply exactly (git apply --check)
rc=1
$ git -C .build/src/vpp status --porcelain        (tree untouched by the refused patch)
 M build/external/packages/dpdk.mk
```

### Default build is unpatched (`build.sh --prepare-only`, log /root/ngfw-wt/logs/F-vpp-debs-fix-prepare-default.log)
```
[build.sh 03:01:38] expect: version 26.06-release (0 version-relevant patch(es)); packages: libvppinfra … vpp-plugin-dpdk
[build.sh 03:01:38] skipped demo patch(es) (use --demo): 0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch
[build.sh 03:01:46] checked out v26.06-0-gc3200b88d (pristine, upstream version 26.06-release)
[build.sh 03:01:46] applied build-patches/0001-build-dpdk-hash-locked-python-deps.patch (build)
     M build/external/packages/dpdk.mk
[build.sh 03:01:47] tree version: 26.06-release
[build.sh 03:01:49] build dependencies: all 46 DEB_DEPENDS entries satisfied (apt-get -s rc=0)
[build.sh 03:02:02] pydeps: fetched meson-0.57.2.tar.gz        (first run: https download from files.pythonhosted.org + sha256 check)
[build.sh 03:02:02] pydeps: fetched pyelftools-0.33-py3-none-any.whl
[build.sh 03:02:03] pydeps: fetched setuptools-84.0.0-py3-none-any.whl
[build.sh 03:02:03] pydeps: fetched wheel-0.48.0-py3-none-any.whl
[build.sh 03:02:03] pydeps: fetched packaging-26.3-py3-none-any.whl
[build.sh 03:02:21] pydeps: pip install --require-hashes --no-index --find-links /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/pydeps/wheelhouse OK: meson==0.57.2 packaging==26.3 pyelftools==0.33 setuptools==84.0.0 wheel==0.48.0
[build.sh 03:02:21] --prepare-only: stopping before the compile
```

### H1 + H2 on a full build: `nohup deploy/vpp/build.sh --demo > /root/ngfw-wt/logs/F-vpp-debs-build-r2.log 2>&1 &`
The first attempt (03:03) failed after 8 min: with `PIP_NO_INDEX=1` the python3-vpp-api PEP 517 build could no longer
fetch `setuptools>=61` from PyPI (`ERROR: No matching distribution found for setuptools>=61.0`) — the unpinned input
above. Fixed in `554a40d` (`PIP_FIND_LINKS=<wheelhouse>`, `PIP_NO_CACHE_DIR=1`); second run (same log file):
```
[build.sh 03:12:04] pinned: https://gerrit.fd.io/r/vpp v26.06 (c3200b88dc46bd380f00a49ca3392a102cc1980b)
[build.sh 03:12:04] expect: version 26.06-release+vrx1 (1 version-relevant patch(es)); packages: libvppinfra libvppinfra-dev python3-vpp-api vpp vpp-crypto-engines vpp-dbg vpp-dev vpp-drivers vpp-plugin-core vpp-plugin-devtools vpp-plugin-dpdk
[build.sh 03:12:04] build dir: /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build  out: /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/out/26.06-release+vrx1-demo  jobs: 8  trace-plugins: devtools  reference: /root/vpp  offline: 0
[build.sh 03:12:05] verified: v26.06 = tag object 29c51fb8b95a92d0d325cff2e502549e652aad9f → commit c3200b88dc46bd380f00a49ca3392a102cc1980b (source: upstream)
[build.sh 03:12:09] checked out v26.06-0-gc3200b88d (pristine, upstream version 26.06-release)
[build.sh 03:12:09] applied build-patches/0001-build-dpdk-hash-locked-python-deps.patch (build)
[build.sh 03:12:09] applied patches/0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch (demo)
     M build/external/packages/dpdk.mk
     M src/scripts/version
     M src/vnet/ipfix-export/flow_api.c
[build.sh 03:12:10] tree version: 26.06-release+vrx1
[build.sh 03:12:12] build dependencies: all 46 DEB_DEPENDS entries satisfied (apt-get -s rc=0)
[build.sh 03:12:12] pydeps: cached meson-0.57.2.tar.gz
[build.sh 03:12:12] pydeps: cached pyelftools-0.33-py3-none-any.whl
[build.sh 03:12:12] pydeps: cached setuptools-84.0.0-py3-none-any.whl
[build.sh 03:12:12] pydeps: cached wheel-0.48.0-py3-none-any.whl
[build.sh 03:12:12] pydeps: cached packaging-26.3-py3-none-any.whl
[build.sh 03:12:31] pydeps: pip install --require-hashes --no-index --find-links /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/pydeps/wheelhouse OK: meson==0.57.2 packaging==26.3 pyelftools==0.33 setuptools==84.0.0 wheel==0.48.0 
[build.sh 03:12:31] external tarball cache: libcbor-0.13.0.tar.gz rdma-core-62.0.tar.gz xdp-tools-1.5.5.tar.gz quicly_0.1.6-vpp.tar.gz daq-3.0.21.tar.gz dpdk-26.03.tar.xz v2.0.2.tar.gz 
[build.sh 03:12:31] free disk: 163 GB (>= 40)
[build.sh 03:12:31] make -C /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/src/vpp pkg-deb MAKE_PARALLEL_JOBS=8 JOBS=8 DL_CACHE_DIR=/root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/pydeps/empty-dl-cache VRX_PYDEPS_LOCK=/root/ngfw-wt/F-vpp-debs/deploy/vpp/pydeps.lock VRX
make: Entering directory '/root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/src/vpp'
make[1]: Entering directory '/root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/src/vpp/build-root'
@@@@ Arch for platform 'vpp' is native @@@@
@@@@ Finding source for external @@@@
...
--- configuring dpdk 26.03 - log: …/external/dpdk.config.log
cd …/external/build-dpdk && … rm -rf ../dpdk-meson-venv && … python3 -m venv ../dpdk-meson-venv && source ../dpdk-meson-venv/bin/activate && test -n "…/pydeps.lock" -a -n "…/wheelhouse" && pip3 install --require-hashes --no-index --find-links=…/wheelhouse -r …/pydeps.lock && …
Looking in links: /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/pydeps/wheelhouse, /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/pydeps/wheelhouse
Processing /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/pydeps/wheelhouse/meson-0.57.2.tar.gz (from -r /root/ngfw-wt/F-vpp-debs/deploy/vpp/pydeps.lock (line 7))
Processing /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/pydeps/wheelhouse/pyelftools-0.33-py3-none-any.whl (from -r …/pydeps.lock (line 8))
Processing …/wheelhouse/setuptools-84.0.0-py3-none-any.whl (from -r …/pydeps.lock (line 9))
Processing …/wheelhouse/wheel-0.48.0-py3-none-any.whl (from -r …/pydeps.lock (line 10))
Processing …/wheelhouse/packaging-26.3-py3-none-any.whl (from -r …/pydeps.lock (line 11))
Successfully installed meson-0.57.2 packaging-26.3 pyelftools-0.33 setuptools-84.0.0 wheel-0.48.0
The Meson build system
...
Looking in links: /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/pydeps/wheelhouse          (python3-vpp-api build isolation)
Processing /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/src/vpp/src/vpp-api/python
  Installing build dependencies: finished with status 'done'
Successfully installed vpp_papi-2.3.2
...
[build.sh 03:23:49] make pkg-deb finished in 11m18s
[build.sh 03:23:50] DPDK meson venv matches pydeps.lock: meson==0.57.2 packaging==26.3 pyelftools==0.33 setuptools==84.0.0 wheel==0.48.0
[build.sh 03:24:00] OK: 11 packages, version 26.06-release+vrx1, variant demo; 3.4G used under /root/ngfw-wt/F-vpp-debs/deploy/vpp/.build
```
Packages (`dpkg-deb -f <deb> Package Version`) in `deploy/vpp/.build/out/26.06-release+vrx1-demo/`:
```
libvppinfra            26.06-release+vrx1
libvppinfra-dev        26.06-release+vrx1
python3-vpp-api        26.06-release+vrx1
vpp                    26.06-release+vrx1
vpp-crypto-engines     26.06-release+vrx1
vpp-dbg                26.06-release+vrx1
vpp-dev                26.06-release+vrx1
vpp-drivers            26.06-release+vrx1
vpp-plugin-core        26.06-release+vrx1
vpp-plugin-devtools    26.06-release+vrx1
vpp-plugin-dpdk        26.06-release+vrx1
```
SHA256SUMS:
```
2514e32c2e94c7f34538bad5969295566f80e8b61ed3541054fd8fb8130bf4f6  libvppinfra_26.06-release+vrx1_amd64.deb
6e30e687de0d0819bf0d9d3d1b71baed9f0a8648b1d8e251c99c3a2764681977  libvppinfra-dev_26.06-release+vrx1_amd64.deb
4a1c616c99dd937d8e0ba9598fa43c2ef02522ae874a124fa96079e2435782b0  python3-vpp-api_26.06-release+vrx1_amd64.deb
70833ad5ee4447d7c3d78d55a1e14daa63807612a1acda76c93d86edea41d718  vpp_26.06-release+vrx1_amd64.deb
a623a54d1906baefdf768c2a9b2aef45e327342183f3fb786404f1364087ca3e  vpp-crypto-engines_26.06-release+vrx1_amd64.deb
501bf93849ed8d5a1760a0acf5032d0bdbe71de769c59e4f1bc56e7daddfe458  vpp-dbg_26.06-release+vrx1_amd64.deb
1d7979b0a195b6cff7ceec7e5a8b7161a4ccde252e9405ff8522c9de4086000e  vpp-dev_26.06-release+vrx1_amd64.deb
af1b7a02e3ae550fc149ac32bdf2138aecfa6cab9919c7d2ea558f3f50bb1471  vpp-drivers_26.06-release+vrx1_amd64.deb
cff15278df4b339e5b9bb605377ecf197e13744830a7b7d5ed67eb3b3af9a5ea  vpp-plugin-core_26.06-release+vrx1_amd64.deb
505c6be27bfe3fc58e34e30e68c8e70d0a91fb73c8600a21b469023d653a0c58  vpp-plugin-devtools_26.06-release+vrx1_amd64.deb
4ab3a318271bd19a01c237c976c743e151a538d682ced2c61d8e354c37241727  vpp-plugin-dpdk_26.06-release+vrx1_amd64.deb
```
manifest.json (packages[] shortened to 2 of 11, inputs.python to 1 of 5):
```json
{
  "schema": "vrx.vpp-debs.manifest/v2",
  "upstream": {
    "url": "https://gerrit.fd.io/r/vpp",
    "branch": "stable/2606",
    "tag": "v26.06",
    "tag_object": "29c51fb8b95a92d0d325cff2e502549e652aad9f",
    "commit": "c3200b88dc46bd380f00a49ca3392a102cc1980b",
    "describe": "v26.06-0-gc3200b88d"
  },
  "version": "26.06-release+vrx1",
  "upstream_version": "26.06-release",
  "local_rev": 1,
  "variant": "demo",
  "patches": [
    {
      "name": "patches/0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch",
      "sha256": "00bef2354f263bf91971186c9830a0122b4c78b60ed47342552db17c83205df6",
      "kind": "demo"
    }
  ],
  "options": {
    "trace_plugins": "devtools",
    "demo": true
  },
  "build": {
    "builder": "deploy/vpp/build.sh",
    "builder_commit": "554a40de83591a0b05bb23a5f61f0822c38fa4e8",
    "builder_dirty": false,
    "source": "upstream",
    "command": "make pkg-deb (upstream defaults: CMAKE_BUILD_TYPE=release, LTO, all plugins)",
    "build_patches": [
      {
        "name": "build-patches/0001-build-dpdk-hash-locked-python-deps.patch",
        "sha256": "2535d7edabe1b54a6fdecf85f91b979e3f2731303e474b931d617b70dc093eb0",
        "kind": "build"
      }
    ],
    "inputs": {
      "python": [
        {
          "name": "meson",
          "version": "0.57.2",
          "sha256": "3a83e7b1c5de94fa991ec34d9b198d94f38ed699d3524cb0fdf3b99fd23d4cc5",
          "file": "meson-0.57.2.tar.gz",
          "url": "https://files.pythonhosted.org/packages/5d/0e/0c72dadd01af2da712eb987e2b7662e2e2c2c34fcdfe3cc6d765bddb2db3/meson-0.57.2.tar.gz"
        }
      ],
      "dpdk_meson_venv": [
        "meson==0.57.2",
        "packaging==26.3",
        "pyelftools==0.33",
        "setuptools==84.0.0",
        "wheel==0.48.0"
      ]
    },
    "jobs": 8,
    "seconds": 678,
    "source_date_epoch": 1782050576,
    "os": "Ubuntu 26.04.1 LTS",
    "arch": "x86_64",
    "finished": "2026-09-23T23:53:53+00:00",
    "missing_build_deps": []
  },
  "packages": [
    {
      "package": "vpp-dbg",
      "version": "26.06-release+vrx1",
      "architecture": "amd64",
      "file": "vpp-dbg_26.06-release+vrx1_amd64.deb",
      "size": 92173594,
      "sha256": "501bf93849ed8d5a1760a0acf5032d0bdbe71de769c59e4f1bc56e7daddfe458",
      "ship": false,
      "installed_on_vrx_a": false
    },
    {
      "package": "vpp",
      "version": "26.06-release+vrx1",
      "architecture": "amd64",
      "file": "vpp_26.06-release+vrx1_amd64.deb",
      "size": 4748308,
      "sha256": "70833ad5ee4447d7c3d78d55a1e14daa63807612a1acda76c93d86edea41d718",
      "ship": true,
      "installed_on_vrx_a": true
    }
  ]
}
```
Validation:
```
$ deploy/vpp/verify.sh --no-tests --require-files .build/out/26.06-release+vrx1-demo
ok   VERSION: v26.06 c3200b88dc46bd380f00a49ca3392a102cc1980b → 26.06-release (patched: 26.06-release+vrx1), 11 packages, ship 7
ok   series: patches 1 · build-patches 1 (build/ only) · optional optional/trace-plugins-core.patch
ok   pydeps.lock: meson==0.57.2 pyelftools==0.33 setuptools==84.0.0 wheel==0.48.0 packaging==26.3 (sha256-pinned)
ok   scripts parse + shellcheck; no .deb/.whl/.build in git; .gitignore covers deploy/vpp/.build and *.deb
ok   output .build/out/26.06-release+vrx1-demo: 11 .deb, version 26.06-release+vrx1, variant demo, patches ['patches/0001-DEMO-ipfix-export-classify-dump-reply-msg-id-base-V16.patch']; files/control fields/sha256/SHA256SUMS consistent
verify.sh: OK
rc=0
$ deploy/vpp/verify.sh --no-tests --require-files .build/out/26.06-release+vrx1-demo --install-gate
ok   VERSION: v26.06 c3200b88dc46bd380f00a49ca3392a102cc1980b → 26.06-release (patched: 26.06-release+vrx1), 11 packages, ship 7
ok   series: patches 1 · build-patches 1 (build/ only) · optional optional/trace-plugins-core.patch
ok   pydeps.lock: meson==0.57.2 pyelftools==0.33 setuptools==84.0.0 wheel==0.48.0 packaging==26.3 (sha256-pinned)
ok   scripts parse + shellcheck; no .deb/.whl/.build in git; .gitignore covers deploy/vpp/.build and *.deb
FAIL output: install gate: demo patch in this build — never install it
verify.sh: 1 finding(s)
rc=1
```

### Before / after (round 2) — unchanged
Before (03:02:45) and after (03:24:37) are identical apart from the timestamp, and identical to round 1:
```
2026-09-24T03:24:37+03:30
$ git -C /root/vpp status
On branch stable/2606
Your branch is up to date with 'origin/stable/2606'.

nothing to commit, working tree clean
$ git -C /root/vpp log -1 --format="%H %s"
c3200b88dc46bd380f00a49ca3392a102cc1980b misc: VPP 26.06 Release Notes
$ dpkg -l | grep -i vpp
ii  libvppinfra                             26.06-release                               amd64        Vector Packet Processing--runtime libraries
ii  python3-vpp-api                         26.06-release                               amd64        VPP Python3 API bindings
ii  vpp                                     26.06-release                               amd64        Vector Packet Processing--executables
ii  vpp-crypto-engines                      26.06-release                               amd64        Vector Packet Processing--runtime crypto engines
ii  vpp-drivers                             26.06-release                               amd64        Vector Packet Processing--runtime device drivers
ii  vpp-plugin-core                         26.06-release                               amd64        Vector Packet Processing--runtime core plugins
ii  vpp-plugin-dpdk                         26.06-release                               amd64        Vector Packet Processing--runtime dpdk plugin
```
`/root/Downloads` does not exist and was not created; nothing under `/root/vpp` was written.

### CI gate (round 2)
```
$ tools/ci.sh --base main      # 2026-09-24 03:28, HEAD dce386c (only this report changed after it)
== contract guard: HEAD vs main ==
no contract files changed in the 10 commit(s) of HEAD since main (87f84b2)
WARN commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(F-vpp-debs): findings
...
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   0m26s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   0m27s
  apps/agent: make lint test build                   0m14s
  test/ Go modules, unit mode (test/integration/smoke)   0m01s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(F-vpp-debs): findings
  mode quick · wall time 1m14s · logs /root/ngfw-wt/logs/ci/F-vpp-debs-20260924-032757-2394977

CI GATE PASSED
(the warning is the manager's review commit afb833a, not a worker commit; a first run at 03:27 failed on a gitleaks
 generic-api-key false positive in this report's wording — the commit was amended with the sentence rephrased)
```

### Decisions (round 2, for the LOG)
- F-vpp-debs-6: build-infrastructure patches (`build-patches/`, `build/` only) are always applied and do not trigger the
  D-089 suffix; the product/demo/optional series does (questions Q6 — a one-line flip if the manager disagrees).
- F-vpp-debs-7: all Python used at build time (DPDK meson venv *and* python3-vpp-api's PEP 517 build isolation) comes
  from one hash-locked wheelhouse; the make runs with `PIP_NO_INDEX=1`, `PIP_NO_CACHE_DIR=1`.
- F-vpp-debs-8: `--build-dir`/`--out` are confined to `deploy/vpp/.build`; build.sh never deletes outside a marked root.
