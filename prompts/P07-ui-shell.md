# Task P07 — Web UI shell + SchemaForm + pending-change bar   (prepend 00-CONTEXT.md)

This prompt is executed as **two board tasks**; your envelope says which:
- **P07a** (deps: P02s) — items 2, 3, 4, 5, 6 below plus the frame skeleton of item 1 (routes, nav, placeholders); no login, no server calls.
- **P07b** (deps: P07a, P06) — items 1 (login, protected routes), 7, 8, 9, 10 against the real P06 endpoints.

## Goal
The React/MUI application frame every feature screen will plug into: auth, layout, theme,
RTL/i18n, the schema-driven form renderer, the server-side DataGrid wrapper, the WebSocket
client, and the product's signature UX — the pending-change bar with diff and commit dialog.

## Read first
`docs/05-ui-spec.md`, `packages/api-client` (P06 output), `packages/schema/dist/json-schema`.

## Build exactly this
1. **App frame**: login page (username/password → cookie session via api-client), protected
   routes, left nav grouped as in docs/05 (Dashboard, Interfaces, Routing, Firewall/NAT, VPN,
   Services, System, Tools) — only Dashboard, Interfaces, System>Users, System>Revisions are
   real routes now; others render an "not yet available" page, **not** fake data.
2. **Theme** in `packages/ui-kit`: `createVrxTheme(mode, dir)`, light/dark, dense tables,
   semantic status tokens (up/down/degraded/admin-down) used everywhere via `theme.vrx.status.*`.
   RTL via Emotion cache + `stylis-plugin-rtl`; `<html dir lang>` follow the selected language.
3. **i18n**: `react-i18next`, namespaces per area, `en` and `fa` complete for everything in
   this task; Persian digits option; date/number via `Intl`. A CI check fails on any
   hardcoded string in JSX (eslint-plugin-i18next or similar).
4. **`<SchemaForm>`** in `packages/ui-kit`: renders MUI fields from JSON Schema
   (string/number/boolean/enum/array/object/oneOf) + `x-vrx-ui` hints (widget, group, order,
   help, dependsOn). Uses react-hook-form + zod resolver from the same schema. Maps
   problem+json `pointer` errors back onto fields. Storybook or a `/dev/schema-form`
   route demonstrating every widget with the `interfaces` schema.
5. **`<ServerDataGrid>`**: MUI X DataGrid (MIT) wrapper with server-side pagination/sort/
   filter bound to TanStack Query; virtualised; typed columns; empty/loading/error states.
6. **WS client**: single multiplexed connection to `/api/v1/stream`, `useTopic('iface.counters')`
   hook with subscribe/unsubscribe on mount/unmount, reconnect with backoff, 1 Hz buffering.
7. **Pending-change bar**: appears when candidate ≠ running (poll `/config/diff` + WS event);
   shows count, lock owner, buttons Review / Commit… / Discard. Commit dialog: structured diff
   viewer (added/removed/changed with pointers), comment field, "auto-revert in [N] minutes
   unless I confirm" checkbox default ON, then a countdown banner with a Confirm button after
   commit. Handles losing the server gracefully (shows "reconnecting…", keeps countdown local).
8. **Revisions page**: list revisions, view diff between any two, rollback with confirmation.
9. **Users page**: CRUD via SchemaForm from the `management.users` schema (admin only).
10. Tests: Vitest for SchemaForm (each widget, pointer→field error mapping), for the WS hook
    (reconnect), and Playwright E2E: login → edit → pending bar → commit with confirm → confirm
    → revisions shows it → rollback.

## Acceptance
- [ ] Lighthouse a11y ≥ 90 in both themes; keyboard-only path through login→commit works
- [ ] `fa` renders RTL with no `margin-left/right` in the codebase (lint rule)
- [ ] Initial JS bundle < 600 kB gzipped; routes code-split
- [ ] No component opens its own WebSocket (grep)

## Out of scope
Feature screens beyond Interfaces list placeholder (P08 does that), dashboards/charts,
CLI terminal, packet capture UI.
