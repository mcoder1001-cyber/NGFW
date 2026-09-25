# F-ipfix-sflow — contract changes (additive)

All on `task/F-ipfix-sflow` as separate `contract(…)` commits (P08 pattern; no own branch).

| commit | scope | change |
|---|---|---|
| `contract(proto): IpfixState RPC and its messages` | `packages/proto/vrx/v1/dataplane.proto` | `rpc IpfixState(IpfixStateRequest) returns (IpfixStateResponse)` under the `wave-BC: F-ipfix-sflow` service anchor; messages `IpfixStateRequest`, `IpfixStateResponse`, `IpfixExporterState`, `IpfixFlowprobeParamsState`, `IpfixFlowprobeInterfaceState`, `IpfixSflowGlobalState`, `IpfixSflowInterfaceState`, `IpfixCounter` in the `// ----- F-ipfix-sflow -----` section. Regenerated Go/TS stubs; UNIMPLEMENTED fake stub + `AgentClient.ipfixState`; `docs/contracts/proto.md` §11 `### F-ipfix-sflow: IpfixState` (unanchored: that file has no wave-BC anchors). |
| `contract(schema): services.ipfix semantic rules + ipfix-sflow fixture` | `packages/schema/src/semantic/ipfix-sflow.ts`, one spread in `semantic/index.ts` (unanchored), `packages/proto/test/fixtures/ipfix-sflow-lan.json` | validators `services.ipfix-sflow-flowprobe-exporter`, `-header-bytes`, `-collector-unique` (and `-flowprobe-variant`, removed by the next commit). |
| `contract(schema): drop the flowprobe one-variant rule` | same files | the shipped `services-snmp-lldp-ipfix-ntp.json` example uses the schema default `ip4`+`ip6`; the agent realises it as ip4 with a warning instead (Q2). |
| `contract(api): regenerate api-client for GET /api/v1/state/ipfix` | `packages/api-client/src/generated/schema.d.ts` | new route only. |

No schema field was added: the reserved `IpfixService` 4 / `.Exporter` 9 / `.Flowprobe` 7 / `.Sflow` 9 are **unused**.
No `ActionRequest` member, no `EventKind`.
