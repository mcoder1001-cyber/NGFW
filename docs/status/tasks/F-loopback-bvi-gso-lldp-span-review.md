# F-loopback-bvi-gso-lldp-span — review

Reviewer session 2026-09-25. Branch `task/F-loopback-bvi-gso-lldp-span` @ 552d29f7, speculative base `task/F-bridge-l2@a735aa9`.
Contract commits: cb1c84f5 (schema), fb6418d5 (proto), b2041a4d (api-client). The worker's CI passed at c97508f3.
Nothing was run against VPP. No host tests, no vppctl, no trace. VPP facts come from reading `/root/vpp` (26.06, c3200b88d).

## Verdict: APPROVE WITH CHANGES

The feature is well built:
- Descriptors are declarative and built on dfkit/df7.
- VPP message names come from binapi only.
- The contract is additive, and its numbers match the D-109 allocation, with no collision on any branch.
- The evidence is real and complete: SPAN and ERSPAN, LLDP, the BVI, restart at +0.61 s, rollback, and the 400 pointer.
- The tests are green here.

There is no H finding. Six M items must be done before the merge. They belong in the **rebase round** that the branch needs anyway: its base is not on main, and TD-23, TD-11c, F-bridge-l2, F-rpf-adl-pbr and WEB-1 come first. After that round, a **focused re-verify** of M1–M6 plus L2 is enough; a full re-review is not needed.
- Two items are nsim VPP-crash and CPU hazards read from the VPP source (M1, M2).
- Two are TD-11b and TD-23 rebase obligations that are not mechanical, so the merger must not do them (M3, M4; D-134).
- One is a false safety claim in the user guide (M5).
- One is that the forms are never submitted in a test (M6).

## What I ran (real output)

Go, with the race detector, over the touched descriptor packages, desired, subsystems, core/coretest and agent. VRX_INTEGRATION and VRX_NSIM_HOST were unset.
```
$ cd apps/agent && go test -race -count=1 ./internal/descriptors/{gso,nsim,span,lldp,core}/... ./internal/desired/... ./internal/subsystems/... ./internal/agent/...
ok  	ngfw/agent/internal/descriptors/gso	1.235s
ok  	ngfw/agent/internal/descriptors/nsim	1.329s
ok  	ngfw/agent/internal/descriptors/span	1.187s
ok  	ngfw/agent/internal/descriptors/lldp	1.350s
ok  	ngfw/agent/internal/descriptors/core	1.391s
?   	ngfw/agent/internal/descriptors/core/coretest	[no test files]
ok  	ngfw/agent/internal/desired	1.265s
ok  	ngfw/agent/internal/subsystems	1.242s
ok  	ngfw/agent/internal/agent	13.738s
```
The first run of `internal/agent` failed three socket tests with `bind: invalid argument`. My TMPDIR was the long session scratch path, and the unix socket path went over 108 characters. The rerun with a short TMPDIR passed, as above. This was my environment, not the branch.

Web. The worker's cleanup had removed `dist/` from the four workspace packages, so I first built them with `tsc` only: no `gen`, and no tracked file changed. I removed those `dist/` directories afterwards.
```
$ pnpm --filter @ngfw/web test
 ✓ src/domains/interfaces/loopback-bvi-gso-lldp-span/pages.test.tsx (4 tests) 12032ms
 Test Files  16 passed (16)
      Tests  104 passed (104)
```
Schema, the feature's semantic rules (D-105, mirror rules, nsim rules):
```
$ npx vitest run src/semantic/loopback-bvi-gso-lldp-span.test.ts
 ✓ src/semantic/loopback-bvi-gso-lldp-span.test.ts (24 tests) 584ms
```
Merge simulations (`git merge-tree --write-tree`, read-only):
- against `task/F-bridge-l2@8e6741d4`, which includes its fix round: **clean**.
- against `task/F-rpf-adl-pbr@fb1120ea`, which is already squashed: 9 conflicts.
  - 4 generated files: regenerate them.
  - `coretest/fakevpp.go`: handled by M4.
  - `docs/vpp-code-track.md`: append both.
  - import lines at the top of `interfaces.ts` and `services.ts`, which have no anchor: take the union of both.
  - `routing.ts`: F-bridge-l2's hunk from the base.
