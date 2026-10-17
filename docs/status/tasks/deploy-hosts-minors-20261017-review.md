# Combined review: deploy-hosts-minors-20261017 (PR #242)

Reviewed tree: `e1157fa6e` (branch `fix/deploy-hosts-minors-20261017`, stacked on PR #240 head
`d50d4fd2c`; product tree identical to `1a42cb3a0`). Panel per
[review plan](deploy-hosts-minors-20261017-review-plan.md): R1, R2, R7 (every branch) and R8
(`deploy/**`). Each reviewer ran in a fresh read-only agent in this worktree; the reports are saved
verbatim.

| reviewer | verdict on `e1157fa6e` | BLOCKER | MAJOR | MINOR / NIT | report |
|---|---|---|---|---|---|
| R1 correctness & tests | BLOCK | 0 | 1 | 1 / 5 | [review-R1](deploy-hosts-minors-20261017-review-R1.md) |
| R2 security | APPROVE WITH CHANGES | 0 | 0 | 1 / 2 | [review-R2](deploy-hosts-minors-20261017-review-R2.md) |
| R7 docs & evidence | APPROVE WITH CHANGES | 0 | 1 | 7 / 7 | [review-R7](deploy-hosts-minors-20261017-review-R7.md) |
| R8 operability & packaging | APPROVE WITH CHANGES | 0 | 1 | 6 / 6 | [review-R8](deploy-hosts-minors-20261017-review-R8.md) |

**Combined verdict on `e1157fa6e`: BLOCK** (R1 BLOCK: one MAJOR without a debt row). No tech-debt
row was requested; every MAJOR is fixed in checkpoint 3 and R1, R7 and R8 re-check only their
MAJORs in a verify round (R2 raised no MAJOR).

## Disposition of the MAJORs (checkpoint 3)

| finding | fix |
|---|---|
| R1-1 / R7-3 / R8-4: real `ssh -G` lowercases an unconfigured name, so a mixed-case underscore host (`Unconfigured_Host`) passed the alias check and the guide's "refused before any connection" was false | `configured_alias` compares `${hostname,,}` with `${1,,}` (ssh matches `Host` blocks case-insensitively); the fake `ssh -G` lowercases like OpenSSH; fixtures: `Lab_Appliance` deploys as the configured alias, `My_Host` is refused with the others; guide reworded ("a `HostName` other than the alias itself") |
| R7-1: D-250 also added by PR #241 (`8d34740d4`); two D-250 rows on `main` if both merge as they are | resolved by the manager (2026-10-17): the merge order is #240 → #242 → #241, this branch keeps D-250 and PR #241 renumbers its line; the LOG line here is unchanged apart from the wording fixes below |
| R8-1: after a failed `nginx -t` the message "nginx keeps its previous configuration" hid that the broken site file is already on disk (the unit's start-time test fails on the next restart or reboot), that agent/API health was not checked, and gave no recovery step | `nginx -t` output is kept in `nginx-test.txt` in the recovery directory and echoed; the message states the running/on-disk states, that nginx will not start again until fixed, that health was unchecked, and names the previous copy (member `etc/ngfw/nginx.conf` of `preupgrade-config.tar.gz`) and the command `nginx -t && systemctl reload nginx.service`; guide and D-250 updated |

## MINOR / NIT items also fixed in checkpoint 3

R1-2 plain SSH commands' `</dev/null` tested (the fixtures feed the tool a line of operator input;
the fake `ssh` records any stdin a plain command receives) · R1-NIT/R2-M1 `exec 3<&-` tested (every
fake child records an inherited FIFO on descriptor 3; launcher controls with and without the close)
· R2-M1 `LAUNCHER` anchored on the `env -i PATH=…` prefix · R1-NIT the 3.11 threshold is really
compared (the fake interpreter runs the probes with `sys.version_info` spoofed to 3.10) · R1-NIT
`ssh -G` errors shown as is · R1-NIT last deploy in the underscore test starts from a reset host ·
R2-N1/R8-NIT10 `Match exec` evaluation documented · R7-2/R1-other targeted-run evidence rewritten as
what actually happened · R7-4/R8-5 `--allow-no-default-route` accepted with exactly one host, local
warning, two-host fixture · R7-5 NOT RUN appliance acceptance recorded in `DEFERRED-ACCEPTANCE.md`
and the WIP · R7-6 SHAs and push results in the WIP · R7-7 one more command-scoped `SC2029`
annotation disclosed (5, was 4) · R7-8 `docs/install/upgrade.md` carries the `ngfw-web` reload note ·
R7-NIT9/R8-NIT11 Ubuntu 26.04, not Debian 12 · R7-NIT10 D-250 lists the options for item 4 ·
R7-NIT11 verbatim test lines · R7-NIT12/R8 nginx reason reworded (test first as firstboot does; no
"bare reload hides it" claim) · R7-NIT13 "empty standard input (`/dev/null`)" · R8-2
`nginx-test.txt` evidence · R8-3 `systemctl reload` runtime-rejection limit documented · R8-6 the
preflight refuses a host without a default route before anything is transferred (install step
rechecks under its lock) · R8-7 hosted packaging job duration recorded (124 tests in 150 s) ·
R8-NIT8 restart-counter baseline read before the nginx steps · R8-NIT9 `/dev/fd/3: line N` and the
pipe assumption documented · R8-NIT12 `--force-confold` / `.dpkg-dist` documented · R8-NIT13 the
preflight names a missing tool.

## Won't fix (one line each)

- R2-N2 `python3 -I -` for the two heredocs: pre-existing and outside the five items; recorded as a
  follow-up in the WIP.
- R7-NIT14 board row: the manager adds `plan/tasks.yaml` rows (as for the lineage task).
- R7-NIT15 final `<id>.md`: AGENTS.md names `<task>-wip.md` as the maintained status file; the
  lineage task kept the same convention.
- R8-3 worker-PID check after the reload: the limit is documented instead (the fixtures cannot model
  nginx's runtime rejection without a real nginx).

## Verify rounds

Pending on checkpoint 3: R1 (finding 1), R7 (MAJOR 1), R8 (MAJOR 1). Results are appended here.
