# Checkpoint

Root owns the named branch/worktree in the envelope. Native REST commit currently returns a false unsupported IPsec warning from the WireGuard builder. IPsec and PKI implementations are registered; remove only their obsolete blanket warnings and preserve remote-access and intrinsic feature guards.

Before-fix regression TestWireguardOtherVpnCapabilities failed for both IPsec and PKI, with remote-access warning observed, on base 40fa9fe (later fast-forwarded to unchanged relevant code on d6e1646). Private execution log: /root/ngfw-wt/logs/two-ready-vpn-warning-before.log. No active command at checkpoint.

Next: focused desired/PKI/native tests, independent reviews, unchanged full quick gate and hosted CI. Not Done.

After-fix focused desired/subsystems WireGuard, PKI and IKEv2 tests PASS (0.115s / 0.182s); actual log /root/ngfw-wt/logs/two-ready-vpn-warning-after.log. Both obsolete warnings removed; remote-access warning asserted retained. Independent source review and complete quick gate next.
