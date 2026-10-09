# PPP VLAN carrier R1 correctness and tests

Baseline local `1b8c55df`. Product source `2d5963a6a14b8a3c2d62966f6e52daa5a5ee487a`; final focused-test freeze `8ea0d371ced30600f791b8f9eadcd0340531e0e3`, independently observed local tree `54dc53586b8baa16e9d35d2348427ea6ea0247ee`. Author reports published `47d75d83f0cf5724260ee58f09aad20787279753`, tree `757c73811b4f39103923aaacd3dfb409dfc20c11`; manager must verify product correspondence, since review/publication receipt differences can change whole trees.

No scoped source correctness finding. Configuration resolution preserves explicit canonical VLAN child identity, selects only that leaf, rejects conflicting attachments and clones daemon references without mutating source configuration. Multiple selected children survive reference merging while unrelated siblings remain excluded. POP_1/POP_2 projection uses the existing VTR descriptor and explicit XC dependency; no new generated VPP bindings or raw TAP push are introduced. Live checks require owned child identity consistent with its parent/sub-ID, exact one/two-tag classification, valid tag IDs, expected POP and an owned untagged raw TAP. Wildcard/default/inconsistent classification, foreign child ownership and existing admission rewrite fail closed.

Independent focused commands:

```sh
go -C apps/agent test -race -count=1 -v -run '^TestCarrierVLAN' ./internal/descriptors/pppoe ./internal/desired
pnpm -C packages/schema exec vitest run src/semantic/pppoe-parent.test.ts src/semantic/pppoe.test.ts
```

Go 1.26.0 race execution returned exit 0, no skips: configuration/manifest, exact live ownership (including QinQ dot1ad and missing-inner negative control), readiness/admission and projection dependencies PASS. Package times: descriptors 1.037s, desired 1.073s. Focused schema returned exit 0: 2 files, 15 tests PASS, duration 1.96s. Independent `git diff --check`: PASS. No aggregate CI or native packet/host operation performed.

## Required combined integration

This foundation does not wire full projection, namespace admission or verified readiness. Parent must actually invoke the resolver/projection/reference/dependency/admission/readiness seams, preserve rollback/restart ordering, and check physical-root bond/LCP/L2 exclusion for selected VLANs. Require combined graph/readiness tests before complete product acceptance.

The readiness helper validates live classification but its CarrierSpec does not contain configured outer/inner VLAN IDs. Combined readiness must additionally compare the current live classifier to the current desired subinterface tags; merely accepting a valid but different VLAN on the same named child must not establish readiness. Add a wrong-but-valid tag mismatch negative test in the actual combined wiring. Native discovery/session packets over VLAN and QinQ remain outstanding lab acceptance; upstream inverse-POP/push source receipts do not replace packet evidence.

Optional leaf MTU currently follows existing explicit leaf-field projection rather than inheriting the root field. Combined carrier validation must document/test its effective MTU policy; this review makes no claim about physical tag overhead or native MTU acceptance.

Verdict: APPROVE scoped helper/schema source and independently executed focused controls. Whole VLAN PPP acceptance remains pending actual parent integration/readback checks, final unchanged gate and native packet acceptance.
