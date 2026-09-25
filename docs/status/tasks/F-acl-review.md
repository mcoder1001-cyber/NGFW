# Review — F-acl (task/F-acl @ a6fcdff2; contract 410b5471)

Reviewer, 2026-09-25. Scope: this task's own changes. The branch sits on `task/F-object-model@c3965939` and merged main
at `6fe374ce` (= main `ae359680`). `task/F-object-model` has since been squashed to `97316879` (same product tree as
c3965939 plus its verify doc), so `git diff task/F-object-model HEAD` also shows main's TD-8/TD-7/TD-20 in reverse. I
reviewed `git diff $(git merge-tree --write-tree task/F-object-model ae359680) HEAD`: 74 files, +18 085 / −929, F-acl
only.

## Verdict: **APPROVE WITH CHANGES**

The architecture holds. The agent stays declarative: the DF-4 descriptors are wrapped, not bypassed. VPP names come
only from binapi; the one CLI (`show acl-plugin tables mask`) is read-only, sent through binapi `cli_inband`, and used
because V7 has no getter. Node reaches VPP only through the `AclState` RPC. The contract is additive and paged. The
projection, caps, v4/v6 split, schedules and FQDN handling are right. The D-066 foreign-first ordering is proven on
the host, and D-132 is respected: no binary-API walk on any timer.

One defect must be fixed before the merge:
- **H1.** A DryRun rewrites the record that Retrieve and AclState use. After `POST /config/validate` of a candidate
  whose VPP content is unchanged, Retrieve reports the unapplied candidate (a false drift), and the counters of the
  running rules disappear.

The M items are small: a docs correction, a test-hygiene fix, the review-3.4 obligation, and rebase work. A
**focused re-verify** of H1, M1, M3 and M4 is enough; a full re-review is not needed.

### Summary in Persian (for the product owner)
- **معماری سالم است:** عامل اعلانی مانده، نام پیام‌های VPP فقط از binapi آمده، Node فقط با RPC `AclState` کار می‌کند، و قرارداد افزایشی و صفحه‌بندی‌شده است.
- **ایراد H1 پیش از ادغام رفع شود:** اجرای DryRun (دکمهٔ «اعتبارسنجی») رکوردی را که Retrieve و شمارنده‌ها از آن می‌خوانند بازنویسی می‌کند. نتیجه: گزارش drift کاذب و گم‌شدن شمارنده‌های قوانین فعال. آزمون probe آن را بازتولید کرد.
- **سوئیچ شمارنده‌های ACL در VPP:**
  - یک تنظیم سراسری است و در محصول فقط globals owner آن را روشن می‌کند.
  - با `enable=false` می‌شود آن را خاموش کرد؛ بنابراین جملهٔ V7 و مستند کاربر دربارهٔ «خاموش‌نشدن» نادرست است.
  - با هر بار ری‌استارت VPP صفر می‌شود و الان خاموش است.
  - هزینهٔ آن برای هر بسته اندک است، ولی صفر نیست.
- **صفرشدن شمارنده‌ها:** شمارنده‌ها با هر تغییر فهرست صفر می‌شوند (`acl_add_replace`)، از جمله با تغییر زمان‌بندی یا FQDN؛ مستند کاربر عکس این را می‌گوید.
- **پیام ۴ مگابایتی gRPC:** مانع اجرای ۱۰۰هزار قانون است. پیشنهاد: سقف ۳۲ مگابایت با محدودیت تعداد stream، به‌علاوهٔ بررسی اندازه در API.
- **کندی ویرایش:** ویرایش ۴۰ ثانیه‌ای فهرست ۱۰۰هزارتایی مشکل datastore است، نه F-acl. ردیف بدهی فنی لازم است.

## What I ran

