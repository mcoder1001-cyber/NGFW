# S-proto-rootkeys — derive proto root-key drift guard from the schema

## What
`packages/proto/test/desired-state.test.ts` "has exactly the 13 root keys" failed on origin/main.

Root cause: the test kept a hand-copied `ROOT_KEYS` list (13 keys) "mirroring" `packages/schema`. The `security`
domain (auto-block) was added to both `RootConfig` (schema) and `DesiredState` (proto, field 14), but not to the
test's copy, so the guard rotted. **No real contract drift**: schema ROOT_KEYS and DesiredState fields are the same
14 keys in the same order.

Fix: import `ROOT_KEYS` from `@ngfw/schema` (already a devDependency; parsed-documents.test.ts imports it too) so
the expected set cannot rot; the assertion now reports `{missingFromProto, missingFromSchema}` first, then the
order. Deriving the list also exposed a second stale spot: the "typed construction compiles for every domain"
test never built `security`; added `security: { autoBlock: { enabled: false } }`.

## Verification
- `npx turbo run lint typecheck test --concurrency=1 --filter=@ngfw/proto --filter=@ngfw/schema`:
  `@ngfw/schema Tests 1577 passed`, `@ngfw/proto Tests 104 passed`, `Tasks: 9 successful, 9 total`
  (lint needs `buf` on PATH: `export PATH=$PATH:$HOME/go/bin`; without it `buf lint` exits 127, also on main).
- `tools/ci.sh check`: `check PASSED`.
- Loud failure checked by temporarily adding `'tenants'` to the expected keys:
  `AssertionError: schema ROOT_KEYS vs proto DesiredState fields ... + "missingFromProto": [ "tenants" ]`.

## Out of scope / open questions
- docs/contracts/proto.md still says "field 1–13" / "all 13 domains" (lines ~26, 88, 161); stale since `security`
  (field 14). Not edited (outside this task's files); needs a docs follow-up.
- Go side drift guard (contracttest drift_test.go) untouched; no Go changes.
