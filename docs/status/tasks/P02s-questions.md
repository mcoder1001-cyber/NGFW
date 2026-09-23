# P02s — questions / notes for the manager (none blocking; work continued)

1. **`wt.sh push` breaks git on the host.** `rsync -a` from the desktop (uid 1000) sets the owner of
   `/root/ngfw-wt/<id>` (the directory itself + every pushed file) to that uid; git then fails with
   `fatal: detected dubious ownership in repository`. Workaround used: `chown -R root:root /root/ngfw-wt/P02s`
   after each push (my worktree only; no global git config touched). Suggested fix in `tools/operator/wt.sh`
   (not my file): add `--chown=root:root` (or `--no-o --no-g`) to the `push` rsync.
2. **`docs/contracts/schema.md`** (P02 prompt item 7) lives outside `packages/schema/**`, so P02s did not create it.
   The layout/ownership/conventions text is in `packages/schema/README.md`; proposal: P02a creates
   `docs/contracts/schema.md` (it owns index/primitives/gen) and P02b/P02c append their sections.
3. **Contract guard on a `task/` branch.** `tools/ci.sh --base main` only checks for a commit subject starting with
   `contract`; the branch is `task/P02s`, not `contract/P02s`. I used the subject `contract(schema): …` on
   `task/P02s`. If the manager wants the branch renamed, it is a one-liner on the host.
4. **Coverage tooling.** `@vitest/coverage-v8` is not installed, so the "100% branch coverage" acceptance for
   primitives/validators (P02a) cannot be measured yet. Adding it is a devDependency + lockfile change — P02a
   should add it (or the manager pre-installs it on main) so all three groups measure the same way.
5. **`.prefault({})` instead of `.default({})`** on the root keys (D-016 below). Both emit `default: {}` in JSON
   Schema; only prefault fills nested field defaults on `RootConfig.parse({})`. Say so if you prefer `.default`.
6. **OpenAPI component names** are `RootConfig` + `<Key>Config` (`InterfacesConfig` …); domain JSON Schemas carry
   `default: {}` and `x-vrx-ui: { order }`. P06 (API) and the UI SchemaForm should build on these names —
   flag if a different convention is already assumed.

## Decisions taken (to be copied to docs/decisions/LOG.md by the manager)
| date | id | decision | options considered | why | reversal cost | tasks |
|---|---|---|---|---|---|---|
| 2026-09-23 | D-016 | Root keys use `<Key>Schema.prefault({})`; domain schemas must accept `{}` | (a) `.default({})` (b) `.prefault({})` | (b) runs the domain schema so nested defaults are filled; identical JSON Schema | trivial | P02* |
| 2026-09-23 | D-017 | Semantic validators live in `semantic/<key>.ts` (one array per domain) aggregated by `semantic/index.ts`; registry class + `validateSemantics()` | (a) one registry file all groups edit (b) per-domain arrays | (b) avoids three groups editing the same file | trivial | P02*, P06 |
| 2026-09-23 | D-018 | `x-vrx-ui` hints via `withUi()` → Zod `.meta()`; Zod 4 copies meta into JSON Schema/OpenAPI | (a) side table of hints (b) `.meta()` | (b) zero extra plumbing, one source | low | P02*, UI |
| 2026-09-23 | D-019 | Placeholders use `z.looseObject({})` (Zod 4 name for passthrough) and root `z.strictObject` | (a) deprecated `.passthrough()`/`.strict()` (b) Zod 4 functions | same semantics, not deprecated | trivial | P02s |
| 2026-09-23 | D-020 | Pointers/diff/merge-patch shipped as small working implementations (RFC 6901/6902 ops/7386), arrays are diff leaves | (a) throwing stubs (b) minimal working code | P02a/b/c can use them immediately; tests define the contract | low | P02a |
