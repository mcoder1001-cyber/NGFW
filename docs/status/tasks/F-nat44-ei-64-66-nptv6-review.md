# F-nat44-ei-64-66-nptv6 — review

**Verdict: APPROVE WITH CHANGES.** A short fix round (R1–R4 below) and then a focused verify. No full re-review is needed.

- Reviewed: `task/F-nat44-ei-64-66-nptv6@309084fd`, only this task's own diff: `git diff task/F-nat44-ed-sessions...HEAD` (83 files; the merge base is ED's `bf093407`).
- The ED base moved during the review. `task/F-nat44-ed-sessions` is now `4421baec`, with fix round 2 `88de9a56` and ED's verify APPROVE. That move is taken into account below: a merge with it is clean and the merged tree passes the tests.
- Nothing was run on the host. I edited no product code.
- VPP behaviour was checked read-only in `/root/vpp`.

## What holds up

- **Architecture.**
  - The EI, NAT64, NAT66 and NPTv6 projections sit under ED's anchors: `desired/nat.go:96-101` and `AssembleNat`, and `subsystems.go` (A1).
  - Everything is declarative, through builders, descriptors and the scheduler.
  - Node talks only to the agent (`features/nat44-ei-64-66-nptv6/service.ts` → `AgentClient`, and the running datastore for NPTv6).
  - VPP names come from binapi: `binapi/npt66.Npt66BindingAddDel` and `binapi/nat64.Nat64BibDump`. There are no secrets.
  - Rule 8 holds: `/state/**` is read-only, and the kill is `/actions/**`, audited with the endpoint as the resource.
- **Q4: npt66 is write-only with no D-076 record. Verified in VPP.**
  - `npt66.api` has exactly one message, and binapi has only `Npt66BindingAddDel`. The only listing is the CLI `show npt66 bindings`, which prints no interface.
  - In `npt66.c:55-91`, the add looks up the interface's binding. If one exists it is overwritten in place, and `configure_feature` is set only for a new binding, so a repeated add never stacks `npt66-input`/`npt66-output`.
  - The delete (`npt66.c:92-101`) goes by `sw_if_index` only and ignores the prefixes.
  - The fake models exactly this (`coretest/npt66.go`: `FeatureEnables`, `Stale`). The tests `TestBindingResyncIsIdempotent` and `TestBindingThroughReconciler` and the host run (3 adds → 1 line) prove it.
- **Q4: deleting a binding the agent does not know after a restart.** Nothing is sent. The binding is not retrieved and not in the write-only cache (`reconciler.go:402-408`), so the scheduler never plans it, and it stays until VPP restarts.
  - If the configuration still has the binding, the resync re-adds it (an overwrite). A later delete then works: it resolves the interface by name, uses the Meta only as a fallback, and treats NO_SUCH_ENTRY as done.
  - One exception is L5 below: if the interface is then deleted, the stale entry poisons a reused `sw_if_index`.
- **npt66 key `<if>/<internal>`.** It is sound. There is one binding per interface, enforced by the builder (`nptv6.go:366-369`) and by the schema (`nat.nptv6-valid`).
  - The scheduler plans every delete before any create (`reconciler.go:478-484`). So an internal-prefix change on one interface (a delete and a create of two keys) cannot have the `sw_if_index` delete wipe the new binding.
  - An external-prefix change is an Update, done as an in-place overwrite.
  - The binding depends on `interface/<name>` (D-095c). `TestNptv6BindingDeletedBeforeItsInterface` shows the delete order, and the topology cleanup removes leftover bindings before `rig down` (`nat_test.go:194-206`).
- **Variants as optional enum fields.** Sound, and the contract commits `651d620`/`c8828ae` are additive:
  - fields 5, 8 and 7 are appended after ED's maxima; ED's fix rounds added no proto numbers;
  - the enum is in the task's section;
  - there is no RPC, no `ActionRequest` member and no `NatConfig` number;
  - `optional` keeps ED's TypeScript and fake unchanged.

  The price is that NAT64 reuses `NatSession` with a remapped meaning (documented in the enum comment and proto.md §11). That is acceptable under the prompt's "no parallel RPCs".
