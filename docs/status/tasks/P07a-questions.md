# P07a — questions / notes for the manager (none blocking; work continued)

1. **WebSocket wire protocol (for P06).** docs/04 only sketches `WSS /api/v1/stream { subscribe: [...] }`. The client
   (`packages/ui-kit/src/ws/client.ts`) implements: client → server `{ "subscribe": string[] } | { "unsubscribe": string[] }`;
   server → client `{ "topic": string, "data": unknown, "ts"?: number }` (one message per sample) and
   `{ "error": { "title"?, "detail"? } }` (non-fatal). The shape is isolated in `handleMessage`/`send`; if P06 chooses a
   different envelope, only those two functions change.
2. **RFC 9457 shape assumed by `<SchemaForm problem>`:** `{ type, title, status, detail, instance, pointer?, errors?: [{ pointer, detail?, title? }] }`
   — a single `pointer` for one-error responses and an `errors[]` array for validation reports. Anything else is shown in
   the form-level alert with its pointer. P06 should confirm or the mapping in `packages/ui-kit/src/schema-form/SchemaForm.tsx` (`problemEntries`) adapts.
3. **Generated domain schemas are still `{}` placeholders on `main`** (P02a/b/c not merged at base `2de6c2f`). `/dev/schema-form`
   renders them honestly as key/JSON editors and says why; the full widget demo uses a schema composed from the real
   `packages/schema` primitives + `withUi()` (`apps/web/src/pages/dev/demoSchema.ts`). Re-check the demo once P02a merges.
4. **`z.fromJSONSchema` ignores non-standard formats** (`cidrv4`, `cidrv6`). Zod's `toJSONSchema` emits a `pattern` next to
   them so generated schemas validate; `compile()` in ui-kit also injects Zod's regex when a schema has only the `format`.
   P02a: keep using `z.cidrv4()`/`z.cidrv6()` (not a bare `format` string) so both sides agree.
5. **Schema titles are English strings inside the schema.** `<SchemaForm translateLabel>` and `nav:domains.<key>` provide
   translation hooks; a `schema` i18n namespace keyed by property path (`interfaces.mtu.title`) is the proposed home for
   fa titles once the domain schemas exist — owner to decide (P02a/b/c vs. P07b).
6. **`x-vrx-ui.dependsOn`** is consumed by the renderer (`string | { field, value?, values? }`, sibling name or absolute `/path`)
   but `packages/schema/src/ui.ts` `UiHints` does not declare it yet (P07 prompt lists it). Adding the optional field is an
   additive contract change for the schema owner; the demo passes it via `as UiMeta`.
7. **Root `eslint.config.js` is shared** (not owned by P07a), so the i18n rule lives in `apps/web/eslint.config.js` and
   `packages/ui-kit/eslint.config.js` (ESLint 9 resolves the nearest config from each package's cwd, which is how turbo runs lint).
   The root package.json lacks `"type": "module"`, so ESLint prints a harmless MODULE_TYPELESS warning — one line for the manager.
8. **`@ngfw/schema` does not export `generateSchemas()`** (it lives in `src/generate.ts`, used only by the `gen` CLI; the
   package exposes just `.`). The web app therefore calls `z.toJSONSchema(RootConfig[.shape[key]], { target: 'draft-2020-12', io: 'input' })`
   itself in `apps/web/src/schema/registry.ts` — the same two calls with the same options. Suggest P02a re-exports
   `generateSchemas`/`componentName` from `src/index.ts` (additive) so there is one call site.
9. **`pnpm install` reports "Issues with peer dependencies found"** (pre-existing, not introduced by the two web deps added here:
   `@ngfw/schema`, `zod`).
