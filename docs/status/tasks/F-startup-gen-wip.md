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

fix round 2 (re-review BLOCK afbe6ae, D-088), 2026-09-24 ~02:35–02:50
- [x] branch task/F-startup-apply at afbe6ae; deploy/vpp removed here; manual procedure in vppstartup.md
- [x] N4 management NICs (default routes + control connections, linux-cp taps skipped, bonds via lower_*), N5 rendered sha256
- [x] CI green (1689e32); Fix round 2 section in F-startup-gen.md
