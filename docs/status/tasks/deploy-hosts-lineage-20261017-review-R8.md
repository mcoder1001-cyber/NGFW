# Review R8: deploy-hosts-lineage-20261017 (PR #240)

Reviewer: independent R8 panel agent (read-only session; report saved verbatim by the developer).
Branch `fix/deploy-hosts-lineage-routes-20261017`, diff `99a774734...HEAD` (HEAD `1649dd100`), worktree
`/root/NGFW/.claude/worktrees/recursing-lichterman-91e003`. No network, real service or repository
file was touched.

**Verdict: APPROVE WITH CHANGES.** 1 MAJOR, 7 MINOR, 4 NIT, no BLOCKER. The MAJOR is a docs and
error-message fix to make in this branch. Per the shared rules it becomes BLOCK if it is neither
fixed nor covered by an accepted tech-debt row.

## Findings, ranked

### MAJOR

**1. Routine D-112 path always hits the "divergent" refusal, and the refusal has no review procedure.**
- **Where:** `deploy/deploy-hosts.sh:148`, `:157`; `docs/install/deploy-hosts.md:78-93`; DEC-248 amendment.
- **Failure scenario:**
  - Under D-112 every task branch is squashed into a new commit before merge. An appliance running a pre-merge branch build (the exact case that motivated this task) is therefore never an ancestor of the later `main` build.
  - So every "branch build then main" upgrade is refused with `is not an ancestor ... (use --allow-divergent after review)`. If the branch was deleted, the default-refspec fetch cannot bring the commit back and the message is `is not in $REPO`. GitHub still keeps it as `refs/pull/<n>/head`.
  - Neither message nor the docs say what "review" means. Operators will learn to pass the flag routinely.
  - The flag applies to the whole run and to every host. It also allows a plain development rollback (the new build is an ancestor of the installed one) with `--allow-downgrades`. For that case the message wrongly says "commit counts do not order divergent histories".
- **Fix:**
  - In `check_lineage`, report three cases separately:
    - rollback (`merge-base --is-ancestor new installed`), e.g. "Y is older than installed X; migrations cannot be undone";
    - divergent: print the merge-base and `git rev-list --left-right --count X...Y`;
    - unresolvable: say which of the two commits is missing.
  - Put a concrete review command in the message, e.g. `git -C $REPO diff --stat X Y`.
  - Docs and DEC-248: describe the squash-merge case and fetching `refs/pull/<n>/head` for deleted branches, and recommend `--allow-divergent` for one reviewed host per run.
  - Optional automatic fix: treat the install like a descendant when `git merge-tree --write-tree Y X` equals `Y^{tree}` (the installed changes are already in Y; git ≥ 2.38).

### MINOR

**2. "is not in $REPO" (`:148`) is ambiguous.**
- It does not say which commit is missing.
- It gives the same message for a missing git repository, a shallow clone, or a script copied out of the repo.
- Before this PR, `-p` mode needed no repository. Now every development-to-development upgrade needs a full-history checkout containing both commits, and `docs/install/deploy-hosts.md:36-60` does not say so.
- **Fix:** check `rev-parse --is-inside-work-tree` and `--is-shallow-repository` with their own messages, and document the requirement for `-p`.

**3. The fetch (`:146`) can stall silently.**
- Nothing is logged before the fetch, stderr goes to `/dev/null`, there is no `GIT_TERMINAL_PROMPT=0` and no timeout.
- On a black-holed network a git HTTPS connect can wait for minutes with no output. The remote `run.XXXXXXXX` directory already exists at that point.
- An HTTPS remote without a credential helper prompts on `/dev/tty` despite `2>/dev/null`.
- With the flag and several hosts, the fetch is repeated per host.
- The "fetch from origin failed; using local history only" line itself is clear, but the reason is hidden.
- **Fix:** log "source not in local history; fetching origin" first, run `GIT_TERMINAL_PROMPT=0 timeout 60 git ... fetch`, and log git's last error line on failure.

**4. "agent/API restarted during the stability window" (`:341`) is not actionable.**
- It names no unit and no counts, and the window it reports on also includes the health loop.
- Nothing is saved to the recovery directory.
- If `systemctl show` fails inside `[[ $(restart_counts) == ... ]]`, the output is partial and the same misleading message appears.
- The next preflight on that crash-looping host fails silently at `systemctl is-active` (shown only as "preflight failed"), so the operator cannot redeploy a fixed build with the tool. Docs `:115-134` do not say this.
- **Fix:**
  - Print `unit NRestarts before -> after`.
  - Assign `after=$(restart_counts)` before comparing.
  - On failure, have the trap write `systemctl status --no-pager` and `journalctl -u ngfw-agent -u ngfw-api -n 200` into `$BACKUP`.
  - Document the manual next step.

