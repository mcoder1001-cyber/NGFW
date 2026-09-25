# F-srv6 — SRv6 policies, local SIDs, steering (status)

Branch `task/F-srv6` (worktree `/root/ngfw-wt/F-srv6`, slot 4), base `main@1d3ccf31`. Envelope:
`docs/status/tasks/F-srv6.envelope.md`; contract notes: `F-srv6-contract.md`; questions: `F-srv6-questions.md`.

**Host runs are pending TD-25** (manager: af_packet creates on the shared VPP fail until TD-25 merges; host runs stay
closed until the manager opens them). Everything else is built and verified on the fake VPP model and the fake agent;
the host steps are written and listed under "Pending host steps".

## What was built

| layer | files | what |
|---|---|---|
| schema (contract) | `packages/schema/src/domains/ext/srv6.ts`, `semantic/srv6.ts` (+ tests) | `routing.srv6{encapSource, encapHopLimit, localSids, policies, steering}`; rule ids `routing.srv6-*` (canonical form, unicast SIDs, SID≠BSID, behaviour fields, VRF/interface exist, encap source D-074, steering BSID/encap/unique, L2-steered interface without addresses) |
| proto (contract) | `packages/proto/vrx/v1/dataplane.proto` | `RoutingConfig.srv6 = 17`, `Srv6Config/LocalSid/Policy/SidList/Steering`, rpc `Srv6State` + `Srv6State*` messages |
| agent | `internal/desired/srv6.go`, `internal/subsystems/srv6.go`, `internal/agent/rpc_srv6.go`, `descriptors/sr/{register,stats}.go` (gap), `core/coretest/srv6.go` | projection onto DF-6's `sr.*` descriptors and Retrieve assembly; sr family wiring (PairClaims("df6"), globals flag); Srv6State (claimed objects, counters, one walk at a time); TD-11b declaration for the df6 globals; the fake VPP sr model |
| API | `apps/api/src/features/srv6/**`, `test/e2e/srv6.e2e.test.ts` | `GET /api/v1/state/srv6` (Srv6State joined with the running configuration's VRF names), the fake agent's Srv6State |
| UI | `apps/web/src/domains/vpn/srv6/**`, `locales/{en,fa}/srv6.json` | SRv6 tab on the VPN page: Local SIDs / Policies / Steering, SchemaForm, SID-list editor, counters column |
| docs | `docs/user/vpn/srv6.md`, `docs/agent/descriptors/sr.md`, `docs/contracts/proto.md` (F-srv6: Srv6State), `docs/vpp-code-track.md` (V-new F-srv6) | user guide with the L3VPN-over-SRv6 example and the CLI equivalent; limits |
| tests (host) | `internal/agent/srv6_integration_test.go`, `test/topology/srv6/stack.sh` | written, run after TD-25 |

(Evidence, shared hunks, decisions and CI: completed at the end of the task.)
