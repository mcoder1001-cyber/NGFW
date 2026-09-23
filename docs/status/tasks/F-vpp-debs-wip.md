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
- [ ] prepare-only run (clone + verify + patch + dep check) — log /root/ngfw-wt/logs/F-vpp-debs-prepare.log
- [ ] full build — log /root/ngfw-wt/logs/F-vpp-debs-build.log
- [ ] README.md
- [ ] after-state, CI gate, F-vpp-debs.md
