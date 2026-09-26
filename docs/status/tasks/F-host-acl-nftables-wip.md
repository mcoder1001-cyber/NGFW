# F-host-acl-nftables — WIP log

- 2026-09-24 20:57 session-limit stop; salvage bf765d8 (envelope only). Resumed 22:45.
- 3fdb302 contract(proto): host acl state (+ acl.hostSettings, AclConfig 8) — questions Q1.
- 2bb164a renderer `renderers/nftables` (Build, anti-lockout, Render/Validate/Apply/Retrieve, descriptor, store, modes, nftest).
- 8acb296 agent wiring: `Domains["acl"]`, projection, HostAclState.
- 23fc88e netns integration test (packets, counters, update, rollback, re-render, foreign table, removal) — PASS on slot 9.
- 3f366ed docs (renderer mapping, user page, README).
- 1828875 / 9f10058 API state route + web page (built in parallel by a forked worker of this task; committed here).
- c26707b topology test on slot 9 — PASS (evidence in F-host-acl-nftables.md).
- 43ff6a4 screenshots (en + fa/RTL).
- 942f292 lint fixes, TD-11b RecordsNoOwnership, D-132 (30 s poll + Refresh).
- CI GATE PASSED on 942f292 (main's ci.sh, D-127; inherited D-128 file excluded). Cleanup verified. **Status: done.**