- against `main`: 42 conflicts, which is expected. The base carries P08 and W-seed unsquashed, while main merges squashed (D-112). The branch must be rebased onto main; merging it as it is will not work.

## Findings

| id | sev | where | finding | fix |
|---|---|---|---|---|
| M1 | M | `apps/agent/internal/descriptors/nsim/nsim.go:73-83` | The agent's `Config.Validate` does not bound the scheduler wheel; only the API's semantic rule does (`NSIM_WHEEL_SLOTS_MAX`). VPP crashes on an oversized model: `nsim_wheel_alloc` (`src/plugins/nsim/nsim.c:116-129`) calls `clib_mem_vm_alloc`, which returns 0 when mmap fails (`vppinfra/mem.h:292-294`). The result is only `ASSERT`ed, and the next line writes `wp->wheel_size`, a NULL dereference (SIGSEGV) in a release build. The schema maxima allow about 2·10⁹ slots (~62 GB). The agent is the only process that talks to VPP, so it must enforce the bounds that crash VPP itself (D-064 spirit). | Mirror the bound in `Config.Validate` with the same per-thread formula and cap (delay × bandwidth / 8 / packet size + 1 ≤ 2²⁰). Add a unit test with the schema maxima. |
| M2 | M | `nsim.go:452-456`, `subsystems/loopback_bvi_gso_lldp_span.go:72-75`, docs | nsim is safe on this host but not on a product box. The single global cross-connect pair is fine: D-071 guarantees one writer, and the key is `nsim.cross-connect/global`. Two VPP hazards remain. **(a) Worker boxes:** without `nsim { poll-main-thread }`, `wheel_by_thread[0]` is never allocated (`nsim.c:196-198`). `nsim_inline` dereferences `wp` behind an `ASSERT` only (`node.c:201-207`). So any frame the main thread sends through `nsim-output-feature` (an API or CLI ping, other main-thread control traffic) makes VPP SIGSEGV. **(b) Irreversible polling:** `nsim_configure` sets `nsim-wheel` to POLLING (`nsim.c:203-209`), and nothing ever sets it back. Any polling input node stops the main thread from sleeping (`vlib/file.c:139`). On a box without workers, one core spins at 100 % until VPP restarts. "Leaves the model inert" (user guide :125, `en/…json:72`) understates this. | Register nsim only when `GlobalsOwner` **and** an explicit lab opt-in are set (an agent config or env flag, off by default in the product). `nsim.config` Create refuses with a clear error when VPP has workers (the thread count from binapi `show_threads`), unless an explicit override says `poll-main-thread` is set. Add (a) and (b) to V-new (c), `docs/agent/descriptors/nsim.md`, the user guide and the nsim page warning. The opt-in host test (`VRX_NSIM_HOST=1`) should run only just before a planned VPP restart. |
| M3 | M | `span/span.go:143-147`, `lldp/lldp.go:226-235`, `gso/gso.go:170-192`, `nsim/nsim.go:268-279, 398-406` | TD-11b obligation (TD-11b-questions Q3 names **lldp** and **span**: "each row switches to `Target.ClaimFirst` … when it wires its family"). Every Create here writes VPP first and claims afterwards. On an untagged interface (product NICs), a failed claim leaves an enable that Retrieve cannot see and a rollback does not remove. In the same way, `gso` Create returns nil Meta when `store.Put` fails after the enable (`gso.go:186-191`), so the rollback's Delete skips the disable. The base predates TD-11b (`ClaimFirst` and `scheduler.PartialCreate` do not exist there), so this is expected, but it is not a mechanical swap. | In the rebase round: `c, err := tg.ClaimFirst(ctx)` before each VPP write, then `c.Adopt()` or `c.Undo(err)`. A write after the enable returns `scheduler.PartialCreate(err)` with Meta (TD-11b Q2). Add one claim-failure test per family (the TD-11b `claimfirst_test.go` pattern). |
| M4 | M | `core/coretest/loopback_bvi_gso_lldp_span.go:139-156`, `core/coretest/fakevpp.go:98-99` | Q8 is not "one registration line". On TD-23 (`task/TD-23@8ce21c49`), the core `install()` owns `feature_is_enabled` and dispatches to `RegisterFeatureIsEnabled` answers. Core install records no owner, so this extension's `v.On("feature_is_enabled", …)` would **silently replace** the dispatcher; `VPP.On` does not panic. That breaks F-rpf-adl-pbr's adl/urpf answers and F-bridge-l2's mactime answer, which is exactly the silently-winning handler D-134 forbids. | The worker, not the merger (D-134), should: drop the fakevpp.go hunk; add `func init() { RegisterExtension("loopback-bvi-gso-lldp-span", (*VPP).installLoopbackBviGsoLldpSpan) }` in the feature's own coretest file; move the gso-ip4 answer to `RegisterFeatureIsEnabled("gso-ip4", …)`; delete the replicated mactime branch. |
| M5 | M | `docs/user/interfaces/loopback-bvi-gso-lldp-span.md:114-116` | False safety claim: "where they differ the agent refuses the interface loudly (`ErrIndexMismatch`) instead of enabling LLDP on another interface." DF-7's Create enables **first**, then detects the mismatch and deliberately does not undo it (`lldp/lldp.go:236-247`). LLDP then runs on another hardware interface until VPP restarts; it could be another owner's port or a product NIC. No binapi message exposes `hw_if_index` (grep over `apps/agent/binapi`), so a pre-check is not possible through the API. | Correct the text: LLDP may be enabled on another hardware interface, the agent reports the error but cannot undo it, and LLDP should be used only on interfaces created at start-up. Add one line to the LLDP page's info alert. |
| M6 | M | `apps/web/src/domains/interfaces/loopback-bvi-gso-lldp-span/pages.test.tsx:163-249`, `NsimPage.tsx`, screen `nsim-en.png` | No test submits the LLDP or the nsim SchemaForm. The tests cover the table, Refresh, the mirror-removal patch and the fa/RTL render only. On this base, without WEB-1's presence toggle, the nsim form materialises the optional `crossConnect` with empty required A and B (visible in `nsim-en.png`). A model without a cross-connect then cannot be saved from the UI (Q7). | Merge after WEB-1. Add one submit test per form: LLDP with empty management fields, and nsim without a cross-connect. Both should assert the PATCH body. |
| L1 | L | `packages/schema/src/domains/services.ts:680-683`, `apps/agent/internal/desired/lldp.go:17-20` | Q5 wording. The schema help still says "Defaults to system.hostname", which is no longer true. The comment in lldp.go says "the screen offers the hostname as the value to save", but `LldpPage.tsx` has no hostname logic; `vrx-w7` in the screenshot came from the test config (`shots_test.go:66`). | Fix the comment. Change the help text to "empty keeps VPP's current name"; this is text only, not a reshape, but it goes in a `contract(schema):` commit. A "use hostname" button is optional. |
| L2 | L | `desired/lldp_services_seam.go`, `desired/gso_lldp_mirror_nsim.go:23-25, 37-39`, `subsystems/loopback_bvi_gso_lldp_span.go:33`, `{gso,lldp,span,nsim}/ownership.go` | Rebase leftovers: <ul><li>Deleting the seam file leaves the `reportUnsupportedServices` hook as dead code.</li><li>`servicesDomain` duplicates `subsystems.Services` as a string.</li><li>`requirePersistent` is copied four times.</li></ul> | At the rebase: <ul><li>Delete the seam file **and** the hook variable and its call.</li><li>Use `Services`.</li><li>Replace the four copies with `dfkit.CheckClaims` / `dfkit.CheckBoot` (Q6).</li></ul> |
| L3 | L | `docs/user/interfaces/loopback-bvi-gso-lldp-span.md:143` | The guide says "converged … in 0.26 s", but the pasted evidence is +0.61 s (reconcile 0.428 s). | Quote the evidence. |
| L4 | L | `apps/agent/internal/agent/rpc_loopback_bvi_gso_lldp_span.go:143` | `lldpWalk.Lock()` ignores ctx. While one walk is stuck, callers queue even after their deadline. TD-9 bounds the VPP calls themselves, so this is minor. | Optional: use a ctx-aware semaphore (a `chan struct{}` of size 1 with `select` on `ctx.Done()`). |
| L5 | L | screens | Cosmetic: <ul><li>The drawer fieldset shows the raw slug `loopback-bvi-gso-lldp-span` (Q9).</li><li>The LLDP status chip is truncated ("neighbour he…").</li><li>The mirror destination is a free-text field although the schema hints `interface-picker`.</li><li>fa pagination shows the English "of" (DataGrid locale; P08/ui-kit, not this branch).</li></ul> | See Q9. The rest is polish for week 4. |

