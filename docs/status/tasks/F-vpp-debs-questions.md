# F-vpp-debs — open questions (surfaced, not decided)

> **Status after the review round (2026-09-24):** Q1–Q4 are **answered by D-089** (see the ANSWERED lines);
> Q5 stays open for P09; Q6 is new.

Q1 — **Does the product ship `vpp-dbg` / `vpp-dev`?** The build produces all 11 upstream packages; vrx-a installs 7.
`vpp-dbg` (~92 MB) is only useful for post-mortem symbolisation (V8/V9 crash backtraces); `vpp-dev` / `libvppinfra-dev`
are needed only to build out-of-tree plugins. Worker's suggestion: ship the 7 installed ones in `vrx-meta`, keep
`vpp-dbg` in the APT repo (not a dependency) so it can be installed for crash analysis. Owner: P10 / product owner.

ANSWERED (D-089): the product ships neither vpp-dbg nor vpp-dev → `VPP_PACKAGES_SHIP` in VERSION, `ship` per package in manifest.json.

Q2 — **tracedump/tracenode by default (V18)?** Finding: 26.06 *does* build both plugins; upstream packages them in
`vpp-plugin-devtools` (together with `unittest`, `perfmon`, `oddbuf`, `bufmon`, `dispatch_trace`, `tracepath`), which vrx-a
does not install — so D-077's "not built" is really "not installed". Options: (a) install `vpp-plugin-devtools` (also
brings the unittest plugin onto the appliance), (b) `build.sh --trace-plugins core` (optional CMake-only patch moves just
those two into `vpp-plugin-core`, no C code), (c) leave them out. Worker's suggestion: (b), decided by F-capture-trace.
Note: binapi for `tracedump.api.json` / `tracenode.api.json` is not in `/usr/share/vpp/api` on vrx-a today either way,
so P04's binapi-gen would need the new package installed first (after handover).

ANSWERED (D-089): tracedump/tracenode go into vpp-plugin-core via the `--trace-plugins core` variant when F-capture-trace starts.

Q3 — **Where are build artefacts stored long-term?** They are git-ignored and currently live only in
`/root/ngfw-wt/F-vpp-debs/deploy/vpp/.build/out/26.06-release/` (~110 MB), which disappears when the worktree is removed.
Candidates: a fixed host path such as `/srv/vrx/artifacts/vpp/<version>/` (manager-owned), or the P10 reprepro pool.

ANSWERED (D-089): artefacts stay outside git under `/srv/vrx-artifacts` (manager publishes `.build/out/<dir>` there; P10 decides the repo) — README "Storage".

Q4 — **Local version suffix for patched builds?** Patches are applied uncommitted so the version stays
`26.06-release` (required by this task's acceptance: same versions as the host). Consequence: a patched package and the
upstream-equivalent host build have the same Debian version, so `dpkg`/APT will not treat one as an upgrade of the other
and `dpkg -l` cannot tell them apart (only `manifest.json` can). Once a real product patch lands, P10 probably wants
`26.06-release+vrx<N>` (would need a small change in how `src/scripts/version` is fed — `build.sh` option). Owner: P10.

ANSWERED (D-089): patched builds are `26.06-release+vrx<N>` (`VPP_LOCAL_REV`); implemented, demo patches only with `--demo`.

Q5 — **Run `deploy/vpp/verify.sh` inside `tools/ci.sh`?** It is cheap (< 1 s, no network) but `tools/ci.sh` is owned by
P09 and outside this task's files, so it is documented as a manual gate. Suggested one-liner for the P09 owner in
`do_forbidden` or a new step: `[[ ! -x deploy/vpp/verify.sh ]] || run vpp-verify deploy/vpp/verify.sh || fail "deploy/vpp/verify.sh"`.
Note: `verify.sh` now also runs `tests/run.sh` (~30 s); `verify.sh --no-tests` is the < 5 s variant for a CI hook.

ANSWERED (D-092): any change to the source tree, build-patches included, gets `+vrx<N>`; implemented (build.sh counts every applied patch).

Q6 — **Build-infrastructure patch (H1 fix).** Upstream `dpdk.mk` hard-codes an unhashed `pip3 download`/`pip3 install`
in the DPDK meson venv; pip cannot be forced into hash mode from outside for requirements given on the command line
(tested: `PIP_CONSTRAINT`/`PIP_REQUIREMENT` + `PIP_REQUIRE_HASHES` fail with "hashes are required… missing").
So `deploy/vpp/build-patches/0001` changes that one recipe to `pip3 install --require-hashes --no-index
--find-links $(VRX_PYDEPS_DIR) -r $(VRX_PYDEPS_LOCK)`. It is always applied, may only touch `build/` (enforced by
verify.sh), and — as build infrastructure, not product code — does **not** trigger the D-089 `+vrx<N>` suffix, so the
default build still matches vrx-a's `26.06-release`. If the manager reads D-089 as "any tree change → suffix", flip it
by counting `kind: build` in `n_version_patches` in build.sh (one line).
