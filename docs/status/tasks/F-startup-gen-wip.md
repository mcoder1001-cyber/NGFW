# F-startup-gen — WIP

started 2026-09-24 01:30 +0330 (worker, direct on host) · finished 2026-09-24 ~01:50

- [x] read context, envelope, schema/proto, live /etc/vpp/startup.conf (read only), FRR renderer style
- [x] renderer `apps/agent/internal/renderers/vppstartup` (model + validation + template + semantic parser + unified diff)
- [x] CLI `apps/agent/cmd/vrx-startupgen`
- [x] golden + hostile + host-equivalence tests
- [x] docs/agent/renderers/vppstartup.md
- [x] questions file, final status (docs/status/tasks/F-startup-gen.md), CI gate PASSED

fix round 1 (review BLOCK e049b31), 2026-09-24 ~01:55–02:30
- [x] merge main · contract/F-startup-gen (schema + proto + drift guard, CI green) merged
- [x] F1 host mgmt NIC · F2 deploy/vpp/apply-startup.sh + fake-host tests · F3 plugins overlay · F4 explicit pinning · F5/F6 bounds · F7 host required · F8 comments · F9 no live file
- [x] CI green on both branches; Review fixes section in F-startup-gen.md