### For the manager (not this branch)

**ifsanitize should clear inherited span and LLDP state.** Owner: ifsanitize is manager-owned (TD-3; TD-5 holds the cap). Proposed: a small row, or an addendum to TD-22, plus a manager VPP window to reproduce V-new. V-new was found by reading the source and has not been reproduced.

What is inherited:
- **span source state.** `span.c` has no interface-delete hook, so a reused **source** index inherits the destination bitmap and `num_mirror_ports`. The feature arcs themselves are cleared on delete.
- **A silent functional bug follows from it.** A new session on that index never re-enables the span feature, because `span.c:66-72` enables only when the previous count was 0. Yet `sw_interface_span_dump` reports the session. So Retrieve == desired while no copies are made.
- **LLDP.** An inherited `lldp_intf_t` can be detected with `lldp_dump` and disabled, but only where sw == hw (V20).

What to do: ifsanitize, on interface Create, disables every `sw_interface_span_dump` entry whose **source** is the new index (a safe no-op on the feature counts), and handles LLDP as above.

Stale state on the **destination** side stays with the source's owner (D-071). This branch's `span.mirror` Delete already clears it on that owner's next resync; see §2.

**Other items for the manager:**
- Record Q4's pattern as sanctioned: read-back is trusted only together with a boot record of the running VPP. It is already used by F-bridge-l2 mactime, F-rpf-adl-pbr adl, and gso here.
- Q2: add this slug to `SIBLING` in `examples.test.ts` when convenient.

