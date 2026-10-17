# Review R2 (security): deploy-hosts-lineage-20261017, PR #240

Reviewer: independent R2 panel agent (read-only session; report saved verbatim by the developer).
Diff reviewed: `git diff 99a774734...HEAD` (HEAD 1649dd10095b). Paths below are under `/root/NGFW/.claude/worktrees/recursing-lichterman-91e003`.

## Findings, by severity

**1. MAJOR: `--allow-divergent` also allows plain rollbacks within the same history, on every host in the run.**
- **Where:** `deploy/deploy-hosts.sh:153-158` (local) and `:263-266` (remote).
- **What happens:** `check_lineage` only tests `merge-base --is-ancestor installed new`. When that is false and the flag is set, it continues. It does not check whether the new build is an ancestor of the installed one, which is an ordinary downgrade and not a divergent history. The remote then sees two development versions and passes `--allow-downgrades`.
- **Before this PR:** the remote refused every downgrade. DEC-248 says a downgrade cannot undo database migrations.
- **Scenario:** `-p olddir --allow-divergent hostA hostB`. hostA runs a stale branch, which is why the operator passes the flag. hostB already runs a newer `main`. hostB is silently rolled back past forward-only migrations. The remote even reports it as "divergent development build accepted".
- **Fix:**
  - In `check_lineage`, also run `git merge-base --is-ancestor "$new_commit" "$installed_commit"`. If it is true and the commits differ, die even with the flag ("downgrade within one history").
  - Tie the override to a reviewed installed source, e.g. `--allow-divergent=<12hex>` honoured only when it matches. Alternatively, refuse the flag with more than one host.
  - Update `docs/install/deploy-hosts.md` and DEC-248 to match.

**2. MINOR: the lineage check uses the version read during preflight, before the lock.**
- **Where:** preflight `deploy/deploy-hosts.sh:217-227`, used at `:351-357`; install-time check at `:260-267`.
- **What happens:** the lock is only taken at install time, so concurrent operators are expected. If another run installs a different build between preflight and lock, the install step applies only the version comparison, never the lineage check.
- **Scenario:** an unrelated git41.b build is installed over an ancestor in that window. This run then puts git42.a over it without any lineage check, which is the bug this PR fixes.
- **Fix:** after validating the preflight version, pass it (or its 12-hex source) as a 5th argument to the install script. Under the lock, refuse if any package's installed version differs. It uses the same character set as `VERSION`; still validate it on the remote, like `$4`.

**3. MINOR: the new `git fetch --quiet origin` (`deploy/deploy-hosts.sh:146`) is not non-interactive and has side effects.**
- **New network traffic:** `-p` mode used to contact only the hosts. Now a host can make the operator's machine fetch from origin just by reporting an unknown 12-hex version. The URL comes from local config, so this cannot leak data to an attacker. It does cause an authenticated request.
- **Credential prompts:** origin here is HTTPS with no `credential.helper`. Git prompts for credentials on `/dev/tty`, which `2>/dev/null` does not suppress. That breaks the batch-mode stance the docs state for SSH and can hang an unattended run with no timeout.
- **Tags:** the fetch follows tags automatically (`tagOpt` is unset), which feeds finding 4.
- **FETCH_HEAD:** the fetch rewrites the checkout's `FETCH_HEAD`, which `build_packages` reads at `:167-169`. A concurrent build from the same checkout could build the wrong commit.
- **Repetition:** it runs once per host, not "once" as the docs say.
- **Fix:** run `GIT_TERMINAL_PROMPT=0 GIT_SSH_COMMAND='ssh -o BatchMode=yes' git -C "$REPO" fetch --quiet --no-tags --no-write-fetch-head origin`, at most once per run. git 2.53 here supports both flags.

**4. MINOR: a branch or tag named with the 12 hex characters shadows the commit lookup.**
- **Where:** `deploy/deploy-hosts.sh:128-132`.
- **What happens:** `git rev-parse --verify "<12hex>^{commit}"` prefers a branch or tag of that name over the abbreviated commit ID. Anyone who can push a tag to origin can make an unrelated installed build look like an ancestor.
- **Fix:** add `[[ $commit == "$1"* ]] || return 1` in `resolve_commit`, plus `--no-tags` from finding 3.

**5. MINOR: no failing-path fixture for the new remote-controlled input.**
- **Where:** `deploy/debian/ngfw/tests/test_deploy_hosts.py`. A grep for the error messages `invalid installed package version` and `unexpected preflight output` finds no test.
- **What is missing:** `test_override_never_accepts_release_downgrade_or_shell_syntax` only checks the script's own `0`/`1` value. Nothing tests a hostile version string from a host.
- **Code status:** my probe shows the code is correct. The gap is regression protection only.
- **Fix:** add a failure mode where the fake `dpkg-query` prints `1.0;id`, `1.0 --x`, a CR, an empty first line, and an extra stdout line. Assert those two error messages, and that neither `scp` nor `git` appears in the log.

**6. NIT: the new `python3 -c` at `deploy/deploy-hosts.sh:142` runs without `-I`.**
- **What happens:** the current directory is on `sys.path`, so a `json.py` in the operator's working directory would be imported. `verify_packages` (`python3 -`) has the same existing pattern.
- **Fix:** use `python3 -I -c`.