- **V-new (b), NAT64 ports.** The defect is real: `nat64_api.c:316-320` sets `il_port` twice (the second time to `ste->r_port`) and never sets `r_port`. The agent's correction works on 26.06: the remote port comes from `il_port`, and the inside port from the BIB by the outside endpoint, which is unique because the outside is keyed in FIB 0. See L1 for the heuristic.
- **V-new (d), EI port forward.** The rule matches VPP: `nat44_ei.c:2487-2497` calls `nat44_ei_reserve_port` unless the mapping is address-only or static-mapping-only, and answers NO_SUCH_ENTRY for an address outside the pool. The builder rule `nat.ei-port-forward-pool` (`nat44ei.go:203-208`) catches static mappings and is unit-tested. See L2 for identity mappings.
- **Shared-VPP plugin ownership.** For a slot, the three enables are `natcommon.Global` with `WithGlobalsOwner(false)`: Create only *requires* the value, Delete is a no-op and nothing is retrieved (`natcommon/config.go:159-183`). Only the globals owner resets, and even then only after the emptiness check (`ErrNotEmpty`).
  - The topology fixtures use nattest's lock path `/run/lock/vrx-nat-fixture-<plugin>.lock`. They disable a plugin only if they enabled it and it is empty for every owner (`natvpp_test.go:262-289`).
  - A slot agent can never disable a plugin that the product agent or another slot uses.
- **D-132 caps.** EI pages through ED's pager, so it has ED's H1 bounds: `natCaps` = 256 user dumps and 200 000 sessions scanned (`rpc_nat44_ei.go:61`). Each EI user dump walks only that user's list (`nat44_ei_api.c:1331-1340`), which is cheaper than ED's full-pool walk.
  - The grids poll every 30 s or on Refresh, and a session-level filter turns polling off (`queries.ts:12`, `EiTab.tsx:302`, `Nat64Tab.tsx:144`).
  - NAT64 is different; see M3.
- **TD-11b.** `npt66.binding` is a `natcommon.Descriptor` (`npt66.go:133`, `natcommon.New` with `Claims: p.cfg.Claims`).
  - On main, `natcommon.Descriptor.CheckPersistent` (`descriptor.go:139`) requires the persisted store. The wiring passes `WithClaims(w.KeyedClaims("nat"))`, the same store as ED, returned once per family (`subsystems.go:295-306`).
  - Every descriptor this task registers, including the domain-less `nat44-ei.ipfix` global, therefore declares ownership. The branch base predates TD-11b, so the guard runs only after the rebase.
  - Create makes a single VPP call (no partial create), and the claim-first order of TD-11b/D-133 applies unchanged.
- **Q2, the edits to ED-owned tests.** All four are additive relaxations: one test case, one line, three `&& n != "nat44_ei_output_interface_get"` conditions, and two `slice(0, 4)` calls.
  - `git merge-tree HEAD task/F-nat44-ed-sessions@4421baec` is clean.
  - On the merged tree, `go test -race ./internal/{agent,actions/...,desired,subsystems}` all pass, so they do not clash with ED's fix round 2.
- **UI.**
  - The four tabs sit under the natTabs anchor.
  - NAT64, NAT66 and NPTv6 use `SchemaForm` over `natSchema().properties[<subtree>]` (one schema, rule 5).
  - WEB-1 holds: no `dropPhantomOptionals` (`SubtreeForm.tsx:45`).
  - The screenshots show `dir=rtl lang=fa`, logical layout and `pageErrors=0`. See L4 for strings that are not translated.
- **The pasted host evidence is credible and complete for the acceptance list.**
  - **EI PAT.** `tcpdump` in `ns-w4-wan` shows `10.4.2.102`, and the session detail matches the API row. The kill is audited as success 200, and a second kill as failure 404.
  - **NAT64.** Traffic goes through the slot /96 in the slot VRF, and the wire shows the pool `10.4.64.1`. The static BIB inbound works.
  - **NPTv6.** The wire shows `fd00:4:20:ffef::2`. `0xffef` = ~`0x0010` is the correct RFC 6296 checksum-neutral subnet word for `fd00:4:10::/48` → `fd00:4:20::/48`.
  - **Restart.** The resync log shows the creates. The slot's `show npt66 bindings` has exactly one line after the loss re-apply and after a second restart without loss. The 0.04 s is measured from agent-up to Retrieve, well inside 30 s.
  - **Crash counter.** NRestarts stayed 1 → 1 in every run. The 04:27 crash is outside every run window (Q8).

## Tests I ran (no host)

