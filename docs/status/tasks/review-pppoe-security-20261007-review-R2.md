# Frozen PPPoE R2 security review

Exact source: `4bcf9f4977e2a9c248548de23cf552e4bcef94da`, tree `7d914836662eb129ada7ac33459b97d4ce449b28`. Independent reviewer; no product changes.

Prior R2-1 and R2-2 resolved. `templates/ipv6.tmpl:89-157` rejects reserved/non-integer PIDs, verifies starttime and refresher argv/generation, pins signals to pidfds, checks child identity, and bounds TERM/KILL exit verification. `paths.go:84` rejects unclean paths and shell expansion characters. Fixed executable argv, validated interface names, network parsing, secret-file modes and password-cleared operational records preserve the existing boundaries. No new API/auth/session privileges or GlobalsOwner exception.

Executed race tests include `TestIPv6PIDIdentityRefusesForeignSignals`, `TestIPv6OwnedShutdownAndLateEvent`, and `TestIPv6PathsRejectShellExpansion`; focused package receipts are in the combined report. Check gate scanned ~696283 bytes with no leaks. No demonstrated injection or secret leak found.

[other: R4/R8] Parent-process identity and configuration-transition fencing remain incomplete; see corresponding reports. This approval covers R2 only and does not authorize integration.

Verdict: APPROVE.
