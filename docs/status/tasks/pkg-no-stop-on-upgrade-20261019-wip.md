# WIP: pkg-no-stop-on-upgrade-20261019 — a package upgrade no longer restarts VPP

Branch: `claude/musing-ramanujan-bd8fe0` (cut from `origin/main` `cb148aafb`), worktree
`/root/NGFW/.claude/worktrees/musing-ramanujan-bd8fe0`.
Envelope: [pkg-no-stop-on-upgrade-20261019-envelope.md](pkg-no-stop-on-upgrade-20261019-envelope.md).
Decision: D-253 in `docs/decisions/LOG.md`.
Origin: `docs/status/tasks/deploy-host-37-20261019-wip.md` (PR #244), follow-up 3.
PR: https://github.com/mcoder1001-cyber/NGFW/pull/246 (open; hosted gates and independent review pending; not to be merged by this task).

## Checkpoints

Each checkpoint is pushed the moment it is committed (local SHA = remote SHA).

| checkpoint | SHA | content |
|---|---|---|
| 1 | `3da09a6fb` | rules flag, fixture module, tests, CI debhelper step, docs, D-253, envelope, this file |
| 1b | `07887dd71` | the fixture strips `DH_*` from the environment of its nested debhelper calls (inside `dpkg-buildpackage`, `dh` exports `DH_INTERNAL_OPTIONS`/`DH_INTERNAL_OVERRIDE` to override targets); this is the commit the package set below was built from |
| 2 | the commit carrying this version of this file | built-package evidence, whole-suite result, PR link |

## Investigation (2026-10-19)

- **Where `vpp.service` requires firstboot:** the `ngfw-meta` package installs the drop-in
  `assets/vpp-firstboot.conf` to `usr/lib/systemd/system/vpp.service.d/` (`ngfw-meta.install`):
  `Requires=ngfw-firstboot.service` + `After=ngfw-firstboot.service`. There is no `.requires`
  symlink and the postinst's "Register future appliance boot" loop only runs
  `deb-systemd-helper enable` (no dependency). The hardware-37 bind and dataplane-performance
  drop-ins the receipt mentions are host-local and not part of any package in this repository.
  Further `Requires=` links: `ngfw-firstboot.service` → `postgresql.service valkey-server.service
  nftables.service`; the `nftables.service` drop-in `assets/nftables-ngfw.conf` →
  `ngfw-firewall-bootstrap.service`; `ngfw-agent.service` → `vpp.service ngfw-firstboot.service
  nftables.service`; `ngfw-api.service` → `ngfw-agent.service ngfw-firstboot.service`.
- **Which script stops the units, and what it lists.** `debian/rules` ran
  `dh_installsystemd --no-enable --no-start`. With debhelper 13.31 (the build host) that generates,
  for every non-template unit a package ships: a **preinst** `deb-systemd-invoke stop …` guarded by
  `[ "$1" = upgrade ]`, a prerm stop guarded by `[ "$1" = remove ]`, a postinst
  `deb-systemd-helper … update-state` block (no start), and postrm `daemon-reload`/`purge` blocks.
  Extracted with `dpkg-deb -e` from the package set installed on appliance 37
  (`/var/tmp/ngfw-build-main-cb148aafb-20261019/packages`, `0.1.0~dev+git695.cb148aafb288`):
  - `ngfw-meta.preinst`: `stop 'ngfw-firewall-bootstrap.service' 'ngfw-firstboot.service'`
  - `ngfw-agent.preinst`: `stop 'apply-executor.service' 'apply-executor.socket' 'ngfw-agent.service'
    'ngfw-ra-namespace-broker.socket' 'ngfw-ra-openfile.service' 'ngfw-ra-openfile.socket'
    'ngfw-upgrade-health.service' 'ngfw-upgrade-prepare.service'`
  - `ngfw-api.preinst`: `stop 'ngfw-api.service'`
  - `ngfw-web`: no maintainer scripts.
  So the stop the receipt attributes to "`ngfw-meta.prerm`" is the **incoming** package's preinst
  (`new-preinst upgrade old-version`); the installed package's prerm stops only on `remove`.
  Consequence: the first upgrade onto packages built with the fix is already free of the stop.
- **Why VPP restarted:** stopping a `RemainAfterExit=yes` oneshot has no process to stop; systemd
  only flips its state and, per systemd.unit(5) `Requires=` ("this unit will be stopped (or
  restarted) if one of the other units is explicitly stopped (or restarted)"), stops every unit that
  requires it: `vpp.service`, `ngfw-agent.service`, `ngfw-api.service` (via firstboot) and
  `nftables.service` (via firewall-bootstrap; its drop-in empties `ExecStop=` so the ruleset stays
  loaded). `deploy-hosts.sh`'s `systemctl restart ngfw-agent.service ngfw-api.service` then pulled
  the whole `Requires=` chain back up, which is also why `firstboot.sh` ran again (exit 0 on the
  marker) on every upgrade.
- **Second defect found on the way:** the agent preinst also stopped the three sockets
  (`apply-executor.socket`, used by the API's dataplane apply over `/run/ngfw-apply/executor.sock`;
  `ngfw-ra-openfile.socket`; `ngfw-ra-namespace-broker.socket`). `--no-start` means no postinst
  start, and `deploy-hosts.sh` restarts only agent and API, so after every upgrade the sockets stayed
  stopped until a reboot. Not checked on appliance 37 (no host access in this task); recorded as a
  follow-up for the manager.
- **Options weighed** (full text in D-253): the flag for every unit (selected); the flag only for the
  provisioning oneshots through explicit per-package unit lists; `Requires=` → `Wants=` on
  `vpp.service`; `--restart-after-upgrade`.

## Completed code (checkpoint 1)

- `deploy/debian/ngfw/debian/rules`: `dh_installsystemd --no-enable --no-start --no-stop-on-upgrade`
  with a comment stating the contract. Nothing else in the packaging changes: units, drop-ins and
  the hand-written postinst scripts are untouched, so first-boot semantics are unchanged.
- `deploy/debian/ngfw/tests/maintainer_scripts.py` (new, shared fixture): copies `debian/control`,
  `debian/changelog` and the `*.postinst` scripts into a private tree, stages every unit each
  `*.install` line places under `usr/lib/systemd/system/` (from `stage/` when present, else from the
  repository), runs the real `dh_installsystemd` with the arguments parsed from `debian/rules` and
  `dh_installdeb`, and returns the `DEBIAN/*` scripts. `Recorder` executes a script with recording
  fakes of `deb-systemd-invoke`, `deb-systemd-helper` and `systemctl` on `PATH`, with
  `/run/systemd/system` rewritten to a fixture directory so the systemd-only branches run on any
  host. `stop_propagation_closure('vpp.service')` derives, from the shipped units and drop-ins, the
  units whose explicit stop stops VPP.
- `test_packaging.py`: `MaintainerScripts` — four packages generated and `ngfw-web` has no scripts;
  no `deb-systemd-invoke` on any dpkg upgrade path (preinst `upgrade`/`abort-upgrade`, prerm
  `upgrade`/`failed-upgrade`/`deconfigure`, postrm `upgrade`/`abort-upgrade`/`failed-upgrade`) and
  none in the postinst debhelper blocks; `prerm remove` still stops exactly the shipped units
  (positive control); the closure of `vpp.service` ∩ shipped units is exactly
  `{ngfw-firstboot.service, ngfw-firewall-bootstrap.service}` and no upgrade path stops either
  (removal does: not vacuous); the pre-D-253 arguments reproduce the stop (negative control). The
  existing rules assertion now expects the new flag.
- `test_firstboot.py`: `FirstbootUpgrade` — no upgrade path stops, starts or restarts
  `ngfw-firstboot.service` (removal does); the gate stays `Requires=`+`After=` (not `Wants=`), the
  unit stays a `WantedBy=multi-user.target` oneshot with `RemainAfterExit=yes` and
  `Before=vpp.service`, and it is in VPP's stop-propagation closure.
- `.github/workflows/packaging-fixtures.yml`: installs `debhelper` on the runner, checks
  `dh_installsystemd`/`dh_installdeb`, and triggers on `deploy/upgrade/*.service` and
  `deploy/support-bundle/*.service` (the fixture reads those unit files). Without debhelper the new
  classes `SkipTest`, which the strict fixture gate rejects.
- Docs: `docs/install/upgrade.md`, `docs/install/deploy-hosts.md`, `deploy/debian/README.md`,
  `docs/status/DEFERRED-ACCEPTANCE.md` (laboratory acceptance NOT RUN), D-253.

## Actual test results

- `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_packaging.py -v`:
  `Ran 22 tests in 2.847s — OK` (22 = 17 existing + 5 new).
- `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_firstboot.py -v`:
  `Ran 9 tests in 19.775s — OK` (9 = 7 existing + 2 new).
- `python3 -m unittest discover -s deploy/debian/ngfw/tests -v` (whole suite) on `3da09a6fb`:
  `Ran 132 tests in 680.446s — OK`, no skips (132 = 125 on `origin/main` + 7 new); on `07887dd71` (the
  commit the package set was built from; later commits touch only this file):
  `Ran 132 tests in 676.872s — OK`, no skips.
- Inside the package build (`dh_auto_test`, see below): test_packaging 22 OK, test_system_identity
  12 OK, test_firstboot 9 OK, test_base_policy 3 OK, test_pppoe_assets 4 OK.

## Built package set (checkpoint 2)

- Source `07887dd71` (this branch, full history, count 697); verified VPP output
  `/root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64`
  (`deploy/vpp/verify.sh --require-files … --install-gate` passed inside `prepare.sh`, umask 022).
- Commands (as `deploy-hosts.sh -b` runs them): `GOMAXPROCS=4 tools/heavy.sh deploy/debian/ngfw/prepare.sh <VPP>
  /var/tmp/ngfw-build-no-stop-on-upgrade-20261019/pkgsrc`, then `dpkg-buildpackage -us -uc -b` in `pkgsrc`;
  exit 0; packages `0.1.0~dev+git697.07887dd71432` (agent/api `amd64`, meta/web `all`) in
  `/var/tmp/ngfw-build-no-stop-on-upgrade-20261019/` (retained for review; not deployed anywhere).
- `dpkg-deb -e` of the four packages:
  - **no package ships a `preinst`** (before: `ngfw-agent`, `ngfw-api`, `ngfw-meta` each had one with
    `[ "$1" = upgrade ] … deb-systemd-invoke stop <every shipped unit>`);
  - `prerm` of `ngfw-agent`/`ngfw-api`/`ngfw-meta`: only
    `if [ -z "$DPKG_ROOT" ] && [ "$1" = remove ] && [ -d /run/systemd/system ]; then deb-systemd-invoke stop <shipped units>`
    — ngfw-meta: `'ngfw-firewall-bootstrap.service' 'ngfw-firstboot.service'`; ngfw-agent: the eight
    non-template units; ngfw-api: `'ngfw-api.service'`;
  - `postinst`: no `deb-systemd-invoke` anywhere (hand-written part unchanged, then the
    `deb-systemd-helper … update-state` blocks of `--no-enable`);
  - `postrm`: unchanged (`systemctl --system daemon-reload` on `remove`, `deb-systemd-helper purge` on `purge`);
  - `ngfw-web`: still no maintainer scripts (conffiles, control, md5sums).

## Remaining

- Nothing in code. PR #246 is open (link at the top); the hosted gates were pending when this file was
  last updated (`gh pr checks 246`); independent review and the merge belong to the manager; no host is
  upgraded by this task. Laboratory acceptance stays NOT RUN (`docs/status/DEFERRED-ACCEPTANCE.md`).
  The app's PR bar could not be bound to #246 from this session (repository match refused), so CI is read
  with `gh pr checks`, not watched automatically.

## Follow-ups for the manager (not done here)

1. `ngfw-firstboot.service` keeps `Requires=postgresql.service valkey-server.service
   nftables.service`: an explicit restart of PostgreSQL or Valkey (their own package upgrades, an
   operator's `systemctl restart`) propagates through firstboot to `vpp.service`, the agent and the
   API exactly like the stop fixed here. Needs its own decision (probably `Wants=`/`After=` for the
   dependencies firstboot only needs while it runs, or a `ConditionPathExists=` gate) and a
   laboratory check; out of scope for this task.
2. Appliance 37 (and any host upgraded with pre-D-253 packages): check
   `systemctl is-active apply-executor.socket ngfw-ra-openfile.socket ngfw-ra-namespace-broker.socket`
   and start them if inactive.
3. `deploy-hosts.sh` could show `systemctl show -p ActiveEnterTimestamp vpp.service` before and after
   the APT step and fail when it changes, turning "no VPP restart" from a claim into a check.

## Exact next command

```sh
gh pr checks 246   # then: independent review per prompts/REVIEW-PROMPT.md; merge only after the complete hosted quick gate is green
```
