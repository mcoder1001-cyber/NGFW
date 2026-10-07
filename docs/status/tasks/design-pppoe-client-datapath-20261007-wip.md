# PPPoE datapath design WIP

## Completed design handoff (authoritative)

Source-only design complete. Owner decision remains PENDING; no implementation, live acceptance or support-completion claim. Deliverable: [PENDING-pppoe-client-datapath-20261007.md](../../decisions/PENDING-pppoe-client-datapath-20261007.md). It compares A bounded native single-WAN, B multiple-WAN client+server, C kernel PPP with VPP routed transit/no planned VPP C, and D current diagnostic/mirror limitations; includes exact C/security changes, source provenance, effort/reversal, phased test/integration plan and a concrete owner question.

Recommendation A: one untagged IPv4 client, server mutually exclusive appliance-wide; authorize two historical VPP safety fixes plus an additive read-only CP ownership getter, and only scoped private PPP/runtime unit/owned-namespace sandbox paths. Estimate40–64agent-hours/reversal16–24. IPv6 native traffic remains unsupported until separately proven (follow-on16–32h); existing IPv6 state/helper recovery/reviews proceed unchanged. Multiple-WAN/client+server requires per-parent CP/role demux, collision-safe keys and parent-aware APIs (B96–160h/reversal40–64). C64–104h/reversal32–48 uses a kernel routed hop and needs architectural/security approval; it is not already deployable through current config. No global plugin-disable or Linux NAT accepted as product transit.

Exact initial checkpoint local=verified remote `41c28ae751bf5a30e45434b7746f215e952bba05`, tree `dab763c1b0d94ab1006b4f2055b238c7a561f3ae`; CLI push succeeded and ls-remote matched. Final PENDING/WIP documentation checkpoint is its descendant. Resolve containing commit externally with `git rev-parse HEAD HEAD^{tree}` and `git ls-remote origin refs/heads/codex/design-pppoe-client-datapath-20261007`; final publication must match before reporting success. No remote history rewritten.

Actual host-independent checks:
- Initial `tools/ci.sh check --base origin/main`: PASS0m13s, gitleaks no leaks, board valid212, slot scheme no collisions.
- Design-document check rerun: PASS0m21s; same guards, gitleaks no leaks. Full unchanged quick is still a manager integration requirement; no full local/hosted quick claimed for this branch.
- `git diff --check`: exit0.
- `git diff --quiet origin/main HEAD -- apps deploy packages tools test`: exit0; no executable changes in committed checkpoint. Final owned-file check additionally covers working-tree changes before publication.
- Read-only Python source assertions: PASS singleton cp_if_index/no existing CP getter in pinned API; current NORMAL physical FibPath/no SessionID; historical len(ss)>1 refusal/native NAT role copy; historical native runtime and PPPoE safety patch absent on main. No source or bytecode files written.
- Read-only VPP reference HEAD c3200b88dc46bd380f00a49ca3392a102cc1980b and empty git status; no patch/plugin restored.

No Go/product tests rerun here: there are no product edits. The prior audit's selected three-package race/static checks at the same main source remain source-specific evidence (remote1098a41b22a1ae5a3f433e0230a4471743237530), not fresh native-datapath acceptance. Historical native PASS depends on its private artifact; current product discovery FAIL is retained explicitly. No claim of final independent review/APPROVE.

Final read-only GitHub observation: main3ddb1680e, hosted main gate37590128647 SUCCESS; PPPoE recovery remote4bcf9f497 unchanged. Open PRs198(wizard),197(TD19),196(PPPoE),193(RA),180(P12); none modified by this task. Worker liveness unverifiable. Board/LOG/deferred ledger/VPP code track remain manager-owned. OSPF recovered private proofs remain scoped, awaiting manager reconciliation/future missing acceptance; no false DONE.

Remaining design code: none; no code authorized. Current actionable human prerequisite: select A/B/C/D and explicitly approve its listed support limits and C/security exceptions. Decision-policy #4 and owner no-C/host-change envelope require this approval; elapsed time does not supply it. While pending, exact-source IPv6 review/hosted gate, source/test-plan refinements and manager OSPF/P12 work continue. Implementation starts only in a separate manager-assigned file/branch/worktree envelope after decision; shared host modifications remain unauthorized.

Exact next recovery command: `cd /root/ngfw-wt/design-pppoe-client-datapath-20261007 && git status --short && git rev-parse HEAD HEAD^{tree} && git ls-remote origin refs/heads/main refs/heads/codex/design-pppoe-client-datapath-20261007 && sed -n '1,28p' docs/decisions/PENDING-pppoe-client-datapath-20261007.md`. Read owner decision before planning dependent implementation. Do not run an old plugin/driver or live acceptance from this envelope.

## Initial published source checkpoint (historical)

Branch/worktree/base/ownership: adjacent envelope. Initial local head equals base main3ddb1680e; this source-audit checkpoint publication pending. Exact local SHA `git rev-parse HEAD`; exact remote SHA `git ls-remote origin refs/heads/codex/design-pppoe-client-datapath-20261007`. Containing documentation commit hashes must be resolved externally, not self-embedded.

Prior audit durably published local=remote `1098a41b22a1ae5a3f433e0230a4471743237530`, tree6be8fee0c213ea07fc04db70cc7ccb28f387a4df. Its only changes are its envelope/WIP. Source recovery and plan are available from that branch; OSPF full feature is not declared DONE.

Completed: created authorized isolated design branch from fetched origin/main; read current ClientMirror, globals-only write-only CP singleton, server session descriptor, PPP unit/hook, schema/semantics, architecture/datamodel/master prompt; inspected historical4d0260dc7d7bd992ae5417e6525f7abeda8495dc native runtime/projection/unit/hook/two-hunk safety patch and historical native NAT packet receipts. Current upstream VPP source c3200b88dc46bd380f00a49ca3392a102cc1980b is clean and unchanged by this task.

Findings: current mirror/default route has no PPPoE encapsulation or AC/session metadata. Historical native path refuses more than one client; L2 physical↔LCP cross-connect during discovery, L3 native session and CP feature after IPCP, AC/session hook metadata, default/MSS on session and NAT-outside inheritance. CP is still a single VPP-global index. Historical patch fixes missing-CP control drop and session dump message-base; it does not implement multiple CPs or collision-safe client/server demux. Historical product pass is IPv4 only. Source rewrite supports IPv4/IPv6 PPP protocol values, but one decap FIB and server-oriented API/family handling require separate validation before claiming dual-family client support. Namespace/systemd private paths differ materially from current sandbox.

Actual checks: read-only Git/source/evidence inspection only so far; no live test or product edit. Remaining: complete options/cost/security matrix, bounded recommendation and test plan/PENDING approval question; run documentation/static validation and publish. Current blocker: owner authorization for any future VPP C/security or support-scope implementation, not for this ongoing source design. Worker inventory unverifiable. Main hosted quick37590128647 SUCCESS was observed in prior audit; not a gate on this new branch.

Exact next command: `cd /root/ngfw-wt/design-pppoe-client-datapath-20261007 && tools/ci.sh check --base origin/main`, then commit/push the owned checkpoint and finish the PENDING design.
