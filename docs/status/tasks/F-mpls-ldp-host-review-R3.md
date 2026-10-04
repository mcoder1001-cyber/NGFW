# F-mpls-ldp-host — R3 contracts/API review

Reviewed exact head: `339f400c1a137dcc0e988223d368b4ba97704420` against `main`. Independent reviewer; no product edits.

Findings: no BLOCKER, MAJOR, MINOR or NIT contract findings.

Existing `MplsLdp`, label-range, neighbor/binding/sync response and MplsLdpState RPC fields remain unchanged. FRRDoc projects the existing optional LDP configuration by protobuf clone, preserving original desired state. The RPC uses existing owner/cancellation/unavailable semantics; no REST route or DTO changes. Prospective labelRange now filters cached routes, with regression passing; existing nil label-range remains supported.

Executed contract-path check:

```text
git -C /workspace/scratch/e4f791ef53f7/ldp diff --name-only main -- packages/schema packages/proto apps/agent/gen packages/api-client/src/generated
(no output)
```

No schema/proto/generated directory was changed, so no new contract commit or migration is needed. Generated files were not edited by this reviewer; generation/full CI was not rerun or claimed. Existing public messages and service declarations were read directly. Behavioral verification: LDP prospective label-range regression PASS; PIM snapshot-cap preservation regression PASS; detector port-window/event compatibility tests 8/8 PASS (commands/output in R2 reports). Live lab acceptance remains outside R3 verification.

Verdict: **APPROVE**.
