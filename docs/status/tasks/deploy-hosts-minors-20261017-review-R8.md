# Review R8 (operability & packaging): deploy-hosts-minors-20261017

- Reviewer: independent R8 panel agent. Read-only session; the only file written is this report.
- Branch `fix/deploy-hosts-minors-20261017`, HEAD `e1157fa6e` (product tree unchanged since `1a42cb3a0`).
- Diff range `d50d4fd2c...HEAD`. The base is `origin/fix/deploy-hosts-lineage-routes-20261017`, the head of PR #240, which is still open.
- Worktree `/root/NGFW/.claude/worktrees/gracious-payne-469ec3`.
- Not used: network, live host, real `nginx` or `systemctl`, commits or branch switches.

**Verdict: APPROVE WITH CHANGES.** 0 BLOCKER, 1 MAJOR, 6 MINOR, 6 NIT. MAJOR 1 needs a change to a message, the docs and D-250 in this branch. Under the shared rules it becomes BLOCK if it is neither fixed nor covered by an accepted tech-debt row.

## Findings, ranked

### MAJOR

**1. After a failed `nginx -t`, the tool says nginx is fine, but nginx will not start again.**
- **Where:**
  - `deploy/deploy-hosts.sh:427`
  - `docs/install/deploy-hosts.md:160-166`
  - D-250 (`docs/decisions/LOG.md`, last line): "a failed test stops the run with nginx on its previous configuration"
- **Failure scenario:**
  - A release changes the site file `/etc/ngfw/nginx.conf`, for example to reference a certificate or path that only a fresh firstboot creates. On upgrade dpkg replaces the unmodified conffile and `nginx -t -q` fails.
  - The operator reads: `nginx configuration test failed after the upgrade; nginx keeps its previous configuration`.
  - That is true only for the running master process. The broken site is now the file on disk. The Ubuntu unit has `ExecStartPre=/usr/sbin/nginx -t -q ...`, so the next `systemctl restart nginx`, or the next reboot, fails, and the web UI and API proxy go down.
  - Nothing tells the operator this. Nothing points to the previous site: it is inside `$BACKUP/preupgrade-config.tar.gz` under `etc/ngfw`.
  - The run also stops before the API health poll and the stability window. The new agent and API are running but were never health-checked, and the message does not say so.
  - Reassured by "keeps its previous configuration", an operator can close the maintenance window and get an outage at the next reboot.
- **Fix:**
  - Reword the message, for example: `nginx configuration test failed after the upgrade; the running nginx still serves the previous site, but nginx will fail to start on its next restart or reboot. Fix /etc/ngfw/nginx.conf or restore it from $BACKUP/preupgrade-config.tar.gz, then run nginx -t && systemctl reload nginx.service. API health was not checked.`
  - Give the docs paragraph (`:160-166`) and the D-250 wording the same meaning.
  - Extend the `nginx-config` fixture to assert the new message.

### MINOR

**2. The recovery directory keeps no nginx evidence.**
- **Where:**
  - `deploy/deploy-hosts.sh:334`: the EXIT trap saves `systemctl status` for `ngfw-agent.service ngfw-api.service` only.
  - `:427-428`
  - `docs/install/deploy-hosts.md:175`
- **Failure scenario:**
  - When `nginx -t` or `systemctl reload nginx.service` fails, the only diagnostic is the remote stderr. That is the `[emerg]` line, or systemd's "Job for nginx.service failed", and it goes to the operator's terminal.
  - `service-status.txt` does not include nginx. Once the terminal output is gone, the recovery directory does not show why the run failed.
- **Fix:**
  - Add `nginx.service` to the trap's `systemctl status` list.
  - Capture the test output into the recovery directory, for example `nginx -t -q 2> >(tee "$BACKUP/nginx-test.txt" >&2)`, or write it to the file and then `cat` it to stderr on failure.
  - Update the docs sentence at `:175`.

**3. A reload that nginx rejects at runtime is reported as success.**
- **Where:** `deploy/deploy-hosts.sh:423-428`; `docs/install/deploy-hosts.md:160-166`
- **Mechanism:**
  - The Ubuntu unit on this host (nginx 1.28.3-2ubuntu1.10) has `ExecReload=/usr/sbin/nginx -g 'daemon on; master_process on;' -s reload`.
  - The `-s reload` signaller parses the configuration and then only sends SIGHUP. `systemctl reload` returns 0 as soon as the signal is sent.
  - If the master then rejects the new cycle, it logs `[emerg]`, keeps the old configuration and stays `active`.
