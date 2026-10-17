# Review R1 (correctness & tests): deploy-hosts-minors-20261017

- Branch: `fix/deploy-hosts-minors-20261017`
- HEAD: `e1157fa6e` (product tree identical to `1a42cb3a0`; `git diff --stat 1a42cb3a0 HEAD` lists docs/status files only)
- Diff range: `git diff d50d4fd2c...HEAD` (base = PR #240 head `origin/fix/deploy-hosts-lineage-routes-20261017`)
- Worktree: `/root/NGFW/.claude/worktrees/gracious-payne-469ec3`
- Reviewer: R1, read-only. No product code was changed. Mutation runs used scratch copies outside the worktree. No live host, network or connecting SSH was used (`ssh -G` only, and a wrapper that refuses any other `ssh` call).

## Acceptance map (envelope items 1 to 5)

| item | code | fixture | fails without the fix? (scratch mutation) |
|---|---|---|---|
| 1 stdin | `deploy/deploy-hosts.sh:286` launcher, `:290`/`:320` `exec 3<&-`, `:486`/`:492`/`:507` `</dev/null` | `test_remote_children_cannot_consume_the_script`, `test_success_...` (stdin-draining recorder, `test_deploy_hosts.py:159-163`) | launcher reverted to `bash -s --`: **FAIL** (`unexpected preflight output`); launcher without `</dev/null`: **FAIL**; `exec 3<&-` removed: passes (NIT 4); plain-command `</dev/null` removed: passes (MINOR 2) |
| 2 nginx | `:293` preflight tool, `:427-428` | `test_nginx_is_validated_and_reloaded_after_install` (order plus `nginx-config`/`nginx-reload` subtests) | `nginx -t` removed: **FAIL** |
| 3 no default route | `:385-389`, sixth literal `:321`/`:323`/`:504` | `test_no_default_route_needs_explicit_flag_and_is_recorded` (refusal, flag, marker 0600, route appearing after, flag no-op with route) | flag check bypassed: **FAIL** |
| 4 Python >= 3.11 | `:72-73` local, `:296` remote, usage header, guide | `test_old_python_is_refused_on_this_machine_and_on_the_target` | local check removed: **FAIL**; remote check removed: **FAIL**; threshold changed to (3, 10): passes (NIT 5) |
| 5 underscore hosts | `:58-66` | `test_underscore_hosts_need_a_configured_ssh_alias` | `configured_alias` always true: **FAIL** (3 subtests); the case bug in MAJOR 1 is not caught |

I checked the launcher by hand. This host's OpenSSH 10.2 `sshd-session` gives non-pty children pipes (`fd0=pipe:[...]` on `sftp-server`, `node`, `server`), so reopening `/dev/fd/3` works on a real Linux sshd. Under the launcher the script shell holds fd0=`/dev/null`, fd1, fd2 and fd255=`pipe:` (bash's script descriptor: byte-wise on a non-seekable fd, close-on-exec). fd 3 is closed by the script, a `head -c 100` child gets EOF at once, the last line runs, and positional arguments (`a`, `'b c'`) pass through. `sys.exit(True)` returns 1 and `sys.exit(False)` returns 0, so the probe's exit status is correct. The nginx rationale matches packaging: `ngfw-web.install` ships `assets/nginx.conf etc/ngfw/`, there is no `ngfw-web.postinst`, and `firstboot.sh:77-85` links the site and runs `nginx -t`. The `tar` exemption in the recorder hides nothing: the remote script's own `tar` calls use `-f` and do not read stdin, and `id` (the first child) would swallow the script first anyway.

## Findings

### MAJOR

