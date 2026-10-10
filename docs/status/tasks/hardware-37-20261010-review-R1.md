# R1 correctness review — API native-library package dependencies

Date: 2026-10-10. Independent reviewer `/root/host_37`, worktree `/root/ngfw-wt/hardware-37-20261010`. Reviewed source SHA `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`; final single-commit [PR217](https://github.com/mcoder1001-cyber/NGFW/pull/217) SHA `5bd7e8b545fc765fd2babd8dda15175d6f33af1b`. Independently verified final product tree/tests/CI are identical to source2045; only manager task evidence/review-plan/WIP differ. The earlier bde83bc snapshot was superseded by this documentation-only amendment. No product code or target host was modified by this reviewer.

## Scope and findings

The only product diff adds `${shlibs:Depends}` to `Package: ngfw-api` in `deploy/debian/ngfw/debian/control:18`. Existing Node22 bounds, interpreter/system-user dependencies and all other package stanzas are unchanged. `dh binary` already runs `dh_shlibdeps` and `dh_gencontrol`; consuming their generated substitution variable is the appropriate correction for native Argon2's missing runtime library dependencies. Contracts, transaction semantics, API behavior, generated files, tests, CI/workflows and security boundaries are unchanged.

No source correctness BLOCKER/MAJOR/MINOR findings. Final actual API archive inspection and regression assertion PASS. The unchanged complete mandatory hosted quick gate completed success on the final PR/integration SHA, with actual checkout identity and final output independently verified. Prior green runs were not substituted for final-SHA evidence.

## Actual independent evidence

The following commands ran in the reviewer's own worktree at checkout `86a2f06eafc6406e3e5395769d923a09bbfa60aa`. The test files are identical to reviewed source `2045ab8` (`git diff 2045ab8^ 2045ab8` changes no tests).

```text
$ python3 deploy/debian/ngfw/tests/test_packaging.py
Ran 16 tests in 1.195s
OK
$ python3 deploy/debian/ngfw/tests/test_pppoe_assets.py
Ran 4 tests in 0.223s
OK
$ python3 tools/slot-check.py
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
```

An independent Python assertion reads `git show 2045ab8:deploy/debian/ngfw/debian/control` and its parent, checks `${shlibs:Depends}` is in the API Depends line, removes only the new macro and asserts the result is byte-identical to the parent:

```text
PASS source 2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c: sole control change consumes API shlibs:Depends; all other stanzas identical
```

`readelf -d /dev/shm/ngfw-hardware-package-fixed-20261010/debian/ngfw-api/usr/lib/ngfw/api/node_modules/.pnpm/@node-rs+argon2-linux-x64-gnu@2.2.1/node_modules/@node-rs/argon2-linux-x64-gnu/argon2.linux-x64-gnu.node`:

```text
Shared library: [libdl.so.2]
Shared library: [libgcc_s.so.1]
Shared library: [librt.so.1]
Shared library: [libpthread.so.0]
Shared library: [libc.so.6]
Shared library: [ld-linux-x86-64.so.2]
```

Manager build log `/root/Documents/Codex/2026-10-10/hardware/package-build-fixed.log` was read independently and shows package source version `0.1.0~dev+2045ab8b3d2f`, all41 unchanged build fixtures passing (16+12+6+3+4), and `dh_installsystemd --no-enable --no-start`. Build now reaches `dpkg-source --after-build .`; original build process has exited. This log inspection is not an independently reproduced package build.

Independent final archive regression probe uses `dpkg-deb -f` on both original and fixed archives, asserts libc6/libgcc-s1 present only in the fixed archive, reads the entire fixed archive with `dpkg-deb -c`, verifies one native x64 Argon2 addon and computes SHA256:

```text
Package: ngfw-api
Version: 0.1.0~dev+2045ab8b3d2f
Architecture: amd64
Depends: libc6 (>= 2.34), libgcc-s1 (>= 4.2), nodejs (>= 22), nodejs (<< 23), adduser, python3
sha256 47683ec5b2194ea360c656648718f48943cb04480717cf59804e5ff6ec6309a0
PASS final archive: libc6/libgcc-s1 declared; original archive reproduces missing dependencies; native x64 Argon2 present; archive full contents readable
```

Fixed archive inspected: `/dev/shm/ngfw-api_0.1.0~dev+2045ab8b3d2f_amd64.deb`. Original reproducer: `/root/Documents/Codex/2026-10-10/hardware/runtime/ngfw-api_0.1.0~dev+cc80be66edcf_amd64.deb`. Generated substvars independently read back: `shlibs:Depends=libc6 (>= 2.34), libgcc-s1 (>= 4.2)`.

The manager explicitly forbids a broad duplicate local quick build on the nearly full root disk (939MiB observed free). Independently retrieved completed fresh [run38033766837/job114159912573](https://github.com/mcoder1001-cyber/NGFW/actions/runs/38033766837/job/114159912573): source HEAD `5bd7e8b545fc765fd2babd8dda15175d6f33af1b`, `status=completed`, `conclusion=success`, job completed `2026-10-10T07:36:18Z`. Read-only REST job log download from own worktree succeeded (60874 bytes) into protected `/tmp/hardware-37-20261010-ci/job-114159912573.log`; no raw auth URLs or credentials were printed. Actual hosted checkout is `8a15d644c53cc3ef4abde339efd3b2a0331221a5`, merging final5bd7e8b into current maind2d55984. Earlier run38033113750 on bde83bc was canceled/superseded and is not counted as PASS or as a product-test failure. No test/CI weakening was observed in the source diff.

Independent remote Git-tree/parent assertion confirms tested integration commit8a15d644 has current main d2d55984 and final source5bd7e8b as parents, and tree `a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2` exactly equals local final-head tree. Independently retrieved completed latest-head packaging run38033766825 and provisioning run38033766876 metadata/logs: both success on5bd7e8b;7+81 and46+23+11+18 tests respectively report OK. Full command/output evidence is in this task's T1 receipt.

Actual completed mandatory quick log excerpts (full gate, no check removed):

```text
2026-10-10T07:15:22.6958486Z HEAD is now at 8a15d644 Merge 5bd7e8b545fc765fd2babd8dda15175d6f33af1b into d2d55984d74fa1d06c32e8271886f11f16375407
2026-10-10T07:25:54.8326964Z Tasks:    35 successful, 35 total Cached:    6 cached, 35 total Time:    8m50.903s  
2026-10-10T07:36:11.4197063Z apply-startup harness: green (2 shards; 149 checks passed in the parallel run)
2026-10-10T07:36:11.4241589Z CI GATE PASSED
```

The hosted `tools/ci.sh quick --base origin/main` gate covers contract/generated output, security/forbidden patterns, lint/typecheck/unit/build, agent/CLI lint/test/build, all test Go modules in unit mode and the VPP startup fake-host harness. This is package/source correctness evidence; physical installation, live dataplane and reboot acceptance remain NOT RUN because root storage recovery is required.

Verdict: **APPROVE** on final source `5bd7e8b545fc765fd2babd8dda15175d6f33af1b` and tested integration tree `a0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2`. No source correctness findings. Hardware installation acceptance remains blocked offline filesystem recovery and is not included in this approval.