- **Two realistic ways `nginx -t` passes but the master rejects the reload:**
  - A new `listen` port held by another process. Test mode tolerates EADDRINUSE; that is why `nginx -t` passes while nginx itself holds ports 80 and 443.
  - A path the sandboxed master cannot open, while `nginx -t` passes because it runs as root from the SSH session, outside the drop-in. The drop-in sets `ProtectSystem=strict`, `ReadWritePaths=-/var/log/nginx -/var/lib/nginx /run` and a reduced `CapabilityBoundingSet`. Example: a new `access_log /var/log/ngfw/...`.
- **Result:** the run prints `[host] OK` and writes `upgrade-complete`, but the new site is not active.
- **The stated rationale is only half right.** "A bare reload would hide the error" holds only for these post-parse failures. The signaller already fails on syntax errors, so a bare reload would catch those. It is these post-parse failures that the current sequence still misses.
- **Fix (one of):**
  - Verify that the reload took effect. Record the worker PIDs (children of `systemctl show -p MainPID --value nginx.service`) before the reload, and require a new set within a few seconds. Or require no `-p err` entries in `journalctl -u nginx.service --since <timestamp of the reload>`.
  - At minimum, document the limitation in the paragraph: a reload rejected at runtime, including sandbox-only failures, is not detected; check the nginx error log.

**4. Unconfigured underscore names with capital letters are accepted, so the docs' "refused before any connection" is false.**
- **Where:**
  - `deploy/deploy-hosts.sh:61` (`[[ -n $hostname && $hostname != "$1" ]]`)
  - `docs/install/deploy-hosts.md:77-79`
  - fixture `deploy/debian/ngfw/tests/test_deploy_hosts.py:113-117`. The fake `ssh -G` prints the host unchanged; real OpenSSH lowercases it. [other: R1]
- **Failure scenario:**
  - Real OpenSSH prints `hostname` lowercased.
  - `Other_Host` has no `Host` block at all, yet `ssh -G` prints `hostname other_host`. The string comparison differs, so `configured_alias` returns success. This was confirmed with real `ssh -G` and the shipped function (evidence E below).
  - The tool then opens a real SSH connection, which means a DNS lookup of `other_host`. It fails at "preflight failed" instead of giving the documented clear refusal with no connection.
  - The failure is still closed, but the documented guarantee does not hold and the fixture cannot detect it.
- **Fix:**
  - Compare case-insensitively: `[[ -n $hostname && ${hostname,,} != "${1,,}" ]]`.
  - Make the fake `ssh -G` lowercase the host like OpenSSH.
  - Add `Other_Host`, which must be refused, and `LAB_APPLIANCE`, which must be accepted, to `test_underscore_hosts_need_a_configured_ssh_alias`.

**5. `--allow-no-default-route` covers every host in the run, and there is no local warning.**
- **Where:** `deploy/deploy-hosts.sh:45`, `:385-389`, `:526`; `docs/install/deploy-hosts.md:193-201`
- **Failure scenario:**
  - The docs frame the flag as the operator declaring one management-only appliance ("because an empty route table is also what a broken host looks like").
  - In a fleet run, though, the flag also accepts any other host that has lost its default route. That is exactly the broken-host signal the refusal was meant to keep.
  - Unlike `--allow-divergent`, which logs `WARNING` at `:526`, the flag logs nothing locally.
  - The remote line `no default route; --allow-no-default-route accepted by operator` has no `[host]` prefix. The operator has to work out from the surrounding lines which host it was.
- **Fix:**
  - Docs: say the flag applies to every host of the run, and recommend running management-only appliances on their own.
  - Log a local `WARNING` when the flag is set.
  - Prefix the acceptance with the host. It can stay a remote message; the `deploy_host` log already names the host.
  - Optional: restrict the flag to one host per run, like `--allow-divergent`.

**6. The no-default-route refusal comes after the package transfer and is reported as a failed upgrade.**
- **Where:** `deploy/deploy-hosts.sh:385-386` (inside `REMOTE_INSTALL`, under the lock, after `scp` at `:496`), the trap at `:334`, and `die` at `:504`.
- **Failure scenario:**
  - A management-only appliance run without the flag first receives the full package set. Only then is it refused.
  - The refusal prints `upgrade failed; preserve $BACKUP for manual recovery; no automatic rollback attempted`, writes `service-status.txt`, and locally reports `install/health failed; inspect ... on the host`. Nothing was installed.
  - The staged packages stay in `/root/ngfw-deploy/run.*/packages`. Each retry adds another copy, which matters for the full-disk case.
  - The placement predates this branch, but this is the refusal item 3 tells operators to expect and recover from.
- **Fix:**
  - Add a read-only default-route probe to `REMOTE_PREPARE`, before `mktemp`. It needs the 0/1 literal as a preflight argument, or the preflight can print a route-presence marker that the local side checks against the flag.
  - Keep the recapture under the lock.
  - Or at least make this refusal say `nothing was installed; remove $BACKUP` instead of the recovery banner.

