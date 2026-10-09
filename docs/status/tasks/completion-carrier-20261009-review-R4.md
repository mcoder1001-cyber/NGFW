# Cumulative PPP carrier R4 source review

Target: local `13379ae8`, product/tree checkpoint `e143000cad04d647dbca220fcc2e93d658ff8225`, published as `c847b8dab684c5048303633e081de3662bc4fc7b`. Review branch `codex/review-security-20261009`; owned file is this report only. This is an interim independent cumulative review, not repetition of prior helper-only approvals.

## Findings

1. **BLOCKER — observation failure retains live automatic defaults.** `apps/agent/internal/subsystems/pppoe_watch.go:74-77,88-91`: an unreadable or malformed hook state clears cached carrier readiness but returns without withdrawing the remembered VPP mirror. A standalone session with `DefaultRoute=true` can retain its default route and address while the kernel carrier remains configured. Clear readiness first, attempt cleanup of the remembered mirror, retain cleanup identity if removal fails, and test actual polling with malformed hook state plus cleanup retry. Assigned to `observation_fix`.
2. **MAJOR — combined readiness/probe failure coverage absent.** Source has tests for individual negotiated-address and NCP helpers, but the reviewed checkpoint does not invoke actual `prepareCarrierForwarding` or `ProbeForwarding` in a focused regression. Add actual runtime tests through fake VPP and the fixed broker adapter: matching topology positive control; foreign/missing link and negotiated-address mismatch rejection; in-flight NCP/session replacement discards the probe result. Assigned to `carrier_finish`.

## Boundary inspected

The implementation separates logical IP transit from exclusive raw PPP transport; uses generated VPP bindings without modifications to binapi or its generator; rejects conflicting LCP, bridge, xconnect, bond, unnumbered and live server attachments. Explicit VLAN children require owned exact classifiers and committed tag matching. The fixed broker admits finite operations and token/boot/nonce/digest/expiry-bound receipts, with a second expiry check after lock acquisition. Namespace mutation and deletion use boot/inode/generation receipts, and missing TAP repair is an explicit dependent recreation rather than adoption.

The PPP child retains its own mount sandbox, read-only ledger and peer configuration, with only the exact resolver output bind writable. Launch drops to NET_ADMIN and NET_RAW with no inheritable/ambient capabilities; fixed daemon/plugin trust is an explicit assumption. This is not containment of arbitrary malicious Ethernet emission by a compromised NET_RAW descendant. The carrier does not add privileges to the main agent unit; pre-existing combined packaging CHOWN/DAC_OVERRIDE changes remain under their separate reviews.

Native installed systemd mount propagation, capability/cgroup enforcement, PPP/DHCPv6 negotiation, VLAN/QinQ frames, real VPP policy/NAT return path and restart packet proof remain NOT RUN. No host service, namespace, sysctl or packet operation was activated. CI was not run, as requested by the owner.

## Independently executed checks

At reviewed product source:

```text
python3 scripts/tests/pppoe-kernel-carrier.py
Ran 28 tests in 0.017s
OK
```

Focused Go race command is still in progress at this checkpoint; descriptor package has reported `ok ngfw/agent/internal/descriptors/pppoe 1.098s`, but the whole command is not yet classified as passing. Exact command: `go test -race -count=1 ./internal/descriptors/pppoe ./internal/subsystems -run 'Carrier|Pppoe.*(Recovery|WAN|Resolver)|PPP.*Forwarding'` from `apps/agent`, Go1.26 with local offline module/cache paths. Next: inspect its completion, review assigned fixes, execute their focused regressions, and record the final exact cumulative source target.

Verdict: **BLOCK** pending the findings above.
