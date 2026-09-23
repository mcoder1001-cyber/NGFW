# P04 — work in progress (kept current; final report is P04.md)

started: 2026-09-23T13:13 (+0330) · slot 3 (`w3`) · worktree /root/ngfw-wt/P04

| step | state | notes |
|---|---|---|
| inventory `test/topology/*.yml` | done | vrx-a real facts (7× vmxnet3, 6 DOWN — questions #1); planned VMs `state: planned` |
| `tools/lab` status/vppctl/provision/restart-vpp/kill-vpp/lock/env/inventory | done | bash + python3-yaml; kill/restart refuse rc=2 while handover pending |
| `tools/lab rig up/down/gc/show` | done | ping through VPP, idempotent up, clean down (w3) |
| `tools/binapi-gen.sh` + `apps/agent/binapi/` (143 pkgs) | done | idempotent (git status empty on rerun); go build/vet ok |
| PostgreSQL + Valkey | verified | already installed (18.6 / 9.0.4), localhost-bound; nothing installed |
| `deploy/dev/pg-test.sh` + README | done | create/dsn/list/drop w3 verified |
| smoke test `test/integration/smoke` | done | PASS 3.2 s, rx counters 1→9 / 0→6, path af_packet |
| `docs/lab/vmware.md` | done | |
| CI gate + P04.md | done | `tools/ci.sh --base main` → CI GATE PASSED (log /root/ngfw-wt/logs/P04-ci.log) |
