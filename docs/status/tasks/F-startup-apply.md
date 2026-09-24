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
