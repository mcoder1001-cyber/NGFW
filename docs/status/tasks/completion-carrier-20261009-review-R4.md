# Cumulative PPP carrier R4 source review

Verdict: **APPROVE** the cumulative carrier source in final local candidate `33db7c632c38e72239e8861bcabf23ac0c45a6aa`, tree `bf2e858290a8789321ba92e30cd944fec23ecd0b` (product freeze `87cbddc0`). Independent reviewer branch: `codex/review-security-20261009`. Only this report was authored by the reviewer; candidate fixes and tests were applied from their respective authors. `git diff` against the final candidate for `apps`, `scripts`, `deploy`, `packages` and `tools` was empty in the review checkout.

This supersedes the interim BLOCK on `13379ae8`. It approves source within R4 scope; it does not certify native packet/service acceptance or substitute for mandatory final CI.

## Findings and resolution

1. **BLOCKER, resolved — observation failure retained automatic defaults.** At `13379ae8`, `pppoe_watch.go` cleared readiness on hook read/parse error but retained the remembered VPP mirror. Standalone `DefaultRoute=true` could retain forwarding indefinitely. Author fix `1f727d75` validates negotiated IPv4 data before replacement, clears readiness first, independently withdraws VPP mirror and verified kernel policy, and preserves failed cleanup identity for retry. Actual malformed/unreadable hook regressions cover both cleanup failures, retries and denied delegated-prefix admission. Independent focused race check passed in 1.167 seconds.
2. **MAJOR, resolved — actual runtime readiness/probe coverage missing.** Test author `148a9e55` exercises the product `prepareCarrierForwarding`, `ForwardingGateway`, `ProbeForwarding` and broker adapter with only process/VPP boundaries simulated. Positive control reaches list/configure/verify/probe; negative controls cover foreign/missing transit, missing reverse cross-connect, negotiated-address mismatch, missing kernel link, foreign namespace and process replacement. In-flight NCP, process and cached-readiness replacement rejects the probe result.
3. **BLOCKER, resolved — NCP generation sampled after verification.** The prior runtime could label kernel/VPP evidence from one NCP session with a new generation after redial inside the same persistent process. Independently running the new regression on the old product produced `FAIL: TestCarrierForwardingProductRejectsNCPReplacementDuringVerification`, with `forwarding evidence spanning two NCP sessions accepted` (package 0.090 seconds). Fix `1ab6d30c` captures generation before the first identity/kernel observation and requires the same generation after all checks. The unchanged regression passes on the fixed cumulative product.

The companion WAN change in `1ab6d30c` wraps the existing core route descriptor with an optional PPP client dependency. Thus a membership change withdraws the standalone default before the WAN route write, and inverse ordering protects rollback/removal. The actual scheduler/client/runtime/route regression exercises an injected WAN route write failure and rollback, successful join, all-down health route removal without restoring a bypass default, and leave.

## Boundary inspected

Logical IP transit is distinct from exclusive raw PPP transport. Generated VPP bindings and their generator are unchanged. Admission rejects conflicting LCP, bridge, xconnect, bond, unnumbered and live PPP server attachments. Explicit VLAN children require owned exact classifiers and committed tag matching; unrelated siblings are preserved. The fixed broker admits finite operations and token/boot/nonce/digest/expiry-bound receipts, with a second expiry check after lock acquisition. Namespace mutations and deletion use boot/inode/generation receipts. Missing TAP repair triggers dependency-safe recreation instead of silently adopting replacements. Carrier registration preserves the separate remote-access TAP ownership boundary.

The PPP child has a private mount sandbox, read-only ledger and peer tree, and only its exact resolver output bind writable. Resolver preparation rejects symlinks and shared inodes before truncation. Launch drops to NET_ADMIN and NET_RAW with no inheritable/ambient capabilities and NoNewPrivileges. Fixed packaged daemon/plugin trust is explicit: this does not claim containment of malicious Ethernet emission by a compromised NET_RAW descendant. The carrier introduces no widening of the main agent unit; prior combined packaging CHOWN/DAC_OVERRIDE changes remain covered by their own reviews. D243 records the selected fixed broker boundary.

## Independently executed evidence

From the review checkout, Go1.26 with local offline caches and `GOMAXPROCS=2`:

```text
python3 scripts/tests/pppoe-kernel-carrier.py
Ran 28 tests in 0.017s
OK
python3 -m unittest deploy/debian/ngfw/tests/test_pppoe_carrier.py
Ran 28 tests in 0.061s
OK

go test -race -count=1 ./internal/descriptors/pppoe ./internal/subsystems -run 'Carrier|Pppoe.*(Recovery|WAN|Resolver)|PPP.*Forwarding'
ok ngfw/agent/internal/descriptors/pppoe 1.098s
ok ngfw/agent/internal/subsystems 1.182s
```

The above broad focused command ran on the pre-correction source. Final corrections were then independently checked with this exact command from `apps/agent`:

```text
go test -race -count=1 ./internal/subsystems -run 'TestCarrierForwardingProduct|TestCarrierWANTransactionOrdersRoutesAndRollsBackFailure|TestPppoe(ObservationFailureWithdrawsAndRetries|IPv6MirrorFollowsHookState|RenderedHookUpDownWithdrawAndRepair|PartialMirrorFailureIsTrackedAndWithdrawn)'
ok ngfw/agent/internal/subsystems 1.490s
```

The command completed successfully with no skips. `git diff --check` passed. These are focused fake/file controls, not native forwarding proof. No CI was run by this reviewer, respecting the requested single final campaign.

## Remaining acceptance

Native installed systemd mount visibility/propagation, capability/cgroup enforcement, actual PPP/DHCPv6 negotiation, VLAN/QinQ frames, VPP policy/NAT return path and restart packet proof remain **NOT RUN**. No host service, namespace, sysctl or packet operation was activated. Final aggregate CI and its exact tested-tree merge verification remain manager gates. No unresolved R4 source blocker or major finding remains in the reviewed carrier product.
