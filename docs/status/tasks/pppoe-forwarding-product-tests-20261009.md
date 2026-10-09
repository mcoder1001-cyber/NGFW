# PPP forwarding product controls, 2026-10-09

Owner: toolchain_restore. Branch: codex/forwarding-tests-20261009.
Source baseline: 13379ae8. Owned product file: only
`apps/agent/internal/subsystems/pppoe_forwarding_product_test.go`.

The tests invoke actual PppoeRuntime.prepareCarrierForwarding,
ForwardingGateway and ProbeForwarding, with fake host process and VPP API
boundaries. Positive evidence crosses broker list/configure/verify/probe,
both raw cross-connect directions, owned transit, connected IPv4/IPv6 addresses
and kernel PPP address readback. Negative controls cover foreign/missing transit,
missing reverse cross-connect, negotiated-address mismatch, missing kernel link,
foreign namespace and process replacement. Failed preparation must withdraw old
readiness. In-flight probe controls replace the NCP event, process identity or
published readiness and require Unavailable.

Baseline regression result: `go test -race ./internal/subsystems -run
'^TestCarrierForwardingProduct' -count=1` on 13379ae8 FAILED (0.146s), solely
TestCarrierForwardingProductRejectsNCPReplacementDuringVerification:
"forwarding evidence spanning two NCP sessions accepted". All other controls
passed. Carrier author is implementing a pre/post NCP generation bracket.
This is an actual source defect, not a laboratory deferral.

Next: replay against author's bracket fix and record green result before
integration. No full CI or native laboratory test executed.

## Fixed-source verification

Applied author's product checkpoint `1ab6d30c8357bf3585d70e69f091458d44e56839`
locally as `8cdbcff2`; no product edits by the test author. Same control set,
`go test -race ./internal/subsystems -run '^TestCarrierForwardingProduct' -count=3`,
PASS (1.305s), no skips. This establishes red-before / green-after for the NCP
cross-observation race. The test-only checkpoint is local `148a9e55`, published
as `536598d588d718da3768684cc3cbb9d8c60bf2f1`, identical tree
`25e997e70c2e8e50b2066df785eadf828dd64add`.