## 1. Architecture

**Rules (00-CONTEXT, 01-architecture):**
- Node never talks to VPP: the API reads only the agent RPC (`LoopbackBviGsoLldpSpanController`).
- The agent is declarative. `gso.interface` and `nsim.*` implement Create/Update/Delete/Retrieve/Dependencies on dfkit (D-077).
- The DF-7 lldp and span descriptors are reused, not rebuilt (D-104).
- There is one schema: the pages build their forms with `z.toJSONSchema` of the `packages/schema` Zod schemas, the API types come from the generated client, and the proto drift guard is green in CI.
- Message names come only from binapi (`gso`, `feature`, `nsim`, `span`, `lldp`). The only string literals are the arc and node names `ip4-output`/`gso-ip4`, as in F-bridge-l2.
- No secrets are involved.

**Q4 — gso readable instead of write-only: sound, accept.**
- *Why it is not an echo.* Retrieve reads real VPP state (`feature_is_enabled`) and trusts it only when a D-080 boot record (index + name) exists for the running VPP. That guards against V23(a): VPP returns `VNET_API_ERROR_INVALID_SW_IF_INDEX` cast to true for an index the arc never reached (`feature.c:341-344`). So D-063's "no echo of desired state" holds.
- *Why it does not stack, and heals.* D-076's "no stacking" holds too. A lost enable is detected and re-applied: `applied && !on` leads to a single enable.
- *The normalise loop cannot spin.* A disable grows the arc vector before its early return (`feature.c:259-266`), so one disable clears the out-of-range answer.
- *The host proves it:* three Creates and one Delete leave it off, and it is not inherited on a reused index (feature arcs are cleared on delete).
- *Advantage over write-only:* GSO shows in `/state/interfaces` `config`, and drift is real. The envelope's "write-only" is superseded; see "For the manager" above.

**Q5 — no systemName default in the agent: correct.**
- The agent stores only its implemented domains, so after a restart a resync would not know `system.hostname` and would change the value.
- VPP keeps its name when the name is empty: `lldp_cfg_set` ignores an empty vector (`lldp_cli.c:196-201`).
- Only the wording is wrong (L1).
- I advise against the API filling the name when saving, because it would silently change the stored config. If the product wants the hostname, the LLDP page should offer a button.

