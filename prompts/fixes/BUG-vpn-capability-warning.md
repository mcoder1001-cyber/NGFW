# BUG-vpn-capability-warning

Necessary prerequisite for TEST-traffic-B native IPsec REST acceptance. Remove obsolete blanket IPsec/PKI unsupported warnings from the WireGuard builder now that their own registered implementations validate them. Preserve remote-access unsupported warnings and every intrinsic IPsec/PKI validation, licence and secret guard. No API contract or privileged host changes.

Owned: apps/agent/internal/desired/wireguard.go and wireguard_test.go; this prompt; docs/status/tasks/BUG-vpn-capability-warning*; append only the named board row and final verified backup closeout metadata.

Require independent R1/R2/R4/R7 review, unchanged complete quick CI, exact hosted CI and expected-head merge, followed by main CI. The regression must fail before the fix and pass afterwards. Native PSK/certificate REST packet acceptance remains the separate traffic task.