**7. Fixture runtime is not yet checked against the hosted packaging job budget.**
- **Where:** `.github/workflows/packaging-fixtures.yml:51` (`timeout-minutes: 15`); `.github/scripts/packaging-fixtures.py:24-25`, which discovers all `deploy/debian/ngfw/tests/test_*.py` in one process.
- **Scenario:**
  - `test_deploy_hosts.py` alone now takes 604.668 s locally. It took 455 s with 28 tests, so this branch adds 33%.
  - My three targeted tests took 98.586 s.
  - That leaves about 5 minutes for checkout, Node setup and ten other suites. There is no hosted measurement yet, because the PR is not open.
  - A hosted timeout is a failed gate. The fix must not be a higher timeout or skipped tests.
- **Fix:**
  - Record the hosted packaging-job duration from the PR run in the WIP before merge.
  - If it is close to the limit, reduce the full deploys per test, for example by sharing one successful deploy across the assertions of the success, stdin and nginx-order tests.

### NIT

**8. The NRestarts baseline is read later than the comment says.**
- **Where:** `deploy/deploy-hosts.sh:422-443`.
- The `restarts_before` read now happens after `nginx -t` and `systemctl reload`, not right after the manual restart, which is what the comment at `:429` says.
- With `RestartSec=3` this misses nothing in practice.
- **Fix:**
  - Read the baseline immediately after `systemctl restart`.
  - Optionally, move the nginx block after the stability window, so an nginx failure does not hide the API verdict (see MAJOR 1). The final `is-active` loop would then need to run after the reload.

**9. The launcher assumes the SSH session's stdin is a pipe, and remote errors now begin with `/dev/fd/3:`.**
- **Where:** `deploy/deploy-hosts.sh:281-286`.
- Verified on this host's real OpenSSH 10.2p1 (Ubuntu): the children of a `notty` session have `pipe:[…]` as fds 0, 1 and 2.
- With a socket as stdin, the launcher fails closed at the preflight with `bash: /dev/fd/3: No such device or address` (exit 126; evidence D).
- Remote errors are now prefixed `/dev/fd/3: line N`, for both scripts. In the install script, N counts from the `declare -f verify_packages` text, not `deploy-hosts.sh` lines.
- **Fix:** mention the pipe assumption in the comment, and the error prefix in the docs.

**10. `ssh -G` runs the operator's `Match exec` commands and discards their stderr.**
- **Where:** `deploy/deploy-hosts.sh:60`; docs `:77-78`.
- Confirmed with a scratch configuration (evidence E).
- A configuration error, or a failing `Match exec`, ends in the misleading "underscores are valid only in an SSH configuration alias…" message.
- Aliases that reach a literal address through `ProxyCommand` or `ProxyJump` without a `HostName` are refused. The message for that case is actionable.
- **Fix:**
  - Docs: "read without connecting; `Match exec` commands in your configuration still run".
  - Show the `ssh -G` stderr when it exits non-zero.

**11. Python docs point at the wrong base system.**
- **Where:** `docs/install/deploy-hosts.md:16`.
- "Debian 12 ships 3.11": the appliance base is Ubuntu 26.04 (`docs/09-os-packages.md:3`). Name the supported base's interpreter instead. [other: R7]
- A versioned `python3 (>= 3.11)` dependency in `debian/control` is not needed for this task:
  - the base is pinned and ships a newer interpreter;
  - `python3` is already a declared dependency, so nothing new is added for `docs/09-os-packages.md`;
  - the preflight check plus the docs give a clear refusal.
- At most, add an optional tech-debt row for a later packaging task.

**12. A locally modified site file is silently kept.**
- **Where:** `deploy/deploy-hosts.sh:400`, `:420`.
- `/etc/ngfw/nginx.conf` is a conffile (debhelper compat 13), and APT runs with `--force-confold`. If the site was modified on the host, dpkg keeps it and installs the package version as `.dpkg-dist`.
- The reload then applies nothing new, and the run reports OK.
- This APT option predates the branch, but it undercuts the nginx rationale.
- **Fix:** after APT, list any `/etc/ngfw/*.dpkg-dist` and `*.dpkg-new` files into the recovery directory and stderr.

**13. A missing preflight tool fails with no message.**
- **Where:** `deploy/deploy-hosts.sh:293-295`.
- The loop, which now includes `nginx`, exits under `set -e` without naming the missing tool. The operator sees only "preflight failed". This pattern predates the branch.
- **Fix:** `command -v "$tool" >/dev/null || { echo "missing $tool" >&2; exit 1; }`.

