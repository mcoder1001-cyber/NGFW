# P08 review — final verdict (reviewer, 2026-09-24 14:25)

Branch `task/P08` @ 9863788 (code identical to 82d699d/ffecf13; later commits are docs only). Worktree `/root/ngfw-wt/P08`.
This review continues `P08-review-partial.md`, where items 1, 4–6, 8–10 were settled. Here I settle items 2, 3, 7 and 11. I also
cover the "not yet checked" list: interfaces_test.go, subsystems/stores*.go, InterfaceDrawer.tsx/InterfacesPage.tsx, fake-agent
fidelity and F5. No product code was changed. The only commit is this file.

**Verdict: BLOCK** (last line). The slice is sound and the code is careful.

**Why it blocks:** the branch turns the integration gate red on its own. `TestAgentOnHost` and `TestAgentProcessOnHost` fail
deterministically on P08 and pass on main (N1). The fix is one test fixture plus a `tools/ci.sh full` re-run.

**Lifting it:** a fix round that covers N1, the manager decisions F1/F2/F4, N2 and F5, followed by a verify-only re-review that
also re-runs the topology test. Today that test could not get past its first commit on this VPP (see "Topology re-runs").

## Integration result (item 11) — what ran, where, NRestarts

**VPP stayed up throughout:** NRestarts was **0 before and 0 after every run below** (vpp pid 8760, up since 13:03:29, handover
pending).

**How the runs were made:**
- one Go package at a time
- under `flock -s /run/lock/vrx-lab.lock`
- with `VRX_INTEGRATION=1` and `eval "$(tools/lab env N)"`

Logs are in the reviewer scratchpad `…/a859b866-…/scratchpad/reruns/`; the agent log of the last topology run is in
`/run/vrx-test/w1/p08/`.

| run | tree | slot | result | NRestarts |
|---|---|---|---|---|
| `tools/ci.sh full --base main` (previous reviewer, PID 10894, 13:19–14:01) | P08 @ 3c318d0 | 1 | **EXIT 1** in apps/agent integration. The gate stops there, so `test/topology/interfaces` was never reached | 0 → 0 |
| `go test ./internal/agent/` 14:05 | P08 @ 9863788 | 1 | **FAIL**: TestAgentOnHost, TestAgentProcessOnHost (+2 load flakes, below) | 0 → 0 |
| `go test ./internal/descriptors/df2/idempotency/` 14:09 | P08 | 1 | FAIL (ra-prefix lifetime tick) | 0 → 0 |
| `go test ./internal/agent/` 14:09 | **main @ 52682c3** (`/root/ngfw-wt/ci-main`, `checkout --detach main`) | 12 | **PASS**: TestAgentOnHost 4.87 s, TestAgentProcessOnHost 9.41 s, `ok 21.6s` | 0 → 0 |
| `go test ./internal/descriptors/df2/idempotency/` 14:10 | main | 12 | PASS | 0 → 0 |
| `test/topology/interfaces` #1 14:10 (`go test` direct, not run.sh) | P08 | 1 | FAIL: first commit → **504** after 60 s. VPP API stall (environment, see below) | 0 → 0 |
| `test/topology/interfaces` #2 14:13 (load 11) | P08 | 1 | FAIL: same pattern, first commit → **502** "no answer to Apply … Call cancelled" | 0 → 0 |
| df2 idempotency ×6, 14:15 | P08 / main | 1 / 12 | P08 **6/6 pass**; main **5/6** (1 FAIL, same ra-prefix 7199/3599) | 0 → 0 |
| `TestPendingSurvives…` + `TestGRPCRoundTrip` `-count=5` (fake VPP, load 9) | P08 | – | pass 5/5 | – |

**Classification of every failure seen:**
- **`internal/agent` TestAgentOnHost + TestAgentProcessOnHost: P08 regression (N1).** Deterministic: it failed in the CI run at 13:41
  and in my re-run at 14:05, and passes on main.
