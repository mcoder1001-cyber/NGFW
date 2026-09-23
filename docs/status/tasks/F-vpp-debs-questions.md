# F-vpp-debs — open questions (surfaced, not decided)

Q1 — **Does the product ship `vpp-dbg` / `vpp-dev`?** The build produces all 11 upstream packages; vrx-a installs 7.
`vpp-dbg` (~92 MB) is only useful for post-mortem symbolisation (V8/V9 crash backtraces); `vpp-dev` / `libvppinfra-dev`
are needed only to build out-of-tree plugins. Worker's suggestion: ship the 7 installed ones in `vrx-meta`, keep
`vpp-dbg` in the APT repo (not a dependency) so it can be installed for crash analysis. Owner: P10 / product owner.

Q2 — **tracedump/tracenode by default (V18)?** Finding: 26.06 *does* build both plugins; upstream packages them in
`vpp-plugin-devtools` (together with `unittest`, `perfmon`, `oddbuf`, `bufmon`, `dispatch_trace`, `tracepath`), which vrx-a
does not install — so D-077's "not built" is really "not installed". Options: (a) install `vpp-plugin-devtools` (also
brings the unittest plugin onto the appliance), (b) `build.sh --trace-plugins core` (optional CMake-only patch moves just
those two into `vpp-plugin-core`, no C code), (c) leave them out. Worker's suggestion: (b), decided by F-capture-trace.
Note: binapi for `tracedump.api.json` / `tracenode.api.json` is not in `/usr/share/vpp/api` on vrx-a today either way,
so P04's binapi-gen would need the new package installed first (after handover).

Q3 — **Where are build artefacts stored long-term?** They are git-ignored and currently live only in
`/root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/out/26.06-release/` (~110 MB), which disappears when the worktree is removed.
Candidates: a fixed host path such as `/srv/vrx/artifacts/vpp/<version>/` (manager-owned), or the P10 reprepro pool.

Q4 — **Local version suffix for patched builds?** Patches are applied uncommitted so the version stays
`26.06-release` (required by this task's acceptance: same versions as the host). Consequence: a patched package and the
upstream-equivalent host build have the same Debian version, so `dpkg`/APT will not treat one as an upgrade of the other
and `dpkg -l` cannot tell them apart (only `manifest.json` can). Once a real product patch lands, P10 probably wants
`26.06-release+vrx<N>` (would need a small change in how `src/scripts/version` is fed — `build.sh` option). Owner: P10.

Q5 — **Run `deploy/vpp/verify.sh` inside `tools/ci.sh`?** It is cheap (< 1 s, no network) but `tools/ci.sh` is owned by
P09 and outside this task's files, so it is documented as a manual gate. Suggested one-liner for the P09 owner in
`do_forbidden` or a new step: `[[ ! -x deploy/vpp/verify.sh ]] || run vpp-verify deploy/vpp/verify.sh || fail "deploy/vpp/verify.sh"`.
