# Ethernet display consumers — task envelope

- Branch: `codex/ethernet-display-pages`
- Worktree: `/root/ngfw-wt/ethernet-display-pages-20261013`
- Base: `74dc791cd`
- Owner: interface_pages worker; manager: root
- Owned: consumer TSX and tests under `apps/web/src/domains/interfaces`; this envelope and corresponding WIP. Excludes `display-names.ts`, `display-names.test.ts`, interface mapping provider, queries and model contracts.
- Intent: show shared Ethernet aliases in interface-related tables, menus, accessibility labels and dialogs while retaining original API and action identifiers.
- Dependency: ui-kit `useInterfaceLabel` and `InterfaceLabelsProvider`, plus root suffix-free display names and authenticated provider.
- Integration: root sequentially integrates consumer commit and runs mandatory full quick gate; independent review required.