## Checked, no finding

- **nginx ordering:**
  - The sequence is apt → agent/API restart → `nginx -t -q` → `systemctl reload` → health poll → stability window → `is-active` (which includes nginx).
  - A reload after the API restart lets the old workers drain quickly, because the API restart has already closed the upstream WebSockets.
  - A crash of the SSH session during `systemctl reload` leaves the systemd job to finish on its own. The nginx reload is atomic for new connections.
- **Hardened unit:**
  - `ExecReload` (`nginx -s reload`, run as root inside the drop-in sandbox) needs only to read the configuration and `/run/nginx.pid`, and to signal a same-UID master. `/run` is in `ReadWritePaths`; `kill` is in `@system-service`; no `CAP_KILL` is needed between processes of the same UID.
  - The drop-ins in `deploy/hardening/systemd/` are installed only into an offline root by `stage.py`. The package ships them inert under `/usr/lib/ngfw/hardening`, and the APT plan forbids upgrading nginx itself. So nothing in an upgrade needs a restart rather than a reload.
- **`nginx` in the preflight tool list:** correct. The binary is `/usr/sbin/nginx`, and `/usr/sbin` is first in the launcher's `PATH`. `ngfw-web` depends on `nginx`, and `nginx.service` is already checked to be active.
- **Python:**
  - The local check runs before any connection; `-h` exits earlier, so help still works on an old Python. The message names the version found ("found 3.9", or "unknown" for Python 2, which does not understand `-I`).
  - The remote check runs before `mktemp` and prints `python3 --version`.
- **Launcher on a real appliance:**
  - `/dev/fd` is a symlink to `/proc/self/fd`, and `env -i PATH` finds `bash`.
  - Both a `dash` and a `bash` login shell parse the single-quoted launcher. Arguments pass through.
  - `exec 3<&-` runs at the top of both scripts. That matters: debconf uses fd 3 for its maintainer-script protocol.
  - bash keeps its script fd close-on-exec, and a child reading stdin gets EOF (evidence D).
- **`--allow-no-default-route`:**
  - It is the sixth validated literal, `$6`, checked to be `[01]` remotely, and independent of `--allow-divergent`.
  - `no-default-route.txt` has mode 0600 and is written only when the flag was used.
  - An empty before-state is compared with the after-state, so a default route that appears is caught (fixture `no-route-after`).
- **`ssh -G`:** it needs OpenSSH 6.8 or later (2015). An alias without `HostName` is refused with an actionable message.
- **CI gates:**
  - Shellcheck passes with `--external-sources`.
  - One new `# shellcheck disable=SC2029`, with a justification comment, is scoped to the preflight call (4 → 5 directives).
  - `.github` and `tools/ci.sh` are unchanged against both `origin/main` and the base.
  - No new tool needs a `tools/README.md` entry.

## Commands run (this worktree) and output

A. Syntax, shellcheck, help, CI gate files:
```text
$ bash -n deploy/deploy-hosts.sh                     -> bash -n rc=0
$ shellcheck --version | head -2; shellcheck --external-sources deploy/deploy-hosts.sh
ShellCheck - shell script analysis tool
version: 0.11.0
shellcheck rc=0                                      (no findings)
$ git grep -n 'shellcheck disable' d50d4fd2c -- deploy/deploy-hosts.sh | wc -l   -> 4
$ git grep -n 'shellcheck disable' HEAD -- deploy/deploy-hosts.sh | wc -l        -> 5
$ git diff --stat origin/main...HEAD -- .github tools/ci.sh                       -> (empty)
$ git diff --stat d50d4fd2c...HEAD -- .github tools/ci.sh                         -> (empty)
$ deploy/deploy-hosts.sh -h
Build reviewed NGFW packages, or upgrade explicitly named installed appliances.
Usage: deploy/deploy-hosts.sh [-r REF] [-v VERIFIED_VPP_DIR] [-w NEW_WORK_DIR]
       [-p PACKAGE_DIR] [-b] [-u SSH_USER] [--allow-divergent]
       [--allow-no-default-route] [HOST ...]
-b builds/verifies only. -p requires a complete SHA256SUMS; never creates one.
Hosts are DNS names, IPv4 addresses or SSH configuration aliases; an alias with
underscores must set HostName. Options must precede hosts. --allow-divergent
installs, on exactly one reviewed host, a development build whose source is
divergent from or unresolvable against the installed build's source; it never
permits a rollback within one history or a stable-release downgrade.
--allow-no-default-route upgrades a management-only appliance that has no
default route. Needs python3 >= 3.11 here and on the appliance.
See docs/install/deploy-hosts.md for details.
help rc=0
```