1. **`deploy/deploy-hosts.sh:61`: underscore hosts in mixed case get through the alias gate (item 5, against real OpenSSH).** `ssh -G` prints the destination lowercased. A name with no `Host` block prints `hostname <lowercased name>`, which is not equal to the argument, so `configured_alias` returns success. `Unconfigured_Host` and `Web_01.Example.com` are therefore accepted, although no `Host` block sets `HostName`. A real run would then connect to the underscore DNS name `web_01.example.com`. That breaks item 5 ("keeping DNS names strict") and the guide's promise (`docs/install/deploy-hosts.md:78-79`, "otherwise it is refused before any connection"). The fixture cannot catch this: the fake `ssh -G` (`test_deploy_hosts.py:113-118`) echoes the host unchanged, which real `ssh` does not do. Impact is limited: the alphabet regex still blocks shell syntax and strict host-key checking still applies, which is why this is MAJOR and not BLOCKER.
   Evidence (real `/usr/bin/ssh -G` with a scratch `-F` config; anything other than `-G` refused):
   ```text
   lab_box              -> 192.0.2.7         (Host lab_box / HostName 192.0.2.7)
   Lab_Box              -> lab_box
   unconfigured_host    -> unconfigured_host
   Unconfigured_Host    -> unconfigured_host
   self_named           -> self_named        (Host self_named / HostName self_named)
   Upper_Box            -> 192.0.2.8         (Host Upper_Box / HostName 192.0.2.8)
   # shipped script, -p /nonexistent-ngfw-pkgdir HOST (an accepted host reaches the package-dir check):
   lab_box               : ERROR: package directory must be a real directory
   unconfigured_host     : ERROR: invalid host unconfigured_host: underscores are valid only in an SSH configuration alias whose Host block sets HostName
   Unconfigured_Host     : ERROR: package directory must be a real directory
   Web_01.Example.com    : ERROR: package directory must be a real directory
   web_01.example.com    : ERROR: invalid host web_01.example.com: underscores are valid only in an SSH configuration alias whose Host block sets HostName
   ```
   Fix: compare against the lowercased argument, `[[ -n $hostname && $hostname != "${1,,}" ]]`. Make the fake `-G` lowercase the host the way OpenSSH does, and add `Other_Host` and `Web_01.Example` to the refused subtests. Optionally add a configured mixed-case alias that is accepted. With the one-line fix applied to a scratch copy, `test_underscore_hosts_need_a_configured_ssh_alias` still passes (`Ran 1 test in 25.116s OK`).

### MINOR

2. **`deploy/deploy-hosts.sh:486,492,507`: the `</dev/null` on plain remote commands is claimed (WIP item 1, guide) but untested.** The fake ssh reads stdin only for the launcher (`test_deploy_hosts.py:125`). Removing all three `</dev/null` survives `test_success_...`, `test_remote_children_...` and `test_stale_branch_...` (`Ran 3 tests in 28.404s OK`). In real use, a missing redirect lets `ssh` eat the operator's stdin, as in the classic `while read host; do deploy-hosts.sh ... "$host"; done < hosts` loop. Fix: have the fake ssh read stdin on every non-`-G` call and log what it got. Run one deploy with `input='sentinel\n'` and assert no plain command received it.

### NIT

3. **`deploy/deploy-hosts.sh:60`: `2>/dev/null` hides `ssh -G` errors.** A syntax error in the operator's `~/.ssh/config` makes `ssh -G` exit 255, and the operator sees the misleading "underscores are valid only in an SSH configuration alias..." message. Fix: keep stderr, or say in the message that `ssh -G` failed.
4. **`deploy/deploy-hosts.sh:290,320`: no test covers `exec 3<&-`.** Removing it from both scripts passes `test_success_...` and `test_remote_children_...` (`Ran 2 tests in 23.578s OK`). It is hygiene, but the WIP lists it as done. Fix: have one recorder log whether fd 3 is open (`os.fstat(3)`), and assert it is not.
5. **`test_deploy_hosts.py:718-744`: the 3.11 threshold is never evaluated.** The fake interpreter matches the substring `version_info` and does not compare versions, and the passthrough interpreter here is 3.14. Changing both `(3, 11)` to `(3, 10)` passes (`Ran 1 test in 14.477s OK`). Fix: add a static assertion that the script contains `sys.version_info < (3, 11)` exactly twice, as line 828 already does for the version regex.
6. **`docs/install/deploy-hosts.md:76-79`: the alias rule is worded too broadly.** An alias whose `HostName` equals the alias (`self_named` above), or a `Host` block with only `ProxyCommand`/`ProxyJump`, is refused although it "sets a HostName" or is a configured alias. Fix: one clause, "...sets a `HostName` different from the alias".
7. **`test_deploy_hosts.py:772-774`: the last deploy in the underscore test reuses state left by the earlier `lab_appliance` run in the same test.** The `installed` marker and run directory remain, so it is really a same-version reinstall. It is within one test, not across tests. Fix: call `self.reset_host()` first and also assert `upgrade-complete`.

### Outside R1 (not graded)