- **`renderers/frr` (4 tests), `frrtest` TestHarnessSlotLockSerialises, `renderers/chrony`: environmental (D-106).** tools/app's
  umask 077 created `/run/vrx-test` as 0700 at 13:50, so frr/_chrony got EACCES. **But see N2:** P08's own test, and main's
  `pg-test.sh`, still reset the slot dir to 0700.
- **`df2/idempotency` TestApplyTwiceEmptyPlan: pre-existing timing flake, not P08.**
  - It also fails on main (1 of 6 runs).
  - `go list -test -deps` of that package contains no package P08 changed.
  - The failing op is `ip6-nd.ra-prefix …: valid_lifetime:7199 preferred_lifetime:3599` against a desired 7200/3600. VPP reports the
    *remaining* lifetime, so a one-second tick between Create and Retrieve makes a phantom Update.
  - Owner: DF-2/ip6_nd. Fix: normalize the lifetimes in Retrieve or compare with ±1 s tolerance.
- **`TestPendingSurvivesRestartAndRevertsAfterDeadline`, `TestGRPCRoundTrip` (failed only in my 14:05 re-run): load flakes, not
  P08.** They are sleep-based (50 ms subscription sleep; a 1 s confirm deadline) and an in-memory apply took 3.2 s at load 37. They
  pass 5/5 at load 9 and passed in the CI run.

### Topology re-runs (items 2/3) — environment, not P08 code
**Both runs failed the same way:**
1. The agent's commit creates `host-w1l0` with `af_packet_create_v3` → OK.
2. About 1 s later VPP stops answering API health probes for about 3 s. govpp drops the connection and reconnects 1–9 s later.
3. The in-flight `af_packet_create_v3 host-w1w0` never gets a reply. It waits until the API's 60 s agent timeout cancels it:
   `create failed … context canceled` → ROLLED_BACK → 504/502 to the test.

**Evidence the VPP stall is not P08's:**
- The independent tools/app agent (owner `vrx`, `/var/log/vrx-app/agent.log`) logs the **same stalls at the same moments**. That
  includes 14:14:01–14:14:11, before P08's agent had started, while only `tools/lab rig up` and the test's binapi hand-over were
  creating and deleting af_packet interfaces.
- At present **every af_packet create or delete on this VPP stalls its API for seconds**, for every client. P08's green runs at
  07:46 and 08:12 ran against the previous VPP instance. `startup.conf` is unchanged since 00:21.
- The VPP journal shows host-w1w0 *was* created (fd 30, removed by `rig down`). The reply was lost in the disconnect → I2.

**Decision:** I stopped after two runs. Every run adds af_packet deletes, and every one of those hits the EBADF double-close path
(I1). More churn on the shared VPP is a crash risk for all slots.

**What stands for items 2/3:** the pasted P08.md runs (07:46 and 08:12, NRestarts 6 → 6; the trace checked genuine against VPP
uptime), plus my code read. They are **not reproduced today**. The re-review must re-run the topology test once the af_packet
stall is understood (I6).

## Findings, ranked (each: where · failure scenario · fix)

### N1 — HIGH (blocking). P08 breaks the P05 host integration tests in its own package
- **Where:** `apps/agent/internal/agent/agent_integration_test.go:68-78` (`hostDoc` canonical form) versus
  `apps/agent/internal/desired/interfaces.go:384-385,401-406` (Assemble now always sets `enabled` and `promiscuous`).
- **Failure scenario:**
  - Retrieve now reports `"enabled": false, "promiscuous": false` for every interface.
  - The canonical `want` of both tests lacks those fields, so `waitConverged` gives up after 30 s: `agent_integration_test.go:235`
    and `:398` "not converged within 30 s", got loop101 `enabled:false`.
  - P08 updated the fake-VPP twins in `service_test.go` (4 canonical docs gained `enabled/promiscuous`) but not this
    VRX_INTEGRATION-only file. The branch's CI evidence is quick mode, where these tests skip.
