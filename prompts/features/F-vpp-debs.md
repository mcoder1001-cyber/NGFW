# Task: F-vpp-debs — reproducible VPP package build (pinned 26.06 + patch series)   (prepend 00-CONTEXT.md)

## Goal
Make the VPP that the product ships reproducible from this repository (WBS D0.2 in `plan/wbs.csv`): a pinned upstream
`stable/2606` (tag `v26.06`) source reference, an (initially small) patch series, a build script that produces the `.deb` set the host
runs today, and a manifest P10 (Debian packaging) and P14 (installer ISO) consume. **The live host's VPP is never touched.**

## Inputs to read first
- `docs/lab/host-vrx-a.md` — what is installed today (vpp, vpp-plugin-core, vpp-plugin-dpdk, vpp-drivers, vpp-crypto-engines, libvppinfra,
  python3-vpp-api 26.06-release; built from `/root/vpp`) and what was built but not installed (vpp-dev, libvppinfra-dev, vpp-dbg, devtools)
- `/root/vpp` — **read only**: `git -C /root/vpp log -1`, `git -C /root/vpp status`, `build-root/*.deb` names, the build flags used
- `docs/vpp-code-track.md` — V7…V18: candidate patches (the product owner decides funding; this task only provides the mechanism)
- `docs/decisions/LOG.md` D-012 (VPP is owned by the bring-up agent until handover), D-060, D-077 (tracedump/tracenode not built — V18)
- `prompts/P10-packaging-deb.md`, `prompts/P14-iso-installer.md` — the consumers

## Scope — build exactly this
1. `deploy/vpp/` — `VERSION` (upstream repo URL, tag, full commit hash), `patches/` (quilt-style series file; start with **zero or one**
   trivial patch — e.g. the two-line V16 `REPLY_MSG_ID_BASE` fix — clearly marked as a demonstration, not applied to the host),
   `build.sh`: clone/fetch into a **scratch dir under /root/ngfw-wt/F-vpp-debs/.build/ (git-ignored)** or reuse `/root/vpp` only via
   `git worktree add`/`git clone --reference` read-only, verify the commit hash, apply the series, `make install-dep` check (report missing
   build deps, never `apt-get install` them), `make pkg-deb` with the flags the host build used plus the plugin enable list needed by the
   product (tracedump/tracenode per V18 as an option flag), collect `.deb` + `SHA256SUMS` + a `manifest.json` (package, version, sha256).
2. Long builds (> 8 min) run with `nohup … > /root/ngfw-wt/logs/F-vpp-debs-build.log 2>&1 &` and are polled.
3. `deploy/vpp/README.md` — how to bump the tag, add a patch, rebuild, and how P10/P14 consume `manifest.json`; how the manager would
   install the result on vrx-a only after handover (backup, flock -x, dpkg -i, restart, verify, rollback).
4. A CI-friendly check (`deploy/vpp/verify.sh`) that validates VERSION/series/manifest consistency without building (runs in `tools/ci.sh`
   only if it is cheap; otherwise document it as a manual gate).

## Acceptance (paste the evidence)
- [ ] one full `build.sh` run producing the same package *names and versions* as installed on the host (`dpkg -l | grep vpp`) — pasted
      side by side; byte-identity is not required
- [ ] `SHA256SUMS` + `manifest.json` produced; the demo patch applies cleanly and is listed
- [ ] nothing under `/root/vpp`, `/etc/vpp`, `/usr/lib/*/vpp_plugins` or dpkg state changed (`git -C /root/vpp status` and `dpkg -l | grep vpp` before/after pasted)
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Installing packages anywhere; restarting VPP; an APT repository or package signing (P10 / F-hardening-lite); writing real VPP fixes for
V7–V18 beyond the one demo patch; cross-compilation; the ISO.

## Open questions to surface, not to decide silently
Whether the product ships `vpp-dbg`/`vpp-dev`; whether tracedump/tracenode should be enabled by default (V18); where build artefacts are
stored long-term (they must not be committed — `.deb` files are git-ignored).