B. Targeted fixtures:
```text
$ cd deploy/debian/ngfw/tests && time python3 -m unittest -v \
    test_deploy_hosts.DeployHosts.test_nginx_is_validated_and_reloaded_after_install \
    test_deploy_hosts.DeployHosts.test_no_default_route_needs_explicit_flag_and_is_recorded \
    test_deploy_hosts.DeployHosts.test_underscore_hosts_need_a_configured_ssh_alias
test_nginx_is_validated_and_reloaded_after_install ... ok
test_no_default_route_needs_explicit_flag_and_is_recorded ... ok
test_underscore_hosts_need_a_configured_ssh_alias ... ok
Ran 3 tests in 98.586s
OK
real 1m38.895s
```

C. Real sshd non-pty session stdin type (read-only `/proc` inspection of an existing `sshd-session: root@notty`, OpenSSH_10.2p1 Ubuntu-2ubuntu3.6):
```text
child 706019: /usr/lib/openssh/sftp-server
/proc/706019/fd/0 -> pipe:[1017655]
/proc/706019/fd/1 -> pipe:[1017656]
/proc/706019/fd/2 -> pipe:[1017657]
```

D. Launcher, run locally with the exact `REMOTE_SHELL` text extracted from the script:
```text
LAUNCHER: env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C HOME=/root bash -c 'exec 3<&0 </dev/null && exec bash /dev/fd/3 "$@"' ngfw-deploy
/dev/fd -> /proc/self/fd
== login shell dash            (same output for bash)
args:2:/root/ngfw-deploy/run.ABCDEFGH/packages:0
0 -> /dev/null                  (child stdin; no fd 255 leaked, fd 3 closed by the script)
after cat                       (a child reading stdin got EOF, the script continued)
/dev/fd/3: line 2: nosuchcmd_xyz: command not found
tail
# same launcher with a socketpair as stdin:
rc 126 out b'' err b'bash: /dev/fd/3: No such device or address\n'
```

E. `ssh -G` behaviour (scratch config via `-F`, no connection):
```text
ssh_config: Host lab_fw1 / ProxyCommand ...; Host Lab_FW2 / User root; Host lab_fw3 / HostName lab_fw3;
            Host lab_fw4 / HostName %h.mgmt.example; Match exec "echo MATCH-EXEC-RAN >&2; false"
lab_fw1      -> lab_fw1                 stderr: MATCH-EXEC-RAN
Lab_FW2      -> lab_fw2                 stderr: MATCH-EXEC-RAN
lab_fw3      -> lab_fw3                 stderr: MATCH-EXEC-RAN
lab_fw4      -> lab_fw4.mgmt.example    stderr: MATCH-EXEC-RAN
Other_Host   -> other_host              stderr: MATCH-EXEC-RAN
# shipped configured_alias() + SSH_OPTS (sed-extracted), ssh wrapped to add -F with
# "Host lab_fw1 (ProxyCommand only)" and "Host lab_appliance / HostName 192.0.2.7":
lab_appliance: accepted as configured alias
other_host: refused
Other_Host: accepted as configured alias        <- no Host block at all (MINOR 4)
LAB_APPLIANCE: accepted as configured alias
lab_fw1: refused
```

F. Packaging context:
```text
/usr/lib/systemd/system/nginx.service (nginx 1.28.3-2ubuntu1.10, this host, read only):
ExecStartPre=/usr/sbin/nginx -t -q -g 'daemon on; master_process on;'
ExecReload=/usr/sbin/nginx -g 'daemon on; master_process on;' -s reload
deploy/debian/ngfw/debian/ngfw-web.install: stage/usr/share/ngfw/web usr/share/ngfw/ ; assets/nginx.conf etc/ngfw/ ; assets/tls-bootstrap.sh usr/lib/ngfw/
deploy/debian/ngfw/debian/: no ngfw-web.postinst; ngfw-web Depends: ${misc:Depends}, nginx, openssl
deploy/hardening/stage.py:72-73: systemd/*.service.d/*.conf -> usr/lib/systemd/system/... of an offline root only
.github/workflows/packaging-fixtures.yml:51  timeout-minutes: 15
.github/scripts/packaging-fixtures.py:24-25  discover(deploy/debian/ngfw/tests, pattern='test_*.py')
docs/09-os-packages.md:3  base system Ubuntu 26.04.x LTS
```

Verdict: APPROVE WITH CHANGES. Fix MAJOR 1 in this branch; the MINORs and NITs are optional; MINOR 7 needs the hosted duration recorded before merge.

## Verify round on 083a7ce87