- **Consequence after merge:** main's `tools/ci.sh full` is red, and the gate never reaches `test/topology/interfaces`.
- **Fix:** add `"enabled": false, "promiscuous": false` to both loopbacks in `hostDoc`'s `canon` (lines 71-72). Then run
  `VRX_INTEGRATION=1 go test ./internal/agent/` on slot 1 and `tools/ci.sh full` (after merging main), and paste both in P08.md.

### F1 — MEDIUM. `/state/interfaces` changed the meaning of `items[].config` (manager decision: keep `config` = Retrieve view, add a new field for running)
- **Where:** `apps/api/src/state/state.controller.ts:264-265`. Reader: `apps/cli/internal/cli/cmd_op.go:184-221`.
- **Decision check:** correct. The CLI stays right and the change really is additive.
- **What the fix must also touch:**
  - The web reads `item.config` as the *configured* value: `apps/web/src/domains/interfaces/InterfacesPage.tsx:46,55-57` (MTU/VRF
    fallback) and `model.ts:104` (`addressesOf` for rows not in VPP). Both must move to the new running field.
  - `apps/api/test/e2e/interfaces.e2e.test.ts` asserts `config` = running.
  - Once `config` is the Retrieve view again, the P08-new `actual` means the same thing. Drop `actual` (it was never released) or
    document it as an alias; two fields for one meaning will confuse P09+.
  - `test/topology/interfaces/interfaces_test.go:253-262,330-358` reads `actual`.
  - Correct the "additive" claim in `P08-contract.md` and the stale OpenAPI summary at `state.controller.ts:229`.
  - Regenerate the client in a `contract(...)` commit.

### F2 — MEDIUM. The CLI operations table is stale (manager decision: regenerate)
- **Where:** `apps/cli/internal/api/operations_gen.go` lacks `State_counters`.
- **Status:** `TestOperationsTableMatchesOpenAPI` FAIL (partial review).
- **Fix:**
  - merge main (fc0fe68: `tools/ci.sh` now runs `make -C apps/cli lint test build`)
  - run `make -C apps/cli gen` **after** the F1 contract change, so it is regenerated only once

### N2 — MEDIUM (shared host). P08's test resets the slot run dir to 0700 (the D-106 hazard again)
- **Where:**
  - `test/topology/interfaces/run.sh:13`: `install -d -m 0700 "$RUN"`. `install -d -m` also re-modes an *existing* directory
    (checked in scratch: 0755 → 0700).
  - `test/topology/interfaces/interfaces_test.go:134`: `os.MkdirAll(s.runDir, 0o700)`. If `/run/vrx-test` is missing, it creates
    that directory as 0700 too.
- **Failure scenario:** after a P08 run, `/run/vrx-test/w<N>` is 0700 root. frr/_chrony, which drop privileges, cannot traverse it.
  Slot N's FRR and chrony integration tests then fail with EACCES, which is the 13:54 pattern. On CI slot 12 it breaks the next
  `ci.sh full`.
- **Main has the same problem:** my topology runs (direct `go test`, no run.sh) still flipped w1 to 0700. The cause is main's
  `deploy/dev/pg-test.sh:38` (`install -d -m 0700 "$dir"`), which the test calls. I restored 0755 after each run; it is 0755 now.
- **Fix (P08):** create `/run/vrx-test/<prefix>` as 0o755 (as `frrtest/harness.go:120` does) and keep 0700 only for `p08/`.
- **Fix (manager, main):** the same at `pg-test.sh:38`. `pg.env` itself is already 0600 via umask 077. This completes D-106.

### F5 — LOW-MEDIUM (settled). `defaultTolerant.Update` can leave VPP changed while it reports ROLLED_BACK
- **Where:** `apps/agent/internal/subsystems/tolerant.go:74-94`.
- **Mechanism:** when DF-1 answers `isDefault`, the wrapper first calls `Descriptor.Delete(old)`, which resets VPP to the default
  MTU or rx-mode. Only then does it check `inEffect(new)`. If that check errs or is false, Update returns an error. Examples: a dump
  error, a vanished interface, an rx queue that did not take interrupt mode.