| what | result |
|---|---|
| `go test -race -count=1` over `actions/{nat44-ed-sessions,nat44-ei-64-66-nptv6}`, `agent`, `descriptors/{nat44ed,nat44ei,nat64,nat66,natcommon,npt66}`, `desired`, `subsystems` (apps/agent, `VRX_INTEGRATION` unset) | 11/11 packages ok on 4 multi-package runs. **One `internal/agent` FAIL in the very first run** (load 31; the test name was lost to a truncated tail). It did not reproduce in 15 later runs (10 × `./internal/agent` alone, 4 × all packages, 1 × merged tree). Most likely a load-sensitive existing test (the visible lines were the confirm-revert tests); see L9 |
| the merged tree `HEAD` + `task/F-nat44-ed-sessions@4421baec` (from `merge-tree`, run in scratch): `go test -race` on `agent`, `actions/...`, `desired`, `subsystems` | all ok |
| D-134 simulation: TD-23's `coretest/fakevpp.go` (`task/TD-23@39e07a0f`) and the four `init()`s turned into `RegisterExtension` | **panic** in every `coretest.New()`, see M1. With ED's stub removed, `npt66`, `agent`, `desired` and `actions/...` all ok |
| `npx vitest run src/domains/firewall` (apps/web, after building the packages the web app depends on) | 2 files, 15/15 (NatV6Tabs 6, NatPage 9) |
| `npx vitest run src/features/nat44-ei-64-66-nptv6 src/features/nat44-ed-sessions` (apps/api) | 2 files, 8/8 |

## Findings

### H1: deleting a VRF that NAT64 used wedges the commit (product path), and the agent does nothing about it
VPP 26.06 locks the tenant table on `nat64_add_del_prefix` add and never unlocks it (`nat64.c:1207-1224`, "TODO: missing fib_table_unlock"). `nat64_add_del_static_bib_entry` takes a lock on every add **and** delete (`nat64.c:864-866`).

The delete ordering is correct: the nat64 objects depend on `vrf/<id>` and are deleted first. The VRF delete itself does this:
1. It removes only the API lock (`ip_api.c:583-586`, `fib_table_lock_clear`; repeating it is safe, so there is no underflow).
2. The IPv6 table stays, still named `<owner>:<vrf>`.
3. `VRFDescriptor.Retrieve` still reports `vrf/<id>` (`core/vrf.go:184-194`, `MissingIp4`).
4. `verify` fails with "vrf/<id> still present" (`reconciler.go:687`, `854`).
5. The **whole transaction is rolled back**, or ends DEGRADED when a revert fails (`reconciler.go:709-728`). The worker's run 8 shows this: 422 `apply-failed`, `applyStatus: degraded`, "vrf/4064 still present" (`/root/ngfw-wt/logs/F-nat44-ei-64-66-nptv6-topo-8.log:359`).

Consequences until VPP restarts:
- no commit that deletes the VRF can ever succeed, and its other changes are rolled back with it;
- a rollback to a revision older than the VRF fails the same way;
- a **confirmed-commit auto-revert** of a commit that added a NAT64 tenant VRF cannot complete. The agent stays degraded and retries on every resync, so the AD-4 backstop is broken for this case.

Today the builder accepts a non-default NAT64 VRF silently (`desired/nat64.go:52-73`, `93-116`). The user page says only "cannot be deleted" (`nat44-ei-64-66-nptv6.md:73-75`), and V-new (c) says "→ DEGRADED".

