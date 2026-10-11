# Setup WAN defaults and remove LAN step

User request (2026-10-11): discover and prefill WAN settings from the system, allow administrator changes, remove LAN creation from setup.

Branch: codex/setup-system-wan-no-lan-20261011
Worktree: /root/.codex/worktrees/f077/NGFW
Implementation initially located in codex/root-ngfw-synced-20261003 (d2d55984d), then task commits rebased onto origin/main (76362bdc7). Original desktop checkout ba55d2744 did not contain setup implementation.

Owned files: packages/schema/src/setup.ts and setup.test.ts; apps/api/src/features/setup/controller.ts and controller.test.ts; apps/web/src/domains/system/setup/**; apps/web/src/locales/{en,fa}/setup.json; apps/web/test/e2e/screens/setup.mjs; docs/user/getting-started.md; task status and generated API client artifacts.

Contract helper: codex/wan-only-contract, isolated worktree /root/.codex/worktrees/wan-only-contract/NGFW. Independent reviewer: read-only review_wan_ui.

No lab configuration changes or deployment authorized by this task. WAN detection uses running config and existing read-only agent-backed interfaces/routes endpoints; host management NIC ownership is preserved.