**7. NIT: the preflight output is captured with no size limit.**
- **Where:** `deploy/deploy-hosts.sh:351-355`. The version check has no length limit.
- **What happens:** a hostile host can exhaust local memory or flood the terminal through the log line at `:138`. This was already true for the staging path.
- **Fix:** add `[[ ${#installed} -le 128 ]]`, or cap the stream with `| head -c 4096`.

## Out of my area

- **[other: R1/R4]** `stable_routes` (`deploy/deploy-hosts.sh:270-283`) hides some real route changes. Probe output:
  - `unreachable default dev lo metric 100` becomes `[default dev lo metric 100]`, identical to a normal route.
  - `default via inet6 fe80::1 dev eth0` and `default via inet6 fe80::99 dev eth0` both become `[default via inet6 dev eth0]`, so the gateway is lost.
  - Two routes with different `src 192.0.2.10` / `src 198.51.100.7` both become `[default via 192.0.2.1 dev eth0 metric 100]`.
  - Fix: keep the route type and `src`, and take the address after `via inet6`.
- **[other: R8]** On refusal, `check_lineage` runs after preflight has already created `/root/ngfw-deploy/run.XXXXXXXX`. Each refused attempt leaves an empty staging directory behind.
- **[other: R8]** In `-p` mode from a directory that is not a git checkout, every dev-to-dev upgrade needs the override.
- **[other: R7]** `deploy-hosts-lineage-20261017-wip.md` names checkpoint 455023193; HEAD is 1649dd100.

## Answers to the review questions

- **Can host output steer local commands?** No, only through `source_of_version` (`[a-f0-9]{12}`) into git argv. That value cannot start with `-`, and git's output is re-checked as 40 hex. The probe below rejects CR/LF, NUL, spaces, `;`, ESC, UTF-8, a leading dash, an empty first line and line counts other than 2.
- **Is host output reused in another remote command?** No. Only the existing, strictly checked `$backup`/`$dir` path is. `installed` never reaches an ssh command.
- **Is the 0/1 value the script's own?** Yes. It is set unconditionally at `:19` (overriding any environment variable) and set to `1` only at `:38`. The remote re-checks `== [01]` at `:241`.
- **Does `--allow-downgrades` reach beyond the four packages?** No. The plan filter at `:299-306` rejects any `Inst`/`Conf` of another package and any `Remv`. `--no-remove` and `--no-download` are unchanged. The simulation and the real install get identical arguments, and a test asserts this. It does widen what can happen to the four packages: see finding 1.
- **Do the fixtures touch the real repo or network?** No. The fake `git` comes first on PATH, raises on any unexpected call, and treats `fetch` as a no-op. The script calls plain `git`. Fixture addresses are only 192.0.2.0/24 and fe80::.
- **Any secrets or host details in log or doc text?** None in the changed files.

## Evidence (commands run in the worktree)

```text
$ shellcheck --version | head -2; shellcheck --external-sources deploy/deploy-hosts.sh; echo "shellcheck exit=$?"
ShellCheck - shell script analysis tool
version: 0.11.0
shellcheck exit=0
$ bash -n deploy/deploy-hosts.sh -> exit=0 ; bash -n deploy/debian/ngfw/prepare.sh -> exit=0
$ git --version -> git version 2.53.0
$ grep -n ALLOW_DIVERGENT deploy/deploy-hosts.sh -> assignments only at 19 (=0) and 38 (=1); remote 239/241/264
```

Secret scan: `gitleaks detect --no-git --redact -s <file>` returned "no leaks found" for all seven changed files. The whole-tree scan found 8 hits, all in existing unrelated test files (strongswan, rfkit, rsyslog, scheduler, cli transport, schema `vpn.test.ts`). The grep pattern from R2.md found nothing in the changed files.

Preflight parsing probe (a copy of `:351-356`, scratchpad `parse.sh`):

```text
ok: ACCEPT installed=0.1.0~dev+git42.aaaaaaaaaaaa backup=/root/ngfw-deploy/run.ABCDEFGH
crlf: REJECT version      cr-only-v: REJECT version    empty: REJECT lines
one-line: REJECT lines    empty-v: REJECT version      three: REJECT lines
dash/space/semi/esc/utf8: REJECT version               nul (byte dropped): REJECT version
pathnl: REJECT path       tilde '1:~root': ACCEPT (only logged, never in a command)
```

Ref shadowing demo (throwaway repo in the scratchpad, no network):

```text
before tag: 1e06a78bd6b7e21160f2848875d3f7af6b69d709
after tag named 1e06a78bd6b7 -> two: d7cb225df6031b0c44a9ce6d3bbf1952d102fac9
```

Fetch config:

```text
origin = https://github.com/mcoder1001-cyber/NGFW.git
no credential.helper
no tagOpt (default auto-follow tags)
FETCH_HEAD path: /root/NGFW/.git/worktrees/recursing-lichterman-91e003/FETCH_HEAD
```

Single fixtures (the full suite is R1's):

```text
$ python3 -m unittest -v ...DeployHosts.test_unsafe_arguments_fail_before_ssh ...DeployHosts.test_override_never_accepts_release_downgrade_or_shell_syntax
Ran 2 tests in 20.422s
OK
```

**Verdict: BLOCK** (finding 1 is a MAJOR with no accepted debt row). Re-check only finding 1 once it is fixed.
