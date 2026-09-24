# F-rpf-adl-pbr — review (architecture focus)

Reviewer: independent review agent, 2026-09-24. Branch `task/F-rpf-adl-pbr` @ `8d6782a`; diff base `task/W-seed...task/F-rpf-adl-pbr`
(16 commits, 67 files). Scope: architecture questions (1)–(6) from the manager plus the REVIEW-PROMPT checklist where it bears on them.
No host runs: no VPP, no lab lock, no `tools/ci.sh` (manager instruction). VPP claims were checked against the pinned source in `/root/vpp`
(`c3200b88d`, 26.06).

**Verdict: APPROVE WITH CHANGES.** The design is sound and the VPP analysis holds up against the source. Two changes are needed before merge:
a test harness that will panic once F-acl merges (M2), and a permanent false drift for ADL, which needs a one-line manager change (M3). The
manager also has two merge-time items (M1). Everything else is Low/Info.

## Unit tests run by the reviewer
```
$ cd apps/agent && go vet ./... && go test -count=1 ./...        → exit 0 (all packages ok)
  ok ngfw/agent/internal/descriptors/{abf,adl,urpf,auto_sdl}  ok internal/desired  ok internal/subsystems  ok internal/agent (7.6s)  ok internal/scheduler
$ packages/schema: vitest src/semantic/rpf-adl-pbr.test.ts        → 10 passed
$ apps/api:        vitest src/features/rpf-adl-pbr                → 3 passed
$ apps/web:        vitest src/domains/routing/rpf-adl-pbr src/nav → 13 passed (2 files)
```
Merge check against current `main` (P08 merged), done with `git merge-tree` only: the branch does not merge cleanly (`subsystems.go`
add/add, `app.module.ts`, `router.tsx`/`nav.ts`/`i18n.ts`, `dataplane.proto`, generated files). `merge-tree main task/W-seed` gives the same
conflict set, so all of it comes from `task/W-seed` not being on main yet. This branch adds nothing to it. Merge W-seed first, then this branch,
then regenerate (rule 3).

## Answers to the six architecture questions

