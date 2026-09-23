# P04 — work in progress (kept current; final report is P04.md)

started: 2026-09-23T13:13 (+0330) · slot 3 (`w3`) · worktree /root/ngfw-wt/P04

| step | state | notes |
|---|---|---|
| inventory `test/topology/*.yml` | in progress | vrx-a real facts (7× vmxnet3 found, 6 DOWN, not in host-vrx-a.md); planned VMs as `state: planned` |
| `tools/lab` status/vppctl/provision/restart-vpp/kill-vpp/lock/env | in progress | bash, python3+PyYAML for inventory |
| `tools/lab rig up/down/gc/show` | todo | af_packet host-interfaces on veth+netns |
| `tools/binapi-gen.sh` + `apps/agent/binapi/` | todo | govpp v0.13.0 generator present in /root/go/bin; probe: 143 pkgs in 3.7 s |
| PostgreSQL + Valkey | verify only | postgresql-18 (18.6) and valkey-server 9.0.4 already installed, active, localhost-bound → no apt needed |
| `deploy/dev/pg-test.sh` | todo | |
| smoke test `test/integration/smoke` | todo | own Go module, replace → ../../../apps/agent |
| `docs/lab/vmware.md` | todo | |
| CI gate + P04.md | todo | |