**R1 (required, in this task's files):**
- a DryRun warning at the prefix's or static BIB's `/vrf` pointer when it is not the default VRF, with its own rule id;
- the user page and V-new (c) state the real failure mode: the commit fails verify and is rolled back (DEGRADED if a revert fails), and rollback and confirm-revert are affected.

**For the manager:**
- a core row (P05's `vrf.go`, not this task's files): a boot-scoped (D-080) record of "deleted by us, kept alive by VPP", so Retrieve hides that table until the next VPP boot, and a later create of the same VRF re-locks it normally;
- a product-owner decision on whether tenant-VRF NAT64 ships before V-new (c) or the C-track fix.

### H2: every NAT64 host run leaves a VPP-undeletable `w4:w4-n64` table that wedges the next slot-4 agent
Both `nat_test.go:216-222` and `shots_test.go:71` remove the slot VRF through binapi (`gcSlotVRF`, `natvpp_test.go:605`). By H1's mechanism, the IPv6 table 4064 survives with its name until VPP restarts; the evidence shows `locks:[API:1, nat64-hi:24]`.

The name parses as owned by `w4`. Any later task on slot 4 whose configuration lacks VRF 4064 therefore plans `delete vrf/4064` in every transaction that covers the `vrfs` domain, including the start-up resync (`reconciler.go:410-431`). Each such transaction fails verify and rolls back, and neither the manager's nightly gc nor that worker can clear it. Q5 mentions only the nightly-gc noise.

The 04:27 restart has cleared the current leftover, so there is nothing to do on the host now.

**R2 (required):**
- put the tenant-VRF NAT64 phases (topology `nat64`/`restart-nat64` and the NAT64 screenshot) behind an opt-in env var, D-064 style (the evidence already exists);
- write in the questions file that any run of them quarantines slot 4's table 4064 until a VPP restart (a manager note for the next slot-4 envelope).

H1's core record would make the leftover harmless.

### M1: D-134/TD-23 — the rebase panics on the ED stub (proven)
ED's `coretest/nat44ed.go:177-179` stubs `nat44_ei_show_running_config`. This branch's `coretest/nat44ei.go:124` models the same message and relies on running later ("replaces the stub", `nat44ei.go:9-10`).

TD-23's `VPP.On` panics when a second extension claims a message. Simulated with `task/TD-23@39e07a0f`:
`panic: coretest: extension "nat44ei" replaces VPP message "nat44_ei_show_running_config" already modelled by extension "nat44ed"` in every `coretest.New()`. With the 3 stub lines removed, everything passes.

**R3 (required, so that the squash stays mechanical):** in this branch, delete ED's stub (`nat44ed.go:177-179` and the then-unused `nat44_ei` import) as Q2 item 6, and fix the comment at `nat44ei.go:9-10`.

The three `init()` bodies that become registration lines at the rebase are:
- `coretest/nat44ei.go:98-99`: `RegisterExtension("<slug>-nat44ei", func(v *VPP) { nat44EIModels.Store(v, v.installNat44EI()) })`
- `coretest/nat64.go:105-106`: `RegisterExtension("<slug>-nat64", func(v *VPP) { … installNat64 … installNat66 … })`
- `coretest/npt66.go:105-106`: `RegisterExtension("<slug>-npt66", func(v *VPP) { npt66Models.Store(v, v.installNPT66()) })`

Other collisions: none.
- The simulation with the full registry passes once the stub is gone.
- This branch never hooks `feature_is_enabled`, so it cannot clash with F-rpf-adl-pbr, F-bridge-l2 or F-loopback (TD-23's `RegisterFeatureIsEnabled`).
- No other worktree's coretest models `nat44_ei_*`, `nat64_*`, `nat66_*` or `npt66_*`.

### M2: D-132 — two walk locks after the merge, not one
- ED's fix round 2 (`88de9a56`) added a per-Service, context-aware walk slot, `s.natWalk(ctx)` (`rpc_nat44_ed.go` in ED@4421baec: lines 64-73, used at 163 and 252).
- This branch serialises EI/NAT64 under its own package `sync.Mutex`, `natVariantWalk` (`rpc_nat44_ei.go:36-46`). That lock is not context-aware: a cancelled caller still queues and then runs its walk.
- After the merge, an ED walk (NatSessions/NatSummary) and an EI/NAT64 walk can hold the barrier at the same time. That breaks "one walk at a time in the agent" (D-132).

**R4 (required):** merge `task/F-nat44-ed-sessions` again (a clean merge; generated files unchanged) and use `release, err := s.natWalk(ctx)` in `natSessionsVariant`. Add a test in which an ED walk blocks an EI walk.

### M3: a NAT64 page costs two whole-table walks and holds the whole table in agent memory
- `ListNat64` calls `src.Sessions(ctx, protocol, 0, 0)` (`actions/…/sessions.go:232`), which materialises **every** owner's sessions: DF-3's `nat64.go:600-630` has no early stop. The scan cap only bounds how many *owned* rows are counted.
- A page with rows then adds a full `nat64_bib_dump` of all protocols (`rpc_nat44_ei.go:119-135`).
- Under D-132's cadence this is tolerable. On a large product table, however, one call allocates on the order of the table size, and paging through the table repeats both walks.

A follow-up or a tech-debt row is acceptable: stream the dump and keep only offset+limit rows plus the counters; dump the BIB only for the protocols on the page; document "each NAT64 page = one ST walk + one BIB walk".

### Low
- **L1** (`sessions.go:193-202`): the port fix treats `r_port == 0` as "defective VPP". On a fixed VPP a real remote port 0 (for example ICMP) would get `il_port` as its remote port. Always take the inside port from the BIB, and swap only when `il_port` ≠ the BIB's `in_port`.
- **L2** (`nat44ei.go:218-246`): EI identity mappings **with a port** also reserve on a pool address. `nat44_ei.c` sets `l_addr = e_addr` for identity and then takes the same `reserve_port` path at 2487-2497. Extend `nat.ei-port-forward-pool` to them; otherwise VPP answers NO_SUCH_ENTRY at apply time (422) instead of a 400 with a pointer.
- **L3** (`rpc_nat44_ei.go:32-34`): an unknown enum value (> NAT64) falls through to ED. Answer INVALID_ARGUMENT instead.
- **L4** (screens `nat-nptv6-fa-rtl.png`, `nat-nat64-fa-rtl.png`): strings left in English in fa.
  - This task's: the NPTv6 `external` help "Network prefix — host bits must be zero" (no `field.external.help`), and the NAT64 `inside`/`outside` help "IPv6-only side" / "IPv4 side" (schema text, `nat.ts:301-302`).
  - Not this task's: "seconds" and the pager's "of", which come from ui-kit.
- **L5** (`npt66.go:141-147`, `npt66.md:31`): a binding whose interface was deleted first (a crash mid-transaction, another slot, manual) stays on the freed `sw_if_index`, because npt66 has no interface-delete hook while `feature.c:696-738` clears the features.
  - The next add on a reused index then overwrites the stale entry and **never re-enables the features**: the binding is silently inert while the agent reports APPLIED. V-new (a) mentions the hook; the descriptor page should state the hazard.
  - Also, "the agent forgets what it applied" is not exact: the persisted claim keeps the key, which a later garbage-collection pass could use. The delete needs only the interface.
- **L6** (`service.ts:29-39`, `en/…json:84-88`): `GET /state/nat/nptv6` reads the running configuration, yet the UI labels it "applied (write-only)". The API cannot know that. Prefer "configured (not readable)".
- **L7:** the EI tab is always shown, with a mode bar, whereas the prompt said "shown when mode ei". It is acceptable UX; note it in the status file.
- **L8** (`nat64.go:52-73`): the agent does not reject two NAT64 prefixes in one VRF; the schema does (`nat.nat64-valid`). VPP keeps one per VRF and overwrites (`nat64.c:1195-1215`), so a direct gRPC client would get a verify failure. Add the check for parity with the npt66 one-per-interface check.
- **L9:** the single unreproduced `internal/agent` race failure (above). If it recurs, it belongs on D-121's flake list.
- **L10** (`fake.ts:56-60`): the fake `natSessions` overrides ED's by spread order, on purpose, delegating unset/ED to ED's handler. TD-23 has no guard for unary handlers, so keep the anchor order at the rebase.

## Q2–Q9 recommendations

- **Q2:** keep all four edits. Add item 6, ED's `nat44_ei_show_running_config` stub removal (M1/R3). Also note the one-line link in ED's `docs/user/firewall/nat44.md` for the merger.
- **Q3:** OK. Merge ED's head again for fix round 2 and use `natWalk` (R4). The EI adapter already follows ED's streamed pager.
- **Q4:** accept option (a): write-only, V-new (a), no D-076 record (verified against `npt66.c`). The product owner does not need to wait for (b) before the merge. Add L5's hazard to V-new (a) and the descriptor page.
- **Q5:**
  - (b) accept, with L1;
  - (c) is H1/H2: R1 and R2, plus the core TD row and the product-owner decision;
  - (d) accept, with L2.
- **Q6:** accept. It is documented, it needs no agent change, and it matches VPP (outside keys in FIB 0).
- **Q7:** the TD-11b claim is correct. D-132 needs R4. WEB-1 is OK.
- **Q8:** accept. Not this task; no rerun. The next "before" value is NRestarts 2.
- **Q9:** accept. The D-112 rebase onto main replaces P08's pre-merge `interfaces_test.go`, and the merger runs main's gate on the rebased branch.

## For the focused verify

Check R1–R4 only:
- R1: a builder unit test for the tenant-VRF warning, and the doc and V-new wording;
- R2: the opt-in gate, and the note in the questions file;
- R3: the TD-23 simulation passes, or the equivalent after TD-23 merges;
- R4: `natWalk` is shared, with a test in which ED blocks EI.

M3 and L1–L10 may go to docs/tech-debt.md.

Housekeeping: to run the web and API tests I built the `dist/` of `packages/{schema,proto,api-client,ui-kit}` and `apps/api` in this worktree. They are gitignored, and deleting them was not permitted in this session. The worker or the merger should remove them.
