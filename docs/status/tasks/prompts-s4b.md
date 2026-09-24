# prompts-s4b: wave-A prompt regeneration (D-104)

This was a manager-assistant task on branch `task/prompts-s4b`. It changed docs only; no product code, VPP or daemons were touched.
Sources checked: `apps/agent/internal/descriptors/**`, `docs/agent/descriptors/**`, `packages/schema`, `packages/proto` on this branch
(= main at adeeb05), and P08 read-only via `git show task/P08:<path>` (ffecf13).

## Regenerated
- `prompts/features/F-vlan-qinq.md`. P08 already builds dot1q sub-interfaces end to end, and its `desired/interfaces.go` already passes
  `InnerVlan`/`Dot1Ad` to DF-1's `interface.subinterface`. The schema (`SubinterfaceSchema`) and the semantic rules (`vlan-unique`,
  `subinterface-mtu`) exist too. The task is reduced to:
  - QinQ proof (host + restart + rollback);
  - builder/schema tests;
  - UI tag-stack columns (the table is extracted to `interfaces/subinterfaces/`);
  - `vlan-qinq.md`, plus a one-line fix to `basics.md` ("QinQ not supported").

  Defect fixes in `desired`/DF-1 are allowed only when a test proves one.
- `prompts/features/F-nat44-ed-sessions.md`. The DF-3 `nat44ed` descriptors (+ `Users`/`UserSessions`/`DeleteSession`), the P02b NAT schema,
  its semantic rules and the `NatConfig` proto all exist. The task is reduced to:
  - the `nat` domain builder `desired/nat.go` + registration in `subsystems`/`projection.go` (P08 pattern);
  - a paged `NatSessions` RPC + `NatSessionKillAction` (the only contract);
  - an API module, a NAT page with a `natTabs` hook for the EI/CGNAT siblings, and docs.

  The kill route moved from `DELETE /state/...` to `POST /actions/nat/sessions/kill` (rule 8). `tools/lab restart-vpp` was replaced by the
  agent-restart simulation (handover), and the D-071 globals fixture is spelled out.

Both prompts have Dependencies (DF-1/DF-3/P08), explicit files-owned, gap-only and shared-hunk lists, and a mandatory "Out of scope"
fence that starts with **"Use, do not rebuild"**.

## Board follow-up for the manager (plan/tasks.yaml not touched)
- `files_owned` is empty for F-vlan-qinq and F-nat44-ed-sessions. Copy it from each prompt's "Files you own" list.
- F-nat44-ed-sessions needs a `contract/F-nat44-ed-sessions` branch (proto: NatSessions RPC + kill action; optionally a schema rule for
  adjacent pools, from the DF-3 doc caveat).

## Other wave-A prompts (priority 3, S4): findings

Same problem ("rebuild an existing descriptor"): **none found.** All ten were generated after the factories (commit a74d191 and prompts-s4).
They say "reuse" for the DF-1/2/3/4/7 descriptors, and they own descriptor dirs only for gaps (bond weight, mactime, gso, nsim, svs,
auto_sdl, npt66 — verified absent on main).

Stale references to merged work (same kind, small, **fixed**):
- `F-loopback-bvi-gso-lldp-span.md`: LLDP/SPAN were "read-only on `task/DF-7` until merged". DF-7 is merged, so the prompt now points to
  `descriptors/{lldp,span}/` on main.
- `F-vrf-static-ecmp.md`, `F-neighbors-ra.md`: they pointed to `task/P06 (read-only until merged)` for `state.controller.ts`. P06 is merged,
  and the prompts now note that P08 reworked the file.
- `F-bonding.md`: its out-of-scope line said sub-interfaces work "via F-vlan-qinq's descriptor". It is DF-1's `interface.subinterface` +
  P08's projection.

Not fixed (different kind; needs a board/manager decision):
1. **Agent wiring path vs P08.** Every wave-A prompt (and the board's `files_owned`) gives the worker
   `apps/agent/internal/agent/project_<slug>*.go` plus "one-line appends (agent registry/projection hook)". P08 instead established:
   - builders in `apps/agent/internal/desired/<domain>.go`;
   - registration + `Domains` in `apps/agent/internal/subsystems/subsystems.go`;
   - the hook in `apps/agent/internal/agent/projection.go`.

   Recommendation: after P08 merges, either add `apps/agent/internal/desired/<slug>*.go` to each wave-A `files_owned`, or state in the
   envelope that `project_<slug>*.go` may live in `desired/`. The two regenerated prompts already use `desired/`.
2. **Shared hot spots in wave A** (hunks, not ownership):
   - `subsystems.go` `Domains`: interfaces domain for bonding/bridge/loopback/vlan, nat for ED then EI;
   - `projection.go`;
   - `server.go` Action dispatch: vrf-static-ecmp, neighbors-ra, nat44-ed-sessions;
   - `ActionRequest` oneof field numbers: the same three;
   - `state.controller.ts`: vrf-static-ecmp, neighbors-ra;
   - `InterfaceDrawer.tsx`: vlan-qinq, rpf-adl-pbr "Security" group, bonding tab.

   Recommendation: merge in wave order and have the integrator resolve these; tell the workers the oneof numbers in the envelopes.
3. **Other waves (not wave A, left as is).** `task/DF-7` "until merged" wording remains in F-lb, F-bfd-redistribution, F-mpls-srmpls, F-qos-flat,
   F-igmp-mfib and F-vrrp-config-sync. `task/P06` read-only wording remains in F-dashboard-prom-alarms, F-capture-trace, F-licensing,
   F-backup-restore, F-restconf-yang and F-aaa. DF-7 and P06 are both merged, so regenerate these before waves B/C.