**5. The stability window has a real bound that the docs overstate.**
- NRestarts semantics are right for this use:
  - It counts only automatic restarts.
  - A manual `systemctl restart` flushes it on the next start; this is from my knowledge of systemd's source, not tested on a live unit. It does not matter here, because the baseline is read after the manual restart.
  - `--value` exists since systemd v230 and NRestarts since v235. Ubuntu 26.04 ships 259; the hosted runner, ubuntu-24.04, ships 255.
- It is not theatre. With `RestartSec=3`, any exit before the final probe shows up either as `activating (auto-restart)` (fails `is-active`) or as a higher NRestarts. So start-up crash loops are caught every time.
- The bound: the window is anchored to `/api/v1/health`, which is "Liveness of the API process (not of VPP)" (`apps/api/src/health/health.controller.ts:21-24`) and says nothing about the agent. A crash later than about health + 5 s escapes, e.g. a panic in the first reconcile after the agent connects to VPP.
- The docs line "a crash-looping daemon that is momentarily active is reported as a failure" overstates this.
- **Fix:** qualify the docs, and/or anchor the window on agent readiness (`test -S /run/ngfw/agent.sock`) or make it longer than agent start-up time plus `RestartSec`.

**6. "management default route changed" (`:342`) is not actionable.**
- It names no address family and shows no before/after; the after-state is not saved.
- An `ip` failure inside `[[ $(stable_routes …) ]]` is reported as a route change (demonstrated below). This existed before, but the line was rewritten here.
- **Fix:** `after4=$(stable_routes 4); after6=$(stable_routes 6)` so errexit catches `ip` failures; on mismatch print both normalised forms and save the raw after-state to `$BACKUP`.

**7. `stable_routes` misses some changes (`:270-283`).**
- Multipath `nexthop` lines are separated from their parent route by `sort`. Two multipath defaults (metric 100 and 200) swapping their nexthops compare equal (demonstrated).
- `weight` and the route type (`blackhole`/`unreachable`/`prohibit`) are dropped.
- Unlikely on a management plane.
- Dropping `src`, `proto`, `expires`, `hoplimit`, `pref`, `mtu`, `onlink` and `nhid` is acceptable: the final fresh SSH connection is the real reachability proof.
- **Handled correctly:** RA routes with changing `expires`/`hoplimit`/`pref`/`mtu`; several RA default routes (sorted); missing metric; `onlink`; `nhid`; `default dev ppp0`; and a host with no IPv6 routes (empty output, exit 0, compares equal).
- `ipv6.disable=1` kernels are untested: if `ip -6` exited non-zero, `:285` would abort after the transfer with only the generic trap message.
- **Fix:** prefix nexthop lines with their route's header line and keep `weight` and the route type.

**8. "mixed installed package versions require manual review" (`:221`) prints no versions, and no recovery is documented.**
- `--allow-divergent` cannot help; the tool cannot repair such a host.
- It is rare: `ngfw-meta` pins exact `(= ${binary:Version})` versions of the other three, so this state needs forced dpkg.
- **Fix:** include the `dpkg-query -W ngfw-agent ngfw-api ngfw-meta ngfw-web` output in the message and add one docs line on manual recovery.

### NIT

**9. Empty run directories are left behind.** The lineage check runs after `REMOTE_PREPARE` has created `/root/ngfw-deploy/run.XXXXXXXX`. Each refusal leaves an empty, unreported run directory. Fix: name it in the message, or `rmdir` it.

**10. The `0.1.0~dev` base version is hard-coded in three places** (`:109`, `:126`, `:258`) and must stay in step with `prepare.sh:94`. If the base version is bumped and there is no `build-info.json`, the lineage check silently stops applying ("release ordering applies"). Fix: add a cross-reference comment, or a fixture asserting that `source_of_version` accepts `prepare.sh`'s format.

**11. Option-parsing messages are confusing.** `-p dir host --allow-divergent` fails with "invalid host"; it should say options must come before hosts. An unknown long option prints usage without naming the option.

**12. Fixture runtime grew.** The successful deploy test takes about 15% longer: base 9.4 s / 9.7 s, head 11.1 s / 11.0 s, measured on this host at load about 8.5. The per-run limit is 30 s. The wip records 324 s for 25 tests, against 45 s for 20 tests on 2026-10-14; that is mostly host load. The hosted packaging job budget is 15 minutes for all suites; R1/R7 should confirm the hosted run time. Gate discovery itself is unchanged.