### (1) Q2 — the `pbr.acl-ref` stand-in: sound. Keep it; do not reorder the merges
- **Declarative model:** it is an observe-only descriptor (`DeleteOnAbsence() == false`), it belongs to no domain (so it is never in a
  transaction's scope), it never writes, and it resolves `acl.acl/<name>` through `KeyProvider`. The scheduler resolves a direct key before an
  alias (`reconciler.go` `resolves`), and aliases never plan operations. Its Retrieve runs in `plan` with `strict=false`, so a failing
  `acl_dump` only drops the alias and never fails the transaction. This is the same pattern as DF-1's `interface/<name>` alias (D-065).
- **No duplicate-registration panic in the product:** the name `pbr.acl-ref` is distinct from `acl.acl`. The switch-off (`active()`,
  `subsystems/rpf_adl_pbr.go:270-276`) is decided on the registry at plan time, so it does not depend on registration order under the A1
  anchors. The registry is fixed after start, so the answer is the same throughout a transaction. **The test harness does panic, though:**
  see M2.
- **No silent behaviour change when F-acl lands:** in the product, nothing creates owner-tagged ACLs before F-acl (the `acl` domain is
  unimplemented, so `acl.lists` is never applied). So in the product build the bridge resolves nothing, and every commit with a PBR policy
  fails with `agent.dependency-missing` (`TestRpfAdlPbrWithoutFAcl` proves it). After F-acl, the ACL comes from desired `acl.acl/<name>`, and
  ordering becomes managed: ACL before policy on create, policy before ACL on delete. That is a strict improvement and not a silent change. The
  "F-acl registered" path is already exercised, because the harness registers DF-4's `acl.acl` (`withACL=true`) in the fake and host tests.
- **Recommendation:** keep the bridge and let F-rpf-adl-pbr merge first. Making PBR wait for F-acl gains nothing, since the product
  behaviour before F-acl is identical either way, and F-acl already carries a soft dependency on this task. What the bridge buys is testability
  (the real-binary screenshots and TestRpfAdlPbrWithoutFAcl). It is ~110 lines that go inert after F-acl, so **put its removal into F-acl's
  envelope** (the "3-line removal" plus `TestRpfAdlPbrWithoutFAcl`). Also fix M2 and L1.

### (2) Contract commits `b7439d4` / `63be7be`: additive only, numbers exact
The proto numbers are exactly as allocated in wave-A-hotspots §2: `Interface.urpf = 18`, `Interface.adl = 19`, `RoutingConfig.pbr = 11`,
`ServicesConfig.auto_sdl = 8`. Each is under its own anchor. `Subinterface` 16–17 are unused, and nothing else is numbered in an existing
message. The new messages are in the `// ----- F-rpf-adl-pbr -----` section. The schema keys are one line each under the task anchors, and
the sub-schemas live in `domains/ext/rpf-adl-pbr.ts`. The shapes match the prompt's contract. No existing field is renamed or reshaped. Every
later edit to a contract path is a regeneration (`ee6a3ef`: api-client for the new route). Cosmetic issue: see L6.

### (3) The V23 (a) Retrieve fix and the ADL allow-list workaround: correct and restart-safe
- **V23 (a):** `vnet_feature_is_enabled` (`vnet/feature/feature.c:327-345`) returns negative errors for an unknown arc, an unknown feature,
  and an out-of-range `sw_if_index`. The handler turns each of those into `true`. The control query is `ethernet-input`, which is
  device-input's `last_in_arc` (`devices.c:32-38`). It becomes the config's default end node (`config.c:180-183`) and is never in a feature
  vector, so it reads `true` only when the query failed. The local0 probe is reached only after the control query shows the index is inside
  the vector, so index 0 is in range and its answer is real. Both "true" error sources are therefore excluded, and the check is a pure read,
  so it is restart-safe. The fake and host tests exist (`TestInterfaceRetrieveV23`, `TestADLRetrieveV23OnHost`).
- **ADL crash vector (V-new):** confirmed. `adl.c:283-307` stores the result of `vnet_config_del_feature`, which is `~0` when the feature is
  missing (`config.c:406-408`), as the family's config index. `default-adl-allowlist` is a stub (`node.c:272-284`). Every call moves *every*
  family's instance count by ±1, so after n calls all three families share the parity of n. A single-family state is therefore unreachable,
  and **the two-call sequences are the minimum possible, not just a workaround**. The add `(1,1,1)+(ip4,ip6,0)` and the remove
  `(!ip4,!ip6,1)+(0,0,0)` never delete a family that is absent and never leave "default" configured.
- **Restart safety:** the applied-once record is keyed by boot identity plus `sw_if_index` and holds fib and families:
  - agent restart on the same VPP: nothing is sent (evidence: 0 calls);
  - VPP restart: exactly one add sequence (fake test);
  - Delete removes only what the record says.

  `adl.interface`'s optional dependency on `adl.allowlist` gives the right order in both directions, and `executor.dependents` follows
  optional dependencies, so a recreate of the allow-list wraps adl-input. Refusing `defaultAllow: false` is correct given the stub. Two small
  gaps remain: L3 and L4.

### (4) Policy-id hashing with a persisted name record: no correctness collision, one churn case
Ids are unique by construction (linear probing), stay inside the range, and are stable across agent restarts because they are a pure function
of the name set. Slot ranges are disjoint on the shared host. The record is a reverse map only, so ids are **not sticky**: see L2 for the
renumbering case and its odds. In the product range (2^31) the risk is negligible. Foreign ABF policies are not skipped by the probe: in the
product, an operator-made policy at the same id makes Create fail and the transaction roll back. That is safe, but the error is obscure. Info.

### (5) Hotspot discipline and the hunks outside the file list
- A1, A2, C1–C5, P1, W1–W3: every hunk sits under this task's anchor. Every Register call passes a Wiring store: `KeyedClaims("acl")`,
  `BootStore()`, and the state-dir name store (review-checklist line satisfied). The schema import lines have no anchor (every peer does the
  same; a trivial union). The formatting churn in web is L5.
- **`coretest/fakevpp.go` seam, justified:** registering urpf/adl/abf in the product registry makes P08's agent tests send their dumps to the
  model, and A6 offers no extension point. But F-nat44-ed-sessions added the **identical** seam, with the same name `extensions`, at the same
  place. `merge-tree` shows a textual conflict in `fakevpp.go`; the manager should keep one copy. See M1 for the semantic hazard.
- **`agent/service_test.go`, justified:** the hard-coded `"interfaces,vrfs,routing"` becomes `implementedDomains()`, and `feature_is_enabled`
  is allowed because it is adl.interface's read-back. F-nat44-ed-sessions and F-object-model change the same lines in the same way, and
  `merge-tree` shows the conflict. The union is trivial.

### (6) `Domains["services"]` first registration vs F-loopback: acceptable, but not merge-safe as written
Being the first registrant is fine, and F-loopback's envelope says it should append. There are three hazards:
- If F-loopback is written in parallel and adds its own `Services = "services"` and `Services: {…}` under *its* anchor (which sits directly
  above this task's, `subsystems.go:55`, `:109`), git merges it cleanly and the Go build fails: duplicate const and duplicate map key.
- `projectServices` hard-codes the only implemented member (`desired/rpf_adl_pbr.go:398`: `fd.JSONName() == "autoSdl"`), so F-loopback's
  nsim/lldp would be flagged `agent.unsupported-field` unless F-loopback edits this task's owned file.
- The unconditional domain-level `agent.unimplemented-domain` on `/services` (`:395`) would hide F-loopback's readable nsim leaf from
  drift.

Fix: see M3.

## Findings, ranked

### M1 — Merge hazard with no textual conflict: coretest `feature_is_enabled` override vs F-bridge-l2 (manager, at merge)
`coretest/rpf_adl_pbr.go:132-143` answers `IsEnabled: true` for every (arc, feature) other than device-input/adl-input and ethernet-input.
It is installed through `extensions`, which runs *after* everything in `New()` (`fakevpp.go:103`). F-bridge-l2 installs its own
`feature_is_enabled` model in `New()` (`task/F-bridge-l2:coretest/bridge_l2.go:503`) for mactime, which is also on device-input
(`descriptors/mactime/enable.go:81`), and `fake.Client.On` overwrites that handler.

**Failure:** after both merge, mactime reads "enabled" on every interface in F-bridge-l2's agent-level tests (or they pass for the wrong
reason). Git reports no conflict. The same override also replaces sanitizetest's `adl_interface_enable_disable` model (`sanitizetest`
`:309`) for every coretest user. Its `acl_dump` will collide with F-acl's model later.

