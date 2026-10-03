# P10 standalone recipient helpers — bounded task envelope

Branch: `codex/p10-standalone-20261003`; isolated worktree `NGFW-p10-standalone`.
Base: `71cee90b2a28421480dfd67a6d64ca7f47f37200` (PR #98, not merged).
Owned: `deploy/debian/bundle/**`, `docs/user/install/bundle.md`, `docs/status/tasks/P10-standalone-*` only.

Implement a separate canonical helper delivery and authenticated recipient launcher without requiring a source checkout. The runtime manifest, transport report and executable helper provenance must remain externally trusted. Never execute code from an unknown delivery artifact; no self-authenticating digest shortcut. Preserve the full real VPP verification gate and its tests. No packages, VPP, services, privilege or permissions changes on this development host.

Import/script inventory observed: bundle install imports verify; verify reads `scripts/10-install-runtime.sh` and executes `deploy/vpp/verify.sh`; VPP verifier sources `lib.sh`, reads VERSION, pydeps.lock and patch/series data, parses build.sh and tests/run.sh, and runs tests/run.sh. Its tests require ordinary Git/patch/APT/coreutils as well as Python/Bash/dpkg. System dependencies must be documented rather than quietly skipping these checks.

Separate helper tar and trusted report will carry byte-identical canonical files in their existing paths. A self-contained launcher, authenticated separately before execution, verifies the helper archive and every member before running a private copy of the existing installer. No source files are vendored into a second maintained implementation.

Signing, real release artifact provenance, clean Ubuntu installation/remove/reinstall, firstboot and hardware acceptance remain open P10 release requirements. This task does not mark all P10 complete. Commit/publish each coherent checkpoint and obtain independent review; never merge or self-review.
