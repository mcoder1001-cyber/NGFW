# WIP: vpp-tests-mkdeb-umask-20261019

Branch: `fix/vpp-tests-mkdeb-umask-20261019`, cut from `origin/main` `cb148aafb` (2026-10-19).
PR: https://github.com/mcoder1001-cyber/NGFW/pull/245 (open, base `main`; not merged by this agent).
Origin: follow-up item 2 of the host 37 upgrade receipt
`docs/status/tasks/deploy-host-37-20261019-wip.md` (branch `claude/machine-172-30-126-37-update-c90ed0`,
PR #244): the operator build `deploy/deploy-hosts.sh -b` on 2026-10-19 had to pass `NGFW_VPP_IN_TESTS=1`.

## Envelope (task as received, 2026-10-19)

Test-only fix: make the `mkdeb()` fixture in `deploy/vpp/tests/run.sh` independent of the caller's
umask so `deploy/vpp/verify.sh` (and therefore `deploy/deploy-hosts.sh -b`, which sets `umask 077`)
passes without `NGFW_VPP_IN_TESTS`. Smallest correct change; keep the product scripts (`verify.sh`,
`lib.sh`, `build.sh`, `deploy-hosts.sh`) unchanged. Done means: `(umask 077; deploy/vpp/tests/run.sh)`
and the plain run both report 72 passed, 0 failed; `deploy/vpp/verify.sh` static run is OK;
shellcheck on `tests/run.sh` is clean if it was before. Commit, push, short wip note, small PR against
main; do not merge; do not run `deploy-hosts.sh` against any host.

## Checkpoints (local SHA = remote SHA verified after each push)

| checkpoint | SHA | content |
|---|---|---|
| 1 | `b86d288a6` | the fix: three lines inside `mkdeb()` (two comment lines, one `chmod` line) |
| 2 | the commit carrying this file (branch head after `b86d288a6`) | this wip note |

## Owned files

- `deploy/vpp/tests/run.sh` (the `mkdeb()` fixture only)
- `docs/status/tasks/vpp-tests-mkdeb-umask-20261019-wip.md` (this file)

## Completed code

`mkdeb()` now runs `chmod 0755 "$d" "$d/DEBIAN"; chmod 0644 "$d/DEBIAN/control"` before
`dpkg-deb --build`, with a two-line comment naming the reason (deploy-hosts.sh runs verify.sh under
`umask 077`; dpkg-deb refuses a `DEBIAN` directory outside 0755..0775). Cause as reproduced before the
fix: under `umask 077`, `mkdir -p "$d/DEBIAN"` produced a 0700 control directory, `dpkg-deb --build`
refused it ("control directory has bad permissions 700 (must be >=0755 and <=0775)"), no `.deb` was
produced, and tests 61, 62 and 69 reported "package set != VERSION VPP_PACKAGES" (69 passed, 3 failed).
`LC_ALL=C` is not involved. The alternative `umask 022` at the top of `tests/run.sh` was not chosen:
the fixture is the only mode-sensitive place in the suite, and the explicit `chmod` keeps the
requirement next to the `dpkg-deb` call that imposes it.

## Actual test results (this worktree, head `b86d288a6`, 2026-10-19)

| command | result |
|---|---|
| `deploy/vpp/tests/run.sh` | 72 passed, 0 failed, exit 0 |
| `(umask 077; deploy/vpp/tests/run.sh)` | 72 passed, 0 failed, exit 0 (before the fix: 69 passed, 3 failed: 61, 62, 69) |
| `deploy/vpp/verify.sh` | `verify.sh: OK` (static: VERSION, series, pydeps.lock, shellcheck, tests 72/0) |
| `(umask 077; deploy/vpp/verify.sh)` | `verify.sh: OK` |
| `cd deploy/vpp && shellcheck -x -S warning build.sh verify.sh lib.sh tests/run.sh` | exit 0 (the gate's own invocation) |
| `shellcheck deploy/vpp/tests/run.sh` (no `-x`) | only SC1091 info on `source "$HERE/lib.sh"`, present before the change, unchanged |

Not run: `deploy-hosts.sh` against any host (out of scope by the envelope).

## Remaining code

None. Product scripts untouched (`git diff cb148aafb..b86d288a6 --stat` lists only `deploy/vpp/tests/run.sh`).

## Current failure

None.

## Exact next command (manager)

```bash
gh pr checks 245
```

Then obtain an independent review, confirm the hosted quick gate is green and merge PR #245
sequentially per `docs/decisions/decision-policy.md` (D112). After the merge, `deploy-hosts.sh -b` no
longer needs `NGFW_VPP_IN_TESTS=1`; update follow-up item 2 of the host 37 receipt accordingly.