- Reviewer: R8, a fresh read-only agent. The only file written is this appended section.
- Tree: HEAD `083a7ce87`, which is a descendant of `main` `3f099b530`. Its tree `9741770f3` is identical to the pre-rebase checkpoint `a01b58d67`.
- Response diff: `git diff 2fd8d5a64 083a7ce87 -- deploy docs/install docs/decisions/LOG.md`.
- Scope: MAJOR 1 is graded. Items 2 to 13 get one line each. New observations are listed separately and do not change the verdict.
- Not used: network, live host, real `nginx` or `systemctl`, commits or branch switches.

### MAJOR 1: RESOLVED

- **Message** (`deploy/deploy-hosts.sh:452-456`). It is now accurate on every point the finding raised:
  - the running nginx still serves the previous configuration;
  - the new site file is on disk, and nginx will not start again until it is fixed;
  - agent/API health was not checked;
  - recovery: restore or fix `/etc/ngfw/nginx.conf` (the previous copy is member `etc/ngfw/nginx.conf` of `$BACKUP/preupgrade-config.tar.gz`), then run `nginx -t && systemctl reload nginx.service`.
  - nginx's own output is echoed first and kept in `nginx-test.txt` with mode 0600 (`umask 077`).
- **Recovery copy:**
  - The archive is written at `:419` with `tar -C / ... etc/ngfw ...`, before APT at `:428`. So the named member exists and holds the pre-upgrade file.
  - Confirmed with a scratch archive built the same way (evidence V3).
- **Order:**
  - The `NRestarts` baseline (`:445`) comes before the test (`:452`).
  - The failure exits before the reload and the health poll (`:457-463`).
  - The EXIT trap still prints the recovery banner, and locally `install/health failed; inspect ...` is reported (evidence V4).
- **Docs and D-250:**
  - `docs/install/deploy-hosts.md:173-181` carries the same meaning.
  - `docs/install/upgrade.md:15-18` adds the manual test-then-reload step.
  - D-250 (`docs/decisions/LOG.md:287`) now says "the running nginx keeps the previous configuration, the new file is on disk, nginx would not start again, agent/API health unchecked".
- **Fixture** (`test_nginx_is_validated_and_reloaded_after_install`). It asserts:
  - the new sentences, the member path, the recovery command and the echoed nginx output;
  - the content of `nginx-test.txt`;
  - that no reload and no `curl` follow the failure.
  - It passes (V2).
- **Residual NITs, optional (N-a, N-b):**
  - **N-a.** D-250 still says the tool "runs `nginx -t -q`". The code at `:452` and the docs run `nginx -t`. Fix the wording in the LOG line.
  - **N-b.** The message assumes the failed test is caused by the upgraded site. Two other cases point the operator at the wrong file:
    - nginx's configuration was already broken before the run. The preflight checks only `is-active`, not `nginx -t`.
    - `--force-confold` kept a locally modified site, so no new site was installed.
  - The echoed `[emerg]` line names the real file, which mitigates this.
  - Optional fixes:
    - Run `nginx -t` in the preflight, so a pre-existing break is refused before the transfer.
    - Name the exact one-member restore, `tar -C / -xzf <recovery dir>/preupgrade-config.tar.gz etc/ngfw/nginx.conf`. Without it, nobody should extract the whole archive, which also holds `var/lib/ngfw` and `etc/ngfw/api.env`, over a live host.

### Items 2 to 13 (one line each)

2. Partly addressed:
   - `nginx-test.txt` is kept and echoed, and the docs at `:174-175` say so.
   - The EXIT trap at `:342` still saves `systemctl status` for agent/API only. A failed `systemctl reload nginx.service` therefore leaves no nginx status in the recovery directory. The docs at `:196-197` state this accurately.
   - The remainder is optional.
3. Addressed by documentation:
   - `docs/install/deploy-hosts.md:181-185` documents the runtime-rejection limit: a port held by another process or a sandbox path; check nginx's error log.
   - The worker-PID check was declined with a one-line won't-fix in the combined review. Accepted for a MINOR.
4. Addressed:
   - `${hostname,,} != "${1,,}"` at `:63`.
   - With real `ssh -G`, `Other_Host` and `My_Host` are now refused (V5).
   - See new observation O-1: the stated rationale and the fixture model are wrong in the other direction.
5. Addressed:
   - The flag is limited to exactly one host (`:69`), with a local `WARNING` (`:542`).
   - The docs say so at `:88` and `:219-220`.
   - A two-host fixture covers the refusal (V2).
6. Addressed:
   - The preflight refuses at `:312-315` before `mktemp` and any `scp`, with "nothing was transferred".
   - The fixture asserts that no `scp` ran, that no run directory was created, and that there is no "upgrade failed" banner.
   - The install step rechecks under its lock at `:393-397`.
