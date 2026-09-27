# WEB-4a — Pre-built routing screens: OSPF, IS-IS/RIP, BFD + redistribution (UNROUTED)

State: review. Scope: web-ahead (D-123).

## What
- `apps/web/src/domains/routing/ospf/`
  - `queries.ts` — candidate/running `routing` queries and PATCH `/api/v1/config/routing` (existing endpoints, no
    contract changes); `mergePatch` (RFC 7396 diff: removed record entries such as areas/interfaces/redistribute
    sources become `null`); `routingSubSchema`, `problemUnder` (pointer `/routing/<proto>/…` → sub-form).
  - `ProtocolForm.tsx` — SchemaForm over `routing.<proto>` with committed / not-committed chip vs running and a
    "Remove from configuration" button sending `{ <proto>: null }`. Shared by all three screens.
  - `locale.ts` — `routingIgp` namespace (en + fa) registered via `i18n.addResourceBundle`; no shared file touched.
  - `OspfPage.tsx` — form + areas and interfaces tables.
- `isis-rip/IsisRipPage.tsx` — tabs IS-IS / RIP: form + interfaces table (RIP also networks).
- `bfd/BfdPage.tsx` — tabs BFD sessions (form + sessions table) and a read-only redistribution matrix
  (bgp/ospf/isis/rip × sources; edited in each protocol's form). `redistributionMatrix` helper.
- Tests (`*.test.tsx`) mount each page on a one-route memory router inside `<App>` with the fake API: tables,
  committed state, removal patch `{ ospf: null }`, unconfigured state, matrix, helpers, en/fa key parity,
  nav still unavailable.

## Not routed
No router/nav change. F-ospf, F-isis-rip and F-bfd-redistribution add the routes/nav entries (and may move the
locale to `locales/{en,fa}/routingIgp.json` and `queries.ts` to a shared place).

## Untested / open
- No live state (OSPF neighbours/LSDB, IS-IS adjacencies, BFD session state): no state endpoints exist yet.
- Saving through the full SchemaForm (record editors for areas/interfaces) is not exercised in tests; only
  `mergePatch` and removal are.
- Persian rendering checked only via key parity; not exercised in a real browser.
