# WEB-4b — Pre-built HA/VRRP + cluster screens (UNROUTED)

State: review. Scope: web-ahead (D-123).

## What
- `apps/web/src/domains/system/ha/HaPage.tsx` — System › High availability page with two tabs:
  - **VRRP**: SchemaForm over the `ha.vrrp` sub-schema (record of virtual routers) + a table of virtual routers
    (interface, VRID, family, priority, addresses, engine) with committed / not-committed chips vs running.
    Saving sends an RFC 7396 merge patch (`vrrpPatch`: removed names become `null`).
  - **Cluster**: SchemaForm over `ha.cluster` + a membership summary (node, peers, port, config/state sync).
  - Server problem pointers (`/ha/vrrp/...`, `/ha/cluster/...`) are mapped onto the sub-forms.
- `queries.ts` — candidate/running `ha` queries and PATCH `/api/v1/config/ha` (existing endpoints; no contract changes).
- `locale.ts` — `ha` namespace (en + fa), registered via `i18n.addResourceBundle` from inside the folder so no
  shared file (i18n.ts, locales/) is touched. F-vrrp-config-sync may move it to `locales/{en,fa}/ha.json`.
- `HaPage.test.tsx` — mounts the page on a one-route memory router inside `<App>` (fake API): router table,
  cluster summary, problem-pointer mapping, nav still unavailable, patch helper, en/fa key parity.

## Not routed
No router/nav change: `/system/ha` still renders "Not yet available" (App.test.tsx). F-vrrp-config-sync adds the
route (`element: <HaPage />`), nav availability, and live VRRP state (master/backup) which has no API yet.

## Untested / open
- No live VRRP/cluster state (no state endpoint exists); screens show config only.
- `secretRef` is a plain reference field (no secret picker).
- Not exercised in a real browser.
