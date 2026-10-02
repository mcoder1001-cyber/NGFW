# TEST-traffic-A correlation continuation envelope

A3 ruling469c6d7e: finish foundation bd443810/fda0ddc7 separately; split new
functionality, preserve history and re-review continuation. This is one phase
of the existing board row, not a new WBS item or whole-task completion.

- Branch: `task/TEST-traffic-A-correlation-20261002`.
- Isolated worktree: `NGFW-traffic-a-correlation`.
- Starting commit: `8f38ab39fbc2616398f8a33803826befcb64a40c`, preserving foundation
  history, cap948e9ebd and the first pure correlator checkpoint.
- Old development branch/worktree/history are unchanged. Manager owns publication,
  archives and foundation integration on current main; no backward ref was forced.
- Own `test/topology/traffic-a/**`, `docs/status/tasks/TEST-traffic-A*` only.
- No live slot/daemon allocated or used. Host traffic/SSH/VPP/netns/nft/tcpdump
  commands are not executed. Pure packet fixtures and self-owned processes only.
- Existing source-fixture tests are not live acceptance. Capture producer,
  transaction/shared-lock/slot-lease lifecycle and composed executors remain
  genuinely NOTIMPLEMENTED. Live CLI stays refused.
- Partial matcher status keeps packet_outcomes_proven=false and
  whole_chain_proven=false; input/output bytes matching typed expectations cannot
  establish live provenance or configuration cause.
- Continue coherent ≤15min commits; manager publishes every checkpoint.
- Fresh applicable independent panels and exact-head unchanged hosted quick
  required before merging this continuation; foundation approval does not apply.
