# BUG-vpn-capability-warning acceptance

Built: remove only two obsolete blanket unsupported warnings for IPsec and PKI in the WireGuard builder. Both features already have registered implementations and intrinsic validation. Remote-access unsupported warning, IPsec/PKI secret refusals and licence checks remain. No API contract, host privilege, systemd or dataplane materializer change.

Decision: trivial deletion of stale checks; existing validators own their capability findings. No new architectural decision is introduced. Test warnings cannot be ignored to make native REST acceptance pass. Native packet acceptance belongs to TEST-traffic-B, not this prerequisite.

Actual before-fix regression on main40fa9fe, relevant code unchanged on fresh main d6e1646:

```text
--- FAIL: TestWireguardOtherVpnCapabilities (0.01s)
W agent.unsupported-field /vpn/ipsec
W agent.unsupported-field /vpn/pki
W agent.unsupported-field /vpn/remoteAccess
FAIL ngfw/agent/internal/desired 0.119s
```

Actual after-fix command on coherent source dfa993405:

```text
GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test ./internal/desired ./internal/subsystems -run 'Test(Wireguard|PKI|IKEv2)' -count=1
ok ngfw/agent/internal/desired 0.115s
ok ngfw/agent/internal/subsystems 0.182s
```

Logs: /root/ngfw-wt/logs/two-ready-vpn-warning-before.log and two-ready-vpn-warning-after.log. Independent R2 APPROVE: five native/desired tests plus actual PKI refusal overlay and gitleaks PASS. Independent R4 APPROVE: retained host/privilege boundaries and eleven native/desired tests PASS. Reports committed alongside this document. R1 APPROVE, T1 PASS and R7 APPROVE. Unchanged complete local quick passed30m15s on exact source dfa993405/tree53ad32352e21300e6e1a7a018c113e3e83427a34, with fresh149/149 startup checks (two shards59/0 and90/0). Independent intrinsic ACME/HSM/native/certificate refusal probes passed0.143s. Actual full output and individual reports are committed here. Initial exact PR192 hosted head e89fe734 run37357809360 passed; final single-commit hosted gate, expected-head merge and actual main CI remain pending. Not Done.

Questions: none for this minimal fix. No live host mutation or packet/laboratory PASS claimed. New named board row212 preserves all original211 rows; verified backup closeout modifies only F-backup-restore after actual main CI success.

All source differences after independently tested dfa993405 are documentation/board only under D226; every product file remains byte-identical. Reviewed history is archived before final one-commit integration; exact final squashed range receives gitleaks scanning. No actual protocol or laboratory PASS is introduced by these documentation additions.
