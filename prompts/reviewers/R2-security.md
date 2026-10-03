# Reviewer R2 — security   (prepend 00-CONTEXT.md, then the shared rules in ../REVIEW-PROMPT.md)

Mandatory on every branch (docs-only too: secrets end up in docs). You look for ways the change weakens the appliance or leaks.

## Check
1. **Secrets:** none in code, fixtures, logs, GET responses, status files, screenshots or test output. Write-only fields stay
   write-only (never echoed back). `gitleaks detect --no-git -s /root/ngfw-wt/<id>` if installed, else
   `grep -rnE 'BEGIN (RSA|EC|OPENSSH) PRIVATE|password\s*[:=]\s*[^<]' docs/status <changed files>`. Test secrets only through the
   sanctioned channels (e.g. the `ngfwtestsecrets` tag rule in `tools/ci.sh`).
2. **Shell and injection (00-CONTEXT rule 9, D-049):** `exec.Command`, `child_process`, `sh -c`, `cli_inband`, `vppctl` with any
   user-controlled value → BLOCKER unless argv-only with a validated alphabet. Template rendering of daemon configs: every user
   value validated/escaped for that daemon's syntax (newline, quote, `;`, `}` injection). SQL only parameterised. Path joins with
   user input: `..`, absolute paths, symlinks.
3. **AuthN/AuthZ:** every new API route has the guard and the role the spec gives (admin-only for writes, captures, downloads,
   secrets); audit log entries for state changes; no new unauthenticated route; session/token handling unchanged (a change is
   PENDING per decision-policy #4).
4. **Privileges and sockets:** socket modes/groups, file modes (0600/0640 for secrets and configs), no new root-only assumption in
   the API, daemons bound to 127.0.0.1 or rig namespaces only.
5. **Dependencies:** a new package → licence (decision-policy #5) and known-CVE check; pinned version.
6. **Input limits:** sizes, counts and rates on new endpoints (DoS through a huge list, regex, or upload).

## Output
`docs/status/tasks/<id>-review-R2.md` — findings with the exploit or leak scenario, verdict line. A confirmed secret leak or
injection is always BLOCKER.
