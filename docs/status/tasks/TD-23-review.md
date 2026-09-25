# TD-23 review — shared test seams: fake-agent Action dispatch + coretest fakevpp extension registry (D-134)

Reviewer pass over `task/TD-23` (4 commits on `main@b5e74c08`, tip `b1fb3290`), against
`/root/ngfw-wt/TD-23.envelope.md`, `docs/status/tasks/TD-23.md`, LOG D-129/D-134.

## Verdict: APPROVE WITH CHANGES

Test infrastructure only, scope respected, core safety claim independently verified true. Two
small, cheap fixes are worth doing before/at merge (F1, F5); the rest are non-blocking follow-ups.

## Scope (dimension 1) — PASS

`git diff $(git merge-base main HEAD) HEAD --stat` touches exactly: `apps/api/src/testing/fake-agent.ts`,
new `apps/api/src/testing/fake-agent-action.test.ts`, `apps/agent/internal/descriptors/core/coretest/fakevpp.go`,
new `.../coretest/extensions_test.go`, and `docs/status/tasks/TD-23*`. No feature handlers, no product code.

## Fake-agent Action dispatch (dimension 2)

- **The `destroy()` claim is correct — verified empirically, not just by reading source.** Built two minimal
  reproductions of `@grpc/grpc-js`'s `ServerWritableStreamImpl` pattern (constructor installs
  `this.on('error', err => { this.pendingStatus = ...; this.end(); })`; `_final` calls `call.sendStatus(...)`):
  calling `.destroy(err)` fires the `'error'` listener but the subsequent `this.end()` is a no-op on an
  already-destroyed stream, so `_final`/`sendStatus` **never** runs — confirmed no `_final` log fires.
  Calling `.emit('error', err)` directly (what `fake-agent.ts:664-684` now does) runs `_final` and would call
  `sendStatus`. This matches `node_modules/@grpc/grpc-js@1.14.5/build/src/server-call.js:108-155`
  (`ServerWritableStreamImpl`) exactly. The dispatcher (`fake-agent.ts:659-687`) never calls `destroy()` on
  any path — good.
- Dispatch on the oneof case (`actionKindOf`, `fake-agent.ts:92-93`), unregistered kind → UNIMPLEMENTED
  (`:671-676`), handler throw → mapped status via `asGrpcError` (`:678-681`) — all correct and covered by
  `fake-agent-action.test.ts`'s four tests, which pass (see Tests below).
- **F2 (medium, non-blocking): no explicit deadline on the hang-detecting tests.**
  `fake-agent-action.test.ts`'s `client.action({...})` calls (lines 63, 84, 100) pass no `CallOptions`, even
  though the generated client supports one (`packages/proto/gen/ts/vrx/v1/dataplane.ts:42868`,
  `action(request, options?: Partial<CallOptions>)`). A regression to `call.destroy()` would not hang forever —
  `apps/api/vitest.config.ts:11` sets a global `testTimeout: 30_000` — but it would fail slowly (up to 30 s)
  with a generic Vitest timeout instead of a fast, specific `DEADLINE_EXCEEDED`. The file's own docstring says
  a regression "shows up as a hang, not a pass" (`fake-agent-action.test.ts:18`), which is true but weaker than
  intended. Recommend adding e.g. `{ deadline: Date.now() + 2000 }` to the three `client.action(...)` calls.
- **F3 (low, non-blocking): the dispatcher's `try/catch` only catches synchronous throws.**
  `handleServerStreamingCall` is typed `(call) => void` (grpc-js `server-call.d.ts:109`); an `async` handler
  that throws after an `await` returns a rejected Promise the surrounding `try/catch` at `fake-agent.ts:678-681`
  will not observe, so the call hangs — the exact class of bug this seam exists to prevent. Today's only
  concrete handler (`arpFlush` in F-neighbors-ra) is synchronous and handles its own errors inline via
  `call.emit('error', …)`, so this isn't triggered yet, but it's an easy trap for a future async handler (the
  NAT "kill" action TD-23.md itself flags as pending). Suggest a one-line README caveat that `ActionHandler`
  must be synchronous or must catch and emit its own errors, or wrap the call as
  `Promise.resolve(handler(call)).catch(err => call.emit('error', asGrpcError(err, status.INTERNAL)))`.