- **The scheduler's revert does not cover it.** A failed Update is recorded as `CodeFailed` with **no journal entry**
  (`apps/agent/internal/scheduler/reconciler.go:905-909`). Rollback only undoes the journal (`reconciler.go:709-727`).
- **Result:**
  - the transaction reports ROLLED_BACK
  - the running config keeps the old MTU
  - VPP stays at the default until a later resync notices
- **Untested:** only the Create no-op has a test (`internal/agent/interfaces_test.go`, MTU 9000). Update-to-default and its failure
  path have none.
- **Fix:**
  - on `isDefault`, return `scheduler.ErrRecreate` from the wrapper's `Update`. The scheduler then runs a journaled `del` + `create`
    (`reconciler.go:896-903`); `create` goes through the wrapper's `Create` (a no-op when in effect, otherwise DF-1's error); and a
    later failure re-creates the old value on rollback
  - add fake-VPP tests for 1400 → default and for `inEffect=false`

### N3 — LOW-MEDIUM (test honesty). The trace assertion can pass without our packet
- **Where:** `test/topology/interfaces/interfaces_test.go:562-564` together with `:287-291`.
- **Failure scenario:** when no trace block matches the run-unique length, `ourTrace` returns "(no trace block…)" plus the first 3000
  characters of the *shared* trace buffer. The node checks (`af-packet-input`, `ip4-lookup`, `ip4-rewrite`, `host-w1w0-output`) then
  match other packets, including earlier runs', and pass. That defeats the purpose of 82d699d.
- **Fix:** return `(block, found)`, call `t.Fatalf` when nothing is found, and assert the nodes only inside the matched block.

### N4 — LOW. The interface drawer can overwrite concurrent candidate edits and discard unsaved edits
- **Where:** `apps/web/src/domains/interfaces/InterfaceDrawer.tsx:100-104`.
- **Failure scenario:** the form opened on an older candidate snapshot, but the merge patch is computed from the *fresh* candidate to
  the form value. So:
  - any field another session changed since the form opened is written back with the form's stale value
  - any member another session added is nulled

  This is the opposite of what `queries.ts:42` intends.
- **Also:** `key={JSON.stringify(formValue)}` (`:220`) remounts the form whenever the candidate refetches (window focus,
  invalidation), which drops unsaved edits.
- **Fix:** diff against the value the form opened with (`createMergePatch(formValueAtOpen, cleaned)`), and use the fresh copy only
  to detect a conflict. Key the form on the name plus an explicit reload.

### N5 — LOW. New sub-interface: errors not mapped to fields, and a new sub can replace an existing one
- **Where:** `InterfaceDrawer.tsx:306` builds the pointer prefix from `subDialog.id`, which is `''` for "add", not from the id typed
  in `SubForm`.
- **Unmapped errors:** `/interfaces/<p>/subinterfaces/<id>/vlanId` becomes `<id>/vlanId` and is only listed as unmapped at the top.
- **Silent replace:** `saveSub` (`:122-133`) with an id that already exists merges over it without a warning.
- **Fix:** lift the id into `DrawerBody` (or build the prefix in `SubForm`), and refuse "add" when the id exists.

### N6 — LOW. Fake-agent fidelity: the e2e cannot see the cases where the real agent differs
- **Where:** `apps/api/src/testing/fake-agent.ts:539-587`.
- **What the fake InterfaceState gets wrong:**
  - lists only *configured* interfaces, all `managed:true`
  - reports sub-interface MTU 9000 (the real agent reports 0 = inherit, which the UI special-cases)
  - reports `tableId` 0 for every VRF
  - never returns an unmanaged live row or a configured-but-absent row (`state:null`)
