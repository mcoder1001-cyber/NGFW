# F-srv6 — WIP log

| time (+0330) | state |
|---|---|
| 2026-09-25 10:32 | started; envelope + prompt + context read; host runs closed until TD-25 (manager) |
| 10:45 | contract committed: schema (ext/srv6.ts, semantic/srv6.ts + 23 tests), proto (RoutingConfig 17, Srv6State, messages), gen, fixture, fake-agent stub |

Next: agent (desired/srv6.go builder + assembler, subsystems/srv6.go, rpc_srv6.go, coretest/srv6.go, sr gaps), API, UI, docs.
| 10:50 | agent: desired/srv6.go, subsystems/srv6.go, rpc_srv6.go, sr gaps (Global TD-11b wrapper, LocalSidCounters), coretest SR model — 91 agent packages green |
| 10:57 | agent tests (builder/assembler, wiring, apply/Retrieve/rollback order/restart/validation/globals/Srv6State); 5 mutations each caught |
| 11:00 | host integration test + test/topology/srv6/stack.sh written (not run: TD-25); docs sr.md, proto.md §11, vpp-code-track V-new |
| 11:08 | API + UI + user doc in progress (forked sub-agent, same worktree, no commits by it) |
| 11:13 | fork done (API, UI, user doc); committed in 4 commits; CI #1 PASSED (11m42s) |
| 11:31 | interim screenshots (real API + FakeAgent, en + fa/RTL); dialog label fix; status + questions written |
| next | final CI; host steps pending TD-25 |
