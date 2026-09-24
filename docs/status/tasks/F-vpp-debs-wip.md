# F-vpp-debs — WIP log

## 2026-09-24 — fresh start (worker on host, worktree /root/ngfw-wt/F-vpp-debs, base main@87f84b2)

Before state recorded 01:42 +0330 (`/root/vpp` stable/2606 clean @ c3200b88d, dpkg: 7 vpp packages 26.06-release).

Findings so far:
- host build = upstream defaults (`make pkg-deb`: CMAKE_BUILD_TYPE=release, LTO, ccache, no VPP_EXCLUDED_PLUGINS,
  no extra cmake args) — read from `/root/vpp/build-root/build-vpp-native/vpp/CMakeCache.txt`
- **tracedump/tracenode ARE built** in 26.06; upstream packages them (with tracepath, unittest, perfmon, …) in
  `vpp-plugin-devtools`, which is not installed on vrx-a → V18 is a packaging choice, not a build flag.
  `--trace-plugins core` applies `patches/optional/trace-plugins-core.patch` (CMake COMPONENT only, no C).
- version `26.06-release` comes from `src/scripts/version` via `git describe` → patches are applied uncommitted.

- [x] VERSION, patches/series, demo patch (V16), optional trace-core patch
- [x] build.sh, verify.sh (shellcheck clean)
- [x] prepare-only run OK (devtools + core), 01:48; CI GATE PASSED 01:51 (early run)
- [x] full build 01:49 → 02:34 (45m01s): 11 packages 26.06-release, SHA256SUMS + manifest.json, verify.sh OK
- [x] README.md, questions Q1–Q5
- [x] after-state == before (VPP itself aborted+auto-restarted 02:23, unrelated), F-vpp-debs.md written
- [x] final CI gate: CI GATE PASSED (02:36, log /root/ngfw-wt/logs/F-vpp-debs-ci-final.log)

## 2026-09-24 — review fix round (review afb833a, D-089)
- [x] H1 pydeps.lock + verified wheelhouse + build-patches/0001 (DPDK venv --require-hashes); all build pip via wheelhouse (vpp_papi)
- [x] H2 +vrx<N> for patched builds, demo only with --demo, round-1 unsuffixed demo output deleted, install gate
- [x] M1 git apply --check/apply; M2 path guards + vrx_rm_rf; M3 apt rc; L1 --require-files; L2 VERSION as data; L3 disk; L4 D-089; L5 no silent fallback
- [x] tests/run.sh 64/64; prepare-only default; full --demo build 03:12→03:24 (11m18s) → 26.06-release+vrx1-demo, verify OK, install gate refuses
- [x] before/after unchanged; Review fixes section in F-vpp-debs.md
- [x] CI GATE PASSED (03:28, /root/ngfw-wt/logs/F-vpp-debs-ci-r2.log)