### [other: R1]
- `--allow-divergent` also authorises a plain development rollback (the new build is an ancestor of the installed one). Confirm this is intended and document it.
- Tiny ordering gap: `is-active` runs before the final counter read. Reading the counters first closes a millisecond-wide window.

## Evidence (commands run in `/root/NGFW/.claude/worktrees/recursing-lichterman-91e003`)

```text
$ shellcheck --version | sed -n 2p      -> version: 0.11.0
$ shellcheck --external-sources deploy/deploy-hosts.sh; echo exit=$?   -> shellcheck exit=0   (no findings)
$ bash -n deploy/deploy-hosts.sh        -> bash -n exit=0
$ bash -n deploy/debian/ngfw/prepare.sh -> bash -n prepare exit=0
# REMOTE_PREPARE / REMOTE_INSTALL heredoc bodies are strings the CI ShellCheck step does not lint;
# extracted (27 / 111 lines, verify_packages stubbed) and checked separately:
$ shellcheck -s bash remote_prepare.sh  -> prepare sc exit=0
$ shellcheck -s bash remote_install.sh  -> install sc exit=0
$ git diff --stat 99a774734...HEAD -- .github tools   -> (empty)
.github/workflows/ci.yml:66   run: shellcheck --external-sources deploy/deploy-hosts.sh   (unchanged)
.github/scripts/packaging-fixtures.py: discover(deploy/debian/ngfw/tests, pattern='test_*.py')  (unchanged;
  workflow paths include deploy/deploy-hosts.sh and deploy/debian/ngfw/**)
$ systemctl --version | head -1          -> systemd 259 (259.5-0ubuntu3.4)
$ systemctl show -p NRestarts --value ngfw-review-nonexistent.service  -> 0, exit 0
$ ip -V -> iproute2-6.19.0 ; ip -6 route show default -> (no output) v6 exit=0
$ ip -4 route show default -> "default via <addr> dev ens192 onlink "   v4 exit=0
```

`stable_routes`, extracted verbatim and run on sample `ip` output through a stubbed `ip`:

```text
v6 RA "…metric 1024 expires 1693sec hoplimit 64 pref medium" / "…expires 1402sec…" / "…expires 1402sec mtu 1500 hoplimit 255 pref high"
   -> all: default via fe80::1 dev ens160 metric 1024            (equal, no false positive)
v4 "…proto dhcp src 192.0.2.10 metric 100" vs "…proto static metric 100" vs "…src 192.0.2.99…"
   -> all: default via 192.0.2.1 dev ens160 metric 100           (src/proto dropped)
v4 onlink, no metric -> default via 192.0.2.1 dev ens160 ; nhid -> kept via/dev/metric ; ppp -> default dev ppp0
two defaults, reordered -> identical sorted output
multipath weight 1/1 vs 10/1 -> identical                          (weight dropped)
two multipath defaults, nexthops swapped between metric 100/200 ->
   default metric 100 | default metric 200 | via 192.0.2.1 dev ens160 | via 198.51.100.1 dev ens192   (identical: false negative)
blackhole default metric 100 == unreachable default metric 100 -> "default metric 100"
empty input -> rc=0, empty string
```

`ip` failure path:

```text
before ok: default via fe80::1 dev ens160 metric 1024
management default route changed (but ip actually failed)     <- [[ $(stable_routes 6) == … ]] with ip exiting 1
now assignment form:
exit=1                                                         <- route6_before=$(…) aborts silently under errexit
```

Single fixture test, run alone:

```text
$ python3 -m unittest -v deploy.debian.ngfw.tests.test_deploy_hosts.DeployHosts.test_unknown_installed_source_is_fetched_once_then_refused_without_override
Ran 1 test in 15.011s
OK
# base vs head copies of test_success_uses_private_backup_and_verified_installs (scratchpad, alternating):
99a774734: Ran 1 test in 9.439s OK | HEAD: Ran 1 test in 11.097s OK | 99a774734: 9.715s OK | HEAD: 10.989s OK
```

Context checked:
- `dh_installsystemd --no-enable --no-start` in `debian/rules`: the maintainer scripts do not restart the units, so the script's own `systemctl restart` is the only restart and the counter baseline after it is correct.
- `ngfw-agent` exits only when `Start` fails; afterwards it blocks until a signal (`apps/agent/internal/agent/agent.go:645-653`).
- `ngfw-agentctl health` exists but is not shipped in `ngfw-agent.install`.

Verdict: APPROVE WITH CHANGES (fix MAJOR 1 in this branch; MINORs optional).
