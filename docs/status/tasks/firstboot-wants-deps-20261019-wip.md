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
| 3 | the commit carrying this version of this file | decision renumbered D-254 → D-255 everywhere on this branch (PR #247 already claims D-254 and merges first; manager instruction 2026-10-20); the inherited D-253 line untouched |

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
  bootstrap.env checks, one `systemctl --quiet is-active <unit>` per gated unit; an inactive one exits 1
  with `first boot requires an active <unit>` (credentials kept, nothing provisioned). One unit per
  call because `is-active` with several names succeeds when any one is active.
- `deploy/debian/ngfw/tests/maintainer_scripts.py`: section-aware `unit_settings()`/`unit_setting()`
  (empty assignment resets a list, comments skipped), `DEPENDENCY_KEYS` + `PartOf`,
  `stop_propagation_closure(start, enabled=False, overrides=None)` (install-time `RequiredBy=` edges;
  text overrides for negative controls).
- `test_firstboot.py`: fixture `systemctl` answers `--quiet is-active` from `INACTIVE_UNITS` and records
  queries separately, `runuser` calls are recorded; `Firstboot`: each inactive daemon refuses before any
  database or file work (credentials kept, no unit state change), the three queries come one per unit in
  order before the first database command, a provisioned appliance never queries;
  `FirstbootDependencies` (D-255): `Wants=` = `After=` = the three, `Before=` = VPP/agent/API/nginx, no
  propagating key, `WantedBy=multi-user.target`; VPP drop-in, agent and API keep `Requires=`/`After=` on
  firstboot and the marker condition; `closure('vpp.service') == {ngfw-firstboot.service}`,
  `enabled=True` adds `ngfw-upgrade-prepare.service`, PostgreSQL/Valkey reach none of VPP, agent, API,
  agent/API closures keep nftables (negative control: the previous text puts the four daemons back);
  `ngfw-upgrade-prepare.service` wants PostgreSQL and keeps its `RequiredBy=`; the helper sees exactly
  the four propagating keys and section boundaries.
- `test_packaging.py`: `test_upgrade_never_stops_a_unit_vpp_requires` asserts the new closure
  (`{ngfw-firstboot.service}`, `+ ngfw-upgrade-prepare.service` enabled, no daemon).
- `deploy/debian/ngfw/tests/lab/firstboot-propagation.sh` (new, not run by unittest): renders stand-in
  `ngfwlab-*` units from the shipped unit texts for a disposable systemd instance (`--user` by default,
  `--system` only with `NGFW_LAB_DISPOSABLE=1`), boots them, restarts Valkey/PostgreSQL/nftables and
  reports the units whose `InvocationID` changed, checks the fail-closed first boot per daemon and the
  provisioned reboot; `--expect d255|previous`, `--firstboot-unit` for the pre-D-255 text,
  `--render-only DIR`. Executed here against root's user manager in both modes (see below).
- Docs: D-255 (`docs/decisions/LOG.md`), `docs/install/upgrade.md`, `docs/install/deploy-hosts.md` (one
  sentence), `deploy/debian/README.md`, `docs/status/DEFERRED-ACCEPTANCE.md` (laboratory acceptance
  NOT RUN), envelope, this file.

## Actual test results

- `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_firstboot.py`: `Ran 16 tests in 22.069s — OK`
  (16 = 9 on PR #246 + 2 gate tests + 5 dependency tests).
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

## Observation in a disposable systemd instance (root's user manager, stand-in units)

The system manager of the shared dev host runs VPP and was not touched; appliance 37 was not
restarted. `deploy/debian/ngfw/tests/lab/firstboot-propagation.sh --manager user` installs
throwaway `ngfwlab-*` units into `/run/user/0/systemd/user` of root's `systemctl --user` manager
(dependency lines copied from the shipped units and drop-ins, names prefixed, sleep daemons, the
firstboot stand-in with the same marker short-circuit and per-unit `is-active` gate as
`firstboot.sh`), boots them, restarts the daemons and compares `InvocationID`s, then removes the
units again (`systemctl --user list-units 'ngfwlab-*'` is empty afterwards). Both runs on this host,
2026-10-19, each `36` checks and `0 mismatch(es)`:

- **With the change** (`--expect d255`, the working tree's unit):
  ```
order: start ngfw-firewall-bootstrap,start postgresql,start valkey-server,start nftables,firstboot run,start vpp,start nginx,start ngfw-agent,start ngfw-api,
OK   restart valkey-server -> restarted units: ngfwlab-valkey-server.service 
OK   restart valkey-server -> firstboot.sh invocations: 0
OK   restart valkey-server -> firstboot.sh provisioning runs: 0
OK   restart postgresql -> restarted units: ngfwlab-postgresql.service 
OK   restart postgresql -> firstboot.sh invocations: 0
OK   restart postgresql -> firstboot.sh provisioning runs: 0
OK   restart nftables -> restarted units: ngfwlab-nftables.service ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service 
OK   restart nftables -> firstboot.sh invocations: 0
OK   restart nftables -> firstboot.sh provisioning runs: 0
OK   postgresql down -> firstboot not active: failed
OK   valkey-server down -> firstboot not active: failed
OK   nftables down -> firstboot not active: failed
OK   provisioned boot skips the gate: 1
OK   provisioned boot runs no firstboot.sh: 0
OK   restart valkey-server -> restarted units: ngfwlab-valkey-server.service 
OK   restart valkey-server -> firstboot.sh invocations: 0
OK   restart valkey-server -> firstboot.sh provisioning runs: 0
== 0 mismatch(es) against the d255 expectation
  ```
- **Without the change** (`--expect previous --firstboot-unit <origin/main text>`, `Requires=`):
  ```
order: start ngfw-firewall-bootstrap,start valkey-server,start postgresql,start nftables,firstboot run,start vpp,start nginx,start ngfw-agent,start ngfw-api,
OK   restart valkey-server -> restarted units: ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-valkey-server.service ngfwlab-vpp.service 
OK   restart valkey-server -> firstboot.sh invocations: 1
OK   restart valkey-server -> firstboot.sh provisioning runs: 0
OK   restart postgresql -> restarted units: ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-postgresql.service ngfwlab-vpp.service 
OK   restart postgresql -> firstboot.sh invocations: 1
OK   restart postgresql -> firstboot.sh provisioning runs: 0
OK   restart nftables -> restarted units: ngfwlab-nftables.service ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-vpp.service 
OK   restart nftables -> firstboot.sh invocations: 1
OK   restart nftables -> firstboot.sh provisioning runs: 0
OK   postgresql down -> firstboot not active: inactive
OK   valkey-server down -> firstboot not active: inactive
OK   nftables down -> firstboot not active: inactive
OK   provisioned boot skips the gate: 1
OK   provisioned boot runs no firstboot.sh: 0
OK   restart valkey-server -> restarted units: ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-valkey-server.service ngfwlab-vpp.service 
OK   restart valkey-server -> firstboot.sh invocations: 1
OK   restart valkey-server -> firstboot.sh provisioning runs: 0
== 0 mismatch(es) against the previous expectation
  ```

So with `Requires=` an explicit restart of Valkey, PostgreSQL or nftables restarts firstboot
(which re-executes `firstboot.sh`, exit 0 on the marker), VPP, the agent and the API; with `Wants=`
only the restarted daemon changes, an nftables restart reaches the agent and the API through the
agent's own `Requires=` and never VPP, the first boot keeps its order and fails closed on each
missing daemon, and the provisioned reboot skips the gate. The `previous` run also shows the
pre-D-255 fail-closed shape (`inactive`: the firstboot job is cancelled as a dependency failure)
against the D-255 shape (`failed`: the script refused). Full logs: `lab-d255.log`,
`lab-previous.log` in the session scratchpad (not committed); rerun the two commands to reproduce.
Laboratory acceptance on an appliance stays NOT RUN (`docs/status/DEFERRED-ACCEPTANCE.md`).

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

## Remaining

- Hosted packaging-fixture gate: PR #248 inherits PR #246's failure (run 39236809650: the fixture's
  `dh_installdeb` fails with `chown(0, 0, debian/ngfw-agent/DEBIAN/postinst): Operation not permitted` on
  the non-root hosted runner; that fixture is #246's `MAINTAINER.generate()`, which this task's new tests do
  not call: `Firstboot.test_fresh_boot_*` run the script against fixture commands and
  `FirstbootDependencies` only parses unit texts). Once #246 publishes its fix, rebase this branch onto its
  new head (or onto `main` after #246 merges), push and wait for both hosted checks on #248 (manager
  instruction 2026-10-20).
- Otherwise nothing in code. PR #248 is open (link at the top); hosted gates, independent review and the merge belong
  to the manager; after #246 merges, rebase with `git rebase --onto origin/main 9cabd7f47`. No host is upgraded
  by this task. Laboratory acceptance on real units stays NOT RUN (`docs/status/DEFERRED-ACCEPTANCE.md`).

## Exact next command

```sh
gh pr checks 248   # then: independent review per prompts/REVIEW-PROMPT.md; merge only after #246 and the complete hosted quick gate
```
