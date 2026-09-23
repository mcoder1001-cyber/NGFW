# @ngfw/schema — the configuration contract

Zod source of truth for the whole VRX configuration document (`docs/04-api-datamodel.md`). One definition,
three consumers: TypeScript types (`tsc`), JSON Schema 2020-12 per domain (UI form renderer) and OpenAPI 3.1
components (API) — all produced by `pnpm gen` into `dist/`.

**Changing this package requires a `contract(schema): …` commit** (enforced by `tools/ci.sh --base main`).
Renaming or reshaping an existing field is always a PENDING decision (`docs/decisions/decision-policy.md` #1).

## Layout and ownership

| path | purpose | owner |
|---|---|---|
| `src/index.ts` | `RootConfig` (strict root, every key optional-with-default via `.prefault({})`), `ROOT_KEYS`, re-exports | P02a |
| `src/domains/<key>.ts` | `<Key>Schema` + `<Key>Config` type for one top-level key | see below |
| `src/semantic/<key>.ts` | `<key>Validators: ValidatorDefinition[]` for that domain | same as the domain |
| `src/semantic/registry.ts`, `semantic/index.ts` | `SemanticRegistry`, `validateSemantics()` → `{ pointer, message }[]` | P02a |
| `src/semantic/unique.ts` | `duplicateIssues()` — uniqueness of list item keys (use `ipKey`/`prefixKey` for addresses) | P02a |
| `src/secrets.ts` | `redactSecrets(doc)`, `secretPointers(doc)` driven by `x-vrx-ui.secret` (D-046) | P02a |
| `src/primitives.ts`, `src/ip.ts` | shared field primitives (addresses, prefixes, names, numbers, secrets, time zone) and pure IP arithmetic (`parseCidr`, `prefixesOverlap`, …) | P02a |
| `src/ui.ts` | `withUi(schema, { title, description, widget, group, order, help, secret, itemKey })` → `x-vrx-ui` hints, merged with the wrapped schema's hints | P02a |
| `src/pointer.ts`, `src/json.ts` | RFC 6901 pointers (escape `/` in VPP interface names!), JSON helpers | P02a |
| `src/diff.ts`, `src/merge-patch.ts` | structured `diff(a, b)` → `{ op, pointer, from, to }[]`; RFC 7386 `mergePatch`, `mergePatchAt(doc, pointer, patch)` (`MergePatchError` on `__proto__`/`constructor`/`prototype`) | P02a |
| `src/validate.ts` | `validateConfig(doc)` = tier (a) schema + tier (b) semantic → `{ ok, config }` / `{ ok: false, tier, issues }`; `pointerIssues()` | P02a |
| `src/generate.ts`, `src/gen.ts` | pure generator + CLI writing `dist/json-schema/*.json`, `dist/openapi-components.json` | P02a |
| `vitest.config.ts` | coverage thresholds, per group (D-048): 100 % on group (a)'s semantic files and the shared helpers; groups (b)/(c) add their own entries | P02a |
| `examples/*.json` | fixtures, each group tests its own (`examples.test.ts` = group (a)); valid ones carry no secret leaf (proto drift corpus) | all |

Domain groups: **P02a** system, dataplane, interfaces, vrfs, routing, management · **P02b** nat, objects, acl ·
**P02c** vpn, tunnels, services, ha. A group edits only its own `domains/<key>.ts`, `semantic/<key>.ts` and
fixtures. Need a shared primitive that is not here? Add it in your domain file and write a questions file.

## Conventions

- Every field goes through `withUi()` so the form renderer gets `title` + `x-vrx-ui`; use the primitives.
- Every modelled object is `z.strictObject` (unknown keys are rejected with a pointer); records validate their keys.
- Every domain schema must accept `{}` (root `prefault`); model "required" settings as defaults or semantic rules.
- Protocol / feature blocks that can be off are `.optional()` objects (absent = disabled), not `enabled: false` shells.
- Single-object consistency (families match, sequence numbers unique …) is a `.refine()` with a `path`; anything that
  looks at another object or domain is a semantic validator.
- Semantic validators are pure functions of the whole document returning `{ pointer, message }[]`, named
  `<domain>.<rule>`, pointers built with `jsonPointer()`.
- Guardrails from `docs/decisions/vdom.md`: VRF first-class on interfaces/routes/NAT/ACL attachments/IPsec; names
  unique per domain only; role assignments `{ role, scope: '*' }`; UI iterates `ROOT_KEYS`; per-instance
  renderer parameters. Secrets (PSKs, keys, password hashes) are referenced or write-only — never emitted.

## Commands

`pnpm gen` · `pnpm typecheck` · `pnpm lint` · `pnpm test` (Vitest, unit) — all run by `tools/ci.sh` ·
`pnpm test:coverage` (v8, thresholds in `vitest.config.ts`). Field-level documentation: `docs/contracts/schema.md`.
