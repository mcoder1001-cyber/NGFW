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

## Verify round on 083a7ce87

- HEAD `083a7ce87` (rebased onto `main` = `3f099b530`); fix diff reviewed: `git diff 2fd8d5a64 083a7ce87 -- deploy/deploy-hosts.sh deploy/debian/ngfw/tests/test_deploy_hosts.py`.
- Scope: finding 1 graded; findings 2 to 7 re-checked with one line each. Read-only. No product file changed, no commit. Mutations ran on scratch copies under the session scratchpad (`.../scratchpad/r1-verify/`). The OpenSSH probes used the real `/usr/bin/ssh -G` (OpenSSH_10.2p1 Ubuntu-2ubuntu3.6) with a scratch `-F` config, behind a wrapper that refuses every call other than `-G`. No connection, no network, no live host.

### Finding 1 (MAJOR): FIXED

`deploy/deploy-hosts.sh:59-63` now compares `${hostname,,}` with `${1,,}`. I ran the shipped script against real `ssh -G`, through a wrapper `ssh` placed first in PATH that execs `/usr/bin/ssh -F <scratch config> "$@"` for `-G` and otherwise logs `R1-WRAPPER-REFUSED-CONNECT` and exits 99. The scratch config:

```text
Host lab_box        / HostName 192.0.2.7
Host Upper_Box      / HostName 192.0.2.8
Host self_named     / HostName self_named
Host Mixed_Self     / HostName mixed_self
Host lab_appliance  / HostName 192.0.2.9
(second config)  Match host Lab_Match / HostName 192.0.2.10
```

What ssh itself resolves (`ssh -G root@<name> | sed -n 's/^hostname //p'`):

```text
lab_box              -> 192.0.2.7
Lab_Box              -> lab_box          <- `Host lab_box` does NOT match Lab_Box
LAB_BOX              -> lab_box
Upper_Box            -> 192.0.2.8
upper_box            -> upper_box        <- `Host Upper_Box` does NOT match upper_box
Unconfigured_Host    -> unconfigured_host
Web_01.Example.com   -> web_01.example.com
self_named           -> self_named
Mixed_Self           -> mixed_self
Lab_Appliance        -> lab_appliance    <- `Host lab_appliance` does NOT match Lab_Appliance
lab_appliance        -> 192.0.2.9
Lab_Match / lab_match / LAB_MATCH -> 192.0.2.10   (`Match host` is case-insensitive)
```

Shipped script, `bash deploy/deploy-hosts.sh -p /nonexistent-ngfw-pkgdir <host> </dev/null`. An accepted host gets as far as the package-directory check:

```text
Unconfigured_Host    rc=1 : ERROR: invalid host Unconfigured_Host: underscores are valid only in an SSH configuration alias whose Host block sets HostName
unconfigured_host    rc=1 : ERROR: invalid host unconfigured_host: underscores are valid only ...
Web_01.Example.com   rc=1 : ERROR: invalid host Web_01.Example.com: underscores are valid only ...
web_01.example.com   rc=1 : ERROR: invalid host web_01.example.com: underscores are valid only ...
My_Host              rc=1 : ERROR: invalid host My_Host: underscores are valid only ...
lab_box              rc=1 : ERROR: package directory must be a real directory      (accepted)
Lab_Box              rc=1 : ERROR: invalid host Lab_Box: underscores are valid only ...
LAB_BOX              rc=1 : ERROR: invalid host LAB_BOX: underscores are valid only ...
Upper_Box            rc=1 : ERROR: package directory must be a real directory      (accepted)
upper_box            rc=1 : ERROR: invalid host upper_box: underscores are valid only ...
self_named           rc=1 : ERROR: invalid host self_named: underscores are valid only ...
Mixed_Self           rc=1 : ERROR: invalid host Mixed_Self: underscores are valid only ...
lab_appliance        rc=1 : ERROR: package directory must be a real directory      (accepted)
Lab_Appliance        rc=1 : ERROR: invalid host Lab_Appliance: underscores are valid only ...
LAB_MATCH            rc=1 : ERROR: package directory must be a real directory      (accepted; Match host config)
appliance.example    rc=1 : ERROR: package directory must be a real directory      (no -G lookup)
--- refused connection attempts by the wrapper: (none: refused.log absent)
```

(Each underscore line is preceded on stderr by `Pseudo-terminal will not be allocated because stdin is not a terminal.`; see new NIT 9.)

