# F-mpls-ldp-host — R8 operability review

Exact final SHA `cb2560ed81d56c2d74f0a35075e4a5e40c0ad5a1`, base `06e4368c`. Independent review; no product edits.

Initial MAJOR resolved: `sync.Installed` counted cached observations despite prospective configuration filtering or failed application. Final state RPC checks VPP connectivity and counts current routes from the named ownership-guarded Retrieve under a 10-second deadline. Retrieval failures return unavailable instead of fabricating stale counts. Regression covers configuration suppression with retained cache and failed apply with surviving dataplane objects. No remaining BLOCKER/MAJOR/MINOR findings.

Production source registers through the FRR runtime/S1 lifecycle; cancellation closes its ticker, bounded daemon reads and scheduler cancellation stop work, dirty snapshots retry failed apply, successful empty state withdraws, and observation failure is visible through existing ERROR events and `sync.LastError`. Hold-down/withdrawal behavior is documented. Persisted boot-family records and restart construction tests support ownership-safe recovery. Prospective range and LCP changes filter stale paths. State RPC returns detached cached neighbor/LIB/error information with actual installed count and owner checks. The read-only topology driver has explicit table-zero opt-in, scoped slot/label checks and documented shared lab/globals locks. User docs explicitly state EOS IPv4 support, unsupported NEOS/explicit-null, supported limits and real-lab acceptance still required. No new dependency, migration, installer/systemd/CI mutation or host daemon restart.

Independent final verification in `ldp/apps/agent`:

```text
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/subsystems -run Ldp
ok ngfw/agent/internal/subsystems 1.095s
```

Earlier exact-product core race suites passed, as recorded in R4 report. Real FRR/VPP packets/session/restart/loss/hold-down acceptance NOTRUN and lab-deferred; no whole CI gate pass claimed.

Verdict: **APPROVE**.
