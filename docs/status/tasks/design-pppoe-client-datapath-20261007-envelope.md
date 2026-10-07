# PPPoE client datapath design envelope

Owner assignment 2026-10-07: continue source-only design after the published topology-prerequisite audit, without rebuilding existing development.

Branch `codex/design-pppoe-client-datapath-20261007`; worktree `/root/ngfw-wt/design-pppoe-client-datapath-20261007`; origin/main base `3ddb1680e475e94d43e8036cd3776bc60c87208b`.

Owned paths only:
- `docs/status/tasks/design-pppoe-client-datapath-20261007-envelope.md`
- `docs/status/tasks/design-pppoe-client-datapath-20261007-wip.md`
- `docs/decisions/PENDING-pppoe-client-datapath-20261007.md`

Deliver a source-backed comparison of bounded single-WAN and multiple-WAN client/server-coexistence mechanisms, exact VPP C and security boundaries, configuration-only limitations, verification plan, agent-hour effort/reversal estimates, recommendation and concrete approval question. Read current mirror and historical `4d0260dc7` and preserve source provenance. Required AGENTS/context/contributing/decision instructions and PPPoE prompts have been read; historical workflow/OS/gate rules yield to current owner instructions.

No product/VPP C edits, old-plugin restoration, host package/unit/sysctl/security changes, live acceptance or shared mutations. No slot/daemon allocated. Existing IPv6 developer/review branches remain their owners' work; P12 runner remains manager-owned. OSPF scoped evidence goes to manager reconciliation/future acceptance, never false DONE. Decision-policy #4/#7 plus the explicit no-C implementation scope require separate owner authority before datapath implementation; pending owner decision does not stop this design/evidence work.

Commit coherent checkpoints immediately, publish each to this named branch, and keep WIP/envelope current within15minutes. Manager owns board/LOG changes, independent review, full hosted quick and expected-head integration. This design task does not merge.
