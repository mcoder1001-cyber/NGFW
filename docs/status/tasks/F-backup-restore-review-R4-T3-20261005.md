# F-backup-restore independent R4 / T3 review

Reviewed source: `9bd8f294917d90ed50e70baf0d4bde47f894af76`, tree `6400dc8b5b4005eab323a57645b49be584a3f63b`, remote checkpoint `eb803` (manager supplied), 2026-10-05 UTC. Reviewer did not author product code.
Owned evidence only: this report, task envelope and WIP. Worktree `/root/ngfw-wt/f-backup-dataplane-review-20261005`; branch `codex/f-backup-dataplane-review-20261005`. No slot needed: all executed operations use fake runners/private temporary files. No host upgrade, systemd start, service restart, VPP mutation, database write or live forwarding test occurred.

## R4 findings
No BLOCKER/MAJOR/MINOR within R4 scope. New actions perform fixed read-only status/collector operations or dispatch four enum-selected dedicated root systemd instances. Existing agent `ProtectSystem=strict` and writable paths are unchanged. Stage validates direct regular `/data/updates` paths; exclusive 0600 handoff and root helper O_NOFOLLOW/fstat checks protect the request. Preparation installs executable/helper/unit files into package staging only. No binapi-generator/bindings changes, new VPP object types, numeric IDs, globals, packet trace, or daemon fixtures. Agent-restart VPP convergence and forwarding packets are not applicable to these appliance actions.

R4 verdict: APPROVE.

## T3 actual execution
Commands ran in this isolated checkout; all exited 0:

`GOMAXPROCS=2 go test -race -count=1 -v ./internal/actions/backup-restore` in apps/agent:
```
=== RUN   TestUpgradeFixedArguments
--- PASS: TestUpgradeFixedArguments (0.00s)
=== RUN   TestSupportFixedBoundsAndSensitiveOutput
--- PASS: TestSupportFixedBoundsAndSensitiveOutput (0.00s)
PASS
ok  ngfw/agent/internal/actions/backup-restore 1.070s
```

`go test -count=1 -v ./...` in test/topology/backup-restore:
```
=== RUN   TestSupportCollectorAndFixedUpgradeInstances
    support_test.go:16: ..
        Ran 2 tests in 0.004s
        OK
--- PASS: TestSupportCollectorAndFixedUpgradeInstances (0.24s)
PASS
ok ngfw/test/backup-restore 0.251s
```
`python3 deploy/support-bundle/test_collect.py`:
```
..
Ran 2 tests in 0.007s
OK
```
Independent additional Python runpy/unittest.mock checks executed production dispatcher with mocked subprocess only: activate/confirm/rollback each require exact `/usr/sbin/ngfw-upgrade OP`, check=False, timeout=840; five invalid instances execute nothing. Stage uses an actual private temporary regular file with mocked shared request/bundle paths; 0600 accepted with exact stage argv and request unlink; 0644 refused before subprocess/unlink. Output:
```
PASS fixed fake dispatch: activate
PASS fixed fake dispatch: confirm
PASS fixed fake dispatch: rollback
PASS rejected instance: ''
PASS rejected instance: 'status'
PASS rejected instance: 'stage;reboot'
PASS rejected instance: '../activate'
PASS rejected instance: '--help'
PASS stage private-file mocked paths mode 0o600 accepted
PASS stage private-file mocked paths mode 0o644 rejected
```
`systemctl show vpp --property=MainPID,NRestarts`, before and after:
```
MainPID=1014
NRestarts=0
```

| Scenario | Observed | Result |
|---|---|---|
| Upgrade enums, fixed argv, invalid bundle/op | fake Runner executes only allowed commands; unsafe requests execute none | PASS |
| Support bounds, sensitive output | fixed collector argv; excessive rows/private-key output refused | PASS |
| Read-only collector | seven fixed service/version commands; no journal/config dump | PASS |
| Dedicated dispatcher | exact argv; unknown instance and unsafe request permissions refused | PASS |
| Shared host | unchanged VPP PID/restarts; no shared writes or real appliance mutations | PASS |

T3 verdict: PASS for F-backup-restore fixed-action/collector scope. Actual A/B boot, grubenv, host activation/watchdog remain NOT RUN, explicitly out of scope and owned by F-ab-upgrade laboratory acceptance. Fake argv tests are not claimed as real host upgrade proof. Full quick gate/API/UI evidence belongs to their independent assigned reviewers.
