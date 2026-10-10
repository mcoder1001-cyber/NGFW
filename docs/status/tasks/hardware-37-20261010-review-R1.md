# R1 correctness review — API native-library package dependencies

Date: 2026-10-10. Independent reviewer `/root/host_37`, worktree `/root/ngfw-wt/hardware-37-20261010`. Reviewed source SHA `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`; final single-commit PR SHA is pending. No product code or target host was modified by this reviewer.

## Scope and findings

The only product diff adds `${shlibs:Depends}` to `Package: ngfw-api` in `deploy/debian/ngfw/debian/control:18`. Existing Node22 bounds, interpreter/system-user dependencies and all other package stanzas are unchanged. `dh binary` already runs `dh_shlibdeps` and `dh_gencontrol`; consuming their generated substitution variable is the appropriate correction for native Argon2's missing runtime library dependencies. Contracts, transaction semantics, API behavior, generated files, tests, CI/workflows and security boundaries are unchanged.

No source correctness BLOCKER/MAJOR/MINOR findings. Required validation is still pending: fixed final API archive must declare the generated native library dependencies, and the unchanged complete mandatory hosted quick gate must pass on the final PR/integration SHA. Prior green runs or a build still running are not acceptance evidence for the new final SHA.

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

Manager build log `/root/Documents/Codex/2026-10-10/hardware/package-build-fixed.log` was read independently and shows package source version `0.1.0~dev+2045ab8b3d2f`, all41 unchanged build fixtures passing (16+12+6+3+4), and `dh_installsystemd --no-enable --no-start`. At this checkpoint build was still running at `dh_strip_nondeterminism`; this log inspection is not an independently reproduced package build or a completed build verdict.

The manager explicitly forbids a broad duplicate local quick build on the nearly full root disk (939MiB observed free). Full quick is pending hosted completion on final PR HEAD. No test/CI weakening was observed in the source diff.

Verdict: **BLOCK pending final archive and final-SHA mandatory quick evidence**. This is a validation hold, not a source-code defect finding.
