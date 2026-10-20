# Task envelope — machine-211-small-packet-limit

Characterise and, where the product allows, raise the 64-byte packet ceiling of appliance 172.30.110.211 (three Intel
X710 quad-port cards, VPP 26.06 / DPDK 26.03, the start-up file of D-256 live since 2026-10-20 06:39 UTC).

Started 2026-10-20 by the owner's session (`ماشین 110.211 ترافیک`, after the apply of D-256). Base origin/main 2a6d1611c.
Branch `claude/zen-knuth-454f68`, worktree `/root/NGFW/.claude/worktrees/zen-knuth-454f68`.

Owned files: `docs/status/tasks/machine-211-small-packet-limit*`, the board row `machine-211-small-packet-limit` in
`plan/tasks.yaml`. Not owned: `docs/status/tasks/machine-211-vpp-tuning*` (PR #251, its session), the renderer
`apps/agent/internal/renderers/vppstartup` (changed only if a renderer change is justified — see the wip file for the verdict).

Host 172.30.110.211: **read-only** (`vppctl show …`, `lspci`, `/sys`, `dmesg`) during the owner's running TRex test. No
xconnect change, no `startup.conf` change, no VPP restart, no `clear` of any counter; every change to the appliance is the
owner's decision in their session and a manager step (docs/agent/renderers/vppstartup.md).

Acceptance: the 64-byte ceiling of each card explained with measured numbers (offered rate, `rx_missed`, TX failure per port
and per chip, VPP per-worker load); the dpdk start-up options the renderer does not expose (`no-multi-seg`,
`no-tx-checksum-offload`, `enable-mbuf-fast-free`, larger `num-tx-desc`, `buffers { default data-size }`) evaluated against
the VPP 26.06 and DPDK 26.03 sources with a verdict each; a clear recommendation for the owner (hardware limit vs tunable,
and the experiments that decide it); a renderer change only when it is justified by the measurements, then with
validation, tests, docs and a PR with passing package tests.
