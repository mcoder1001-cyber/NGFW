# F-dashboard-prom-alarms-host — independent R4 review

Reviewed HEAD: `35c4533e723352c918bb8e842a9155e59c4d6bed`.

Independent reviewer; read 00-CONTEXT, REVIEW-PROMPT, R4/R5 prompts, contributing and shared-host/handover rules. No product changes or commits. Unit/fake evidence is not live data-plane acceptance.

- **BLOCKER (acceptance)** — task status lacks owned real VPP traffic/counter proof, listener commit/rollback verification against real stats and agent-restart reconstruction. Do not mark the host task complete on fake counters. Supply required real-host T3 proof.

Code aspect has no additional blocking finding: stats source is read-only, performs no binary API calls; listener replacement binds first, preserves old bind on failure; same-address changes replace allow handler; Retrieve clones live applied configuration; Delete closes listener; wiring permanently Stop's source before server shutdown. No host-wide restart/config mutation or numeric allocation. The reviewed fixes close active HTTP requests after Shutdown timeout and reject stats reconnect after permanent Stop.

Evidence, GOMAXPROCS=2 GOFLAGS=-p=2:
```
go test -race -count=1 ./internal/promexport ./internal/desired
ok promexport 2.020s
ok desired 10.556s
go test -race -count=1 ./internal/subsystems -run TestPrometheusListenerLifecycle
ok subsystems 1.071s
```
An earlier broad subsystem run failed; it is not claimed green and does not replace a full gate.

Code aspect: APPROVE. Acceptance and reviewer verdict: **BLOCK**.
