# F-startup-apply — robust startup.conf apply tooling

Branch `task/F-startup-apply` (base main@7f5caa3 merged). Fixes re-review N1, N2, N3, N6 (= re-review N5: env/pinning
through systemd-run), N8 handover gate, plus re-review N6 (D-084 plugin omission), N9, N10, N11 and the script side of N4.
Nothing was ever run with `--apply` against real paths; VPP was not restarted; no NIC bound/unbound; nothing written under /etc.

## What was built

| finding | fix |
|---|---|
| N1 hung VPP | every VPP/kernel/systemd call through `timeout` (`--cmd-timeout` 10 s, `--svc-timeout` 120 s; lock fds 8/9 closed in the child); `vrx-vppcheck` has a context deadline + hard `os.Exit` backstop; the run has a time budget; the dead-man **kills the run** (process group / `systemctl kill --kill-whom=all` on its unit, verified by cmdline), takes the locks with a bounded wait (`--deadman-lock-timeout` 60 s) and rolls back **even when they stay busy** (FORCED, logged). An incomplete rollback leaves the dead-man armed to retry. `systemctl stop vpp` hanging → SIGKILL via `systemctl kill` |
| N2 mgmt restore | per management interface (all v4/v6 default routes, the route to `$SSH_CONNECTION`'s client / `--mgmt-peer`, `--mgmt-if`): `ip -j addr` + `ip -j -4/-6 route show table main dev X` snapshot → exact `ip addr replace …` / `ip -4|-6 route replace …` plan (link-local and kernel routes skipped, tokens validated). After rebind: link up, plan re-applied verbatim; only if still broken the detected manager: ifupdown `ifup --force`, systemd-networkd `networkctl reconfigure`, netplan `netplan apply`; then the plan again. Manager detection: ifupdown stanza for the interface → netplan yaml + binary → networkd active → none |
| N3 checker | `apps/agent/cmd/vrx-vppcheck` (version / plugins by `show plugins` content via cli_inband / ifaces via `sw_interface_dump` name filter + exact match), agent's `internal/vpp` client + generated binapi; preflight `ifaces local0` in dry run and inside the lock before anything is installed (refuse 3). `vpp-iface-check.py` and its `.pyc` deleted |
| N5/N6 env + pinning | document, generator, vrx-vppcheck and the script copied into `<work>/bin`; every setting recorded in `<work>/settings` (authoritative) **and** passed as `systemd-run --setenv` (+ PATH); the stage warns if its environment differs. Dry run prints the rendered sha256; `--apply` requires `--expect-new-sha256`, compared inside the lock |
| N8 gate | `--apply` refused (exit 3) unless the canonical `/root/ngfw/docs/lab/host-<vm>.md` **and** the script's tree say `handover: done` (tools/lab rule; `VRX_HANDOVER_EXTRA` can only add sources), or `--i-have-product-owner-approval <PENDING-slug|D-nnn>` — recorded in the run log, `<work>/gate` and syslog |
| re-review N6 | a plugin the old file enabled and the new file does not mention may vanish (D-084 omission) |
| N9 / N11 | setsid dead-man started with `8>&- 9>&-` and killed on commit; `systemctl reset-failed vpp` before start |
| N4 (script side) | every protected interface watched, tap without a device skipped. Generator side of N4/N7/N12 is not in this task's files |

Docs: `docs/agent/renderers/vppstartup.md` procedure section rewritten for the script.

## Verification

### tools/ci.sh --base main
```
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   1m27s
  apps/agent: make lint test build                   0m50s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 3m51s · logs /root/ngfw-wt/logs/ci/F-startup-apply-20260924-034437-2539875

CI GATE PASSED
```

### shellcheck deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh
```
(no output) exit 0
```

### go test -v ./cmd/vrx-vppcheck
```
--- PASS: TestVersion (0.00s)
--- PASS: TestPluginsByContent (0.00s)
--- PASS: TestIfacesExactMatch (0.00s)
--- PASS: TestDisconnectedIsExit2 (0.00s)
--- PASS: TestUsage (0.00s)
--- PASS: TestNoSocket (0.00s)
    main_test.go:154: hung VPP: exit 2 after 700ms: vrx-vppcheck: cannot reach VPP at /tmp/TestHungVPPTimesOut542581545/001/api.sock: waiting for VPP at /tmp/TestHungVPPTimesOut542581545/001/api.sock: context deadline exceeded
--- PASS: TestHungVPPTimesOut (0.70s)
PASS
ok  	ngfw/agent/cmd/vrx-vppcheck	0.731s
```

### vrx-vppcheck against the real VPP on vrx-a (read-only: show_version, cli_inband "show plugins", sw_interface_dump)
```
$ vrx-vppcheck version
vpp 26.06-release
exit=0
$ vrx-vppcheck ifaces local0
present: local0
exit=0
$ vrx-vppcheck ifaces local0 wan
missing: wan
exit=1
$ vrx-vppcheck plugins | wc -l ; grep -E 'dpdk|linux_cp|linux_nl|npt66'
87
dpdk_plugin.so
linux_cp_plugin.so
linux_nl_plugin.so
npt66_plugin.so
exit=0
```

### deploy/vpp/test-apply-startup.sh <generator> (fake host; one line per scenario check)
```
== 1. dry run (default) changes nothing; prints diffs, drivers, management restore plan, preflight, gate, both sha256
  ok   exit 0; unified diff shows the new dev lines
  ok   semantic diff shows the logical names
  ok   drivers of the PCI devices involved are listed
  ok   management interface, ifupdown detected, addresses recorded
  ok   exact restore plan printed (address + default route)
  ok   link-local address and kernel routes are not in the plan
  ok   VPP preflight answered; gate state shown
  ok   sha256 of the live file and of the rendering printed
  ok   live file untouched, VPP not restarted, no ip change
== 2. usage errors → exit 2, nothing changed
  ok   --apply without --expect-new-sha256 refused (exit 2)
  ok   malformed approval id refused (exit 2)
== 3. handover gate: pending and no approval → refused before anything; approval → recorded
  ok   exit 3, no work dir, reason printed
  ok   systemd never touched
  ok   approval recorded in the gate file and the run log (exit 0)
  ok   approval sent to syslog
  ok   handover flag parser (tools/lab rule): done / pending / absent → pending
== 4. what was reviewed is pinned: live file or rendering changed → refused inside the lock
  ok   live file changed: exit 3, no restart
  ok   rendering changed: exit 3, no restart
== 5. VPP hung BEFORE the apply → preflight refuses within the timeout, nothing installed
  ok   exit 3, refused
  ok   bounded: 5s
== 6. healthy apply commits
  ok   exit 0, committed
  ok   new file installed
  ok   backup kept (work dir + next to the file)
  ok   VPP restarted
  ok   logical interfaces + plugins verified through vrx-vppcheck
  ok   drivers recorded before the restart
  ok   management snapshot: ens192, ifupdown, gateway
  ok   gateway pinged through the management interface
  ok   locks taken before backup and diff
  ok   dead-man timer armed with the settings as --setenv
  ok   dead-man timer cancelled after commit
  ok   generator, checker and script pinned into the work dir
== 7. a logical interface missing in VPP → rollback
  ok   exit 1, rolled back, original file restored
  ok   reason logged
  ok   VPP stopped, reset-failed (N11), started on the old file
== 8. ifupdown host (vrx-a), no netplan: VPP steals the management NIC → rebind + EXACT address/route restore
  ok   exit 1, loss detected
  ok   unbound from vfio-pci, bound back to vmxnet3, override cleared
  ok   recorded addresses (v4+v6) and default route re-applied verbatim
  ok   fake kernel: address, default route and vmxnet3 driver back
  ok   no network manager needed; rollback verified healthy
== 9. ifupdown, exact re-apply not enough → ifup --force ens192
  ok   exit 1, ifup --force used, rollback healthy
== 10. systemd-networkd host → networkctl reconfigure
  ok   exit 1, networkd detected, reconfigure used, rollback healthy
== 11. netplan host → netplan apply
  ok   exit 1, netplan detected, netplan apply used, rollback healthy
== 12. a plugin the new file enables is not loaded → rollback
  ok   exit 1, reason: npt66 not loaded
== 13. D-084 omission (re-review N6): the new document drops npt66 → it may vanish; the apply commits
  ok   exit 0, committed without npt66
== 14. VPP crash-restarts during the window → rollback
  ok   exit 1, NRestarts change detected
== 15. VPP HANGS after the restart (socket accepted, no answer) → bounded wait, rollback
  ok   exit 1, rolled back
  ok   bounded: 9s (cmd-timeout 1s, api-wait 2s)
== 16. VPP hangs in the middle of the watch window → rollback
  ok   exit 1, hang detected, rolled back
  ok   bounded: 10s
== 17. systemctl restart itself hangs → svc timeout, rollback
  ok   exit 1, rolled back
  ok   bounded: 9s
== 18. SSH session dies mid-apply (detached through systemd-run, clean environment) → the run still commits
  ok   session killed after install, before commit
  ok   detached run committed anyway
  ok   unit started with every setting as --setenv (N6)
  ok   the unit saw exactly the recorded settings under env -i
== 19. SSH session dies mid-apply, systemd-run unavailable → setsid fallback survives too
  ok   setsid run committed after the session died
== 20. the run HANGS mid-apply → dead-man kills it, takes the locks, rolls back
  ok   run is stuck in 'systemctl restart' holding the VPP lock
  ok   exit 1, run killed, file restored
  ok   locks released by the kill and taken normally
  ok   bounded: 3s
== 21. dead-man: no-op after commit; rollback when the run died before committing
  ok   committed run: dead-man does nothing
  ok   uncommitted run: dead-man restores the backup (exit 1)
== 22. dead-man while someone else holds the lab lock forever → bounded wait, FORCED rollback
  ok   exit 1, rolled back without the lock
  ok   bounded: 4s
== 23. lab lock held by someone else at apply time → refused, nothing changed
  ok   exit 3, lock busy, file unchanged
== 24. timer unavailable → setsid dead-man without the lock fds (N9); stopped on commit
  ok   exit 0, committed, fallback dead-man used
  ok   both locks free right after the commit
  ok   fallback dead-man stopped by the commit
== 25. rollback cannot restore the management path → rollback-incomplete, dead-man stays armed and retries
  ok   exit 1, incomplete, timer NOT cancelled
  ok   dead-man retry restored the management path
== 26. the generator refuses the management NIC as a device (host facts) → nothing changed
  ok   dry run fails with the generator's error (exit 2)

apply-startup tests: 72 passed, 0 failed
```
("Killed" lines from bash job control when scenarios 18/19 kill the fake SSH session are omitted.)

Scenario map: hung VPP = 5 (before apply), 15 (after restart), 16 (mid-window), 17 (restart hangs), 20 (run hangs → dead-man
kills it); SSH death mid-apply = 18 (systemd-run unit, env -i) and 19 (setsid fallback); ifupdown restore = 8 (exact) and
9 (`ifup --force`), networkd = 10, netplan = 11; lock contention = 20 (run's own lock released by the kill), 22 (foreign
holder → FORCED rollback, bounded), 23 (apply refused), 24 (N9 fallback releases locks); gate = 3; pinning = 4.

## Cannot be proven without a real apply (out of scope here)
- That vmxnet3 re-creates `ens192` under the same name after a vfio-pci steal and that `ip addr/route replace` then
  restores SSH on vrx-a (fake sysfs/ip model only).
- Real systemd behaviour: `systemd-run --setenv`, `--on-active` timer firing, `systemctl kill --kill-whom=all` on the
  run unit, `KillMode=process`; real `ifup --force` / `networkctl reconfigure` / `netplan apply` semantics.
- vrx-vppcheck against a VPP that hangs *after* connecting (tested: socket accepts but never answers → exit 2 in 0.7 s).
- Multipath (`nexthops`) routes and policy-routing tables on the management interface are not restored (logged plan skips them).

## Decisions (for the LOG)
- Work dir settings are authoritative for the detached stages; env via `--setenv` too, mismatches warned (N6).
- Dead-man rolls back without the locks after a bounded wait: a lost management path outranks lock etiquette.
- `jq` is required on the target host (snapshot); the script refuses otherwise.

## Open questions
None blocking. Merge order: independent of other branches (owns only deploy/vpp/*, cmd/vrx-vppcheck, the vppstartup doc).

## Review fixes (round 1 — review BLOCK at edd93a5)

`git merge main` first (8adfa86). Nothing run with `--apply` on the host; VPP not restarted; only read-only calls below.

| finding | fix |
|---|---|
| H1 NRestarts reset by a manual restart | NRestarts dropped. `vrx-vppcheck bootid` (D-080 triple via `internal/vpp/bootid`: boot_id / control_ping vpe_pid / /proc start time). Before the restart the identity is recorded; after it the run requires a **new** identity whose PID equals vpp.service's `MainPID`; during the window `MainPID` + `ActiveEnterTimestampMonotonic` and the identity must stay unchanged (a crash + systemd auto-restart changes both). Fake `systemctl` now resets NRestarts to 0 on restart and gives a new MainPID/start time; scenarios 8 (NRestarts 4 → 0 commits), 9 (no real restart → rollback), 10 (crash in the window → rollback) |
| H2 gateway drops ICMP | `--mgmt-probe auto\|ssh-peer\|gateway-ping\|tcp:HOST:PORT`. auto = ssh-peer (the manager's session from `$SSH_CONNECTION`/`--mgmt-peer` ESTABLISHED in `ss`), else gateway-ping only if the gateway answers ICMP now, else **refuse** (dry run exit 3, "NONE VIABLE"). The dry run prints which check will be used; the run takes a baseline before installing (refuse 3 if it fails). In addition every management interface must keep its addresses and routes **exactly** as snapshotted (restore plan regenerated and compared). Fake gateway can drop ICMP; fake `ss` / TCP prober; scenarios 2, 11, 12, 13 |
| M1 locks free between run and dead-man | a separate **lock holder** (`--stage hold`, own unit/session) takes both locks and keeps them until commit or the end of the rollback; the dead-man kills the run's process tree but never the holder, rolls back, then releases. Holder gone → the dead-man takes both locks exclusively (bounded) **before** touching the run; only a foreign holder → FORCED, holder from `lslocks` logged. Scenario 26: a queued `flock -s` waiter gets the lab lock only after the rollback restarted VPP; 27, 28, 29 |
| M2 approval / gate | approval must be `PENDING-<slug>`: `docs/decisions/PENDING-<slug>.md` must exist on main of /root/ngfw and a LOG.md D-row on main must reference it; the gate record carries the file blob + D ids. The planner seals `plan.sha256` (settings, document, gen args, pinned binaries, gate); `--stage run` refuses on a seal mismatch and re-evaluates the gate, which must equal the sealed record. Scenarios 3, 4, 5 |
| M3 budget / retries | explicit budgets (`ITER`, `RB_BUDGET`, `RUN_BUDGET`); dead-man deadline = run budget + one check round + worst-case rollback + 60 s (logged; scenario 8 asserts deadline > run + rollback budget). The run makes one rollback attempt; if incomplete the locks stay held and the dead-man makes up to `--rollback-retries` attempts with exponential backoff, then gives up loudly and releases. Scenario 31 |
| lows | SSH peer shown and protected in the dry run (L2); interface names regex-escaped (`ere_escape`); dry-run temp dir removed by an EXIT trap (scenario 1); `--foreground` refused over SSH unless `--console`; `noprefixroute` kept in the restore plan; restore plan applies addresses before routes. L1 (fake-host test in the CI gate) needs `tools/ci.sh`, which this task does not own — proposed for the manager |

### tools/ci.sh --base main
```
  apps/agent: make lint test build                   0m26s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(F-startup-apply): findings
  mode quick · wall time 3m35s · logs /root/ngfw-wt/logs/ci/F-startup-apply-20260924-045104-2811545

CI GATE PASSED
```
(the warning is the manager's `review(F-startup-apply): findings` subject)

### shellcheck deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh
```
(no output) exit 0
```

### go test -v ./cmd/vrx-vppcheck
```
--- PASS: TestVersion (0.00s)
--- PASS: TestBootID (0.00s)
--- PASS: TestPluginsByContent (0.00s)
--- PASS: TestIfacesExactMatch (0.00s)
--- PASS: TestDisconnectedIsExit2 (0.00s)
--- PASS: TestUsage (0.00s)
--- PASS: TestNoSocket (0.00s)
--- PASS: TestHungVPPTimesOut (0.70s)
PASS
ok  	ngfw/agent/cmd/vrx-vppcheck	0.728s
```

### read-only on vrx-a: boot identity vs vpp.service (the PID matches MainPID)
```
$ vrx-vppcheck bootid ; systemctl show vpp -p MainPID -p ActiveEnterTimestampMonotonic -p NRestarts
b7712a53-c1e7-45e2-98b8-bdb21f3904f9/2808617/5808198
exit=0
ActiveEnterTimestampMonotonic=58081985783
MainPID=2808617
NRestarts=5
```

### deploy/vpp/test-apply-startup.sh <generator> (fake host)
```
== 1. dry run (default) changes nothing; prints diffs, drivers, management restore plan, reachability check, preflight, gate, both sha256
  ok   exit 0; unified diff shows the new dev lines
  ok   semantic diff shows the logical names
  ok   drivers of the PCI devices involved are listed
  ok   management interface, ifupdown detected, addresses recorded
  ok   exact restore plan printed (address + default route)
  ok   link-local address and kernel routes are not in the plan
  ok   reachability check chosen and shown: gateway-ping
  ok   VPP preflight + boot identity; gate state shown
  ok   sha256 of the live file and of the rendering printed
  ok   live file untouched, VPP not restarted, no ip change
  ok   dry run removed its temp dir (L3)
== 2. dry run: SSH peer shown and used; no viable reachability check → exit 3 (review H2, L2)
  ok   exit 0; peer 10.0.0.9 shown; gateway drops ICMP → ssh-peer chosen
  ok   no peer, no ICMP: exit 3, refused early
  ok   --mgmt-probe tcp:10.0.0.1:22 viable (exit 0)
  ok   explicit gateway-ping on a no-ICMP gateway: exit 3
== 3. usage errors → exit 2, nothing changed
  ok   --apply without --expect-new-sha256 refused (exit 2)
  ok   approval id that is not PENDING-<slug> refused (exit 2)
  ok   --foreground over SSH without --console refused (exit 2)
== 4. handover gate: pending → refused; approval verified on main (PENDING file + LOG D-row); unknown PENDING → refused
  ok   no approval: exit 3, no work dir
  ok   unknown PENDING id: exit 3
  ok   systemd never touched
  ok   approval resolved on main (file blob + D-060), recorded in gate + log (exit 0)
  ok   approval sent to syslog
  ok   handover flag parser (tools/lab rule): done / pending / absent → pending
  ok   interface names are regex-escaped (L3)
== 5. --stage run verifies the sealed plan and re-evaluates the gate (review M2)
  ok   forged gate file: exit 3, refused, nothing installed
  ok   altered settings: exit 3, refused
== 6. what was reviewed is pinned: live file or rendering changed → refused inside the lock
  ok   live file changed: exit 3, no restart, locks released
  ok   rendering changed: exit 3, no restart
== 7. VPP hung BEFORE the apply → preflight refuses within the timeout, nothing installed
  ok   exit 3, refused, locks released
  ok   bounded: 6s
== 8. healthy apply commits — NRestarts 4 before, reset to 0 by the restart (review H1)
  ok   exit 0, committed although NRestarts went 4 → 0
  ok   new boot identity, PID = vpp.service MainPID
  ok   new file installed
  ok   backup kept (work dir + next to the file)
  ok   logical interfaces + plugins verified through vrx-vppcheck
  ok   drivers recorded before the restart
  ok   management snapshot: ens192, ifupdown; check gateway-ping
  ok   locks taken (holder) before backup and diff
  ok   dead-man timer and lock holder started with the settings as --setenv
  ok   dead-man timer cancelled, locks released after commit
  ok   dead-man deadline > run budget + rollback budget (review M3)
== 9. VPP did not actually restart (same boot identity) → rollback
  ok   exit 1, detected
== 10. VPP crashes during the window (systemd brings it back) → rollback
  ok   exit 1, crash detected by MainPID/ActiveEnter/boot identity
== 11. gateway drops ICMP (vrx-a): auto check = the manager's SSH session → commits (review H2)
  ok   exit 0, committed with ssh-peer
== 12. gateway drops ICMP, --mgmt-probe tcp:… → commits; path lost → rollback verified by the same probe
  ok   exit 0, committed with a TCP probe
  ok   exit 1, lost TCP path → rollback, healthy afterwards
== 13. no viable check at apply time → refused before anything changes
  ok   exit 3, refused, locks released
== 14. a logical interface missing in VPP → rollback
  ok   exit 1, rolled back, original file restored, locks released
  ok   reason logged
  ok   VPP stopped, reset-failed (N11), started on the old file
== 15. ifupdown host (vrx-a), no netplan: VPP steals the management NIC → rebind + EXACT address/route restore
  ok   exit 1, loss detected (addresses/routes compared exactly)
  ok   unbound from vfio-pci, bound back to vmxnet3, override cleared
  ok   recorded addresses (v4+v6) and default route re-applied verbatim
  ok   addresses before routes
  ok   fake kernel: address, default route and vmxnet3 driver back
  ok   no network manager needed; rollback verified healthy
== 16. ifupdown, exact re-apply not enough → ifup --force ens192
  ok   exit 1, ifup --force used, rollback healthy
== 17. systemd-networkd host → networkctl reconfigure
  ok   exit 1, networkd detected, reconfigure used, rollback healthy
== 18. netplan host → netplan apply
  ok   exit 1, netplan detected, netplan apply used, rollback healthy
== 19. a plugin the new file enables is not loaded → rollback
  ok   exit 1, reason: npt66 not loaded
== 20. D-084 omission (re-review N6): the new document drops npt66 → it may vanish; the apply commits
  ok   exit 0, committed without npt66
== 21. VPP HANGS after the restart (socket accepted, no answer) → bounded wait, rollback
  ok   exit 1, rolled back
  ok   bounded: 12s (cmd-timeout 1s, api-wait 2s)
== 22. VPP hangs in the middle of the watch window → rollback
  ok   exit 1, hang detected, rolled back
  ok   bounded: 13s
== 23. systemctl restart itself hangs → svc timeout, rollback
  ok   exit 1, rolled back
  ok   bounded: 14s
== 24. SSH session dies mid-apply (detached through systemd-run, clean environment) → the run still commits
  ok   session killed after install, before commit
  ok   detached run committed anyway
  ok   unit started with every setting as --setenv
  ok   the unit saw exactly the recorded settings under env -i
== 25. SSH session dies mid-apply, systemd-run unavailable → setsid fallback survives too
  ok   setsid run committed after the session died
== 26. the run HANGS mid-apply; an integration run queues on the lab lock → dead-man kills the run, the locks never go free until the rollback is done (review M1)
  ok   run stuck in 'systemctl restart'; holder 2799224 owns the locks
  ok   exit 1, run killed, file restored
  ok   holder kept the locks across the kill; not FORCED
  ok   queued shared waiter got the lab lock only after the rollback restarted VPP
  ok   locks released at the end, holder gone
  ok   bounded: 5s
== 27. dead-man: no-op after commit; rollback when the run died before committing
  ok   committed run: dead-man does nothing
  ok   uncommitted run, holder gone: dead-man took the locks itself, restored the backup (exit 1)
== 28. dead-man, holder gone AND a foreign process holds the lab lock forever → bounded wait, FORCED rollback, holder logged
  ok   exit 1, rolled back without the lock, holder named
  ok   bounded: 5s
== 29. lab lock held by someone else at apply time → refused, nothing changed
  ok   exit 3, lock busy (holder named), file unchanged
== 30. timer unavailable → setsid dead-man; stopped on commit; locks free
  ok   exit 0, committed, fallback dead-man used
  ok   both locks free right after the commit
  ok   fallback dead-man stopped by the commit
== 31. rollback cannot restore the management path → incomplete, locks STAY held, dead-man retries with backoff
  ok   exit 1, incomplete, timer NOT cancelled, locks still held
  ok   still broken: 2 bounded attempts with backoff, then gives up and releases
  ok   path fixable: dead-man retry restored it and released the locks
== 32. the generator refuses the management NIC as a device (host facts) → nothing changed
  ok   dry run fails with the generator's error (exit 2)

apply-startup tests: 91 passed, 0 failed
```

### Still not provable without a real apply
The ones listed above, plus: the real `ss` output for the manager's session during a VPP restart, real `lslocks`, a real
`systemd-run` lock-holder unit surviving the run unit, and `systemd-run` timer re-use. The ssh-peer check fails (→ rollback)
if the manager closes the SSH session during the window: keep it open, or use `--mgmt-probe tcp:HOST:PORT`.

## Fix round 2 (re-review APPROVE WITH CHANGES at 3c0bafc)

The first round-2 worker died at ~05:45; its uncommitted edits were salvaged as 028eda4 and reviewed and kept by the
respawned worker, which finished N4, the doc and the evidence (2d622eb), then merged main (0f27b57). Nothing was run with
`--apply` on the host; VPP not restarted; nothing written under `/etc`. Real-host actions: `ip -j route`, `ip -j neigh`,
and one **dry run** (read-only) of the branch script.

| finding | fix | verified by |
|---|---|---|
| N1 ssh-peer: closed session → rollback loop + false "console needed" | `ssh-peer` removed as a verdict. `auto` = `neigh`: every default gateway of a protected interface (from the snapshot) must become REACHABLE in the neighbour table after a nudge (ARP/ND — vrx-a's gateway drops ICMP but answers ARP; on the host: `172.30.126.1 REACHABLE`). The SSH session is logged as an informational signal only (`session_signal`), never a failure. On top: exact addresses/routes and NIC driver. The rollback is **one** attempt, verified by the same checks; not healthy → `console-needed` marker, timer cancelled, locks released — VPP is never restarted in a loop; the dead-man does at most one rollback and only if none finished. `--rollback-retries`/`--retry-backoff` removed | scenario 11 (gateway drops ICMP, session closes in the window → commits, one restart), 13 (path stays dead → one rollback, console-needed, 2 restarts total, late dead-man no-op), 2/14 (no viable check → exit 3), 12 (tcp probe) |
| N2 setsid fallback frees the locks | **No fallback**: run unit, lock-holder unit and dead-man timer all need `systemd-run`; if any cannot be started, `--apply` is refused before anything changes (exit 3; the dry run reports a missing `systemd-run`). The holder is therefore never a descendant of the run; `stage_hold`'s wait is `sleep 1 … \|\| true` (a killed sleep cannot end the hold). N5 folded in: `holder_alive` checked before install, in every health round and before commit | scenario 25 (systemd-run fails for run unit / holder unit / timer → exit 3, unchanged, locks free), 26 (hung run, queued `flock -s` waiter gets the lab lock only after the single rollback), 27 (holder killed → rollback) |
| N3 crash between restart and first identity read | `unit_ident` = MainPID + ActiveEnterTimestampMonotonic + **NRestarts**, read right after the `systemctl restart` job returns (NRestarts is 0 then) into `unit.restart`; after the API is up and `--settle` (10 s) the identity must be new, complete, = MainPID, and the unit tuple unchanged; every window round compares against the same tuple; window ≥ interval > 0, ≥ 2 identity reads. `vrx-vppcheck bootid` exits 1 on an incomplete identity (N7) | scenario 10a (crash at the first API probe after the restart → "vpp.service restarted during the settle time", rolled back), 10b (crash in the window), 8 (NRestarts 0 right after restart, ≥2 reads), `TestBootID` |
| N4 approval not bound to the change | The D-row must have `PENDING-<slug>` as the **subject** of its decision column (a mention does not count) **and name the sha256 of the rendering** (`--expect-new-sha256`); the gate record carries that sum. An approval already executed for that rendering (committed work dir whose gate names PENDING + sum) is refused as spent. A work dir that was already installed from cannot be replayed with `--stage run`. The dry run evaluates the gate against its own rendering. Test fixture repo via `VRX_TEST_ROOT` (honoured only when startup.conf, sysfs, systemctl and systemd-run are all under it) — the harness no longer depends on /root/ngfw main | scenario 4 (no-such / other change / mention-only refused; accepted with sum; replay after commit → spent; same PENDING + different rendering → not covered), 15 (replaying a rolled-back work dir refused, no restart), 3 (real startup.conf with a test root refused); real host: `PENDING-handover` → refused (below) |
| N6/N7 lows | budgets scale with PCI devices, management interfaces, restore lines and gateways; HOLD_MAX covers the dead-man's lock wait + one rollback; `--window 0` refused; TCP probe target must route through a management interface (loopback refused) | scenarios 2, 3, 8 |

L1 (harness + shellcheck in `tools/ci.sh`) remains a manager item (`tools/ci.sh` not owned).

### tools/ci.sh --base main (slot 6)
```
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m32s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   1m23s
  apps/agent: make lint test build                   0m42s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - uncommitted changes in the worktree — the gate checks the working tree, but only commits get merged:
      ?? docs/status/tasks/F-startup-apply.continue-envelope.md
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(F-startup-apply): re-review after fix round
      review(F-startup-apply): findings
  mode quick · wall time 3m46s · logs /root/ngfw-wt/logs/ci/F-startup-apply-20260924-073016-3197626

CI GATE PASSED
```
(the untracked file was the manager's continue envelope, committed with this section; the two non-conventional subjects are the reviewer's)

### shellcheck deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh
```
shellcheck exit 0
```

### go test -run TestBootID -v ./cmd/vrx-vppcheck
```
=== RUN   TestBootID
--- PASS: TestBootID (0.00s)
PASS
ok  	ngfw/agent/cmd/vrx-vppcheck	0.025s
```

### read-only on vrx-a: the neigh probe and the gate (dry run of the branch script, no --apply)
```
$ ip -j route show default; ip -j neigh show 172.30.126.1 dev ens192
[{"dst":"default","gateway":"172.30.126.1","dev":"ens192","flags":["onlink"]}]
[{"dst":"172.30.126.1","lladdr":"00:10:f3:40:00:2f","state":["REACHABLE"]}]
$ deploy/vpp/apply-startup.sh --doc apps/agent/internal/renderers/vppstartup/testdata/cases/six-nic-sample.json   (dry run)
== management interfaces protected (manager peer(s): 172.30.126.196; network manager; exact restore plan)
  ens192 pci=0000:0b:00.0 driver=vmxnet3 manager=ifupdown addrs=172.30.126.195/24 
    ip addr replace 172.30.126.195/24 broadcast 172.30.126.255 dev ens192
    ip -4 route replace default via 172.30.126.1 dev ens192 onlink
== management reachability check (--mgmt-probe auto)
  will use: neigh (passes now); manager session with 172.30.126.196: established
== VPP preflight (read-only: vrx-vppcheck ifaces local0, bootid)
  present: local0
  boot identity: b7712a53-c1e7-45e2-98b8-bdb21f3904f9/2808617/5808198
$ … same dry run with --i-have-product-owner-approval PENDING-handover
== handover gate (--apply only; an approval must name the rendering's sha256 below)
  REFUSED: --i-have-product-owner-approval PENDING-handover: no D-row on main answering PENDING-handover names this rendering (sha256 e0b5945eb8b28b4032dc23f463a1c568bd9ee14f535773e32b896b0be1dcc2c7) — the approval does not cover this change
```

### deploy/vpp/test-apply-startup.sh <generator> (fake host, full output)
```
== 1. dry run (default) changes nothing; prints diffs, drivers, management restore plan, reachability check, preflight, gate, both sha256
  ok   exit 0; unified diff shows the new dev lines
  ok   semantic diff shows the logical names
  ok   drivers of the PCI devices involved are listed
  ok   management interface, ifupdown detected, addresses recorded
  ok   exact restore plan printed (address + default route)
  ok   link-local address and kernel routes are not in the plan
  ok   reachability check chosen and shown: neigh (next hop REACHABLE)
  ok   VPP preflight + boot identity; gate state shown
  ok   sha256 of the live file and of the rendering printed
  ok   live file untouched, VPP not restarted, no ip change
  ok   dry run removed its temp dir
== 2. dry run: reachability — gateway drops ICMP (vrx-a) → neigh; SSH peer shown as a signal only; nothing viable → exit 3; loopback TCP target rejected
  ok   exit 0; peer shown; neigh chosen although ICMP is dropped
  ok   next hop does not answer ARP: exit 3, refused early
  ok   --mgmt-probe tcp:10.0.0.1:22 viable (exit 0)
  ok   loopback TCP target rejected (exit 3)
  ok   explicit gateway-ping on a no-ICMP gateway: exit 3
== 3. usage errors → exit 2, nothing changed
  ok   --apply without --expect-new-sha256 refused (exit 2)
  ok   approval id that is not PENDING-<slug> refused (exit 2)
  ok   --window 0 refused (exit 2)
  ok   --foreground over SSH without --console refused (exit 2)
  ok   a test root with the real startup.conf is refused (exit 2)
== 4. gate: pending → refused; approval only when a D-row has the PENDING as subject AND names this rendering; spent approval refused (N4)
  ok   no approval: exit 3, no work dir
  ok   PENDING-no-such refused (exit 3)
  ok   PENDING-closed refused (exit 3)
  ok   PENDING-mentioned refused (exit 3)
  ok   systemd never touched
  ok   D-900 has the PENDING as subject and names this rendering: accepted, recorded in gate + log (exit 0)
  ok   approval sent to syslog
  ok   same approval + same rendering again after the commit: spent, exit 3, no new work dir
  ok   same PENDING, a different change: not covered, exit 3
  ok   the dry run shows the gate against its own rendering
  ok   handover: done → accepted without approval (exit 0)
  ok   one more pending source keeps it pending (exit 3)
  ok   flag parser (tools/lab rule); interface names regex-escaped
== 5. --stage run verifies the sealed plan (forged gate / altered settings → refused)
  ok   forged gate file: exit 3
  ok   altered settings: exit 3
== 6. what was reviewed is pinned: live file or rendering changed → refused inside the lock
  ok   live file changed: exit 3, no restart, locks released
  ok   rendering changed: exit 3, no restart
== 7. VPP hung BEFORE the apply → preflight refuses within the timeout, nothing installed
  ok   exit 3, refused, locks released
  ok   bounded: 6s
== 8. healthy apply commits — NRestarts 4 before, reset to 0 by the restart (H1); settle + ≥2 identity reads (N3)
  ok   exit 0, committed although NRestarts went 4 → 0
  ok   new boot identity = the unit's MainPID, NRestarts 0 right after the restart
  ok   identity stable across 2 reads in the window
  ok   new file installed, backup kept
  ok   logical interfaces + plugins verified through vrx-vppcheck
  ok   management snapshot: ens192, ifupdown; check neigh (ARP of the next hop)
  ok   locks taken (holder unit) before backup and diff
  ok   dead-man timer and lock holder units started with the settings as --setenv
  ok   timer cancelled, locks released, exactly one VPP restart
  ok   dead-man deadline > run budget + rollback budget
== 9. VPP did not actually restart (same boot identity) → rollback
  ok   exit 1, detected
== 10. VPP crashes and is brought back by systemd — inside the settle time (N3) and inside the window → rollback
  ok   crash before the first identity read: exit 1, rolled back
  ok   crash in the window: exit 1, rolled back
== 11. reviewer's scenario: gateway drops ICMP, the manager's SSH session closes during the window → commits (N1)
  ok   exit 0, committed with neigh; closed session logged only; one VPP restart
== 12. --mgmt-probe tcp:… → commits; path lost → rollback judged by the same probe
  ok   exit 0, committed with a TCP probe
  ok   exit 1, lost TCP path → rollback, healthy afterwards by the same probe
== 13. the path stays dead after the rollback → ONE rollback, console-needed, locks released, no restart loop (N1)
  ok   exit 1, file restored, console-needed marker
  ok   locks released, timer cancelled, VPP (re)started twice in total (apply + one rollback)
  ok   a late dead-man does nothing — no further restart
== 14. no viable check at apply time → refused before anything changes
  ok   exit 3, refused, locks released
== 15. a logical interface missing in VPP → rollback
  ok   exit 1, rolled back, original file restored, locks released
  ok   reason logged
  ok   VPP stopped, reset-failed, started on the old file
  ok   replaying the rolled-back work dir with --stage run: refused (exit 3), no restart
== 16. ifupdown host (vrx-a), no netplan: VPP steals the management NIC → rebind + EXACT address/route restore
  ok   exit 1, loss detected (addresses/routes compared exactly)
  ok   unbound from vfio-pci, bound back to vmxnet3, override cleared
  ok   recorded addresses (v4+v6) and default route re-applied verbatim
  ok   addresses before routes
  ok   fake kernel: address, default route and vmxnet3 driver back
  ok   no network manager needed; rollback verified healthy
== 17. ifupdown, exact re-apply not enough → ifup --force ens192
  ok   exit 1, ifup --force used, rollback healthy
== 18. systemd-networkd host → networkctl reconfigure
  ok   exit 1, networkd detected, reconfigure used, rollback healthy
== 19. netplan host → netplan apply
  ok   exit 1, netplan detected, netplan apply used, rollback healthy
== 20. plugins: enabled but not loaded → rollback; D-084 omission (dropped from the document) → commits
  ok   exit 1, reason: npt66 not loaded
  ok   exit 0, committed without npt66
== 21. VPP HANGS after the restart (socket accepted, no answer) → bounded wait, rollback
  ok   exit 1, rolled back
  ok   bounded: 12s (cmd-timeout 1s, api-wait 2s)
== 22. VPP hangs in the middle of the watch window → rollback
  ok   exit 1, hang detected, rolled back
  ok   bounded: 14s
== 23. systemctl restart itself hangs → svc timeout, rollback
  ok   exit 1, rolled back
  ok   bounded: 13s
== 24. SSH session dies mid-apply (the run is a systemd-run unit with a clean environment) → the run still commits
deploy/vpp/test-apply-startup.sh: line 500: 3186442 Killed                     setsid bash -c '"$0" --doc "$1" --apply "${@:2}"; sleep 60' "$SCRIPT" "$DOCF" "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/caller.out" 2>&1
  ok   session killed after install, before commit
  ok   detached run committed anyway
  ok   unit started with every setting as --setenv
  ok   the unit saw exactly the recorded settings under env -i
== 25. systemd-run unavailable → --apply refused, no fallback (N2)
  ok   run unit: exit 3, nothing changed
  ok   lock holder unit (--foreground): exit 3, nothing changed
  ok   dead-man timer: exit 3, refused before install, locks released
== 26. the run HANGS; an integration run queues on the lab lock → dead-man kills the run; the holder keeps the locks until its single rollback is done (M1)
  ok   run stuck in 'systemctl restart'; holder unit 3190751 owns the locks
  ok   exit 1, run killed, file restored
  ok   holder kept the locks across the kill; not FORCED
  ok   queued shared waiter got the lab lock only after the rollback restarted VPP
  ok   locks released, holder gone, exactly one rollback start
  ok   bounded: 5s
== 27. the lock holder dies during the apply → rollback (the locks are not ours any more)
  ok   exit 1, rolled back
== 28. dead-man: no-op after commit; one rollback when the run died before finishing (holder gone → takes the locks itself)
  ok   committed run: dead-man does nothing
  ok   exactly one rollback, locks taken exclusively first, released after (exit 1)
== 29. dead-man, holder gone AND a foreign process holds the lab lock → bounded wait, FORCED rollback, holder logged
  ok   exit 1, rolled back without the lock, holder named
  ok   bounded: 6s
== 30. lab lock held by someone else at apply time → refused, nothing changed
  ok   exit 3, lock busy (holder named), file unchanged
== 31. the generator refuses the management NIC as a device (host facts) → nothing changed
  ok   dry run fails with the generator's error (exit 2)

apply-startup tests: 101 passed, 0 failed
```

### Still not provable without a real apply
Real `ip neigh` transitions while VPP restarts (the nudge is a TCP SYN to port 9 of the gateway, then up to
`2 × --cmd-timeout` polls), a real `systemd-run` holder unit surviving the run unit, real `lslocks`, and `systemd-run`
timer re-use. The SSH session no longer matters for the verdict.
