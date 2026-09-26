# F-lb — WIP log

- 10:30 start (base main@1d3ccf31); envelope + addendum read; pnpm install.
- 10:45 contract commits: schema `services.lb` (+ rules, tests), proto (`ServicesConfig.lb = 11`, `Lb*`, `LbState`, `LbFlushVip`), gen, fixture, fake stubs.
- 11:00 agent: lb gaps (TD-11b declarations, intf-nat claim first, DumpASes, FlushVIP fix, GarbageCollect/GCSafe), desired/lb.go, subsystems/lb.go (GC timer, globals owner only), rpc_lb.go; unit tests; opt-in host tests written.
- 11:07 API: LbController (state + flush), fake, e2e on vrx_w2 (green); regen api-client + CLI table.
- 11:17 web: Load balancer tab (table, sub-table, flush, forms, notice, en/fa), tests green.
- 11:25 docs: user page, descriptor doc, V20 follow-up; questions; CI run 1.
- Left for the host window (TD-25): TestLbOnHost, TestLbGarbageCollectOnHost (manager window), UI screenshot, optional GRE tcpdump.
- 11:35 CI run 1 red (golangci-lint in F-lb files) → fixed (ae9e7f70); CI run 2 GREEN (9m24s).
