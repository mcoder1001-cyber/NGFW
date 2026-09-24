# P08 — WIP log (vertical slice: interfaces end to end)

Slot 1 (`w1`, API 3100, web 5100, metrics 9111, `/run/vrx-test/w1/agent.sock`, DB `vrx_w1`, tables 1000–1999,
rig 10.1.{1,2}.0/24). Runs directly on the host (no wt.sh). NRestarts at start: 5.

## Plan (work order from the manager)
1. [x] Agent wiring: `internal/subsystems` (registry + persisted stores), `internal/desired` (interfaces builder:
       alias objects, creators, attributes, sub-interfaces, descriptions), IfRef = alias, DHCP Reconnected hook
2. [x] Retrieve-backed `/state/interfaces` (additive contract: `RetrieveResponse.interface_status`) + counters endpoint
3. [x] Topology test `test/topology/interfaces` (API commit → V19 check → ping → MTU 1400 DF → rollback; counters ±5%)
4. [x] Restart-safety test (stop agent, delete host-interfaces + addresses via binapi, start → ping within 30 s)
5. [x] UI list + drawer + sub-interfaces (en/fa)
6. [ ] Docs + screenshots, P08.md, vertical-slice.md, CI

## Log
- start: read context/ops/prompt/envelope/rules/LOG/DF-1/P05/P06/P07a/P07b/vpp-code-track.
- 05:32/05:39 (worker 1): agent wiring (a86d3f2), /state/interfaces + counters (bb71ddc), contracts 51b7c42/c02aa32.
- 07:16 manager salvaged test/topology/interfaces helpers (stack_test.go, vpp_test.go — no Test func yet) as de60fa8.
- 07:2x worker 2 (CONTINUE): merged main (DF-5); DF-5 WithBootStore/keyer wiring: Wiring.IPsecOptions/IKEv2Options
  (file BootStore + vpn-<owner>.key 0600, D-096/Q12) + unit test. Next: topology test, restart-safety, UI, docs.
- 07:27 VPP crash during my run 2 (NRestarts 5→6) — D-101/V24: af_packet delete with veth up; test now quiesces veths (peers(false)).
- 07:46 topology + restart-safety + cleanup PASS on slot 1, NRestarts 6→6 (log kept for P08.md).
- UI screen done (7b1a11d). Next: docs + screenshots, P08.md, vertical-slice.md, ci.sh. Pending: TD-3 ifsanitize.Release wiring after TD-3 merges.
- 07:50–08:00 screenshots (TestInterfacesScreenshots, prod build) → docs/user/interfaces; basics.md, vertical-slice.md, P08.md.
- 08:00 ci.sh #1 failed on agent lint (coretest G115/revive + my IKEv2Options comment) → fixed 390b409; merged main (F-startup-apply); ci.sh #3 running.
- 08:05 ci.sh #3 PASSED (after lint fix + main merge). 08:12 topology re-run green after merge (trace matching made run-unique; agent built in-test when no binary given).
- 08:17 final ci.sh --base main PASSED @82d699d; cleanup done (bin/dist/run dir removed, vrx_w1 dropped, rig down, lock free, NRestarts 6). DONE except TD-3 Release wiring (pending its merge).

## Fix round 1 (manager ngfw-46, envelope P08.fix1-envelope.md; review BLOCK 6022f0d) — started 14:22, time box 4 h
- 14:23 step 0: merged main a8d1efb (TD-3 ifsanitize); one conflict coretest/fakevpp.go (kept installIfExt + sanitizetest.Clean);
  `go build ./... && go test ./internal/...` green (87 ok) → 1bcf450.
- 14:25 N1 fixture fixed (5a48d48). Host run on slot 1: TestAgentOnHost/ProcessOnHost now fail EARLIER, in TD-3's sanitizer
  (placeholder cap, every interface create) — identical on main @ a8d1efb (export in scratch). Environment → Q3 (+ pool probe 2084d9c).
- 14:33–14:40 F1 (config = Retrieve again, new `running`, `actual` dropped; contract f6fbdf3), F2 CLI table regenerated,
  web/e2e/topology consumers, N2 (0755, never re-moded), N3 (trace must be ours), I5 → 3a02345. Web 18/18, e2e 1/1, CLI green.
- Next: F5 (tolerant Update → ErrRecreate), F4 (af_packet veth-only, validation + agent guard), F3/I4 docs, lows, merge main
  again (D-108 rig ring), CI, one topology run.
- 14:45–15:00 F5 (b445a0b), F4 (16356d9), N7 (e52de75), TD-3 Release wiring (2ec61f0), N4/N5 (122d14a), gofmt fakevpp.go
  (manager add-on, ac8b6bf), N6 (aa53687). 15:01 merged main again (D-108 rings, health check) → 058bb33.
- 15:03 N1 part 2 (re-apply count 12, ce93fff) found by running TestAgentOnHost with the D-105 M1 cap (scratch); whole
  internal/agent package green with cap 64 (Q3 updated). 15:04 ci.sh #1: api vitest transform timeout under load (collect
  303 s, 31/31 tests passed) → re-run. Next: one topology run (scratch cap-64 agent), P08.md, cleanup.
