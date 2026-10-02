# P10 activation/provider independent R1/R2/R4 review

Reviewed immutable `412f4d30418d7773e76368a489ecf7837cf9dd45`, isolated task/P10-punt-activation-review. Scope: registration, provider, service projection hooks, explicit product activation and privilege compatibility. Helper reviews and complete descriptor lifecycle remain separate reports; final static/dynamic set layout is not covered by this snapshot.

**Verdict: APPROVE this activation/provider scope. No confirmed new BLOCKER/MAJOR.**

- registerBasePolicy is disabled without exact VRX_BASE_POLICY=1 before any file/runner/VPP access. Explicit activation requires owner vrx and GlobalsOwner. Unit enables it only for the appliance and orders/requires nftables; this adds no capability or writable path. Existing CAP_NET_ADMIN, AF_NETLINK and root-namespace nft runner are compatible with the set update. Shared slots fail explicit unauthorized activation and do no implicit work.
- Trusted bounded root0600 config is loaded before registration. Nft allowlist contains only fixed nft executable; MaxOutput limits capture to32KiB. No shell arguments, host service mutation during review, new public API, privilege expansion or broad filesystem write was introduced.
- Actual provider uses existing owned ItfPairDescriptor.Retrieve, not desired lcpmap or unfiltered global pair dump. That existing Retrieve applies interface reportability/claim ownership, then copies VPP pair Netns. `lcp.go:51–53` and `docs/agent/descriptors/lcp.md:25–26` explicitly document that pair readback contains effective namespace even when creation input inherited default. Empty actual Netns is therefore root according to the existing contract; provider must not reapply today's default to historical actual pairs. Actual VPP semantics remain a deferred live acceptance check, not newly proved by source review.
- Projection reads current validated default namespace rather than treating ambiguous/malformed get as root. It rejects read failure and uses known namespace to exclude non-root desired pairs; descriptor Create separately matches actual owned root pair after dependency execution. No NameDefaultNetns registration or public desired projection exists in current subsystems/desired/service, so same-transaction default mutation is not reachable through the current product document. Defensive support for future supplied default-namespace KVs/optional dependencies should receive a narrow subsequent recheck rather than an invented current runtime blocker.
- applyLocked, DryRun and CheckDrift share projectWithBasePolicy, gated to interfaces scope. Startup/reconnect Resync goes through applyLocked under existing transaction deadline and locking. No independent best-effort watcher can label failed admission APPLIED. Global ownership/missing file failure is explicit, not silent feature disable.

Personally executed in exact isolated head using workspace Go binary and persistent caches:

```
go test -race -count=5 ./internal/subsystems -run 'TestBasePolicy'
ok ngfw/agent/internal/subsystems 1.055s
```

Tests exercise disabled projection no-work, root/non-root default namespace projection, failed namespace readback, and nonproduct/non-global explicit activation rejection. These do not prove successful full activation with actual systemd/nft/VPP, bootstrap install, namespace transitions or traffic; those are NOT RUN. Full descriptor ordered lifecycle regressions and subsequent distinct-set layout remain separate reviews.

CAP_CHOWN and global atomic /etc parent decisions remain unresolved and unchanged. This approval does not mark all P10 complete.
