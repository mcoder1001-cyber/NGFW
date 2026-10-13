# Ethernet display consumers — work in progress

- Branch: `codex/ethernet-display-pages`
- Local base: `74dc791cd`; remote checkpoint: not published yet (manager coordinating publication).
- Owned files: interface drawer/table, bonding drawer/table, bridge drawers/table/record picker, LLDP and mirroring pages, consumer regression tests; excludes mapping infrastructure.
- Completed code: formatting consumer labels through shared context; raw drawer caption and table tooltip removed; raw API keys/action arguments retained.
- Actual checks: `git diff --check` passed.
- Remaining: obtain shared context dependency; run consumer tests and ESLint; independent review and complete quick CI on integration tree.
- Current failure: original app-parent worktree disappeared externally; edits recovered from command history into locked `/root/ngfw-wt` worktree. Tests await ui-kit dependency implementation.
- Next command: apply shared context commit for local validation, then `pnpm --filter @ngfw/ui-kit build` and targeted interface Vitest suite.