**Fix:**
- At the F-bridge-l2 / F-rpf merge, fold both into one dispatcher keyed by (arc, feature). Better: seed a shared
  `coretest` feature registry in the manager's anchor commit.
- Keep a single `extensions` seam: F-nat44-ed-sessions' copy is identical.
- In this branch, add the collision to the questions file (Q7 does not mention F-bridge-l2).

### M2 — The test harness panics once F-acl merges (must fix)
`subsystems/rpf_adl_pbr_test.go:79` registers `acl.NewACL(c, owner)` *after* `subsystems.Register`. When F-acl adds `acl.Register` under its
A1 anchor, `MapRegistry.Register` panics with `ErrDuplicateDescriptor`. That breaks `TestRpfAdlPbrOnFake`, the policy-name test, and the host
test `TestRpfAdlPbrOnHost`, which uses `rpfService`.

**Fix:**
- Register only when the name is free: `if _, ok := reg.Get(acl.NameACL); withACL && !ok { … }`.
- Add a unit test that the bridge is inert when `acl.acl` is registered: its Retrieve returns nil, `ProvidedKeys` returns nil, and it sends
  no `acl_dump`.

### M3 — Write-only leaves show as permanent drift; the services domain is hidden by a domain-level note (manager line + branch change)
**Failure:** P08's `driftOf` skips only `agent.unsupported-field` and `agent.unimplemented-domain` (`apps/api/src/state/state.controller.ts:412`,
now on main). So every interface with ADL shows `adl.{ipv4,ipv6,allowVrf,defaultAllow}` as drift in `/state/drift` for ever. The Warnf is at
`desired/rpf_adl_pbr.go:225`. Q6 raises this but it is unresolved. To avoid the same problem for autoSdl, the branch marks the whole
**implemented** `services` domain `agent.unimplemented-domain` (`:395`). That misstates Health, and together with the hard-coded member list
(`:398`) it causes the F-loopback hazard in (6).

**Fix:**
- **Manager:** add `'agent.write-only'` to `COVERAGE_RULES`. That is one line in a main-owned file, done before or with this merge.
- **Then, in this branch:**
  - Drop the domain-level `/services` note and emit a field-level `agent.write-only` note on `/services/autoSdl`.
  - Replace the hard-coded `autoSdl` exclusion with an exported, append-only set of implemented services members (for example
    `desired.ServicesMembers`, one line per feature in its own file).
- **F-loopback's envelope:** reuse the existing `Services` const and map entry, and append its descriptor name there.