- **Fake Retrieve:** echoes the applied doc (`:412-424`). The real `Assemble` adds `enabled`/`promiscuous`/`vrf` defaults, which is
  exactly N1's cause, so the fake cannot catch such changes.
- **Untested:** the 501 fallback that `P08-contract.md` promises (`state.controller.ts:281-288`: an old agent → `state:null`).
- **Fix:**
  - seed the fake's live table per test, including an unmanaged live row and a configured-not-live row
  - add an e2e case where the fake answers UNIMPLEMENTED
  - have the fake Retrieve apply the same defaults as `Assemble`

### F4 — LOW (lab path). af_packet on any netdev (manager decision: veth only, agent-side)
- **Where:** `apps/agent/internal/desired/interfaces.go:74,81-83`.
- **Decision check:** correct; it matches D-010.
- **Refinement:**
  - check in the projection/validation phase (`desired.Interfaces` → `s.Errorf(pt, "interfaces.af-packet-veth", …)`). A commit of
    `host-ens192` then fails validation with a JSON pointer (422, running untouched) instead of failing in Create with a rollback
  - keep the check in the af_packet Create as defence in depth

### F3 — LOW. "neighbor" and "description tag" from the task are neither wired nor listed as out of scope
- **Where:** `prompts/P08-vertical-slice-interfaces.md:13`.
- **Fix:** list both in P08.md "Out of scope": description is kept in agent state (D-073b); there is no neighbor leaf in the schema.

### N7 — LOW. stores.go edge cases
- **Where:** `apps/agent/internal/subsystems/stores.go`.
- **Fail-open claim:** `claim` (`:108-130`) records the claim *without* `sw_if_index` when `Resolve` fails (a dump error). `claimed`
  then trusts any interface of that name. That is fail-open, in a store whose loader deliberately fails closed. Fix: return an error
  when an interface claim cannot be bound to an index.
- **Rename not durable:** `atomicWrite` (`:219-237`) fsyncs the file but not the directory after `rename`. Fix: fsync the parent.
- **Checked and fine:**
  - no deadlock (`iface.Dump` does not call the claim store)
  - records carry the D-080 boot identity
  - corrupt files fail closed

### F6 — LOW → INFO. Evidence reproducibility
- **Video:** the envelope made it optional (`P08.envelope.md:19`) and P08.md says so, so it is no longer a finding.
- **Remaining:** the headless-Chrome script behind `TestInterfacesScreenshots` is not committed, so nobody else can regenerate
  `docs/user/interfaces/img/*`. Commit it (npx cache, nothing installed) or say where it lives.

### INFO — not P08 code, for the manager / other owners
- **I1 — V24/D-101: "veth down first" does not neutralise the double close.**
  - Every af_packet delete in my runs logged `vlib_file_update: epoll_ctl() failed … (fd N), errno 9` right after
    `af_packet_fd_error: Network is down`: 14:10:58, 14:12:12, 14:12:17, 14:14:11, 14:15:24, 14:15:33.
  - All of those deletes were done with the veths down (test hand-over, agent rollback, `rig down`).
  - The fds are reused across interfaces: fd 29 was host-w1w0's and then host-w1l0's. That is the fd-reuse double close behind the
    07:27 SIGSEGV.
  - The quiesce order lowers the risk; it does not remove it. Add this to `docs/vpp-code-track.md` V24, and keep af_packet churn on
    the shared VPP to a minimum until it is patched.
- **I2 — a lost reply leaves an untagged orphan.**
  - When the VPP API stalls, govpp's health check (250 ms × 3) disconnects and reconnects. The in-flight request then waits for its
    reply until the caller's deadline (60 s).
  - VPP had created the interface. The failed Create is not journaled, and the interface is untagged (the tag is set after create),
    so neither rollback nor resync removes it. Only `rig down` did.
  - Owner P05 `internal/vpp` + the af_packet descriptor. Fixes:
    - fail in-flight requests with `ErrDisconnected` on disconnect
    - on a create retry, look up the interface by `host_if_name`
    - consider a less aggressive health-check threshold on a shared, loaded host