- `Unconfigured_Host` and `Web_01.Example.com` are now refused before any connection: the wrapper recorded no non-`-G` call. The original defect is closed.
- A configured alias works exactly as ssh treats it. An alias written as in its `Host` line (`Upper_Box`, `lab_box`) is accepted. A `Match host` alias in any case (`LAB_MATCH`) is accepted. A spelling that ssh itself does not match to the block (`Lab_Box` against `Host lab_box`) is refused, and that is correct: ssh would connect to the DNS name `lab_box`. The script passes the same `$SSH_USER@$host` string to `-G` and to the real connection, so the gate follows ssh whatever ssh's case rules are.
- Regression test: the fixture now catches the original bug. Mutation `alias-revert` (`$hostname != "$1"`) gives FAILED (failures=1), subtest `host='My_Host'`, `AssertionError: 0 == 0` (the deploy succeeded). The one-sided `${hostname,,} != "$1"` also gives FAILED at `host='My_Host'`.

New MINOR 8 (introduced by this fix, optional, does not reopen finding 1). The fixture, the script comment and the guide claim that "ssh matches Host blocks case-insensitively". OpenSSH 10.2 does not: `Host` patterns match the name as typed, case-sensitively. Only `Match host` is case-insensitive. The fake `ssh -G` (`deploy/debian/ngfw/tests/test_deploy_hosts.py:115-117`) lowercases the name *before* the alias lookup. The new subtest `spelling='Lab_Appliance'` (`:799-809`) therefore asserts an acceptance that real ssh plus this script never produce for `Host lab_appliance`. Mutation `fake-case-sensitive` makes the fake follow OpenSSH: case-sensitive lookup, an unconfigured name echoed lowercased. With the product unchanged, the result is FAILED (failures=1) at `spelling='Lab_Appliance'`, `AssertionError: 1 != 0 : ERROR: invalid host Lab_Appliance: underscores are valid only ...`. The behaviour fails safe (refusal). Only the description is wrong, and it could mislead an operator who writes `Host lab_box` and passes `Lab_Box`. Fix:
  - Fake: `host = args[-1].rsplit('@', 1)[-1]`, then `{...}.get(host, host.lower())`.
  - Fixture: turn `Lab_Appliance` into a refused case ("not the alias for ssh either"). If an accepted mixed-case alias is wanted, add one whose Host line is itself mixed case (e.g. `Lab_Appliance2 -> 192.0.2.8`).
  - Wording: `deploy/deploy-hosts.sh:58`, `test_deploy_hosts.py:115,799-800` and `docs/install/deploy-hosts.md:82-83` should say something like "ssh matches `Host` patterns against the name as given and prints an unconfigured name lowercased, so the printed HostName is compared case-insensitively". The same applies to `docs/status/tasks/deploy-hosts-minors-20261017-wip.md:70,211` ([other: R7]).

New NIT 9 (introduced by the fix for NIT 3). Dropping `2>/dev/null` from the `ssh -G` call (`deploy/deploy-hosts.sh:61`) has a side effect. Every underscore host now prints `Pseudo-terminal will not be allocated because stdin is not a terminal.` on the operator's stderr, also on success. The cause: `ssh -G` with no command and stdin `/dev/null` still evaluates the tty request. Verified: `/usr/bin/ssh -F cfg -G -T -o BatchMode=yes root@lab_box </dev/null >/dev/null` prints nothing. Without `-T` it prints the line. Fix: add `-T` (or `-o RequestTTY=no`) to the `-G` call. Real config errors stay visible.

### Optional findings 2 to 7 (one line each)

2. MINOR 2, plain-command `</dev/null`: mostly holds. Every fixture deploy now gets `input='operator stdin\n'` and the fake ssh records `plain command read stdin`. Removing the redirect on `mkdir` gives FAILED, and on `upgrade-complete` FAILED. Removing it on the lineage-refusal `rmdir` (`:501`) still gives OK, because `test_stale_branch_...` (`test_deploy_hosts.py:475-491`) never asserts `assertNotIn('read stdin', log)`. That is a one-line gap, optional.
3. NIT 3, `ssh -G` errors: fixed. With a broken scratch config, ssh's own `line 2: no argument after keyword "hostname"` / `terminating, 1 bad configuration options` is shown before the refusal (no fixture covers it; side effect in NIT 9).
4. NIT 4, `exec 3<&-`: fixed and tested. Removing it from both scripts gives FAILED, from the preflight only FAILED, and from the install only FAILED (`... inherited script pipe on descriptor 3` for `id`/`ip`/`dpkg-query`/`apt-get`/`curl`). The controls in `test_remote_children_...` are positive (`id inherited script pipe`) and negative (`exec 3<&-` script).
5. NIT 5, 3.11 threshold: fixed. The fake interpreter runs the real probe with `sys.version_info` spoofed to 3.10.12. Changing both `(3, 11)` to `(3, 10)` gives FAILED, the local probe only FAILED, and the remote probe only FAILED.
6. NIT 6, guide wording: fixed (`docs/install/deploy-hosts.md:80-81`: "sets a `HostName` other than the alias itself"). `self_named` and `Mixed_Self` are refused above.
7. NIT 7, reset before the last deploy: fixed (`self.reset_host()` at `test_deploy_hosts.py:829`). The suggested `upgrade-complete` assertion on that last deploy was not added (optional).