**nsim, globals-owner only, as a lab tool.**
- The single global pair is safe under D-071. It has one writer, and the product stack on this host runs `VRX_GLOBALS_OWNER=0` (`tools/app:108`), so the UI cannot reach the shared VPP.
- The hazards are VPP-side (M1, M2). "Globals owner" alone is too weak a gate for a tool with a known crash path on worker boxes.
- Keep nsim in Tools, marked "lab" (the envelope's open question), but gate the agent side (M2).

**Globals-owner flag.** It reaches the projection through a process-wide atomic in `subsystems`, the same pattern as F-rpf-adl-pbr's `rpfAdlPbrState`. That is consistent, so there is no finding.

## 2. Safety on the shared VPP

**Stale-destination cleanup in `span.mirror` Delete (`span.go:170-193`, `reresolve` + `staleIndex`): correct.**
- The VPP handler validates no index (`span_api.c:24-37`). `span_add_delete_entry` clears the destination bit of *our* source even when the destination is gone (`span.c:51-67`). A disable with state 0 is allowed with any destination.
- Retrieve spells the vanished index as `#<sw_if_index>`, and Delete parses it back.
- If the index was reused, Retrieve shows the new name, and Delete resolves it and clears only our source's bit.
- Tested by `TestStaleDestinationCleared`, including "the other session untouched".
- The remaining window is between an out-of-band delete of the destination and the next resync. During it, the span nodes copy to a freed index, a crash candidate per V-new (a). That window is inherent.

**Should ifsanitize clear these?** Yes, for source-side and LLDP inheritance. It is manager-owned; see "For the manager".

**D-105: satisfied.**
- `interfaces.loopback-bvi-gso-lldp-span-reserved-loopback` refuses loop16000–16383, including zero-padded names such as `loop016000`.
- It sits in contract commit cb1c84f5, with the pointer on the offending key.
- It is covered by unit tests and by the e2e test (`/interfaces/loop16001`).

**D-132: satisfied.**
- LLDP table and mirroring page: `refetchInterval` 30 000 ms plus a Refresh button.
- The agent serialises LLDP walks with one mutex (L4 is a nit).

**nsim host test.**
- It is opt-in (`VRX_NSIM_HOST=1`) and holds the globals lock exclusively (D-082).
- Its model uses a 2-slot wheel, so M1 cannot trigger, and this host has no workers, so M2(a) cannot trigger.
- **It is not side-effect free:** M2(b) leaves the main thread polling until VPP restarts. Run it only in a manager window just before a planned restart.
- Reconfiguring runs under the API barrier: the nsim handlers are not mp-safe, and `api_shared.c:545-548` syncs for them. So there is no use-after-free with workers, but in-flight buffers leak.

**LLDP after an interface delete.**
- VPP brings the interface down on delete, and `lldp_sw_interface_up_down` unschedules it (`lldp_node.c:285-303`). So there is no TX to a freed hw interface.
- `lldp_dump` then reads a stale pool slot (`lldp_api.c:115-116`). That is a wrong index, not a crash, in a release build. V-new (b) is accurate.

**Mirror loops.** VPP marks clones and never re-mirrors them (`span/node.c:74, 93`). The `mirror-loop` semantic rule is defence in depth.

**Host evidence, pasted in the status file: convincing.**
- `show interface span`: loop775 → gre778 (rx) and → loop776 (both). The GRE fixture is type erspan, so this is real ERSPAN.
- `show lldp`: loop780 hears its own LLDPDU, TTL 121.
- `show bridge-domain 7750 detail`: BVI-Intf loop775.
- `show interface features`: shows gso-ip4, gso-l2-*, and span-input/-output.
- Restart simulation, with dependents deleted first (D-095c): every object is back at +0.61 s with no API call. GSO came back on a new index (5 → 6), so the boot record correctly did not match.
- Rollback to rev A: the span dump is empty, `feature_is_enabled` is false, `lldp_dump` does not list loop780, and Retrieve shows `gso=<nil> mirror=[]`. Rollback to rev 1: the loopbacks are gone.
- The 400 has pointer `/interfaces/loop775/mirror/0/destination`.
- NRestarts was 2 → 2 during this task's run. The 04:27 crash (Q10) came from another slot; the timeline supports that.
- No trace, classify sweep or rig packets were used.

## 3. Contract

The contract is additive, and the numbers are exactly the D-109 allocation (wave-A-hotspots §2, lines 80-81). I checked for collisions over all 29 `task/*` branches and main:

| message | this branch | other branches |
|---|---|---|
| `Interface` | `gso` 20, `mirror` 21 | F-bonding `bond` 13 · F-bridge-l2 `l2` 14 · F-neighbors-ra 15–17 · F-rpf-adl-pbr `urpf` 18, `adl` 19 · P12 `lcp` 22 |
| `ServicesConfig` | `nsim` 9 | F-rpf-adl-pbr `auto_sdl` 8 · `qos` 7 on main |
| new messages / RPC | `MirrorSession`, `NsimService` (+ nested `CrossConnect`), `LldpNeighbors{Request,Response}`, `LldpNeighbor`, `rpc LldpNeighbors` | no other branch defines any of these names |

The RPC is under the service anchor, and the messages are in the `// ----- F-loopback-bvi-gso-lldp-span -----` section. The fake-agent stub and proto.md §11 are present.

For the D-112 squash: the branch touches schema, proto and api-client, so the single commit's subject must start with `contract(schema,proto,api-client):`. The CI contract guard reads the branch's commit subjects.

## 4. TD-11b ownership declarations

Main's `subsystems.Register` runs `persist.Declared` + `persist.Check` over every registered descriptor (`stores.go:450-460`), including in unit tests. After the rebase, these declarations are exercised for real. With the persisted `IfaceClaims` and `FileBootStore` (`Persistent()` true, `dfkit/claimfirst.go:16`), they will pass.

| descriptor | declaration | records in | correct? |
|---|---|---|---|
| `gso.interface` | CheckPersistent | DF-1 claims + BootStore | yes |
| `nsim.config` | CheckPersistent | BootStore | yes (no interface, no claim) |
| `nsim.cross-connect` | CheckPersistent | claims + BootStore | yes |
| `nsim.output` | CheckPersistent | claims + BootStore | yes |
| `lldp.interface` | CheckPersistent | claims only (write-only, but idempotent through the `lldp_dump` check, no boot record) | yes |
| `span.mirror` | CheckPersistent | claims on the untagged source | yes |
| `lldp.global` | RecordsNoOwnership | nothing: re-applies `lldp_config`, globals owner only | yes |

**Q6: accept.** Replace the local `requirePersistent` copies with `dfkit.CheckClaims` / `dfkit.CheckBoot` at the rebase. That part is mechanical (L2). The claim-first half of the TD-11b obligation is not mechanical (M3).

## 5. Merge order

**Order:** TD-23 → TD-11c → F-bridge-l2 (held by D-125 and D-134) → F-rpf-adl-pbr (D-131) → WEB-1 (M6) → this branch.
- This branch is rebased onto main by the **worker** in one round: M1–M6 and L1–L3, delete the seam, TD-23 registration, ClaimFirst.
- Then a focused re-verify, then the D-112 squash, then `tools/ci.sh --base main` with main's own copy of the script. The worker's CI used a patched out-of-tree copy (D-127/D-128b), which a rebase makes unnecessary.

**No duplicate `Services` constant: confirmed.**
- `subsystems.go` gets only a comment line under the new-domain anchor. The names are appended from `init()` in the feature's own file (`Domains[servicesDomain] = append(…)`), which runs after the package-level map literal. It composes with F-rpf-adl-pbr's `Services: {rpfAdlPbrAutoSdl}` entry: there is no second key and no second constant.
- `subsystems.go`, `projection.go`, `service_test.go` and `agent_integration_test.go` merge **cleanly** with `task/F-rpf-adl-pbr`.
- Until `lldp_services_seam.go` is deleted, Go refuses to build ("ServicesMembers redeclared"), which enforces the deletion. The rule strings are identical (`agent.write-only`, `agent.unsupported-field`; `rpf_adl_pbr.go:89-90`).

**P08 test hunks: byte-identical, except as the status file says.**
- `git -C /root/ngfw diff task/F-rpf-adl-pbr task/F-loopback-bvi-gso-lldp-span -- apps/agent/internal/agent/{service_test,agent_integration_test}.go` shows only F-rpf-adl-pbr's two `feature_is_enabled` allowance lines (`service_test.go:157-161, 463-467`), which are theirs.
- The `"services": {}` canonical hunks and the `implementedDomains()` assertions are identical.
- `projection_test.go:55` (`withoutWriteOnly`) exists only on this branch. It is narrow: it filters only this feature's five write-only descriptors, so it hides no other family's round-trip regression.

**Q8:** see M4. It is not one line in fakevpp.go but two registrations in the feature's own file, plus removing the direct `feature_is_enabled` hook.

## 6. UI

Three screens, each in en and fa/RTL; the screenshots were taken against the real stack.
- **LLDP** (Interfaces): settings form plus neighbour table.
- **Port mirroring** (Interfaces): all sessions, live `active` and `pending` marks, and an edit dialog.
- **Delay simulator** (Tools): with the "lab tool" chip and an orange VPP-wide warning.

**Checks:**
- The GSO switch and mirror sessions appear in P08's generated drawer, with no drawer code and no `model.ts` exclusion.
- The pages call no `dropPhantomOptionals`.
- The forms come from the one Zod schema.
- The en and fa key sets are equal (tested).
- The RTL layout is correct: mirrored columns, pagination, and a right-to-left form.

**Q9 — the raw group name in the drawer:** recommend (b), a generic fallback in the web/ui-kit track. The group title comes from `<slug>:group.title`, with the key in the feature's own namespace. Every wave-A feature uses group = slug under hotspots rule C1, so one change fixes bridge-l2, this branch and all later ones. (a), keys added to `interfaces.json` at the merge, is acceptable only as a stop-gap. It is not a blocker.

## 7. Tests and evidence

- The unit and agent-level coverage is broad. It covers: stacking fakes (D-076), -76 before the model, owner scoping in the RPC, limit 1001 → INVALID_ARGUMENT, UNAVAILABLE when disconnected, slot agent vs globals owner, idempotent re-apply, and GSO not stacked after resync.
- Gaps:
  - the form submits (M6);
  - the claim-first tests after the rebase (M3);
  - an agent-side wheel-bound test (M1).
- The default gate has only fake-client evidence for nsim, as the envelope intends; the status file says so.

## Recommendations on the questions

| Q | recommendation |
|---|---|
| Q2 examples in the proto fixture corpus | Accept. The manager adds the slug to `SIBLING` in `examples.test.ts` (P02-owned) when convenient; then copy the fixture to `packages/schema/examples/`. Not blocking. |
| Q3 services seam, merge F-rpf-adl-pbr first | Accept; D-131 order. At the rebase, delete `lldp_services_seam.go` **and** the `reportUnsupportedServices` hook (L2), and use `subsystems.Services`. The drift view's `/services/lldp` entry disappears with F-rpf-adl-pbr's `COVERAGE_RULES` line. |
| Q4 gso readable | Accept (§1). The manager records the pattern "read-back gated by the running VPP's boot record". |
| Q5 no hostname default | Accept (§1). Fix the wording (L1). A "use hostname" button on the LLDP page is optional. The API must not fill the name. |
| Q6 TD-11b declarations | Accept; all seven are correct (§4). Swap to `dfkit.CheckClaims`/`CheckBoot` at the rebase (mechanical), **plus** ClaimFirst (M3, not mechanical). |
| Q7 D-132 / WEB-1 | D-132 is satisfied. Merge after WEB-1, and add the form submit tests (M6). |
| Q8 coretest hook → TD-23 | Two registrations in the feature's own file: `RegisterExtension` and `RegisterFeatureIsEnabled("gso-ip4")`. Drop the fakevpp.go hunk and the replicated mactime branch (M4). The worker does it, not the merger. |
| Q9 raw drawer group name | (b), a generic `<slug>:group.title` fallback in the web track (§6); (a) only as a stop-gap. |
| Q10 VPP crash 04:27 | Accept: not slot 7. This branch's gate ran in unit mode and its host runs before and after show NRestarts unchanged. |
| envelope: LLDP globals on slot agents | As the evidence shows: slot agents report `systemName`/`txHold`/`txIntervalSec` as `agent.unsupported-field`, and VPP keeps its values. |
| envelope: nsim in the product UI | Keep it under Tools, marked "lab", and gate the agent side (M2). |
