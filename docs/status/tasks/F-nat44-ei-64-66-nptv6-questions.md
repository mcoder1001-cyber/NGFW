# F-nat44-ei-64-66-nptv6 — questions and notes for the manager

Written and kept going (never waiting). Newest last.

## Q1 (info, contract): `contract(proto): nat session variants` is committed on this branch
Additive: `optional NatSessionVariant variant` on `NatSessionsRequest` (5), `NatSessionsResponse` (8) and
`NatSessionKillAction` (7), the enum in my proto section. No new RPC, no `ActionRequest` member, no `NatConfig`
number. Details: `F-nat44-ei-64-66-nptv6-contract.md`.
