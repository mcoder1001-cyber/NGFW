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

Logs: /root/ngfw-wt/logs/two-ready-vpn-warning-before.log and two-ready-vpn-warning-after.log. Independent R2 APPROVE: five native/desired tests plus actual PKI refusal overlay and gitleaks PASS. Independent R4 APPROVE: retained host/privilege boundaries and eleven native/desired tests PASS. Reports committed alongside this document. R1 APPROVE, T1 PASS and R7 APPROVE. Unchanged complete local quick passed30m15s on exact source dfa993405/tree53ad32352e21300e6e1a7a018c113e3e83427a34, with fresh149/149 startup checks (two shards59/0 and90/0). Independent intrinsic ACME/HSM/native/certificate refusal probes passed0.143s. Actual full output and individual reports are committed here. Final exact single-commit PR192 head dada9d03c6da4a0fc0b4814827506b5bbeb9263b hosted quick [37362476115](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37362476115) SUCCESS. Expected-head merge produced4788c90f4adaf57737b0cb8394452eadd58d53a1; actual main complete quick [37365022569](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37365022569) SUCCESS. [PR192](https://github.com/mcoder1001-cyber/NGFW/pull/192) merged; Done. Native composed traffic acceptance remains the separate TEST-traffic-B task, including its newly reproduced DHCP readiness failure; no packet PASS is inferred here.

Questions: none for this minimal fix. No live host mutation or packet/laboratory PASS claimed. New named board row212 preserves all original211 rows; verified backup closeout modifies only F-backup-restore after actual main CI success.

All source differences after independently tested dfa993405 are documentation/board only under D226; every product file remains byte-identical. Reviewed history is archived before final one-commit integration; exact final squashed range receives gitleaks scanning. No actual protocol or laboratory PASS is introduced by these documentation additions.