### Commands run (output pasted)

```text
$ bash -n deploy/deploy-hosts.sh; echo "bash -n exit=$?"
bash -n exit=0
$ shellcheck --external-sources deploy/deploy-hosts.sh; echo "shellcheck exit=$?"
shellcheck exit=0
version: 0.11.0

$ cd deploy/debian/ngfw/tests && time python3 -m unittest -v \
    test_deploy_hosts.DeployHosts.test_underscore_hosts_need_a_configured_ssh_alias \
    test_deploy_hosts.DeployHosts.test_remote_children_cannot_consume_the_script \
    test_deploy_hosts.DeployHosts.test_old_python_is_refused_on_this_machine_and_on_the_target \
    test_deploy_hosts.DeployHosts.test_success_uses_private_backup_and_verified_installs  </dev/null
test_underscore_hosts_need_a_configured_ssh_alias (...) ... ok
test_remote_children_cannot_consume_the_script (...) ... ok
test_old_python_is_refused_on_this_machine_and_on_the_target (...) ... ok
test_success_uses_private_backup_and_verified_installs (...) ... ok
----------------------------------------------------------------------
Ran 4 tests in 74.992s

OK
real	1m15.249s
exit=0

$ time python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v </dev/null
... 33 lines "... ok" (the same 33 names as the developer's run), no FAIL/ERROR
----------------------------------------------------------------------
Ran 33 tests in 643.642s

OK
real	10m43.951s
exit=0
(the developer's run: "Ran 33 tests in 625.916s / OK"; reproduced. [other: R8] the duration is still about 10.7 min, against the hosted 15-min packaging-fixtures job budget)
```

Scratch mutations (copies of the script and test under `.../scratchpad/r1-verify/t-<name>`, worktree untouched):

```text
== alias-revert         FAILED (failures=1) | Ran 1 test in 48.254s   host='My_Host': AssertionError: 0 == 0      -> finding 1 regression caught
== alias-one-side       FAILED (failures=1) | Ran 1 test in 47.786s   host='My_Host'
== fake-case-sensitive  FAILED (failures=1) | Ran 1 test in 23.821s   spelling='Lab_Appliance': 1 != 0 (product unchanged) -> MINOR 8
== plain-mkdir          FAILED (failures=1) | Ran 3 tests in 27.866s   'read stdin' unexpectedly found
== plain-complete       FAILED (failures=1) | Ran 3 tests in 27.764s   'read stdin' unexpectedly found
== plain-rmdir          OK                  | Ran 3 tests in 27.735s   -> residual gap on MINOR 2
== fd3-both             FAILED (failures=1) | Ran 2 tests in 23.898s   'inherited script pipe' unexpectedly found
== fd3-prepare          FAILED (failures=1) | Ran 2 tests in 23.308s
== fd3-install          FAILED (failures=1) | Ran 2 tests in 23.253s
== py-threshold         FAILED (failures=1) | Ran 1 test in 12.157s
== py-threshold-local   FAILED (failures=1) | Ran 1 test in 11.570s
== py-threshold-remote  FAILED (failures=1) | Ran 1 test in 11.728s
```

`git status --short` after all runs, before this append: only ` M docs/status/tasks/deploy-hosts-minors-20261017-review-R8.md`, which is another reviewer's file and was not written by R1.

### Verify verdict

APPROVE: finding 1 (MAJOR) is fixed and its regression is covered by a test that fails without the fix. Findings 2 to 7 hold, apart from the optional one-line `rmdir` gap in 2. The new MINOR 8 (the fake and the wording model OpenSSH `Host` matching as case-insensitive, which it is not; the behaviour fails safe) and NIT 9 (`-T` on `ssh -G`) are optional and do not block.

Counts this round: BLOCKER 0 · MAJOR 0 open (1 fixed) · MINOR 1 new (8), plus the partial residual on 2 · NIT 1 new (9).

Correction to the `git status` line above: just before the append, `git status --short` also showed ` M docs/status/tasks/deploy-hosts-minors-20261017-review-R7.md`. Like R8's file, it is another reviewer's file, edited during this round, and R1 did not write it. After the append, the only additional entry is this report.