- **I6 — this VPP instance currently stalls for seconds around every af_packet create or delete, for all API clients.** tools/app's
  agent sees it too (`/var/log/vrx-app/agent.log` 14:11:07–22, 14:14:01–11, 14:14:25–28). Nothing is known to have changed:
  `startup.conf` has not changed since 00:21, and VPP has been up since 13:03. This blocks every af_packet topology test, not only
  P08's. It needs a manager look (possibly with I1) before the P08 re-review re-runs the topology test.
- **I3 — the integrated agent becomes the globals owner.** tools/app starts it with `VRX_OWNER=vrx` and no `VRX_GLOBALS_OWNER`
  (`tools/app:108`). With P08's `ConfigFromEnv` (`agent.go:65-74`), that agent becomes globals owner on the shared host. Harmless
  now, since no global descriptor is registered. Set `VRX_GLOBALS_OWNER=0` before P11 registers ipsec/ikev2 `WithGlobalsOwner`.
- **I4 — edits outside the owned file list.** P08 edited `apps/agent/internal/descriptors/core/coretest/{fakevpp.go,ifext.go}`
  (additive fake-VPP support), which is outside its exclusive file list (`P08.envelope.md:18`). Harmless; list it in P08.md.
- **I5 — the Errors column includes drops.** `InterfacesPage.tsx:58` sums `errors + drops` under "Errors" (`col.errors`). Rename it
  "Errors / drops" or split the column.

## Checklist (final)
| # | item | result |
|---|---|---|
| 1 | Contract | Additive proto + client with contract commits. F1 is the meaning change, handled by the manager decision. |
| 2 | Real verification | By code and pasted evidence: Retrieve, `vppctl show int/address`, trace (see N3), counters vs WS ±5 %, DF pings through the af_packet rig. **Not reproduced today** (VPP af_packet stall, I6); re-run at re-review. |
| 3 | Restart safety | Stop agent → binapi delete (quiesced) → start → recreated and ping < 30 s by agent log timestamps (pasted 07:46/08:12). No new VPP type without Retrieve. No VPP restart: NRestarts 0 → 0 in every run today. |
| 4 | Provenance | OK (partial review). |
| 5 | Shared host | Prefix/ports/DB/lock/cleanup OK. **N2** (slot dir 0700). I1/I6 for the manager. |
| 6 | Security | OK (partial review). F4 decision on file. |
| 7 | Transactions | MTU and address rollback proven on VPP (pasted). **F5:** a failed tolerant Update is not reverted. I2 is outside P08. |
| 8 | UI honesty | Real endpoints, no stubs, screenshots present. N4/N5 are correctness details. |
| 9 | Scope | OK (I4 noted). |
| 10 | i18n | OK (partial review). The type tokens (af-packet, loopback, sub-interface) are data. |
| 11 | Tests actually run | Quick gate green (matches P08.md). **Full gate red because of N1 (P08)**; every other failure is classified above as environmental or pre-existing. |

**Cleanup by the reviewer:**
- `ci-main` detached at main 52682c3, as instructed
- `/run/vrx-test/w1` restored to 0755
- rig w1 down; no w1 objects in VPP; no w1 veths or netns
- `vrx_w1` dropped by the test cleanups
- the reviewer's CI build outputs (`apps/agent/bin`, `apps/{api,web}/dist`, gitignored) removed
- no process left running

**BLOCK** — the branch fails the integration gate deterministically in its own package (N1).

**Required in the fix round:**
- N1, then `tools/ci.sh full` green on the branch, with any remaining failures classified
- F1/F2/F4 as decided by the manager
- N2
- F5

**Recommended:** N3–N7 and F3.

**Re-review:** verify-only, including one topology + restart-safety run once I6 is resolved.
