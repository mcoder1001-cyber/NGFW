# F-vpp-debs — reproducible VPP 26.06 package build (pinned source + patch series), WBS D0.2

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
`build.sh` dry-runs every patch (`patch --dry-run --forward --batch`) before applying and aborts on any fuzz failure;
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
