# TEST-traffic-C execution envelope

- Branch: `codex/test-traffic-c`; worktree: `/tmp/ngfw-traffic-c`.
- Base: `origin/main` at `d314f0728`.
- Owned: `test/topology/traffic-c/**`, `docs/status/tasks/TEST-traffic-C*`.
- Worker: traffic_final delegated by root manager, 2026-10-05.
- Slot: no live slot assigned; dry-run uses 14 as an arithmetic fixture only.
- Daemons: none owned. No live rig/API/agent started.
- Shared VPP: other chats actively use it; no changes, globals or restart.
- Deliver coherent driver checkpoints; code failures cannot be deferred as lab acceptance.
