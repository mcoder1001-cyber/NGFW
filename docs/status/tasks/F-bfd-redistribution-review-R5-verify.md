# BFD R5 verification of two MAJOR findings

Source: local `97f67a2cd3a3584fea37f7956bb302cd722b0a52`, published `0275fe02c887e9d1c9b1a754a1d591409e5cb6eb`, exact tree `5e8ab124e0e189206b987b594ad3d4f099a1a4b8`. Fresh reassigned developer repair; prior report source8f3 retained unchanged for provenance. R5 independently read every product delta and regression from that source in its own RAM snapshot.

## Closure review

1. Endpoint lookup/claim MAJOR: exact endpoint-prefix buckets replace full-record scans. Lookup checks only the tuple bucket under the durable store mutex and filters current complete boot identity; malformed suffixes and ambiguity fail closed. Claim performs conflict check and record mutation atomically under the same mutex. Initial index rebuild is linear; direct store Claim/Release update it, Prune rebuilds it, journal replay occurs before initial index creation. Batched journal writes keep the index synchronized with successful in-memory durable-intent mutations; failed outside-batch persistence leaves the index unchanged. Generic full snapshot persistence costs are inherited, distinct from the eliminated per-lookup scans.

2. Observation lifetime MAJOR: successful descriptor Create, Delete/ENOENT/expired proof, and Retrieve emit owner-scoped lifecycle callbacks for single-hop and multihop. The live-key set bounds recorded events; absent-key late events are ignored. Delete/recreate clears previous history; snapshots prune removed sessions. Connection changes clear membership/history, subscription loss resets history, Wiring.Close removes owner state. Other owners remain isolated.

Meaningful regressions independently inspected: 1,024 sessions with two complete lookup passes require 2,048 visits, reopen requires a 1,024-record rebuild plus one tuple visit; direct Claim/Release, ambiguity, conflicting Claim, foreign boot and Prune are exercised. 2,048 create/transition/delete churn cycles retain zero observations/live keys, deleted-key events cannot recreate state, delete/recreate initially returns unknown last flap, pruning/restart and foreign-owner history are checked. The real descriptor fake-native Create/Retrieve/Delete sequence proves callback wiring also works for single-hop.

## Actual verification

Running in `/dev/shm/r5-review/snapshots/bfd-fixed/apps/agent`:

```text
env TMPDIR=/dev/shm/r5-tmp GOTMPDIR=/dev/shm/r5-tmp GOCACHE=/dev/shm/r5-cache GOMAXPROCS=2 GOFLAGS=-p=2 GOTOOLCHAIN=local ../../../../tools/heavy.sh go test -race -count=1 ./internal/subsystems ./internal/descriptors/bfd ./internal/agent ./internal/desired ./internal/renderers/frr/bfd ./internal/renderers/frr/redistribute
```

The unmodified six-package command exited 0; actual output:

```text
ok  	ngfw/agent/internal/subsystems	31.724s
ok  	ngfw/agent/internal/descriptors/bfd	1.188s
ok  	ngfw/agent/internal/agent	55.272s
ok  	ngfw/agent/internal/desired	30.374s
ok  	ngfw/agent/internal/renderers/frr/bfd	1.142s
ok  	ngfw/agent/internal/renderers/frr/redistribute	1.115s
```

All selected packages passed; capacity/churn/store and descriptor lifecycle regressions actually ran. No product edits, shared VPP/daemon/host faults, throughput benchmark or full quick gate. Unaffected HA48e5 R5 approval is unchanged.

Verdict: **APPROVE** — both prior R5 MAJORs closed; 0 BLOCKER, 0 MAJOR, 0 MINOR on this verify delta. Historical BLOCK report remains applicable only to source8f3. Full quick and other reviewers remain independently required.