- **F4 (informational): registration is a module-global, not per-`FakeAgent`-instance.**
  `actionHandlers` (`fake-agent.ts:69`) is shared across every `FakeAgent` in a test file/worker;
  `resetActionHandlersForTest()` (`:85-89`) is correctly called in this file's own `afterEach`
  (`fake-agent-action.test.ts:46`), but nothing enforces that future feature test files do the same — there is
  no global Vitest `setupFiles` hook (`apps/api/vitest.config.ts` has none; the file at
  `apps/api/test/support/global-setup.ts` isn't wired to it either). Vitest's default file isolation contains
  the blast radius to one file, so this is a convention risk, not a cross-file leak — worth a line in the
  README comment near `resetActionHandlersForTest` telling every registering test file to call it in
  `afterEach`.

## Coretest fakevpp extension registry (dimension 3)

- `RegisterExtension` is applied by every `New()` via `installExtensions()` (`fakevpp.go:97-107`, called at
  `:102`) — confirmed.
- `VPP.On` (`fakevpp.go:162-175`) panics only when two **different** extension names claim the same message
  name while `installingExt` is set; core setup (`install`/`installIfExt`, run before `installExtensions`) and
  an extension replacing its own earlier registration are unaffected — matches
  `TestOnExtensionCollisionPanics` and `TestOnSameExtensionReplacesItsOwnRegistration`, both pass.
- **Panic vs. `t.Fatalf`**: reasonable given the API shape. `RegisterExtension` (`:133-142`) is called from
  `init()` at package-load time, before any `*testing.T` exists, so `t.Fatalf` isn't an option there — panic is
  the only mechanism. `On`'s collision panic fires inside `New()`, which also takes no `*testing.T`; changing
  that signature to thread one through would ripple into every call site in the codebase for a rare
  copy/paste-class bug. Since `New()` is always called synchronously from within a test's own goroutine (no
  test in this branch calls it from a helper goroutine), a panic here fails just that one test with a full
  stack trace — acceptable, and consistent with `fake.Client`'s own style elsewhere.
