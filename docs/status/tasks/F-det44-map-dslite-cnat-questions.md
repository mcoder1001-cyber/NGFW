# F-det44-map-dslite-cnat — open questions (for the manager)

Written and kept going (never waited). Defaults taken are marked **default**.

- **Q1 DS-Lite AFTR/B4 are VPP globals.** Built as D-071 globals (owner sets/resets, slots only require) — **default
  yes** per the prompt. VPP has no delete: reset writes `::`/`0.0.0.0` (vpp-code-track V-new (F-det44-map-dslite-cnat) a).
- **Q2 CNAT + NAT44-ED on the same interface.** **Default: refused** — in the agent projection (rule
  `nat.det44-map-dslite-cnat-cnat-nat44-interface`, pointer at the SNAT policy interface). No TS semantic rule yet
  (the schema/semantic file is part of the remaining work), so the API reports it from the agent's DryRun, not from
  schema validation.
- **Q3 MAP parameters: schema defaults vs VPP defaults.** The schema's `securityCheck.enabled` and
  `trafficClass.copy` default to `false`; VPP's defaults are `true`. The agent treats an *unset* proto leaf as VPP's
  default, so a document that spells the schema defaults explicitly asks for security-check off — a non-default VPP
  global that a slot agent can only require (and will fail on a fresh VPP). Should the schema defaults follow VPP
  (`true`), or should the API drop default-valued leaves? Not changed (P02b's schema file).
- **Q4 cnat interface feature.** `cnat.interface-feature` has no document leaf; the agent enables it on every
  `snat.interfaces[].interface` (the interfaces the SNAT policy consults). Confirm, or add a leaf.
- **Q5 PNAT contract not done.** `nat.pnat` (NatConfig 27) + proto RPCs `Det44Sessions`, `Det44Lookup`, `CnatSessions`
  + ActionRequest 9/10 were not written in this session (time box; order of work dslite+det44+CNAT → MAP → PNAT).
  `descriptors/pnat` stays unwired (reachability `pending`). Follow-up row suggested (D-099).
- **Q6 det44.enable re-apply count.** DF-3's det44 enable is idempotent against VPP (`retval 1` "already enabled"
  tolerated) and records no D-076 AppliedRecord; the fake shows no disable is ever sent. A host check of "re-applied
  exactly once per boot identity" was not possible (no VPP host).
- **Q7 trailer.** The manager envelope's commit trailer names "Claude Fable 5.1"; the session's system attribution
  names "Claude Opus 5.5" (the model that actually ran). Commits use the session attribution.

## Part B (task/F-det44-b)

- **Q3 answered (manager):** schema defaults of `securityCheck.enabled` / `trafficClass.copy` now `true` (VPP's). Done in
  the contract commit.
- **Q4 answered (manager):** kept derived; an info-level notice names the interfaces. The Sink interface has no info
  level, so an optional `desired.InfoSink` was added and `projected.Infof` (ISSUE_SEVERITY_INFO) implements it.
- **Q5 done:** contract, PNAT, RPCs and actions built in part B.
- **Q8 CNAT sessions are VPP-global.** `CnatSessions` returns the whole table to any agent (no owner tag exists in
  `cnat_session_details`); only the purge is restricted (globals owner, D-071, same rule as F-capture-trace's
  `captureGlobalsOwner`). **Default: read allowed for all, purge globals-owner only.**
- **Q9 PNAT names — fixed (review BLOCK 1).** Retrieve takes binding names and binding/attachment order from the last
  applied document, matched by the match tuple (`desired.RelabelPnat`); only bindings the document lacks keep `pnat-<n>`.
  Drift stays empty (service test `TestPnatDomainOnFake`).
- **Q10 pre-existing red gate:** `@ngfw/proto` test "has exactly the 13 root keys" fails on origin/main (not this task).