7. Addressed:
   - The WIP at `:187-191` records hosted run 39029184371, "Run all fixtures without skips": `Ran 124 tests in 150.154s` / `OK`, on the checkpoint-2 tree, against `timeout-minutes: 15`.
   - The checkpoint-3 hosted result is honestly marked pending until merge.
   - I could not re-check it: no network in this round.
8. Addressed:
   - The baseline is read at `:445`, before `nginx -t` at `:452`.
   - The fixture asserts the order restart < `NRestarts` < `nginx -t` < reload < `curl`.
9. Addressed: `docs/install/deploy-hosts.md:96-98` documents the pipe assumption and the `/dev/fd/3: line N` prefix.
10. Addressed:
    - `Match exec` evaluation is documented at `:83-85`.
    - `ssh -G` stderr is no longer discarded (`:62`). See new observation O-2.
11. Addressed: `docs/install/deploy-hosts.md:15-17` names the Ubuntu 26.04 image.
12. Addressed by documentation: `--force-confold` and `/etc/ngfw/nginx.conf.dpkg-dist` are described at `:185-187`. The leftover files are not listed into the recovery directory, which is optional.
13. Addressed: `missing tool on target: $tool` at `:298`.

### New observations from the response commit (do not affect the verify verdict)

**O-1. MINOR [other: R1, R7]. The case-insensitive comparison is correct, but the rationale "as ssh matches Host blocks" is false, and the fixture models the false version.**
- **Where:**
  - `deploy/deploy-hosts.sh:57-58` (comment)
  - `docs/install/deploy-hosts.md:82-83`
  - D-250 ("case-insensitive like ssh")
  - the fake `ssh -G` at `deploy/debian/ngfw/tests/test_deploy_hosts.py:116`, which lowercases the name before the lookup
  - `:801`, where `Lab_Appliance` must deploy as the configured alias
- **What real OpenSSH 10.2p1 does (V5):**
  - It matches `Host` patterns case-sensitively on the typed name.
  - Only the echoed `hostname` of an unmatched name is lowercased.
- **Consequence:**
  - With `Host lab_appliance`, typing `Lab_Appliance` is refused by the shipped check. The `Host lab_appliance` block does not match, so ssh would not use the alias at all; the refusal is fail-closed and correct.
  - But the docs promise the opposite, and the fixture asserts a deploy that real ssh would not produce.
  - The operator gets "underscores are valid only in an SSH configuration alias whose Host block sets HostName" for an alias that exists in a different case. This is confusing, but safe.
- **Fix:**
  - Fake `ssh -G`: look the name up as typed and echo `host.lower()` only when unmatched.
  - Move `Lab_Appliance` to the refused list. Optionally add a `Host Mixed_Alias` block that is accepted only as typed.
  - Reword the docs, the comment and D-250: "an unmatched name is echoed lowercased, so the comparison ignores case; `Host` patterns match case-sensitively, so give the alias exactly as configured".

**O-2. NIT. Every underscore host now prints a spurious OpenSSH notice.**
- The notice is `Pseudo-terminal will not be allocated because stdin is not a terminal.`
- Cause: the call at `:62` passes `</dev/null` and no command, and its stderr is now shown.
- The fake `ssh` does not print this, so the fixtures do not show it.
- **Fix:** add `-T` to the `ssh -G` call. Verified: with `-T`, `ssh -G` writes nothing to stderr (V5).

### Commands run on 083a7ce87 and output

V1. Tree, syntax, shellcheck, help, CI gate files:
```text
$ git rev-parse HEAD                                  -> 083a7ce87f1b692828ff19a87ad9b9e6c3ff78e1
$ git rev-parse a01b58d67^{tree} 083a7ce87^{tree}     -> 9741770f30845b9cba10a5ccf580ef01ab61e046 (both)
$ git merge-base --is-ancestor 3f099b530 083a7ce87    -> main 3f099b530 is ancestor
$ git status --short                                  -> (clean)
$ bash -n deploy/deploy-hosts.sh                      -> bash -n rc=0
$ shellcheck --external-sources deploy/deploy-hosts.sh  (version: 0.11.0)  -> shellcheck rc=0, no findings
$ git grep -n 'shellcheck disable' HEAD -- deploy/deploy-hosts.sh | wc -l  -> 5
$ git diff --stat 3f099b530...HEAD -- .github tools/ci.sh                  -> (empty)
$ deploy/deploy-hosts.sh -h
... --allow-no-default-route upgrades, on exactly one declared host, a
management-only appliance that has no default route. Needs python3 >= 3.11
here and on the appliance. See docs/install/deploy-hosts.md for details.
help rc=0
```