- **F6 (informational): the registry test pollutes real global state, contained today only by there being no
  sibling test file.** `TestRegisterExtensionCompose` and `TestRegisterExtensionDuplicatePanics`
  (`extensions_test.go:30-57`, `:107-115`) call the real, package-level `RegisterExtension` — there is no
  Go-side equivalent of `resetActionHandlersForTest()`. Those registrations (`td23-test-compose-a/b`,
  `td23-test-dup`) are permanent for the rest of the `coretest` package's test binary. `ls
  apps/agent/internal/descriptors/core/coretest/*.go` shows `extensions_test.go` is the only test file in the
  package today, so nothing else observes this, but a future test file added to this exact package should be
  aware the registry carries this residue. Not worth a fix now; worth a one-line comment.
- **Race safety**: `extAll` is guarded by `extMu` on every read/write (`RegisterExtension` and
  `installExtensions` both lock; `installExtensions` copies the slice under the lock before iterating). Per-VPP
  state (`installingExt`, `onOwner`) lives on the `*VPP` instance, not the package, so concurrent `New()`
  calls from different goroutines are independent. `go test -race -count=1
  ./internal/descriptors/core/...` passes (below), though no test in the repo today calls `coretest.New()`
  under `t.Parallel()` (checked with `grep -rl t.Parallel apps/agent/internal | xargs grep -l coretest` — no
  hits), so this path is race-clean by inspection and by the existing suite, not yet exercised under real
  parallel load.

## The real conflict: F-rpf-adl-pbr × F-bridge-l2 on `feature_is_enabled` (dimension 4)

Confirmed by reading both branches directly (not just TD-23.md's summary):

- `task/F-rpf-adl-pbr`'s `coretest/rpf_adl_pbr.go` (`v.On("feature_is_enabled", …)`) dispatches on
  `(req.ArcName, req.FeatureName)`: `device-input`/`adl-input` → its own ADL model;
  `device-input`/`ethernet-input` → `false`; anything else → `true` (its own comment: "V23 (a): an error reads
  as true", i.e. it models VPP's real unknown-feature behaviour).
- `task/F-bridge-l2`'s `coretest/bridge_l2.go` (`v.On("feature_is_enabled", …)`) **ignores `ArcName` and
  `FeatureName` entirely** and answers from its own mactime model keyed only by `SwIfIndex`
  (`m.mtCount[idx] > 0 || !m.reached[idx]`).

These are genuinely incompatible, not just two features that happen to touch the same message: whichever
installs second under plain `fake.Client.On` silently wins for *every* `feature_is_enabled` call, breaking the
other feature's model with no error. `VPP.On`'s collision panic is exactly the right backstop and will fire,
by name, whichever of these two branches rebases second — that part of TD-23 is load-bearing and correctly
built.

**Proposed resolution for the merger, at the second branch's rebase:**

Add one more, narrowly-scoped composable seam — specifically for `feature_is_enabled`, not a generic
per-message multiplexer for every VPP message. Reasoning: of the ~40 features rebasing onto this fake, the
large majority hook messages that are naturally one-feature-owned (`bond_create2`, `svs_route_add_del`,
`ip6_ra_config`, …) and will never collide; `feature_is_enabled` is structurally different — it's a generic,
arc-wide query that *any* feature gating itself on a device-input/output arc will want to answer, so it is the
one message guaranteed to keep attracting new claimants. Building a fully generic multiplexer for every message
today would be solving a problem that (per the actual diffs read across all 7 waiting branches) doesn't exist
anywhere else yet.

Concretely:

```go
// In fakevpp.go, core-owned (installed once by install(), before any extension, so it never
// itself trips the On collision guard):
func RegisterFeatureIsEnabled(featureName string, fn func(*VPP, *feature.FeatureIsEnabled) *feature.FeatureIsEnabledReply)

// install() gains one v.On("feature_is_enabled", ...) that looks up the registry by
// req.FeatureName (or (ArcName, FeatureName) if two features ever reuse a bare name) and falls
// back to IsEnabled: true for anything unregistered — VPP's own unknown-feature cast, already
// documented by F-rpf-adl-pbr's V23(a) comment.
```

Whichever of F-rpf-adl-pbr / F-bridge-l2 rebases second replaces its own `v.On("feature_is_enabled", …)` with
`RegisterFeatureIsEnabled("adl-input"/"ethernet-input", …)` or `RegisterFeatureIsEnabled("mactime", …)`
respectively (confirm at rebase which exact `FeatureName` string mactime's real production code queries with —
not verified here, out of TD-23's scope).

**Should TD-23 build this now?** Recommend **yes, as a small immediate follow-up landed before either
waiting branch rebases** (not scope creep into TD-23 itself, whose envelope explicitly excludes it, and not a
reason to hold this review). The collision is real and known today, not hypothetical; letting the panic surface
for the first time live, during a rebase under the merge lock, is exactly the kind of avoidable stall the
manager's standing orders ask to prevent. TD-23 as shipped is safe either way (the panic guarantees the
collision cannot be silent), so this is a scheduling recommendation, not a blocking condition on this review.

## Registration-line spot-check (dimension 5)

Checked all 7 rows against the branches' actual diffs (not just TD-23.md's prose); already read
F-rpf-adl-pbr and F-bridge-l2 in full for the conflict above (both accurate), plus:

- **F-neighbors-ra — accurate.** `coretest/fakevpp.go` diff shows exactly one inserted line,
  `v.installNeighborsRa()`; `features/neighbors-ra/fake.ts` has a whole `action:` override handling
  `arpFlush` and falling through to a generic UNIMPLEMENTED — matches the row's proposed one-line
  `registerActionHandler('arpFlush', …)` replacement.
- **F-vrf-static-ecmp — accurate, with one informational note.** Confirmed no diff at all to `fakevpp.go`
  (matches "none" in the row). `features/vrf-static-ecmp/fake.ts`'s `action:` override also special-cases
  `traceroute` with a bespoke message ("VPP has no traceroute API (fake)"); collapsing to the row's proposed
  single `registerActionHandler('ping', vrfPing)` line means an unregistered `traceroute` falls through to the
  dispatcher's *generic* UNIMPLEMENTED message instead. Not a TD-23 defect — just something F-vrf-static-ecmp's
  own rebase should check its tests don't assert the old string.
- **F-bonding — row is factually wrong (Finding F1, medium, TD-23.md:87).** The row reads: *"had a direct
  `v.installBonding()` line in `New()`. Drop it; add `func init() { coretest.RegisterExtension(...) }`."*
  The actual diff (`git -C /root/ngfw diff df67a8e task/F-bonding -- .../coretest/fakevpp.go`) shows no such
  call — F-bonding instead **independently declared its own `var extensions []func(*VPP)` package-level slice
  plus a `for _, ext := range extensions { ext(v) }` loop directly in `fakevpp.go`**, the same ad hoc pattern
  TD-23.md itself attributes to F-rpf-adl-pbr and F-nat44-ed-sessions (`coretest/bonding.go`'s
  `func init() { extensions = append(extensions, (*VPP).installBonding) }`). The prescribed fix (drop it, add
  one `RegisterExtension` call) is still the right outcome, but the description of what's currently there
  undersells the conflict surface: the merger doing this rebase should expect to remove a full
  redeclared-`var`-plus-loop hunk from `fakevpp.go`, not a single call line. **Recommend fixing this row in
  `docs/status/tasks/TD-23.md` before/at merge** — it's a one-line doc correction in a file TD-23 already
  owns, and the whole value of this table is the merger trusting it over re-reading every diff themselves.

## Tests (dimension 6)

```
$ cd apps/agent && go test -race -count=1 ./internal/descriptors/core/...
ok      ngfw/agent/internal/descriptors/core          1.383s
ok      ngfw/agent/internal/descriptors/core/coretest 1.064s

$ pnpm --filter @ngfw/api test -- testing
 ✓ src/testing/fake-agent-action.test.ts (4 tests) 185ms
 Test Files  11 passed (11)
      Tests  100 passed (100)
```

Both green. Did not rerun `tools/ci.sh --base main` (worker reports it passed; nothing observed here
contradicts that).

## Summary of findings

| id | severity | area | finding |
|----|----------|------|---------|
| F1 | medium | docs | TD-23.md:87 (F-bonding row) misdescribes the branch's actual hook as a single call line; it's really a redeclared `var extensions []func(*VPP)` + loop. Fix before merge. |
| F2 | medium | TS tests | `fake-agent-action.test.ts` sets no gRPC deadline; hang-detection relies on Vitest's global 30 s timeout instead of a fast, specific one. Non-blocking; recommend adding `{ deadline: ... }`. |
| F3 | low | TS dispatcher | `try/catch` around `handler(call)` only catches synchronous throws; an async handler rejecting after an `await` reproduces the destroy()-class hang. No current handler is async, but nothing stops a future one. Suggest a README caveat or a `Promise.resolve(...).catch(...)` wrapper. |
| F4 | informational | TS tests | `actionHandlers` is a module global reset only by convention (`afterEach` in each consuming file); no global Vitest hook enforces it. |
| F5 | — (see resolution above) | Go registry | F-rpf-adl-pbr × F-bridge-l2 `feature_is_enabled` collision is real; recommend a narrow `RegisterFeatureIsEnabled` seam landed before the second branch's rebase (not blocking TD-23). |
| F6 | informational | Go tests | `extensions_test.go`'s own tests permanently pollute the package-level registry (no Go-side reset); harmless today only because it's the sole test file in the package. |

None of F1–F6 block this merge; F1 is cheap enough it should just be fixed. TD-23's core safety property —
every path through the Action dispatcher ends the call with a status, never `destroy()` — was independently
verified against `@grpc/grpc-js`'s actual stream implementation, not taken on the worker's word.
