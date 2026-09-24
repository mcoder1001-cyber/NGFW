# P07b — WIP

- 2026-09-24 03:50 — auth (session/refresh/WS subprotocols), pending-change bar, commit dialog, confirm countdown,
  revisions + rollback, users page written; typecheck + lint clean. Next: run against the slot-1 API (+ real agent),
  unit tests, Playwright-core E2E script, screenshots, CI gate.
- Found: packages/api-client dist lacks generated types (paths = any in consumers) → web tsconfig maps the package to
  its source for type-checking (questions #1).