- [other: R8] Suite duration against the hosted budget. `test_deploy_hosts.py` now takes 607 s here, against 455 s before per the WIP. The hosted job `.github/workflows/packaging-fixtures.yml:51` has `timeout-minutes: 15`. It discovers every `deploy/debian/ngfw/tests/test_*.py` in one sequential process (`.github/scripts/packaging-fixtures.py:24-25`) on `ubuntu-24.04`.
- [other: R8] After a failed `nginx -t` (`deploy/deploy-hosts.sh:427`), the running nginx keeps its old configuration, but the new package's site file is already on disk. A later restart or reboot would fail to start nginx. The message and guide (`docs/install/deploy-hosts.md:165`) could say so.
- [other: R7] The WIP's "Targeted run" block mixes commentary ("(one failure, fixed below)") into pasted output.

## Commands run in this worktree (output pasted)

```text
$ bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh; echo "bash -n exit=$?"
bash -n exit=0
$ shellcheck --external-sources deploy/deploy-hosts.sh; echo "shellcheck exit=$?"
shellcheck exit=0
version: 0.11.0
$ git diff --stat d50d4fd2c...HEAD -- .github tools/ci.sh .shellcheckrc
(empty)

$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v   (stdin </dev/null)
... 33 lines "... ok", including:
test_nginx_is_validated_and_reloaded_after_install ... ok
test_no_default_route_needs_explicit_flag_and_is_recorded ... ok
test_old_python_is_refused_on_this_machine_and_on_the_target ... ok
test_remote_children_cannot_consume_the_script ... ok
test_underscore_hosts_need_a_configured_ssh_alias ... ok
----------------------------------------------------------------------
Ran 33 tests in 607.350s

OK
real	10m7.608s
exit=0
(the 33 test names are identical to the WIP's pasted full run: "Ran 33 tests in 604.668s OK"; reproduced)

$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
Ran 4 tests in 3.813s

OK

$ NGFW_CI_LOG_DIR=<scratch> tools/ci.sh check --base d50d4fd2c
== contract guard: HEAD vs d50d4fd2c ==
no contract files changed in the 2 commit(s) of HEAD since d50d4fd2c (d50d4fd2c)
== forbidden patterns (+ gitleaks) ==
WARN control-plane lines exempted with 'ALLOW:' ... (pre-existing apps/api test lines; none in this diff)
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: ngfwtestsecrets only in test code
ok: gitleaks — scanned ~35131 bytes (35.13 KB) in 646ms no leaks found
ok: no packet trace (...) outside docs and the generated bindings
ok: every shell interface create is followed by an ip4/ip6 classify reset
board valid: 212 tasks; read-only validation
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m10s)
ci exit=0
```

Scratch mutation runs (copies of the script and test under the session scratchpad; worktree untouched):

```text
== bash-s:             FAILED (failures=2)  Ran 2 tests in 4.373s   (ERROR: appliance.example: unexpected preflight output)
== no-devnull:         FAILED (failures=2)  Ran 2 tests in 4.233s   (ERROR: appliance.example: unexpected preflight output)
== nginx-no-test:      FAILED (failures=1)  Ran 1 test in 12.008s
== route-flag-ignored: FAILED (failures=1)  Ran 1 test in 22.322s
== alias-always:       FAILED (failures=3)  Ran 1 test in 62.943s  (other_host, web_01.example, lab_appliance_)
== py-local-removed:   FAILED (failures=1)  Ran 1 test in 12.297s
== py-remote-removed:  FAILED (failures=1)  Ran 1 test in 11.787s
== no-fd3-close:       OK                   Ran 2 tests in 23.578s  -> NIT 4
== plain-stdin:        OK                   Ran 3 tests in 28.404s  -> MINOR 2
== py-threshold:       OK                   Ran 1 test in 14.477s   -> NIT 5
== alias-lower-fix:    OK                   Ran 1 test in 25.116s   (fix for MAJOR 1 is compatible with the fixture)
```

`git status --short` after all runs: only `?? docs/status/tasks/deploy-hosts-minors-20261017-review-R7.md` (another reviewer's file) plus this report.

## Verdict

BLOCK: 1 MAJOR (finding 1) with no accepted debt row, as the shared rules require. If the developer agrees to fix finding 1 in this branch, the verdict becomes APPROVE WITH CHANGES and the re-check is a verify round on finding 1 only. Findings 2 to 7 are optional.

Counts: BLOCKER 0 · MAJOR 1 · MINOR 1 · NIT 5.