### L1 — The bridge: docs gap and fail-open lookup
- `docs/user/routing/rpf-adl-pbr.md` gives a full `acl.lists` + `routing.pbr` example. It does not say that until F-acl merges, commits with
  a PBR policy fail with `agent.dependency-missing`, because ACL lists are not applied yet. Add one paragraph, and one line in the status
  file.
- `subsystems/rpf_adl_pbr.go:98` `reg, _ := r.(registryLookup)` fails open: with a non-`MapRegistry`, the bridge stays active for ever. It is
  harmless, because a direct key beats an alias. Log a warning, or pass the lookup explicitly.

### L2 — Policy ids are not sticky
`desired/rpf_adl_pbr.go:95-118` recomputes ids from the sorted name set. When an earlier-sorting name that collides is added or removed, a
later policy moves to another id. Its `abf.attach` objects are deleted and recreated in the same transaction, so for a moment its traffic
follows the FIB. For PBR used to force traffic via a VPN or inspection path, that is a short policy bypass. Birthday odds in the 1000-id
slot range are ~4 % at 10 policies and ~17 % at 20; in the product range they are negligible.

**Fix (optional):** seed `PolicyIDs` with the recorded ids from the `pbr.policy` store (through `RpfAdlPbrEnv`) and probe only for new names.
Or accept the behaviour and document it.

### L3 — ADL allow-list sequence runs with adl-input on in two paths
- `descriptors/adl/adl.go:394` is the Create branch "same index, different value", which happens after a restart when the value changed.
- The scheduler's undo of a recreate (`scheduler/reconciler.go:1063-1068`) does not wrap dependents.

In both paths remove+add runs while adl-input may be enabled. Between the calls, the default family briefly holds one instance, so non-IP
frames arriving in that window leak buffers. There is no `~0` path.

**Fix:** switch adl-input off and restore it around that branch, or document it in `docs/agent/descriptors/adl.md`.

### L4 — V-new / V23 text: allow-list instances survive interface delete
`adl.c` `adl_sw_interface_add_del` deletes only the default `*-input` feature on interface delete and re-adds it on add, so allow-list
instances stay on the index. The V23 row's "(ADL per-index config is re-initialised on interface add — not affected.)" is wrong according to
the source. That text is TD-3's, not this branch's. The record handles reuse of the same index for our own key. An out-of-band delete leaves
instances that the next interface on that index inherits; they are inert while ifsanitize keeps adl-input off, and a blind reset is impossible
(no dump; a blind remove would itself store `~0`).

**Fix:** correct the V-new/V23 text; no code change.

### L5 — Reformatting outside the anchors (rule 2)
`9ae52b7` Prettier-reformats `apps/web/src/router.tsx` as a whole (+67/−16) and parts of `nav.ts`/`nav.test.ts`. `merge-tree` against
F-vrf-static-ecmp, F-bridge-l2 and F-nat44-ed-sessions shows no conflict, so the cost is review noise.

**Fix:** reduce the change to the anchor lines.

### L6 — Misleading contract subject
`63be7be contract(proto): prettier-format the rpf-adl-pbr fixture` changes no proto; the proto change is in `b7439d4 contract(schema)`. The
status file cites `63be7be` as the proto contract. Reword it in the status and contract files.

### Info
- `rpfAdlPbrState` is process-global (`subsystems/rpf_adl_pbr.go:66-75`) and the last Register wins. That is fine with one agent per process
  and fragile with two services in one test process. It is pragmatic because `project()` (A2) does not carry the Wiring.
- Auto-SDL: FEATURE_DISABLED without the session SDL backend is confirmed at `auto_sdl_api.c:26-28`. The descriptor is registered only for the
  globals owner, its disable-then-enable is justified by `auto_sdl_config`'s re-register path, and it is counted as write-only.
- Q5, the semantics of `defaultAllow`, is a product-owner decision. The branch's reading ("non-IP passes", `false` refused) is the only safe
  one on 26.06.
- CI: the reviewer did not re-run `tools/ci.sh` (no host runs in this review). The pasted run used a scratch `ci.sh` carrying main's `7edac8c`
  SIGPIPE fix, which is now on main. Re-run the gate on the merge commit.

## Required before merge
1. M2: guard the harness registration and add the inert-bridge test.
2. M3: the manager adds `agent.write-only` to `COVERAGE_RULES`; the branch drops the `/services` domain-level note, adds the field-level note
   on `/services/autoSdl`, and adds the append-only services-members set.
3. M1, at merge (manager): one `extensions` seam; fold the `feature_is_enabled` handlers with F-bridge-l2; merge W-seed to main first.

**APPROVE WITH CHANGES**
