# WIP: firstboot-wants-deps-20261019 — a daemon restart no longer restarts VPP through firstboot

Branch: `claude/trusting-jackson-71c8da` (worktree `/root/NGFW/.claude/worktrees/trusting-jackson-71c8da`),
cut from PR #246's head `9cabd7f47` (`claude/musing-ramanujan-bd8fe0`, D-253), itself cut from
`origin/main` `cb148aafb`. Stacked on PR #246: after #246 merges, `git rebase --onto origin/main 9cabd7f47`.
Envelope: [firstboot-wants-deps-20261019-envelope.md](firstboot-wants-deps-20261019-envelope.md).
Decision: D-255 in `docs/decisions/LOG.md`. Origin: follow-up 1 of
[pkg-no-stop-on-upgrade-20261019-wip.md](pkg-no-stop-on-upgrade-20261019-wip.md) (D-253, PR #246).
PR: https://github.com/mcoder1001-cyber/NGFW/pull/248 (open against `main`, stacked on #246; hosted gates and
independent review pending; not to be merged by this task).

## Checkpoints

Each checkpoint is pushed the moment it is committed (local SHA = remote SHA).

| checkpoint | SHA | content |
|---|---|---|
| 1 | `935ee0e29` | units, script gate, fixture helpers, tests, lab script, docs, D-255, envelope, this file |
| 1b | `072128365` | lab script fixes (per-line states, invocation count), both user-manager observations recorded; the commit the package set was built from |
| 2 | `a99580bad` | whole-suite result, built package set and `dpkg-deb` inspection, PR #248 link |
| 3 | `17eb3556f` | decision renumbered D-254 → D-255 everywhere on this branch (PR #247 already claims D-254 and merges first; manager instruction 2026-10-20); the inherited D-253 line untouched |
| 3b | `cf9ad2e58` | hosted fixture failure traced to #246's `setUpClass` fixtures |
| 4 | `8cf7f6085` | independent review (APPROVE WITH CHANGES, no MAJOR) findings 1–3 and NITs: `pg_isready` gate + test, lab model with the PostgreSQL wrapper/cluster pair and job-settle, README nftables exception, "withheld" wording with the nftables/VPP case, API-requires-agent sentence, prepare-unit comment, user-manager description, enabled closure guarded in test_packaging; PR body patched |
| 4b | the commit carrying this version of this file | this file: checkpoint 4 evidence and the review findings |

## Investigation (2026-10-19)

- **systemd.unit(5)** (freedesktop.org, latest, fetched 2026-10-19): `Requires=` — "with or without
  specifying After=, this unit will be stopped (or restarted) if one of the other units is
  explicitly stopped (or restarted)"; `Wants=` — "if the listed units fail to start or cannot be
  added to the transaction, this has no impact on the validity of the transaction as a whole, and
  this unit will still be started", no stop or restart propagation; `PartOf=` — "limited to
  stopping and restarting of units"; `Requisite=` and `BindsTo=` are "similar to Requires=". So the
  propagating settings are exactly `Requires=`, `Requisite=`, `BindsTo=` and `PartOf=`; the fixture's
  `DEPENDENCY_KEYS` now lists these four (`PartOf=` was missing; no shipped unit uses it).
- **Appliance 37, read-only** (`ssh root@172.30.126.37 systemctl show -p RequiredBy …`, nothing
  changed there): `postgresql.service` and `valkey-server.service` → `RequiredBy=ngfw-firstboot.service`;
  `nftables.service` → `RequiredBy=ngfw-agent.service ngfw-firstboot.service`;
  `ngfw-firstboot.service` → `RequiredBy=ngfw-api.service vpp.service ngfw-agent.service`, all
  `active`; installed `ngfw-meta 0.1.0~dev+git695.cb148aafb288` with the `Requires=` unit text. That is
  the propagation path: `systemctl restart valkey-server.service` → try-restart of firstboot → try-restart
  of VPP, agent and API.
- **Who restarts the daemons.** On this Ubuntu 26.04 host `/var/lib/dpkg/info/valkey-server.postinst`
  runs `deb-systemd-invoke restart 'valkey-server.service'` on upgrade and
  `/var/lib/dpkg/info/nftables.postinst` runs `deb-systemd-invoke try-restart 'nftables.service'`;
  `postgresql-common.postinst` deliberately does not restart ("we don't restart services on install").
  `systemctl restart postgresql.service` is a routine operator command (Debian's `postgresql.service`
  is a `RemainAfterExit` oneshot wrapper, `ConsistsOf=postgresql@<n>-main.service`). So every OS update
  of Valkey or nftables restarted VPP on an appliance, which decided option (b) (keep `Requires=` for
  nftables) against.
- **Second path on A/B images:** `deploy/upgrade/ngfw-upgrade-prepare.service` carries
  `Requires=postgresql.service` and `[Install] RequiredBy=ngfw-api.service ngfw-firstboot.service`;
  `deploy/upgrade/provision` and `ngfw-upgrade` link it into `ngfw-firstboot.service.requires/` and
  `ngfw-api.service.requires/`. With that link a PostgreSQL restart reached VPP through it too. The
  fixture's closure ignored `.requires/` links; `stop_propagation_closure(start, enabled=True)` now
  adds the `[Install] RequiredBy=` edges. Appliance 37 has no such link (`RequiredBy` above).
- **What firstboot needs:** `firstboot.sh` uses PostgreSQL (`psql`, `bootstrap-db.mjs`) and never
  touches Valkey or nftables; Valkey is needed by the API, nftables protects the management NICs before
  anything listens. The daemons are only needed while the first boot runs; afterwards the unit is a
  completed `RemainAfterExit` oneshot that `vpp.service`, agent and API require.
- **daemon-reload:** the generated ngfw-meta postinst has no `daemon-reload` (D-253 fixture output)
  and the Ubuntu 26.04 `systemd` package declares no dpkg trigger on `/usr/lib/systemd/system`
  (`/var/lib/dpkg/info/systemd.triggers`: catalog, binfmt.d, sysctl.d, libc-upgrade). The new unit
  text therefore takes effect at `deploy/deploy-hosts.sh`'s `systemctl daemon-reload` after APT, at
  an operator's daemon-reload, or at reboot; documented in `docs/install/upgrade.md`.
- **Options weighed** (full text in D-255): (a) `Wants=` + `After=` for all three plus an explicit
  per-unit `is-active` gate in `firstboot.sh` (selected); (b) `Wants=` for PostgreSQL/Valkey only;
  (c) `Wants=` without a script gate; (d) `ConditionPathExists=!marker` with `Requires=`; (e) a gate
  target; (f) documentation only.

## Completed code (checkpoint 1)

- `deploy/systemd/ngfw-firstboot.service`: `Wants=postgresql.service valkey-server.service nftables.service`
  (was `Requires=`); `After=`, `Before=`, `[Service]` and `[Install]` unchanged; comment names D-255.
- `deploy/upgrade/ngfw-upgrade-prepare.service`: `Wants=postgresql.service` (was `Requires=`); `After=`,
  `Before=`, the condition and `[Install] RequiredBy=` unchanged.
- `deploy/debian/ngfw/assets/firstboot.sh`: after the completion-marker short-circuit and before the
  bootstrap.env checks, one `systemctl --quiet is-active <unit>` per gated unit, then (checkpoint 4,
  review finding 1) `runuser -u postgres -- pg_isready -q`; an inactive unit exits 1 with `first boot
  requires an active <unit>`, a cluster not accepting connections with `first boot requires a PostgreSQL
  cluster accepting connections` (credentials kept, nothing provisioned). One unit per call because
  `is-active` with several names succeeds when any one is active; pg_isready because Debian's
  `postgresql.service` is an `ExecStart=/bin/true` wrapper that stays active when the
  `postgresql@<version>-main.service` cluster fails (the psql calls would stop there too).
- `deploy/debian/ngfw/tests/maintainer_scripts.py`: section-aware `unit_settings()`/`unit_setting()`
  (empty assignment resets a list, comments skipped), `DEPENDENCY_KEYS` + `PartOf`,
  `stop_propagation_closure(start, enabled=False, overrides=None)` (install-time `RequiredBy=` edges;
  text overrides for negative controls).
- `test_firstboot.py`: fixture `systemctl` answers `--quiet is-active` from `INACTIVE_UNITS` and records
  queries separately, `runuser` calls are recorded; `Firstboot`: each inactive daemon refuses before pg_isready and any
  database or file work (credentials kept, no unit state change), a cluster that does not accept
  connections refuses after the three queries and before the first psql, the three queries come one per
  unit in order and pg_isready precedes the first database command, a provisioned appliance never queries;
  `FirstbootDependencies` (D-255): `Wants=` = `After=` = the three, `Before=` = VPP/agent/API/nginx, no
  propagating key, `WantedBy=multi-user.target`; VPP drop-in, agent and API keep `Requires=`/`After=` on
  firstboot and the marker condition; `closure('vpp.service') == {ngfw-firstboot.service}`,
  `enabled=True` adds `ngfw-upgrade-prepare.service`, PostgreSQL/Valkey reach none of VPP, agent, API,
  agent/API closures keep nftables (negative control: the previous text puts the four daemons back);
  `ngfw-upgrade-prepare.service` wants PostgreSQL and keeps its `RequiredBy=`; the helper sees exactly
  the four propagating keys and section boundaries.
- `test_packaging.py`: `test_upgrade_never_stops_a_unit_vpp_requires` asserts the new closure
  (`{ngfw-firstboot.service}`, `+ ngfw-upgrade-prepare.service` enabled, no daemon) and guards the
  enabled closure against every upgrade path (removal of `ngfw-agent` stops the prepare unit: not vacuous).
- `deploy/debian/ngfw/tests/lab/firstboot-propagation.sh` (new, not run by unittest): renders stand-in
  `ngfwlab-*` units from the shipped unit texts for a systemd manager (`--user` by default, `--system`
  only with `NGFW_LAB_DISPOSABLE=1`), with Debian's PostgreSQL wrapper-plus-cluster pair, boots them,
  restarts Valkey, the PostgreSQL wrapper, the cluster alone and nftables and reports the units whose
  `InvocationID` changed once the job queue is empty (no `sleep`), checks the fail-closed first boot per
  daemon (a failed cluster behind the active wrapper included) and the provisioned reboot;
  `--expect d255|previous`, `--firstboot-unit` for the pre-D-255 text, `--render-only DIR`. Executed
  here in root's user manager in both modes (see below).
- Docs: D-255 (`docs/decisions/LOG.md`), `docs/install/upgrade.md`, `docs/install/deploy-hosts.md` (one
  sentence), `deploy/debian/README.md`, `docs/status/DEFERRED-ACCEPTANCE.md` (laboratory acceptance
  NOT RUN), envelope, this file.

## Actual test results

- `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_firstboot.py`: `Ran 16 tests in 22.069s — OK`
  (16 = 9 on PR #246 + 2 gate tests + 5 dependency tests); after checkpoint 4 (pg_isready test added):
  `Ran 17 tests in 25.208s — OK`, test_packaging `Ran 22 tests in 3.189s — OK`.
- `… -p test_packaging.py`: `Ran 22 tests in 2.803s — OK`. `… -p test_base_policy.py`: `Ran 3 tests — OK`.
- `python3 -m unittest discover -s deploy/debian/ngfw/tests -v` (whole suite, tree of `935ee0e29` plus the
  lab-script and docs edits of `072128365`, no test input changed): `Ran 139 tests in 675.392s — OK`, no
  skips (139 = 132 on PR #246 + 7 new: 2 gate tests + 5 dependency tests).
- Inside the package build (`dh_auto_test`, see below): test_packaging 22 OK, test_system_identity 12 OK,
  test_firstboot 16 OK, test_base_policy 3 OK, test_pppoe_assets 4 OK.
- `firstboot-propagation.sh --render-only`: the rendered `ngfwlab-ngfw-firstboot.service` carries
  `Wants=`/`After=`/`Before=` from the working tree and `Requires=` when given
  `origin/main:deploy/systemd/ngfw-firstboot.service`; `systemd-analyze --user verify` of the rendered
  units exit 0; `shellcheck -S warning` clean. Both full runs: see the observation section.

## Observation with stand-in units in root's user manager on the dev host

Not a disposable instance: the units are throwaway `ngfwlab-*` units under `/run/user/0/systemd/user`
in root's persistent `systemctl --user` manager on the shared dev host; the system manager, VPP and
appliance 37 were not touched, and `systemctl --user list-units 'ngfwlab-*'` is empty afterwards.
`deploy/debian/ngfw/tests/lab/firstboot-propagation.sh --manager user` renders the shipped dependency
lines onto sleep daemons, Debian's PostgreSQL pair (an always-succeeding wrapper oneshot and a cluster
daemon `PartOf=`/`Before=` it; the cluster unit's state stands in for `pg_isready`) and the same
marker short-circuit and per-unit gate as `firstboot.sh`, boots them, restarts the daemons and compares
`InvocationID`s after the manager's job queue is empty. Both runs on this host, 2026-10-20 (checkpoint
4 model), each `41` checks and `0 mismatch(es)`:

- **With the change** (`--expect d255`, the working tree's unit):
  ```
order: start postgresql-cluster,start valkey-server,start ngfw-firewall-bootstrap,start postgresql,start nftables,firstboot run,start vpp,start nginx,start ngfw-agent,start ngfw-api,
OK   restart valkey-server -> restarted units: ngfwlab-valkey-server.service 
OK   restart valkey-server -> firstboot.sh invocations: 0
OK   restart valkey-server -> firstboot.sh provisioning runs: 0
OK   restart postgresql -> restarted units: ngfwlab-postgresql-cluster.service ngfwlab-postgresql.service 
OK   restart postgresql -> firstboot.sh invocations: 0
OK   restart postgresql -> firstboot.sh provisioning runs: 0
OK   restart postgresql-cluster -> restarted units: ngfwlab-postgresql-cluster.service 
OK   restart postgresql-cluster -> firstboot.sh invocations: 0
OK   restart postgresql-cluster -> firstboot.sh provisioning runs: 0
OK   restart nftables -> restarted units: ngfwlab-nftables.service ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service 
OK   restart nftables -> firstboot.sh invocations: 0
OK   restart nftables -> firstboot.sh provisioning runs: 0
OK   postgresql-cluster down -> postgresql wrapper active regardless: active
OK   postgresql-cluster down -> firstboot not active: failed
OK   postgresql-cluster down -> marker absent: absent
OK   postgresql-cluster down -> vpp, agent, api not started: inactive inactive inactive 
OK   valkey-server down -> firstboot not active: failed
OK   valkey-server down -> marker absent: absent
OK   valkey-server down -> vpp, agent, api not started: inactive inactive inactive 
OK   nftables down -> firstboot not active: failed
OK   nftables down -> marker absent: absent
OK   nftables down -> vpp, agent, api not started: inactive inactive inactive 
OK   provisioned boot skips the gate: 1
OK   provisioned boot runs no firstboot.sh: 0
OK   restart valkey-server -> restarted units: ngfwlab-valkey-server.service 
OK   restart valkey-server -> firstboot.sh invocations: 0
OK   restart valkey-server -> firstboot.sh provisioning runs: 0
== 0 mismatch(es) against the d255 expectation
  ```
- **Without the change** (`--expect previous --firstboot-unit <origin/main text>`, `Requires=`):
  ```
order: start ngfw-firewall-bootstrap,start postgresql-cluster,start valkey-server,start postgresql,start nftables,firstboot run,start vpp,start nginx,start ngfw-agent,start ngfw-api,
OK   restart valkey-server -> restarted units: ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-valkey-server.service ngfwlab-vpp.service 
OK   restart valkey-server -> firstboot.sh invocations: 1
OK   restart valkey-server -> firstboot.sh provisioning runs: 0
OK   restart postgresql -> restarted units: ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-postgresql-cluster.service ngfwlab-postgresql.service ngfwlab-vpp.service 
OK   restart postgresql -> firstboot.sh invocations: 1
OK   restart postgresql -> firstboot.sh provisioning runs: 0
OK   restart postgresql-cluster -> restarted units: ngfwlab-postgresql-cluster.service 
OK   restart postgresql-cluster -> firstboot.sh invocations: 0
OK   restart postgresql-cluster -> firstboot.sh provisioning runs: 0
OK   restart nftables -> restarted units: ngfwlab-nftables.service ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-vpp.service 
OK   restart nftables -> firstboot.sh invocations: 1
OK   restart nftables -> firstboot.sh provisioning runs: 0
OK   postgresql-cluster down -> postgresql wrapper active regardless: active
OK   postgresql-cluster down -> firstboot not active: failed
OK   postgresql-cluster down -> marker absent: absent
OK   postgresql-cluster down -> vpp, agent, api not started: inactive inactive inactive 
OK   valkey-server down -> firstboot not active: inactive
OK   valkey-server down -> marker absent: absent
OK   valkey-server down -> vpp, agent, api not started: inactive inactive inactive 
OK   nftables down -> firstboot not active: inactive
OK   nftables down -> marker absent: absent
OK   nftables down -> vpp, agent, api not started: inactive inactive inactive 
OK   provisioned boot skips the gate: 1
OK   provisioned boot runs no firstboot.sh: 0
OK   restart valkey-server -> restarted units: ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-valkey-server.service ngfwlab-vpp.service 
OK   restart valkey-server -> firstboot.sh invocations: 1
OK   restart valkey-server -> firstboot.sh provisioning runs: 0
== 0 mismatch(es) against the previous expectation
  ```

So with `Requires=` an explicit restart of Valkey, PostgreSQL (the wrapper, which `PartOf=` carries to
the cluster) or nftables restarts firstboot (which re-executes `firstboot.sh`, exit 0 on the marker),
VPP, the agent and the API; with `Wants=` only the restarted daemon changes, an nftables restart
reaches the agent and the API through the agent's own `Requires=` and never VPP, and a restart of the
cluster alone (`postgresql@<version>-main.service`, what `pg_ctlcluster` delegates to) reaches nothing
else in either text. The first boot keeps its order and fails closed on each missing daemon; a failed
cluster leaves the wrapper `active` in both texts, so firstboot runs and its PostgreSQL gate refuses
(`failed`: `pg_isready` now, the first `psql` before), while a failed Valkey or nftables cancels
firstboot as a dependency failure with `Requires=` (`inactive`) and is refused by the script with
`Wants=` (`failed`). The provisioned reboot skips the gate. Full logs: `lab-d255.log`,
`lab-previous.log` in the session scratchpad (not committed); rerun the two commands to reproduce.
Laboratory acceptance on real units stays NOT RUN (`docs/status/DEFERRED-ACCEPTANCE.md`).

## Built package set (checkpoint 2)

- Source `072128365` (this branch, full history, count 702); verified VPP output
  `/root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64`
  (`deploy/vpp/verify.sh --require-files … --install-gate` passed inside `prepare.sh`).
- Commands: `pnpm install --frozen-lockfile --prefer-offline` (exit 0), then `GOMAXPROCS=4 tools/heavy.sh
  deploy/debian/ngfw/prepare.sh <VPP> /var/tmp/ngfw-build-firstboot-wants-deps-20261019/pkgsrc` (exit 0) and
  `dpkg-buildpackage -us -uc -b` in `pkgsrc` (exit 0); packages `0.1.0~dev+git702.07212836559c` (agent/api
  `amd64`, meta/web `all`) in `/var/tmp/ngfw-build-firstboot-wants-deps-20261019/` (retained for review; not
  deployed anywhere).
- `dpkg-deb -x`/`-e` of `ngfw-meta` and `ngfw-agent`:
  - `usr/lib/systemd/system/ngfw-firstboot.service`: `Wants=postgresql.service valkey-server.service
    nftables.service`, `After=` the same, `Before=vpp.service ngfw-agent.service ngfw-api.service nginx.service`,
    no `Requires=`; `[Service]`/`[Install]` unchanged;
  - `usr/lib/ngfw/firstboot.sh`: the per-unit `systemctl --quiet is-active` loop with `first boot requires an
    active $UNIT`;
  - `usr/lib/systemd/system/vpp.service.d/vpp-firstboot.conf`: `Requires=ngfw-firstboot.service` +
    `After=` (unchanged);
  - `ngfw-agent`: `usr/lib/systemd/system/ngfw-upgrade-prepare.service` with `Wants=postgresql.service`,
    `After=postgresql.service`;
  - maintainer scripts as D-253 left them: no `preinst`; `ngfw-meta` `prerm` carries one
    `deb-systemd-invoke stop 'ngfw-firewall-bootstrap.service' 'ngfw-firstboot.service'` on `remove` only;
    `postinst` without `deb-systemd-invoke`.

## Review findings (2026-10-20, APPROVE WITH CHANGES, no MAJOR)

1. `is-active postgresql.service` passes with a failed cluster (wrapper unit) → `pg_isready` added to the gate, test `test_fresh_boot_refuses_a_cluster_that_does_not_accept_connections`, acceptance case (f), lab model with the wrapper/cluster pair.
2. README claimed no propagation for nftables → limited to PostgreSQL/Valkey with the nftables exception.
3. "no longer wait" → "still ordered after, no longer withheld", plus the provisioned-appliance nftables/VPP case, in `upgrade.md` and the D-255 row.
NITs: API requires the agent (upgrade.md); prepare-unit comment ("when a snapshot is due"); the observation is root's persistent `--user` manager with throwaway units, not a disposable instance (LOG, DEFERRED, this file); `sleep 1` replaced by waiting for an empty job queue; `test_packaging` guards the enabled closure.

## Remaining

- Hosted packaging-fixture gate: PR #248 inherits PR #246's failure (run 39236809650: the fixture's
  `dh_installdeb` fails with `chown(0, 0, debian/ngfw-agent/DEBIAN/postinst): Operation not permitted` on
  the non-root hosted runner; that fixture is #246's `MAINTAINER.generate()`, which this task's new tests do
  not call: `Firstboot.test_fresh_boot_*` run the script against fixture commands and
  `FirstbootDependencies` only parses unit texts). Confirmed from the job log (job 118007855952, head
  `a99580bad`): the only errors are `ERROR: setUpClass (test_firstboot.FirstbootUpgrade)` and
  `ERROR: setUpClass (test_packaging.MaintainerScripts)`, both `dh_installdeb failed (25)` on that chown;
  `Packaging fixtures: 132 run, 0 failures, 2 errors, 0 skipped` — the 132 include this task's seven new
  tests. Once #246 publishes its fix, rebase this branch onto its
  new head (or onto `main` after #246 merges), push and wait for both hosted checks on #248 (manager
  instruction 2026-10-20).
- Otherwise nothing in code. PR #248 is open (link at the top); hosted gates, independent review and the merge belong
  to the manager; after #246 merges, rebase with `git rebase --onto origin/main 9cabd7f47`. No host is upgraded
  by this task. Laboratory acceptance on real units stays NOT RUN (`docs/status/DEFERRED-ACCEPTANCE.md`).

## Exact next command

```sh
gh pr checks 248   # then: independent review per prompts/REVIEW-PROMPT.md; merge only after #246 and the complete hosted quick gate
```
