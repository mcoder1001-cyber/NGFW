# WAN-only setup contract task envelope

Branch: `codex/wan-only-contract`
Worktree: `/root/.codex/worktrees/wan-only-contract/NGFW`
Owned files: `packages/schema/src/setup.ts`, `packages/schema/src/setup.test.ts`, `apps/api/src/features/setup/controller.ts`, `apps/api/src/features/setup/controller.test.ts`; generated schema outputs and task status files.

Implement optional legacy LAN inputs and WAN-only builder/API support for the owner-requested WAN auto-prefill and removal of LAN wizard step. Without LAN selection, preserve existing LAN interfaces, DHCP, NAT, ACL and management. Existing legacy LAN calls remain valid. Focused schema/API tests required. Parent performs generation, independent review and final integration gate. No lab mutations.