V2. Targeted fixtures:
```text
$ cd deploy/debian/ngfw/tests && time python3 -m unittest -v \
    test_deploy_hosts.DeployHosts.test_nginx_is_validated_and_reloaded_after_install \
    test_deploy_hosts.DeployHosts.test_no_default_route_needs_explicit_flag_and_is_recorded \
    test_deploy_hosts.DeployHosts.test_underscore_hosts_need_a_configured_ssh_alias
test_nginx_is_validated_and_reloaded_after_install ... ok
test_no_default_route_needs_explicit_flag_and_is_recorded ... ok
test_underscore_hosts_need_a_configured_ssh_alias ... ok
Ran 3 tests in 110.542s
OK
real 1m50.779s
```

V3. Name of the archive member. The archive is built like `:419` from a scratch root; restoring the one member:
```text
$ tar -C tarroot -czf preupgrade-config.tar.gz etc/ngfw etc/vpp/startup.conf var/lib/ngfw && tar -tzf preupgrade-config.tar.gz
etc/ngfw/
etc/ngfw/api.env
etc/ngfw/nginx.conf
etc/vpp/startup.conf
var/lib/ngfw/
var/lib/ngfw/secret.key
$ tar -C restore -xzf preupgrade-config.tar.gz etc/ngfw/nginx.conf && find restore -type f
restore/etc/ngfw/nginx.conf
```

V4. The operator-visible output of the `nginx-config` failure. A scratch driver runs the fixture's own `setUpClass`, `setUp` and `deploy(failure='nginx-config')`. The `/tmp/tmp_69kw5v_/host` prefixes are the fixture's path redirection.
```text
rc 1
[12:11:23] [appliance.example] installing; recovery directory: /root/ngfw-deploy/run.r9Uuc9xj
nginx: fixture configuration test
nginx configuration test failed after the upgrade (output in /tmp/tmp_69kw5v_/host/root/ngfw-deploy/run.r9Uuc9xj/nginx-test.txt). The running nginx still serves the previous configuration, but the new site file is on disk and nginx will not start again until it is fixed; agent/API health was not checked. Restore or fix /tmp/tmp_69kw5v_/host/etc/ngfw/nginx.conf (previous copy: member etc/ngfw/nginx.conf of /tmp/tmp_69kw5v_/host/root/ngfw-deploy/run.r9Uuc9xj/preupgrade-config.tar.gz), then run: nginx -t && systemctl reload nginx.service
upgrade failed; preserve /tmp/tmp_69kw5v_/host/root/ngfw-deploy/run.r9Uuc9xj for manual recovery; no automatic rollback attempted
[12:11:28] ERROR: appliance.example: install/health failed; inspect /root/ngfw-deploy/run.r9Uuc9xj on the host
nginx-test.txt: 'nginx: fixture configuration test\n' 0o600
service-status.txt exists: True
```

V5. Real `ssh -G` with a scratch `-F` configuration; no connection is made.
- Setup:
  - OpenSSH_10.2p1 Ubuntu-2ubuntu3.6.
  - The shipped `configured_alias()` and `SSH_OPTS` were sed-extracted from `:27` and `:60-64`. `ssh` was wrapped to add `-F`.
  - The configuration has `Host lab_fw1` (ProxyCommand only) and `Host lab_appliance` / `HostName 192.0.2.7`.
- Shipped function with real `ssh -G`:
```text
lab_appliance: accepted as configured alias
Lab_Appliance: refused
LAB_APPLIANCE: refused
other_host: refused
Other_Host: refused          <- MINOR 4 fixed
My_Host: refused
lab_fw1: refused
(each call also printed: Pseudo-terminal will not be allocated because stdin is not a terminal.)
```
- Plain `ssh -G` with `Host lab_appliance / HostName 192.0.2.7` and `Host Mixed_Alias / HostName 192.0.2.8`:
```text
lab_appliance  -> 192.0.2.7      Lab_Appliance -> lab_appliance   LAB_APPLIANCE -> lab_appliance
Mixed_Alias    -> 192.0.2.8      mixed_alias   -> mixed_alias      (Host matching is case-sensitive; O-1)
stderr of every call: Pseudo-terminal will not be allocated because stdin is not a terminal.
$ ssh -G -T -F ... root@lab_appliance </dev/null 2>&1 >/dev/null   -> (no stderr) rc=0   (O-2 fix)
```

**Verify verdict (R8, 083a7ce87): APPROVE.** MAJOR 1 is resolved. Items 2 to 13 are addressed, items 2, 3 and 12 partly or by documentation, which is acceptable for MINOR/NIT. New observations O-1 (MINOR, [other: R1, R7]) and O-2, N-a and N-b (NITs) are optional and do not block.
