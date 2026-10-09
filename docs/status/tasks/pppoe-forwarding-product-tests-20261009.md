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
