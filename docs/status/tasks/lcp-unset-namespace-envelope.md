# LCP unset namespace recovery

User authorization: fix recurring appliance commit failure `basepolicy.namespace` caused by malformed namespace API readback. Host root@172.30.126.250:22. Preserve the user's staged wizard and password, management NIC ens192, firewall boundaries and VPP process.

Branch: codex/fix-lcp-unset-namespace
Base: deployed source 4c8d1b247c6b
Worktree: /home/mohammad/.codex/worktrees/lcp-namespace-recovery/NGFW
Owned files: apps/agent/internal/subsystems/base_policy.go; apps/agent/internal/subsystems/base_policy_namespace.go; apps/agent/internal/subsystems/base_policy_namespace_test.go; docs/status/tasks/lcp-unset-namespace-*.md.

Design: malformed binary API namespace alone is never root proof. Corroborate through fixed read-only `show lcp` via generated CliInband. Accept only one exact unset status with no conflicting/default namespace lines, controls or oversized output. Keep valid API reads and all failure paths strict. No public contract, privilege or VPP source changes.

Review: independent design APPROVE; implementation review pending.
