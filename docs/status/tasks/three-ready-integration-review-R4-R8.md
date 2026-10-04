# Three-ready integration — independent R4/R8 delta review

Exact integration SHA `6eda5e94acca007bd290e37ab74d53ba45157e30`; reviewed against the individually approved final products LDP `cb2560ed`, PIM `997d4fa4`, detectors `5c88cb5f` and their final documentation. No product edits by this reviewer.

No BLOCKER/MAJOR/MINOR findings. **R4 APPROVE; R8 APPROVE.**

Independent blob comparison confirmed every changed nonshared product/test file matches its approved input, except the LDP installed-count test has additional retrieval-failure/missing-reader assertions; no production delta in that file. Shared `subsystems.go` preserves exactly one `registerP12`, followed by exactly one `registerPim` and one `registerMplsLdp`, with both errors propagated. The automatically merged FRRDoc independently clones PIM and MPLS/LDP into separate fields and preserves FRR content/interface mapping.

Coexistence: source names `pim`/`mpls-ldp`, descriptors `mfib.route.pim`/`mpls-route.ldp`, renderer section names and order 650/600 are distinct. No configuration Domain claims either dynamic family. AddDynamicSource rejects duplicate names/descriptors and cross-domain ownership. PIM and LDP cache/ownership records are separate; they share the established FRR runtime/runner and transaction-serialized S1 scheduler. The existing agent starts each Run once after resync with its own waitgroup entry, cancels the common context on shutdown and waits for both; each source releases its ticker and bounded calls. Neither source tears down the other's FRR processes, cache or descriptors. Detector subscriptions/timers/lifecycle remain separate and unchanged from the reviewed final input. No binapi, host globals/startup, daemon units, schema, packaging or cleanup boundary changed in integration.

Independent exact-integration verification, cwd `integration/apps/agent`:

```text
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/subsystems -run 'Pim|Ldp|AddDynamicSource'
ok ngfw/agent/internal/subsystems 1.090s
```

This delta review preserves the per-task R4/R8 findings and approval scopes. Real FRR/VPP forwarding/session/restart/packet acceptance remains lab-deferred; no whole aggregate CI gate pass is asserted by this report.
