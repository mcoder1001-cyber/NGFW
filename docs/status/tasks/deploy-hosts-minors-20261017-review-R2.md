# Review R2 (security): deploy-hosts-minors-20261017

- Branch: `fix/deploy-hosts-minors-20261017`
- HEAD: `e1157fa6e` (product tree unchanged since `1a42cb3a0`)
- Diff range: `git diff d50d4fd2c...HEAD` (base = `origin/fix/deploy-hosts-lineage-routes-20261017`, PR #240 head)
- Worktree: `/root/NGFW/.claude/worktrees/gracious-payne-469ec3`
- Reviewer: R2 security, independent (did not write this code), read-only. 2026-10-17.
- Inputs read: `prompts/00-CONTEXT.md` (rule 9, D-049), `prompts/REVIEW-PROMPT.md`, `prompts/reviewers/R2-security.md`,
  envelope, WIP, review plan, D-250, `docs/install/deploy-hosts.md`, originating review section 2-الف.4 at `b0cab4260`.

## Result

No BLOCKER, no MAJOR. Rule 9 holds. Every value expanded into a remote command line has a strictly
validated alphabet, and none of them can carry shell syntax, a tilde/brace/glob expansion or an option.
The launcher does what it claims (verified under both `sh -c` and `bash -c`, the two ways a login shell
re-parses it). SSH hardening, `env -i`, the lock and the APT argument set are byte-identical to the base
apart from `nginx` joining the preflight tool list. The change also closes an existing privilege-boundary
weakness: under `bash -s`, `runuser -u ngfw -- pg_dump` (ngfw uid) inherited the root script pipe as stdin.
Any ngfw-uid process could open `/proc/<pg_dump>/fd/0` and consume bytes of the root script while the dump
ran. It now gets `/dev/null`.

One MINOR: the fixtures pin neither of the launcher's two isolation properties (`exec 3<&-`, `env -i`).
Both mutations pass. Two NITs. One out-of-aspect correctness item for R1/R7.

## Findings

### BLOCKER
None.

### MAJOR
None.

### MINOR

**M1. The fixtures do not pin the launcher's isolation guarantees (`exec 3<&-` and `env -i`). Removing either one still passes.**
`deploy/deploy-hosts.sh:286,290,320`; `deploy/debian/ngfw/tests/test_deploy_hosts.py:47,132,161`.
- After this change, fd 3 (`exec 3<&-`) is the only inherited descriptor that still carries the script.
  The new detector drains only stdin (`:161`). The `LAUNCHER` regex (`:47`) starts at `bash -c`. The SSH
  seam strips `env -i ...` (`:132`) and nothing asserts it was there. Mutation evidence is below:
  removing both `exec 3<&-` lines, or dropping the `env -i ...` prefix from `REMOTE_SHELL`, still passes
  `test_remote_children_cannot_consume_the_script` and `test_success_uses_private_backup_and_verified_installs`.
- Scenario: a later edit drops or moves `exec 3<&-` below the first command, for example while reordering the
  script head. Every remote child then inherits a read end of the root script pipe on fd 3. That includes the
  ngfw-uid `pg_dump`. A compromised ngfw-uid process (same uid, dumpable child, PTRACE_MODE_READ not restricted
  by Yama) opens `/proc/<pg_dump>/fd/3` and removes bytes from the root script. Root bash then resumes parsing
  mid-script and skips commands or starts mid-line. This is exactly the class the change closes, and it would
  come back silently. Separately, if `env -i` is dropped, remote bash inherits the sshd/PAM environment
  (`BASH_ENV`, `PATH` from `pam_env` or `~/.ssh/environment`), and no test notices.
- My negative control confirms that fd 3 is load-bearing. Without `exec 3<&-`, a child sees `3 -> pipe:[...]`
  and `cat <&3` reads the next script line (output below).
- Fix:
  - At the top of the fake child recorder, before any `open()`, probe fd 3 (for example `os.fstat(3)` inside
    `try`) and log `inherited fd 3` if it is open. Assert that string is absent in the success and launcher tests.
  - Anchor `LAUNCHER` on the raw recorded prefix
    `env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C HOME=/root bash -c ...`. The recorder logs `args`
    before the seam strips them.

### NIT

**N1. `ssh -G` evaluates the operator's `Match exec` commands, also under `-b`.** `deploy/deploy-hosts.sh:58-65`;
`docs/install/deploy-hosts.md:76-79`. Verified with OpenSSH_10.2p1: a scratch `-F` config with
`Match exec "echo %h %n > file"` ran during `ssh -G root@x_y`. It grants no new privilege. It runs as the same
user from the same config that the real `ssh` connection evaluates anyway, and `%h`/`%n` can only contain
`[A-Za-z0-9._-]`, so nothing can be injected into the exec shell. With `-b` the guide's statement
"`-b` never connects" stays true, but underscore hosts now cause `Match exec` to run. Optional fix: add
"(this evaluates `Host`/`Match` blocks, including `Match exec`)" after "read without connecting".

**N2. (Existed before this branch, outside the diff hunks.) The `python3 -` heredocs are not isolated.**
`deploy/deploy-hosts.sh:78,402`. `python3 -` puts `''` (the current directory) first on `sys.path`. Locally,
`verify_packages` runs in the operator's current directory, so a `json.py`, `hashlib.py`, `subprocess.py` or
`pathlib.py` there runs as the operator (verified below). On the remote the current directory is the
root-only, verified staging directory, so the remote exposure is nil. The new probes (`:73`, `:296`) correctly
use `-I`, and the WIP records declining this as out of scope. Optional follow-up: `python3 -I - ...`.

### [other: R1] (not graded)
**The `configured_alias` check is case-sensitive, but `ssh -G` lowercases unconfigured names**
(`deploy/deploy-hosts.sh:61`, fake at `test_deploy_hosts.py:113-118`). Real `ssh -G` prints
`hostname other_host` for `Other_Host` when no `Host` block matches. That differs from the input, so an
unconfigured mixed-case underscore name is accepted as an alias (evidence below). The fake `ssh -G` echoes the
name unchanged, so the fixture cannot see this. I found no security impact. The alphabet still holds, no
option, URI, port, user or `%` token can be injected, and `BatchMode` plus `StrictHostKeyChecking=yes` still
require a known host key. The usage header (`:7-8`), the guide (`:76-79`) and D-250 overstate the check
([other: R7]). A possible fix: compare `${hostname,,}` with `${1,,}`, and lowercase in the fake.

## Security checks performed (no finding)

1. **The `REMOTE_SHELL` literal (`:286`).** It is a fixed string. The local definition yields exactly
   `env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C HOME=/root bash -c 'exec 3<&0 </dev/null && exec bash /dev/fd/3 "$@"' ngfw-deploy`.
   The inner program is single-quoted for the login shell, and `"$@"` passes the positional words through
   unchanged. `bash` is resolved through the clean `PATH` (env execvp). The preflight (`:477`) appends nothing.
2. **The six positional arguments (`:504` → `:321`).**

   | arg | local validation | remote recheck | shell-safe |
   |---|---|---|---|
   | `$dir` | `backup =~ ^/root/ngfw-deploy/run\.[A-Za-z0-9]{8}$` (`:482`) + `/packages` | `-d`, `! -L $DIR`, `! -L $BACKUP` (`:323`) | yes |
   | `$digest` | `^[a-f0-9]{64}$` (`:494`) | equals `sha256sum SHA256SUMS` (`:336,417`) | yes |
   | `$VERSION` | `re.fullmatch('[0-9][A-Za-z0-9.+:~\-]*')` + `dpkg --validate-version` (`:113-115`) | equals remote `verify_packages` (`:337,418`) | yes: starts with a digit and has no `=`, so no tilde expansion even after `:` (tested `0:~root`, `1:~`); no `{},*?[` |
   | `$OVERRIDE_USED` | own 0/1 literal (`:148,473`, set in-shell by `check_lineage`) | `== [01]` (`:323`) | yes |
   | `$installed` | `<=128` and `^[0-9][A-Za-z0-9.+:~-]*$`, single line (`:478-481`) | same regex (`:324`) and per-package equality (`:348`) | yes (hostile strings rejected, below) |
   | `$ALLOW_NO_DEFAULT_ROUTE` | reset to 0 at `:25` (the environment cannot preset it), 1 only from getopts | `== [01]` (`:323`); `$6` unset fails under `set -u` | yes |

   All five `SC2029` disables (`:476,485,491,503,506`) are justified. Each annotated line expands only a constant
   or one of the values above. ShellCheck 0.11.0 is clean.
3. **Host alphabet (`:64-65`).** The first character must be alphanumeric, and the ssh word is always
   `$SSH_USER@$host` with `SSH_USER =~ ^[a-z_]...` (`:54`). The word therefore never starts with `-`, so it can
   never become an option. `/ : % @ , { }` are excluded, which rules out `ssh://` URIs, ports, `%` tokens, a user
   override and brace expansion. `..` is refused. The host is used only as the ssh/scp destination and in log
   text, never inside a remote command. `ssh -G` gets `</dev/null 2>/dev/null` and its output is only compared.
4. **The `/dev/fd/3` script read.** I ran the launcher under `sh -c` and `bash -c`. Each remote child has
   `0 -> /dev/null` and no pipe descriptor. Bash's script fd (255) is close-on-exec and is read one byte at a
   time (non-seekable, buffer size 1). `exec 3<&-` is the third statement of both scripts. Before it run only
   `set`/`umask` and, in the install script, the `verify_packages` definition, so no child exists while fd 3 is
   open. The plain commands (`:486,492,507`) now get `</dev/null`, so they no longer consume local stdin
   inside the host loop.
5. **`nginx -t -q` and `systemctl reload nginx.service` as root (`:427-428`).** Both have fixed argv, and
   `nginx` resolves through the clean `PATH` (preflight `command -v`, `:293`). The configuration comes from the
   checksum-verified package, and the master parses the same configuration on reload. Under `umask 077`,
   any file `nginx -t` creates is 0600, which is stricter, not weaker. Error text goes only to the operator's
   SSH stderr, and nginx messages carry paths, not key material.
6. **Python probes.** Both use `-I` (`:73`, `:296`). The remote `$(python3 --version 2>&1)` is data inside a
   double-quoted `echo` with no re-parse, and it goes to stderr only.
7. **`no-default-route.txt` (`:387`).** It holds fixed text in a fresh root-only `mktemp -d` directory and is
   mode 0600 under `umask 077`. The fixture asserts `0o600`. It contains no secret.
8. **Nothing weakened.** `SSH_OPTS` (`BatchMode=yes`, `StrictHostKeyChecking=yes`, `ControlMaster=no`,
   `ControlPath=none`), `apt_args`, the `upgrade.lock` handling and `flock -n 9` are identical to the base.
   `env -i` remains, now as one literal used for both calls.
9. **Secrets.** `gitleaks` on the changed files found no leaks. The whole tree has eight `generic-api-key`
   hits, all in pre-existing Go/TS test files outside this diff. The prompt grep found nothing. The
   envelope, WIP and review plan contain no environment values, connection strings or credentials.
10. Out of R2 scope by design: no new route, authN/authZ, socket, dependency or input-size surface.

## Commands run (in the worktree; scratch files under the session scratchpad only)

```text
$ git rev-parse HEAD
e1157fa6eb0c80fa0e2bae0a6b49ef3b476c2196
$ bash -n deploy/deploy-hosts.sh; echo rc=$?
bash -n rc=0
$ shellcheck --version | head -2; shellcheck --external-sources deploy/deploy-hosts.sh; echo rc=$?
ShellCheck - shell script analysis tool
version: 0.11.0
shellcheck rc=0

$ gitleaks version; gitleaks detect --no-git -s <worktree> --no-banner -r <scratch>/gitleaks-tree.json
8.30.1
INF scanned ~87089568 bytes (87.09 MB) in 11.2s
WRN leaks found: 8      (rc=1)
generic-api-key apps/agent/internal/renderers/rfkit/fixround_test.go 20
generic-api-key apps/agent/internal/renderers/rsyslog/rsyslog_test.go 31
generic-api-key apps/agent/internal/renderers/strongswan/apply_test.go 375
generic-api-key apps/agent/internal/scheduler/validator_test.go 371
generic-api-key apps/agent/internal/scheduler/validator_test.go 372
generic-api-key apps/cli/internal/api/transport_test.go 82
generic-api-key packages/schema/src/domains/vpn.test.ts 197
generic-api-key packages/schema/src/domains/vpn.test.ts 265
$ gitleaks detect --no-git -s <scratch copy of the 7 changed files> --no-banner
INF scanned ~287381 bytes (287.38 KB) in 851ms
INF no leaks found      (rc=0)
$ grep -rnE 'BEGIN (RSA|EC|OPENSSH) PRIVATE|password\s*[:=]\s*[^<]' docs/status/tasks/deploy-hosts-minors-20261017-*.md <changed files>
(no output, rc=1)

$ # launcher under both login shells; script prints argv and the fds of a child
$ printf '%s\n' "$script" | sh -c "$REMOTE_SHELL /root/ngfw-deploy/run.AbCd1234/packages <64 x a> 1:0.1.0~dev+git42.aaaaaaaaaaaa 0 0.0.1 1"
argc=6 args=[/root/ngfw-deploy/run.AbCd1234/packages aaaa...aaaa 1:0.1.0~dev+git42.aaaaaaaaaaaa 0 0.0.1 1]
child fds:
0 -> /dev/null
1 -> pipe:[2764342]
2 -> <terminal capture file>
3 -> /proc/1861936/fd          (ls's own directory handle)
after child cat (stdin drained?)
LAST_LINE
(bash -c: identical)

$ # negative control: the same launcher, script WITHOUT exec 3<&-
0 -> /dev/null
1 -> pipe:[2774596]
2 -> <terminal capture file>
3 -> pipe:[2793522]
4 -> /proc/1867803/fd
echo LAST_LINE_SHOULD_BE_EATEN      (cat <&3 in a child read the next script line; it never ran)

$ # tilde/epoch words through sh -c and bash -c
[0:~root][1:~][0~root][9:0.1.0~dev+git1.abcdefabcdef][0.0.1-1~bpo+1]   (sh)
[0:~root][1:~][0~root][9:0.1.0~dev+git1.abcdefabcdef][0.0.1-1~bpo+1]   (bash)
$ # installed-version regex (LC_ALL=C) against hostile strings
ACCEPT 0:\~root
reject $'0.1\nid'   reject 0\;id   reject 0\$\(id\)   reject 0\ 1   reject 0\`id\`
reject 0\*   reject 0\?   reject 0\[a\]   reject 0=\~x

$ ssh -V; ssh -G -F <scratch cfg> -o BatchMode=yes root@<h> </dev/null | sed -n 's/^hostname //p'
OpenSSH_10.2p1 Ubuntu-2ubuntu3.6, OpenSSL 3.5.5 27 Jan 2026
lab_appliance -> 192.0.2.7
Lab_Appliance -> lab_appliance
other_host -> other_host
Other_Host -> other_host          ([other: R1]: differs from input, so it is accepted as an alias)
pct_x -> pct_x.example
$ # Match exec under ssh -G (scratch cfg: Match exec "echo %h %n > <scratch>/ran")
matched.example
ran file:
x_y x_y

$ # mutation 1 (scratch copy only): both "exec 3<&-" lines removed
290d289 < exec 3<&-
320d318 < exec 3<&-
test_success_uses_private_backup_and_verified_installs ... ok
Ran 2 tests in 23.586s
OK
$ # mutation 2 (scratch copy only): REMOTE_SHELL="bash -c '...' ngfw-deploy" (env -i prefix dropped)
Ran 2 tests in 24.106s
OK

$ cd deploy/debian/ngfw/tests && PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v \
    test_deploy_hosts.DeployHosts.test_remote_children_cannot_consume_the_script \
    test_deploy_hosts.DeployHosts.test_underscore_hosts_need_a_configured_ssh_alias \
    test_deploy_hosts.DeployHosts.test_unsafe_arguments_fail_before_ssh \
    test_deploy_hosts.DeployHosts.test_hostile_preflight_output_is_rejected_before_git_or_transfer \
    test_deploy_hosts.DeployHosts.test_no_default_route_needs_explicit_flag_and_is_recorded
... ok (x5)
Ran 5 tests in 91.354s
OK
$ git status --short          (clean before this report was written)

$ # N2: python3 - in a directory holding json.py
SHADOWED json imported from cwd
sys.path[0] = ''
$ # same with python3 -I -
sys.path[0] = '/usr/lib/python314.zip'
```

The full suite is R1's job and I did not run it; the WIP records 33/33 OK on `1a42cb3a0`.

## Verdict

Counts: BLOCKER 0, MAJOR 0, MINOR 1 (M1), NIT 2 (N1, N2); [other: R1] 1 (with an [other: R7] note).

**APPROVE WITH CHANGES.** Nothing blocks the merge. The requested change is M1 (pin `exec 3<&-` and `env -i`
in the fixtures). It is a MINOR, so under the shared rules the developer may fix it or answer with a one-line
won't-fix. R2 needs no verify round unless the launcher itself changes.
