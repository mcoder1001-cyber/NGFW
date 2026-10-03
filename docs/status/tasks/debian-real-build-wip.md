# Debian real build WIP

- Branch: `codex/debian-real-build-20261003`.
- Local/base SHA: `a237827811a4abd2293157d5e8ea0cebf61196fc`; remote task SHA unverified, publication delegated to coordinator.
- Owned tracked files: `debian-real-build-envelope.md`, `debian-real-build-wip.md` in this directory.
- Audit: clean initial checkout; Ubuntu 26.04.1 amd64; Node 22.23.2, pnpm 12.5.1, Go 1.26.0; dpkg-buildpackage/dpkg-deb/debhelper 13.31ubuntu1 present; lintian absent. 143 GiB available.
- Missing inputs: `deploy/vpp/.build/`, `/srv/vrx-artifacts/`, `apps/api/dist/main.js`, `apps/web/dist/index.html`, `node_modules`; no `.deb` found in worktree deploy or `/tmp`. Host `/root/vpp` has exact pinned commit/tag but no discovered build-root archives; never used as product artifacts.
- In progress: existing `deploy/vpp/build.sh --offline-reference --strict-deps --jobs 2`; log `/tmp/debian-real-build-vpp.log`.
- Product artifacts produced: none. No fixtures produced.
- Remaining: establish verified VPP inputs, compile actual applications, prepare source tree, build/inspect packages; full quick and release acceptance remain unverified.
- Exact next command: `tail -n 60 /tmp/debian-real-build-vpp.log`.

## Concrete blockers observed

- Offline VPP build exited 1 after exact pinned checkout/patch/version and successful simulation of all 46 DEB_DEPENDS. Missing owned wheelhouse input: `deploy/vpp/.build/pydeps/wheelhouse/meson-0.57.2.tar.gz`. Other locked wheelhouse files are also not populated; all five exact URLs/hashes are in `deploy/vpp/pydeps.lock`. Host copies are deliberately not substituted (recipe prohibits that).
- Offline pnpm install exited 1: `ERR_PNPM_NO_OFFLINE_TARBALL` for `@swc/core@1.16.2`. Online install reports DNS failure for `registry.npmjs.org`; bounded with `timeout 60`, scripts disabled for input audit. No compiled application payload can be claimed.
- `git add docs/status/tasks/debian-real-build-envelope.md docs/status/tasks/debian-real-build-wip.md` exited 128: cannot create `/root/NGFW/.git/worktrees/NGFW-debian-real-build/index.lock`: read-only filesystem. No sandbox bypass attempted; coordinator must commit exactly these two source files.
- `tools/ci.sh check --base main` PASSED, exit 0, 10 seconds. Local `main` is older than supplied base: guard reports already-integrated contract changes and historical non-conventional commit warning. This is only the lightweight check, not full quick CI.
- No build-code defect established; no product source changes justified so far.

## Completed audit/tests

- Online VPP attempt exited 1: `curl: (6) Could not resolve host: files.pythonhosted.org`, retries exhausted for the exact locked Meson URL. Logs: `/tmp/debian-real-build-vpp-online.log`; offline log: `/tmp/debian-real-build-vpp.log`.
- Packaging regression suite: `python3 -m unittest discover -s deploy/debian/vrx/tests -p 'test_*.py'` exited 0; 30 tests in 36.092s, OK, **1 skipped**. These are host-independent checks, not product archive/install acceptance. Log: `/tmp/debian-real-build-tests.log`.
- VPP preflight's static verifier/tests completed successfully in both actual build attempts. Source is copied into owned `deploy/vpp/.build/src/vpp`; no VPP compilation reached. The second online attempt reused this locally cloned pinned source; its printed `source: upstream` is not evidence of a fresh upstream fetch. No manifest/artifact was emitted.
- Full unchanged quick gate launched with caches/logs isolated under `/tmp/debian-real-build-*`, exact supplied base, 60-second outer bound. Result pending; no missing-tool bypass used.
- Remaining release gates NOT RUN: product archive inspection, lintian, dependency closure, clean target installation/boot, signed APT publication, hardware acceptance.

## Final result / recovery handoff

- Online pnpm input audit reached its 60-second bound, exit 124; repeated `Temporary failure in name resolution` for registry package tarballs. Scripts were not executed. The earlier command using unsupported `--fetch-retries` exited 2 and was corrected; it is not a build result.
- Unchanged full quick gate exited **1**, at pnpm install. Isolating `XDG_CACHE_HOME` caused Corepack to require its uncached `pnpm-12.5.1.tgz`; download failed `getaddrinfo EAI_AGAIN registry.npmjs.org`. Existing `/usr/bin/pnpm` works with its original read-only cached tool (12.5.1); direct pnpm attempts independently prove missing workspace tarballs and network failure. Quick CI did not reach generators/application checks and is **NOT PASSED**.
- Signing regression skip reason: environment cannot start isolated gpg-agent; signing NOT RUN. No synthetic Debian archive was created by this task.
- Final artifact report: **zero product `.deb`, zero fixture `.deb`**. Archive paths/bytes/SHA-256/control metadata: **not applicable**, no archives produced. Expected output `deploy/vpp/.build/out/26.06-release+vrx1/` was not created. Owned scratch source occupies about 179 MiB; it is source, not an installable product.
- Branch remains at `a237827811a4abd2293157d5e8ea0cebf61196fc`. These two status files are prepared but uncommitted due shared-git write denial. Publication **not performed**; remote checkpoint SHA unverified. Coordinator commits/publishes exactly these files after review.
- No host packages/services/configuration were changed; no Docker, host VPP execution, package installation or verification bypass. Only owned worktree and `/tmp` outputs written. No background task remains from this attempt.
- Actual blockers: five hash-locked Python build distributions unavailable in owned wheelhouse; npm locked dependency store unavailable; DNS/network unavailable. Lintian absent, release license/security/install gates remain outstanding and unchanged.

Next action: restore authorized artifact/network availability, or provision the exact five lockfile inputs through the approved provenance path. Then run from this worktree:

```sh
deploy/vpp/build.sh --strict-deps --jobs 2
```

This rechecks the pinned source/patches and verifies downloads before compiling; it does not install host packages. After genuine VPP output verification and actual app compilation/quick CI, use `deploy/debian/vrx/prepare.sh VERIFIED_VPP_OUTPUT NEW_OUTPUT_DIRECTORY` from a clean committed source checkout, then `dpkg-buildpackage -us -uc -b` inside that prepared output. Do not skip clean-source, VPP install/provenance, licensing or privilege gates to get an archive.
