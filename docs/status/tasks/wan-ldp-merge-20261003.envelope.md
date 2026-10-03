# Task envelope: WAN/LDP prepared-source integration

Owner instruction: merge ready works to completion and keep three reviewers active.
Developer: root; readonly reviewers: native_ipsec, drift, restart_socket.
Branch and worktree: codex/wan-ldp-merge-20261003, /root/.codex/worktrees/bbd9/NGFW/.scratch/wan-ldp-integration.
Owned files and scope: see accompanying WIP; additive port only, no generated contracts or host daemon configuration.
Checkpoint each coherent change immediately and publish within 15 minutes. Preserve reviewed histories remotely before final single-commit integration atop actual main. Merge only exact-head complete unchanged local and hosted quick PASS plus independent approvals. Observe actual main CI after merge. Do not classify monitor or LDP foundations as completed full features.
Host: readonly system state and isolated unit socket tests only. No VPP restart, host privileges, sysctls or daemon installs. Continue reviews through final main CI; report only meaningful changes.
