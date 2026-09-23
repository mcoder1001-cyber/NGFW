# Decision: VDOM / multi-tenancy — DEFERRED

- Decided 2026-09-23 by the product owner: **not in the 21-day plan.** Schema stays flat.
- TNSR has no VDOM (only VRF); this stays a later differentiator.

## Guardrails to keep the retrofit cheap (all agents follow these)
1. VRF is a first-class field on interfaces, routes, NAT, ACL attachments and IPsec — never assume VRF 0 in code paths or API routes.
2. Object names are unique **within their domain**; do not build global-uniqueness assumptions into API paths beyond `/config/<domain>/<name>`.
3. RBAC role assignments are stored as `{ role, scope: "*" }` — `scope` will later become a tenant name.
4. UI navigation and the pending-change bar are driven by the schema's top-level keys, not hardcoded lists.
5. Kea/Unbound renderers are written so that "one instance per <something>" is a parameter, not an assumption.

Estimated retrofit cost with these guardrails ≈ 1.5× of building it now; without them ≈ 3×.
