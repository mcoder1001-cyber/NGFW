# RA packet fixture lint checkpoint

Branch: codex/ra-packet-fixture-lint-20261005. Base remote b7e11be2cd1387fcb62042bd8b6c73b6a1a7d83a, local ee8ceb35eae4c1e435c0330c19fb5b770fd80843.

Owned files: apps/agent/internal/ra_vpn/eap_integration_test.go, packet_diagnostics_test.go, vpp_integration_test.go. Product code is unchanged.

Completed: per-test random EAP password is shared by the server resolver and actual client load-shared request; negative password remains distinct. Bounds-check the private message parser. Check private evidence writes/closes and owned temporary cleanup. Clear raw kernel diagnostic data before refusing oversized/error responses. Narrow annotations identify only fixed executables, literal fixture commands and private evidence paths. Existing ESP/no-plaintext, VRF, both ACL denials, session/disconnect, wrong-password and foreign cleanup assertions remain.

Actual verification: complete go test -race -count=1 ./internal/ra_vpn PASS 2.392s. golangci-lint run ./internal/ra_vpn/... exits 1 with 43 findings elsewhere and zero findings in these three owned files; whole lint is not green. Log /root/ngfw-observer-tmp-20261005/packet-three-lint-final.log.

Remaining: independent review; engine must replay the real disposable EAP/VIP/packet integration fixture with these new random credentials. That privileged integration was NOT RUN for this checkpoint. Return these three exact files to engine ownership after publication. Next command: inspect committed diff then consume exactly these paths into the coherent engine supplier source.