| check | result |
|---|---|
| `cd apps/agent && go test -race -count=1 ./internal/{actions/acl,desired,subsystems,agent,descriptors/acl,descriptors/core/coretest}` (branch) | all `ok` (coretest has no tests), 27.8 s |
| Rebase simulation: `git merge-tree --write-tree main HEAD` → export → `go test -race` on the same packages + `dfkit/persist`, `objects` | textually clean. `subsystems` and `agent` **FAIL**, because TD-11b's guard refuses F-object-model's `objects.*` descriptors (they declare neither `CheckPersistent` nor `RecordsNoOwnership`). No `acl.*` descriptor is named. With a scratch-only `RecordsNoOwnership()` on the objects descriptor, both packages pass (6.8 s, 13.6 s). **So F-acl's family passes TD-11b's guard.** The objects gap is F-object-model's (merge note 1) |
| Probe P1, scratch copy (Apply A; DryRun B = same rules renumbered; Retrieve; AclState for A's sequences) | **fails**: Retrieve returns B's sequences 100/200, and AclState for sequences 10/20 gives `total=0 rules=0` → H1 |
| Probe P2 (loopback + binding, then one commit removing both, `subsystems=[interfaces,acl]`) | order is correct: `DELETE acl.interface-binding/loop702` → `interface-ip/…` → `interface.loopback/loop702` (see M3) |
| web: `vitest run src/domains/firewall/acl src/nav src/locales` (workspace packages built, `dist/` removed afterwards) | 4 files, 38 tests passed, including `AclPage` 8 (with RTL/Persian digits) and `model` 12 |
| API: `vitest run src/features/acl` | `rules.test.ts` 6/6 |
| Read-only on the host: `vppctl show acl-plugin tables mask` / `show acl-plugin acl` / `systemctl show vpp -p NRestarts` | `Stats counters enabled for interface ACLs: 0`. VPP up since 04:27:36, NRestarts=2. No ACL at all on VPP, so no w3 leftover. No host test was run (TD-25) |
| `git merge-tree` against task/F-host-acl-nftables and task/TD-23 | host-acl: conflicts only in generated files and `service_test.go`. **`subsystems.go`/`projection.go` auto-merge into a duplicate `ACL` const and a duplicate `Domains["acl"]` key**, which does not compile → Q14 fold. TD-23: textually clean, but D-134 requires its seam (M4) |

**Host evidence** (pasted in F-acl.md, not re-run):
- Counters rose by exactly the packets sent: rule 10 +5 for `ping -c 5`, rule 30 +3 for `ping -c 3`. Bytes are 8 × 84 = 672, the 84-byte ICMP echo, which is consistent.
- The 400 carries pointer `/acl/lists/lan-in/rules/4/destination/name` from the semantic tier.
- After the restart the bindings were back in 0.20 s. The agent log shows `created:4 unchanged:13` in 48 ms, and `input acl(s): 0, 1` keeps the foreign ACL first.
- Rollback left Retrieve at `{}` and only the foreign ACL in VPP. NRestarts stayed 1 → 1.

The evidence is complete and consistent. 10k was measured on the host; 100k was measured unit-side and at the editor
routes. Screenshots are pending TD-25 (Q13), which is accepted and not a blocker.

## Findings

| id | sev | where | finding | fix |
|---|---|---|---|---|
| H1 | **H** | `apps/agent/internal/desired/acl.go:450` (and `:543`, `:644`); `actions/acl/expansion.go:156-168`; readers at `desired/acl.go:694,722`, `actions/acl/runtime.go:277,282`, `subsystems/acl.go:189,220` | **See the H1 detail below the table.** A DryRun (validate, drift, commit pre-check) rewrites the expansion record, so Retrieve reports an unapplied candidate and AclState loses the running rules' counters | Make the record follow what was **applied**, not what was projected; add the P1 probe as a regression test. Details below |
| M1 | M | `docs/user/firewall/acl.md:58-62`; VPP `plugins/acl/acl.c:405`, `:284-289`, `:1824` → `:589` | The counters doc is wrong twice. (1) "Counters restart at 0 when a list is re-created …, **not when it is changed in place**": `acl_add_list` calls `validate_and_reset_acl_counters` for add and replace alike, which clears the vector. So every Update of a list zeroes all its hits: a commit touching it, a schedule flip or an FQDN change through the watcher. The fake models this correctly (`coretest/acl.go:111`). (2) "VPP has no way to switch it off from the API": `acl_stats_intf_counters_enable{enable:false}` switches it off, and DF-4's `integration_test.go:193` does exactly that | Correct both sentences. Say "hits since the list last changed in VPP", and name the watcher's re-projections as a cause. Optional later: an additive `AclListState` "counting since" field |
| M2 | M | `test/topology/acl/acl_test.go:498-510`; DF-4 `apps/agent/internal/descriptors/acl/integration_test.go:191-198`; `docs/vpp-code-track.md` V7 row; `docs/lab/host-vrx-a.md` | Shared-host counters flag: tests flip a VPP-global against each other. F-acl switches it on (opt-in, `flock -x`, never off, as the envelope said) and then relies on it without `flock -s`. DF-4's integration test forces it **off** at cleanup, with no globals lock and no saved value, which violates shared-host-rules §7. The flag is also reset by every VPP restart: it is 0 now. Answers are in §2 | Manager: apply §7 to both tests. Read the previous value with `show acl-plugin tables mask` (the getter V7 lacked), restore exactly that value, and hold `flock -s` while relying on the flag. Test-only edits, not a merge blocker. Fix the V7 wording and add the host-doc row (§2) |
| M3 | M | `apps/agent/internal/descriptors/acl/register.go:54-56` (`Optional: true` for both bindings) | Review item 3.4 ("ACL binding interface dependency optional", carried by F-acl; board note "D-125 envelope: mandatory interfaceDependency + same-commit-removal test, +1 h") is not done. **The envelope the worker got did not contain it.** Probe P2 shows today's order is right when both keys are present. With `Optional`, though, an interface key that does not resolve silently drops the ordering, and V23b says ACL binding vectors survive an interface delete (V19 family) | Make the interface dependency mandatory for `acl.interface-binding` and `acl.macip-interface-binding` (gap edit in the owned `descriptors/acl`), or record why not. Commit probe P2 as the same-commit-removal test |
| M4 | M | `descriptors/core/coretest/fakevpp.go:96`; `subsystems/acl_pbr_test.go:38-72` | Rebase work that belongs in the reviewed rebase commit, not in the merger's squash (D-134). (a) TD-23 (APPROVED) says "never edit `New()`": move `v.installACL()` to `func init(){ coretest.RegisterExtension("acl", (*coretest.VPP).installACL) }` in `coretest/acl.go`. (b) When F-rpf-adl-pbr is in the base, `TestPBRPolicyNamesFACLList` breaks. Its `reg.Register(abf.NewPolicy(...))` **panics** (`MapRegistry.Register` panics on `ErrDuplicateDescriptor`; F-rpf-adl-pbr registers `abf.policy` in `Register`). Its own `abf_policy_*` handlers collide with F-rpf-adl-pbr's coretest abf model | (a) as stated. (b) rewrite the test through the agent: commit an `acl` list plus a PBR policy naming it, then assert the ABF policy's `acl_index`. Remove the stand-in (Q3) |
| M5 | M | core `apps/agent/internal/agent/agent.go:186` `grpc.NewServer()`; `apps/api/src/agent/agent.client.ts` constructor (not F-acl's code) | Q2: both sides are at the default 4 MiB. A 100k-rule `ApplyRequest`/`RetrieveResponse` is about 12 MB, so the 100k commit, Retrieve and `/state/drift` fail with RESOURCE_EXHAUSTED. This blocks the 100k acceptance step (Q8), not this merge | See the Q2 answer. TD row or TD-9 |
| M6 | M | `apps/api/src/datastore/**` (not F-acl's) | Q11: a candidate edit of a 100k-rule list takes about 40 s (Zod over the whole document, redaction ×2, a secret walk, a 15 MB jsonb write). WBS D5.5 ("editor usable at 100 000 rules") is not met by the product, although F-acl's own reads are fine | See the Q11 answer. TD row, not an F-acl blocker |
| L1 | L | `packages/schema/src/domains/acl.ts:138` | "100k per list" is enforced in the agent: VPP rules after expansion, `desired/acl.go:403,437-443`, at the list pointer; plus 10k per rule at `:398`. It is also enforced at the CSV import (100k rows / 64 MiB) and bulk (100k sequences). **The schema has no `.max`** | Additive `.max(100_000)` on `acl.lists.*.rules` (P02b's file → manager). The agent stays authoritative, because config rules ≠ VPP rules |
| L2 | L | `actions/acl/runtime.go:214`, `:216-226` | The combined `pattern` is built but unused. The lists overview does one `DumpStats` (a stats-directory scan) per tracked list | One `DumpStats(pattern)` for all indexes |
| L3 | L | `actions/acl/runtime.go:315-352`, `:431-438` | Attachments: the tag of each foreign ACL is read with `acl_dump(idx)`, which returns **all its rules** (a foreign 100k list = a 100k-rule dump per Refresh). There is also no single-flight for this walk (D-132: "one walk at a time in the agent") | Cache tags by index for the call and serialise `Interfaces()`. The walk is manual-refresh only, so this is not urgent |
| L4 | L | `apps/api/src/features/acl/acl.service.ts:319-333` | `hitsOnly` makes up to 100 AclState calls. Each reads the full counter vector and walks all RuleInfos | Stop once the API page is filled, or let the agent return hit sequences in one call |
| L5 | L | `subsystems/acl.go:42` | `aclResyncGap` of 5 s: with FQDN churn a 100k list resyncs every 5 s. Each resync is an `acl_dump` of every ACL on VPP plus a 2.7 s projection. TD-8's storm guard also applies | ≥ 30 s, in the spirit of D-132 |
| L6 | L | `subsystems/acl.go:79,86` | Go loads `time.Local` once per process, so a `system.timezone` change reaches schedules only after an agent restart (Q4) | Document it in acl.md, or reload the zone on resync |
| L7 | L | `descriptors/acl/acl.go:177` (DF-4) via `binding.go:77`, `:218` | Every binding Create/Update/Retrieve calls `dumpOwnedACLs` = `acl_dump(~0)`, which returns **all rules of all ACLs**. With a 100k list and N bound interfaces (zones), one commit does N+1 full dumps | Resolve indexes from F-acl's tracker (or one dump per transaction); worth doing before the 100k host step |
| L8 | L | `test/topology/acl/janitor_test.go:48-50`, `:52-56` | `acl_del` by index happens after the binding rewrites without re-reading the tag first (D-071: re-verify identity immediately before a delete by index). The MACIP pass covers `w3:` but not `w3-…:` tags, unlike the ACL pass | Re-dump the index before `acl_del`; use the same predicate for MACIP |
| L9 | L | `desired/acl.go:60-86`, `actions/acl/expansion.go:154` | Process-global env and `aclstate.Default` record (Q5): two agents in one test process share records by list name | Tech debt, with Q5 |
| L10 | L | `internal/agent/rpc_acl_test.go` | No agent-level test that a hand edit in VPP (our ACL's rules replaced behind the agent's back) shows in Retrieve and is repaired by a resync. `TestAssembleACLReconstructsUnknownContent` covers the assembler only, with an empty record | Add it to `TestACLDomainOnFake` |
| L11 | L | `descriptors/core/coretest/acl.go:313-327` | A catch-all `cli_inband` handler answers an error for every other command. Once TD-23's `On` collision guard is in, the next feature needing `cli_inband` in coretest will collide | Register it through TD-23 with dispatch by command |
| L12 | L | `apps/api/src/features/acl/acl.service.ts:104-109` | A pass-through `text/csv` parser for the whole Fastify instance: other routes get an unread stream instead of 415, bypassing `bodyLimit`. The import route caps 64 MiB itself | Fine now; note it in the API docs |
| L13 | L | `docs/status/tasks/F-acl-wip.md` | Stale (04:20, "web running") | Update or delete before the merge |

### H1 in detail

**Where the record is written and read**
- The record is written in every projection, including DryRun: `service.go:698`, `POST /config/validate`, `GET /state/drift`, and the commit's own validation.
- `PutACL` keeps the newest record per fingerprint. A candidate whose VPP content is identical (renumber or move, description or tags, `log`, a disabled or schedule-inactive rule, attachment sequences) therefore replaces the applied configuration's entry.
- AssembleACL, AclState and the watcher all read that entry.

**Probe P1**
- Retrieve returns the candidate's sequences 100/200 instead of the applied 10/20. `/state/drift` shows a drift that is not in VPP. This is Retrieve reporting cached desired state, which D-063 forbids.
- `AclState{list, sequences:[10,20]}` returns `total=0`, so the running rows lose their counters, and they appear on the candidate rows instead.
- The state holds until the running document is projected again, and a rolled-back Apply leaves the same state.

**Fix: the record must follow what was applied**
- (a) Preferred, with no core change: an agent-local `acl.config/<name>` descriptor, following the objects-family pattern. It holds the list's config and fingerprint, is written only by Apply and reverted by rollback. The Record is keyed by name + fingerprint + config hash, and every same-fingerprint entry is kept. AssembleACL, AclState and the watcher use the entry equal to the applied config, and bindings get the same treatment.
- (b) A two-line core seam that tells the projection it is a DryRun. That covers validate and drift, but not a rolled-back Apply.

**Regression test:** add probe P1 (plus an attachments variant).

## 1. Architecture
- **Projection** (`desired/acl.go`) is correct:
  - Objects are expanded from the **request's** `objects`, and `acl.objects-required` is raised without them.
  - `ipVersion: any` → one v4 and one v6 block, each only where both sides have the family. ICMP is emitted only in v4 and ICMPv6 only in v6. `tcp-udp` → 2 rules.
  - Disabled rules and inactive schedules are omitted. A duplicate sequence is caught.
  - Caps: 10k per rule via `objects.CheckLimit` at the rule pointer, and 100k per list at the list pointer.
  - FQDN: an unresolved object → WARNING. `log` → WARNING `acl.log-unsupported`. `reflect` → permit+reflect. MACIP without a prefix → v4 any + v6 any. Zones expand to their members, ordered by attachment sequence.
  - `acl.stats-enable` is emitted only for `GlobalsOwner` (`:91-93`, D-071).
- **Retrieve** is fingerprint attribution with a fallback to reconstruction. It is not an echo in the D-063 sense *for applied content*: the VPP rules must match the recorded expansion byte for byte, and anything else is rebuilt from VPP, so drift shows. Hand edits do show (assembler unit test; agent-level test missing, L10). This follows the precedent of D-073b (non-VPP leaves for objects that verifiably exist). The flaw is H1: the attribution source is not apply-only.
- **Expansion record + tracker for counters:**
  - The tracker learns index and fingerprint from Create/Update/Delete/Retrieve of the wrapped DF-4 descriptors, so no 100k dump happens for counters.
  - The record maps config rules to contiguous VPP rule blocks, and the counters sum over them.
  - The watcher compares applied schedule state and FQDN use against now, and asks TD-8's `RequestResync`, coalesced; there is no second reconcile path.
  - All sound, apart from H1 and L5.
- **Declarative, binapi only:** no `os/exec` or vppctl in agent code. The only CLI is the read-only counters-flag read through binapi `cli_inband` (V7 has no getter). All messages come from `binapi/acl` and `vlib`.
- **API talks only to the agent:** no exec, VPP socket or vppctl in `apps/api/src/features/acl`. The config goes through the generic pointer routes. State and actions have their own routes, with audit rows on import and bulk. Response DTOs are hand-written Zod in the controller, as P08's `state.controller.ts` does; the web types come from the generated api-client.

## 2. Shared VPP safety
- **Counters flag (V7), question by question:**
  - *Only set by the globals owner?* In the product, yes: projected only with `GlobalsOwner`; slot agents run `VRX_GLOBALS_OWNER=0` (`acl_test.go:182`). On the shared host it was set by F-acl's **topology test** (raw API, opt-in `VRX_ACL_STATS_GLOBALS=1`, `flock -x`) at 04:22, as the envelope ordered. The VPP restart at 04:27:36 reset it, and it is **0 now** (my read).
  - *Can it be turned off?* **Yes.** The same message with `enable=false` does it (`acl.c:1824` → `:589`, and DF-4's test does it). V7's "no getter/disable path" is wrong about *disable*. What VPP lacks is an API getter (the CLI prints the flag, `acl.c:3646`), and it has the wrong reply id.
  - *Per-packet cost?* Yes, small. On interfaces that carry an ACL, each packet that matches a rule costs one per-thread combined-counter increment, a prefetch and a buffer-length walk (`dataplane_node.c:471-487`). There are no locks; it costs a few ns, and it touches every slot's ACL interfaces. It is irrelevant under FAST MODE and non-zero at line rate.
  - *Should the product agent own it?* **Yes**, as built: the globals-owner agent projects `acl.stats-enable/global` and never disables it, because other owners may rely on it (D-071). A later opt-out knob would use AclConfig number 7, which is reserved for F-acl, if line-rate cost matters.
  - *What `docs/lab/host-vrx-a.md` must record:* a row "ACL counters flag (`acl_stats_intf_counters_enable`)" stating that:
    - it is VPP-global and volatile: 0 after every VPP restart, 0 at 2026-09-25 09:4x;
    - it is read with `vppctl show acl-plugin tables mask`;
    - no slot agent sets it;
    - it is changed only by opt-in tests under `flock -x /run/lock/vrx-globals.lock`, which must save and restore the previous value (§7);
    - DF-4's integration test currently forces it to 0 at cleanup unless `VRX_ACL_STATS_KEEP=1` (M2);
    - its resting value on the shared host is the manager's choice.
- **D-126:** no classify sweep. The only classify writes are the V19 guard's resets to `~0` of the ip4/ip6 and l2 in/out tables on **the two rig ports** returned by `waitIfs` (`vpp_test.go:134-146`, called at `acl_test.go:374` and `shots_test.go:110`). MACIP classify tables exist only through `macip_acl_add` and are deleted only after the unbind. The NORIG screenshot mode creates none. The flag is read with the `mask` qualifier (no hash-table dump), and there is no trace (D-128).
- **Janitor:**
  - It is opt-in (`VRX_ACL_JANITOR=1`) and runs under the shared lab lock.
  - Ownership is decided **by tag prefix `w3:` or `w3-`**. The separator makes it exact: `w1:` never matches `w10:`.
  - It rewrites each interface's list without our ACLs, keeping the others in order, **before** `acl_del`; for MACIP it unbinds, then deletes.
  - Two small gaps are listed in L8.
- **Foreign ACL first after a restart:** `binding.go:73-128` `setList` rebuilds the list as `foreign(current VPP list) + ours (desired order)` on every Create and Update. The restart evidence shows `input acl(s): 0, 1`, and a unit test covers it (`TestACLDomainOnFake`). By DF-4's design (D-066), a foreign ACL that sat *after* ours is moved in front at our next update; the order among foreign ACLs is kept.

## 3. Scale
- **Timings:**
  - 10k commit: 10.8 s, of which the agent reconcile is 2.26 s; the rest is API validation and the datastore.
  - 100k projection: 2.74 s (unit), acceptable.
  - Before the 100k host step: L7 (DF-4 binding descriptors dump every ACL per binding operation) and L5.
- **100k cap:** enforced in the agent (list and rule caps, DryRun errors with pointers), at CSV import and in bulk; **not in the schema** (L1).
- **Q8/Q2, gRPC:** see Q2.
- **Q11:** see Q11.

## 4. D-132 — what each counters refresh dumps
No refresh walks a binary-API table, except Attachments, which runs only on the Refresh button.

| path | trigger | what it reads |
|---|---|---|
| Rule editor | 30 s poll + Refresh, `ServerDataGrid refetchInterval=ACL_POLL_MS` | One `AclState{list, sequences = the visible page}`: the counters flag, one stats-segment read of that list's vector, and an O(rules) mapping walk in memory. See the notes below |
| Lists tab | 30 s poll | The flag, then one vector read per tracked list (L2) |
| Attachments tab | **No timer** (`queries.ts` `useAclAttachments`) | Per Refresh: `sw_interface_dump`, `acl_interface_list_dump(~0)`, `macip_acl_interface_list_dump(~0)`, and one `acl_dump`/`macip_acl_dump` per **foreign** bound ACL (L3) |
| `hitsOnly` | on request | Up to 100 AclState calls (L4) |

Notes on the rule-editor row:
- **Flag read:** a `cli_inband "show acl-plugin tables mask"` that prints two lines plus the mask table. "On" is cached for 5 s; "off" is never cached, so there is one CLI call per poll.
- **Vector read:** `/acl/<idx>/matches`, 100 001 combined counters × threads ≈ 1.6 MB per thread copied from shared memory, after one regex scan of the stats directory.
- No `acl_dump` at all.

## 5. TD-11b / D-131
- **Declarations** (`descriptors/acl/ownership.go`) are correct:
  - ACL, MACIP, both bindings and the stats switch declare `RecordsNoOwnership`. They are owned by tag; the switch is never deleted. The tracker wrappers inherit the declaration through embedding.
  - The ethertype whitelist declares `CheckPersistent` over its claim store. It is the only DF-4 descriptor that claims (`etype.go:151`, claim-first already).
  - Verified against main's real guard: rebase simulation, `acl.*` never named.
- **`KeyedClaims("acl")`:** the store is shared with the DF-2 families of F-rpf-adl-pbr. The keys do not collide: DF-2 claims full scheduler keys (`adl.interface/<if>`, …), the whitelist the bare interface name.
- **PBR test:** see M4(b). As written it breaks once F-rpf-adl-pbr registers `abf.policy`.
- **Q3:** see below.

## 6. Contract (410b5471)
Additive only: one RPC, eight messages, one enum, all prefixed `Acl`. No existing message changes, and no `AclConfig`
or `DesiredState` number is used. F-host-acl-nftables uses `AclConfig.host_settings = 8` and `HostAcl*` messages plus
`rpc HostAclState` (read with `git show task/F-host-acl-nftables:packages/proto/vrx/v1/dataplane.proto`): no
collision. No other task branch defines an `Acl*State*` message or an `ACL_RULE_STATUS_*` value. Paging is capped at
1000, with a sequence filter of at most 1000. The squash subject must be `contract(proto,api-client): …`, because the
generated `schema.d.ts` changes too (D-112, contract guard).

## 7. Q14 — the fold with F-host-acl-nftables (exact)
Whichever of the two branches merges second does the fold, in its **reviewed rebase commit** (D-134), not in the
merger's squash.
- **`subsystems.go` const block:** keep one `ACL = "acl"` under `// wave-A: F-acl`, and delete host-acl's line.
- **`Domains`:** keep one entry under `// wave-A: F-acl`, `ACL: append(aclDescriptors(), hostACLDescriptors()...),`, and delete host-acl's entry. A plain `git merge` auto-merges both lines into a duplicate const and a duplicate map key, which does not compile.
- **`Register`:** keep both calls in anchor order: `registerObjectModel` → `registerACL` → `registerHostACL`.
- **`projection.go`:**
  - keep both `project()` calls;
  - in `assemble()`, F-acl comes **first** (`ds.Acl = desired.AssembleACL(kvs)`, nil when empty), then `ds.Acl = desired.AssembleHostACL(kvs, ds.Acl)`, which handles nil. Reversed, the host leaves are overwritten and drift forever.
- **Remove both unsupported-field blocks:**
  - F-acl: `internal/desired/acl.go:95-100`;
  - host-acl: `internal/desired/hostacl.go:47-54`, and its assertion in `internal/agent/rpc_host_acl_test.go:112-115`.
- **Add one test:** a DryRun with lists **and** host rules has no `agent.unsupported-field`, and Retrieve assembles both.
- **`service_test.go`:** use the registry-derived form (D-129 F5).
- **Generated files:** regenerate them (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`).

## 8. UI
- `/firewall/acl` in en and fa:
  - 302 keys in both locales. The 16 identical values are protocol and unit names.
  - The RTL/Persian-digits test passes, and the logical-CSS check passed in CI.
  - Counters are polled at 30 s + Refresh; attachments have no timer.
  - The ADL/Auto-SDL entry is only a link.
  - No `dropPhantomOptionals` (D-131).
- **Q12:** WEB-1's presence toggle fixes it. `isOptionalObject` (`task/WEB-1:packages/ui-kit/src/schema-form/schema-utils.ts:242`: non-required plain object with properties and no default) is a superset of F-acl's condition (`model.ts:277-306`). With `FORM_DEFAULTS.presence` an absent optional object stays absent.
  - **After WEB-1 merges:** delete `optionalObjectsAsJson`, and use the plain schema in `ruleFormSchema` (`model.ts:309`) and `MacipTab.tsx:452`, so TCP flags become a normal form behind the presence switch.
  - **Test:** a TCP rule without flags submits without `tcpFlags`, and switching presence on edits mask and value. This is a follow-up row, not a blocker.
- **Minor:** the rules page defaults to `source=candidate` and attaches counters by sequence. After a pending renumber or move, the counters appear on different rules until the commit (the pending marks show it). The H1 fix should keep counters keyed to the applied configuration.

## 9. Tests
- **Agent unit tests** are thorough: the domain on the fake, DryRun findings, the D-071 flag, zones and order, the caps, the watcher, the tracker, the ownership declarations. Gaps: L10 and the same-commit-removal test (M3).
- **API unit** 6/6 re-run; **web** 38 re-run. E2e was not re-run (it needs the slot database); its pasted output is consistent.
- **Host evidence:** judged in "What I ran". The counter deltas, the 400 pointer, the 0.20 s restart and the rollback are all credible and internally consistent.
- **Q13:** the pending screenshots are accepted.

## Answers to the questions (Q1–Q14)
- **Q1** — Resolved by TD-8 (`Env.Resync` → `Wiring.RequestResync`). Keep the coalescing, but raise the gap to 30 s (L5).
- **Q2** — gRPC limits. **Raise and bound; do not chunk now, and do not cap below 100k.**
  - **Agent:** `grpc.MaxRecvMsgSize(32<<20)` and `grpc.MaxSendMsgSize(32<<20)`, where 32 MiB is about 2.5× a 100k-rule document; plus `grpc.MaxConcurrentStreams(16)`, so the worst-case buffered memory is bounded.
  - **API client:** `grpc.max_receive_message_length` / `max_send_message_length` = 32 MiB, covering Retrieve, drift and rollback.
  - **API pre-flight:** reject a commit whose serialized DesiredState exceeds the limit with problem+json 413 and the pointer of the largest list, instead of a 502 from RESOURCE_EXHAUSTED.
  - **Security cost:** low. The socket is a local unix socket (0660, vrx group), and every caller can already rewrite the data plane through Apply. The real cost is memory (limit × concurrent streams), which `MaxConcurrentStreams` bounds.
  - **Chunking** is a reshape of Apply (client streaming), which decision-policy #1 makes PENDING, and it is not needed for 100k.
  - **Owner:** the core, `agent.go` A5 plus the `agent.client.ts` constructor. Fold it into TD-9, which is already reworking the gRPC server construction, or open a small TD row. It gates Q8 only.
- **Q3** — The stand-in (`registerACLBridge`, its call in `registerRpfAdlPbr`, the `aclRefs` type, `TestRpfAdlPbrWithoutFAcl`) is deleted by **whichever of F-rpf-adl-pbr / F-acl rebases second**, in its reviewed rebase commit (D-134), together with M4(b).
  - D-131 assumed F-rpf-adl-pbr would merge first, but D-134 holds it for TD-23, so F-acl may merge first. In that case F-rpf-adl-pbr's rebase removes its own files' stand-in and adapts `acl_pbr_test.go`.
  - Note: the stand-in is **not** skipped at registration, because F-rpf-adl-pbr's line runs before F-acl's in `Register`. It only switches itself off at plan time, so the removal is a clean-up, not a safety fix.
- **Q4** — Accept local time, and document L6: a zone change takes effect after the agent restarts.
- **Q5** — Accept for wave A. Tech debt: pass the env from `Service` through `project()` (L9).
- **Q6** — Accept. Verified against main's guard (rebase simulation).
- **Q7** — Superseded by TD-23: use `RegisterExtension` at the rebase (M4a). The `service_test.go` example switch is fine; take main's registry-derived form (D-129 F5).
- **Q8** — Not a merge condition. Run the 100k host step in a manager window **after** Q2's limits and TD-25, and preferably after L7. Check NRestarts around the step (D-064).
- **Q9** — Accept: pull at 30 s + Refresh (D-132). A WS topic would poll the agent anyway.
- **Q10** — Accept: availability comes from VPP's flag, read through the CLI. After any VPP restart the flag is 0 until the globals owner applies it; that is the case on the host right now.
- **Q11** — **Owner: the datastore** (`apps/api/src/datastore`, P06 / TD-10a commit engine). **Severity M** for WBS D5.5, not an F-acl blocker.
  - **Proposed TD row:** edits by subtree. Validate only the edited root domain. Redact and diff only the edited pointer's subtree, since secret-free domains are proven by the same schema check F-acl added. Store the change with `jsonb_set`, not a 15 MB rewrite.
  - **Target:** a bulk edit of a 100k list in under 3 s.
- **Q12** — Yes, WEB-1 fixes it (§8). Follow-up after WEB-1.
- **Q13** — Accepted.
- **Q14** — Exact fold in §7.

## Merge notes (for the merger)
1. **Order: F-object-model → (TD-23) → F-acl**, with F-host-acl-nftables and F-rpf-adl-pbr before or after. F-object-model's rebase onto main will fail TD-11b's guard for `objects.*`: the agent refuses to start, and every `subsystems`/`agent` test fails. It needs a reviewed `RecordsNoOwnership()` (agent-local store) or `CheckPersistent()` on its descriptor **before** F-acl can gate.
2. **The rebase of F-acl itself** (worker, reviewed; D-134): M4(a) TD-23 seam; the Q14 fold if host-acl is already in; M4(b) and the Q3 removal if F-rpf-adl-pbr is already in.
3. **Generated files** conflict (`dataplane.pb.go`, `_grpc.pb.go`, `packages/proto/gen/ts/...`, `schema.d.ts`, `operations_gen.go`): regenerate, never hand-merge.
4. **Squash subject:** `contract(proto,api-client): F-acl — ACL/MACIP lists, attachments, hit counters, AclState, 100k editor` (D-112).
5. **Envelope gap:** the D-125 3.4 obligation (M3) never reached the worker's envelope. Check the other D-125 envelope notes the same way.

## Fix round (then a focused re-verify)
- **H1**, with probe P1 as the test (lists and attachments).
- **M1:** docs.
- **M3:** mandatory interface dependency (or a recorded reason) + same-commit-removal test.
- **M4:** at the rebase.
- **M2:** test edits in F-acl's `acl_test.go` (save/restore + `flock -s`). The DF-4 test, V7 row and host doc are the manager's.
- **L-items:** optional in this round. L7 and L5 should come before the 100k host step.
