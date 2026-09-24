# Task: W-seed — wave-A hotspot anchors and shared seams, no behaviour change   (prepend 00-CONTEXT.md)

## Goal
Up to 17 wave-A/B features will run in parallel. They all register in the same ~25 shared files, the "hotspots" in
`docs/status/wave-A-hotspots.md` §1. Make one commit that seeds **anchors** and **seams** in those files, so that each feature
later adds one line directly under its own anchor and merges without union conflicts. **No behaviour change**: every test
that is green before your commit is green after it, with the same counts.

## Read first
- `docs/status/wave-A-hotspots.md`: §0 (rules), §1 (the hotspot table: your work list), §2 (numbers), §3 (per-task hotspot ids), §4.2
- `docs/status/wave-A-launch-plan.md` §5.2 (extra seams to seed)
- `docs/decisions/LOG.md` D-109 (the decisions this seeding implements) and D-112 (merge procedure: squash + rebase)

## Scope — build exactly this
1. **Anchors.** Add one comment line per touching task, in the form `// wave-A: <task-id>` (`#` or `{/* */}` where the language
   needs it), at the insertion point the hotspot table names. Touching tasks per file come from §3. Anchors go in:
   A1 `subsystems.go` (`Domains` slices one name per line; the `Register()` end), A2 `projection.go` (`project()`, `assemble()`),
   A4 `server.go` (turn `Action` into a type switch with a default `Unimplemented`, plus three case anchors: vrf ping,
   neighbors arp-flush, nat session-kill), C1 the domain objects in `interfaces.ts`/`vrfs.ts`/`routing.ts`/`services.ts`
   (one anchor per feature key; no new keys), C2 `semantic/index.ts`, C3 `schema/index.ts`, C5 `dataplane.proto` (end of
   `service Dataplane`, and a `// ----- <task-id> -----` section stub per feature at the end of the file), C6 a "Feature RPCs"
   heading in `docs/contracts/proto.md`, P1 `app.module.ts` (import + controllers + providers), P4 `agent.client.ts`,
   P5 `fake-agent.ts`, P6 `bus.ts` TOPICS, W1 `router.tsx`, W2 `nav.ts` + `nav.test.ts`, W3 `i18n.ts`.
2. **One entry per line.** Reformat the single-line lists named in W2 (`BUILT_DOMAINS`, the test's `available` list),
   W3 (imports, `NAMESPACES`, the `en`/`fa` literals) and A1 (`Domains` slices) to one entry per line. Content stays identical.
3. **Seams (launch plan §5.2)**, each an empty, compiling, tested hook:
   - `subsystems.Env` gets an event-publish hook and a Resync hook (A5). Default no-op; one unit test proves the default is inert.
   - `subsystems.SlotIDRange()` returns the slot's table/id range from the environment. Unit test.
   - Empty page shells for the `vpn` and `services` web domains: the route is registered but hidden from nav until a feature
     fills it (no user-visible change). i18n keys exist in en and fa.
4. Proto and schema changes are anchors and comments only. The generated files must regenerate **byte-identical** (`pnpm gen` →
   `git status` clean apart from your source edits). If a comment leaks into generated output, keep it; never hand-edit generated files.

## Acceptance (paste into docs/status/tasks/W-seed.md)
- [ ] `pnpm gen && git status --porcelain` shows only your source files (no generated drift beyond comments)
- [ ] `pnpm lint && pnpm typecheck && pnpm test` and `make -C apps/agent lint test` and `make -C apps/cli lint test`: same pass counts
      as on your base (paste both)
- [ ] `grep -rn "wave-A:" apps packages docs | wc -l` and the per-file anchor list
- [ ] `TMPDIR=/tmp/g-w3 tools/ci.sh --base main` green

## Out of scope (do not build)
Any feature logic, schema field, proto field or RPC; any route that a user can reach; any change to P08's interface
behaviour; removing anchors (the manager does that after the wave); `tools/ci.sh`; host tests (none needed).

## Base
You are branched from `task/P08` (its fix round is still running; D-114 speculative start). Merge `task/P08` into your branch
again before your final CI (`git merge task/P08`), and list any conflict you resolved in W-seed.md.
