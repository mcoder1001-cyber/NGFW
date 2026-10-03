# P10 standalone — frozen bounded implementation, merge blocked

Branch: `codex/p10-standalone-20261003`; base `71cee90b2a28421480dfd67a6d64ca7f47f37200`.
Frozen product local SHA: `be337b42`; final documentation checkpoint SHA is obtained with `git rev-parse HEAD` after this file's commit. Published earlier exact-tree counterpart: `3744d104` (PR #105, local `6a98286a`); final mode fix/status publication requested from manager. Connector publication uses different ancestry; never equate local and remote SHAs.
PR: https://github.com/mcoder1001-cyber/NGFW/pull/105 (draft; unchanged hosted quick and provisioning gates requested).
Owned: `deploy/debian/bundle/**`, `docs/user/install/bundle.md`, `docs/status/tasks/P10-standalone-*` only.

## Implemented

Separate canonical helper tar exporter plus isolated standalone recipient launcher. Launcher must be authenticated BEFORE execution. Externally trusted helper-report SHA256 authenticates report BEFORE parsing; that report authenticates full helper tar BEFORE tar parsing; exact inventory authenticates all members BEFORE installer subprocess. Externally trusted runtime manifest remains mandatory. No code is imported from unknown package delivery. All helper sources are byte-identical committed canonical files in existing paths, with no hidden divergent implementation. Full unchanged VPP verifier, install gate and tests retained. Actual distro system dependencies documented (Python >=3.12, Bash, dpkg/APT, Git, patch/coreutils/text tools; optional shellcheck).

The preserved VPP path-guard tests need HOME under `set -u`; sanitized environment now uses deterministic `/nonexistent`, not caller HOME. Strict tar REGTYPE excludes links, FIFO and historical contiguous/sparse file kinds. Every directory component is explicitly created at 0700 (root reviewer found that parents=True otherwise created intermediate dirs at 0755 under umask 022); files use authenticated 0600/0700 modes.

## Actual verification

```text
bash deploy/vpp/verify.sh
ok tests/run.sh: 66 passed, 0 failed
verify.sh: OK
python3 -m py_compile deploy/debian/bundle/helpers.py deploy/debian/bundle/recipient.py
PASS
tools/ci.sh check --base 71cee90b2a28421480dfd67a6d64ca7f47f37200
check PASSED (0m10s)
python3 deploy/debian/bundle/test_verify.py
Ran 23 tests in 44.181s — OK
python3 deploy/debian/bundle/test_install.py
Ran 11 tests in 51.028s — OK
python3 deploy/debian/bundle/test_export.py (FINAL frozen be337b42)
Ran 18 tests in 167.532s — OK
```

Final 18 tests = 10 existing exporter fixtures + 8 standalone fixtures. These are actual tiny Debian archives; outside-checkout launcher runs unchanged real full VPP gate and its 66 tests from delivered files in /var/tmp, with no gate mocking and no host installation. Positive preflight ignores poisoned install.py in delivery. Adversarial archive/report changes, expected runtime-manifest mismatch, omission (even deliberately reauthorized incomplete dependency inventory), unsafe/extra/duplicate/link/FIFO/contiguous/traversal/modified/mode-escaping members, symlink/nonregular and oversize inputs refuse. Normal umask 022 extraction checks EVERY directory/file mode. Synthetic empty payload metadata consistency is NOT genuine upstream build provenance.

Raw focused logs: `/tmp/p10-standalone-{verify,install}.log`, `/tmp/p10-standalone-export-final-mode.log`.

## Genuine inherited Go failure — no waiver

Both initial and final frozen-product unchanged complete quick gates FAILED:

```text
TestHostServicesApplyRetrieveRollback
rpc_dns_test.go:61: not rendered: stat /tmp/.../unbound/unbound.conf: no such file or directory
FAIL ngfw/agent/internal/agent
make: *** [Makefile:19: test] Error 1
```

Focused independent reproduction:
`cd apps/agent && go test -race -count=1 ./internal/agent -run '^TestHostServicesApplyRetrieveRollback$'`
Same FAIL (0.543s). No apps/agent file differs from frozen base. This is outside assigned ownership, so no unrelated Go mutation or gate bypass was made. Initial full quick's 35 turbo tasks passed; final full quick's 35 turbo tasks passed (28 cached, 52.338s) before the genuine Go failure. Local quick is NOT GREEN.

Raw final full quick: `/tmp/p10-standalone-quick-final.log`.
Detailed final agent log: `/root/ngfw-wt/logs/ci/NGFW-p10-standalone-20261003-131131-195997/09-agent.log`.
Raw focused Go reproduction: `/tmp/p10-standalone-go-repro.log`.

## Remaining and next action

Manager must publish final coherent checkpoint to PR #105 and retain local↔remote exact-tree mapping; independent review and unchanged hosted gates must inspect final head. Separately scope/fix the genuine Go failure before any merge. Parent PR #98 must be independently reviewed/integrated first, then validate final integration tree against main. Developer neither self-reviews nor merges.

Bootstrap signing/distribution ownership and security provenance policy, genuine complete product/VPP build provenance, signed release, clean Ubuntu install/remove/reinstall, firstboot and hardware release acceptance remain open P10 gaps. No all-P10-DONE claim.

Next commands: `git rev-parse HEAD`; manager connector-publish that exact tree; inspect PR #105 final-head workflow checks; reproduce/fix the Go failure in a separately owned worktree. All this worker's commands/edits targeted its own isolated worktree or read-only logs; none targeted original /root/NGFW source, main, reviewed98 or manager worktrees. No git reset/clean or original cleanup occurred.
