# Independent P10 pure helper review — R1/R2/R4

Exact head `59471696662f71fd507f3e6d6ff0fc2219e76237`; isolated `NGFW-P10-punt-helpers-review`. Reviewer did not author product code.

## Finding

**MAJOR — obsolete whole-set writer violates the new narrow mutation/uncertainty contract.** `apps/agent/internal/renderers/basepolicy/punt.go:165–193` exports Replace, backed by Transaction at lines58–74. It flushes the complete membership set and returns a plain error if nft has committed but its reply fails. The old fake replacement test injects failure before mutation, so it does not prove this post-commit case. There is no production caller/registration currently, hence no active runtime regression, but this is an unsafe alternate public helper left beside the approved one-element path. Remove the obsolete Replace/Transaction entry points and their obsolete fixture tests; retain genuine Element failure tests and shared parser/member validation tests. Alternatively implement the same confirmed compensation and ownership contract, but there is no consumer need for the broader API.

## Positive checks

- Element validates management exclusion, safe explicit quoted name, actual/final union bound64 and sends only one exact-element mutation; it never flushes a table/ruleset or membership set. Its helper contract explicitly leaves permanent exclusion and owned pair proof to the descriptor; therefore this review does not approve descriptor integration based only on these helpers.
- Failed mutation uses fresh 5-second WithoutCancel recovery context; intended readback resolves success, unchanged original readback resolves failure. Otherwise inverse mutation is followed by independent confirmation. Failed/unconfirmed restoration includes typed ErrUncertainOutcome through errors.Join, including non-timeout EIO. Existing marker compatibility was separately reviewed; no generic changes added here.
- Parse rejects missing/wrong set, unknown set fields/flags, multiple objects, trailing JSON, wrong type, malformed values, invalid duplicates/management names and oversized32KiB JSON. Capture is additionally bounded by existing Runner; no user input reaches argv or shell.
- Config uses O_NOFOLLOW/CLOEXEC/NONBLOCK plus fstat regular-file, root uid,0600 and size bounds4096; LimitReader plus ParseConfig limit handles subsequent growth. Parser does not source shell text and rejects unknown/duplicate/missing assignments. Fixed product path lives under existing root-owned `/etc/vrx`; final-component protection does not by itself authorize arbitrary untrusted parent paths, which future callers must not supply.
- Projection requires known effective default namespace, excludes non-root pairs, rejects duplicate root host identity and static/dynamic overlap, and validates combined bound and management separation. It receives already owned typed pairs; proving owner selection remains descriptor responsibility.
- No capability/unit expansion, daemon startup, host firewall mutation, VPP operation or new public API occurred. This code is not yet registered as an active product feature.

## Independent execution

Initial unqualified `go` command failed because PATH lacked go. Retried exact workspace binary with persistent module/build caches:

```
GOTOOLCHAIN=local GOMODCACHE=/workspace/scratch/96b8b6fbc8a7/toolchain/cache/go-mod GOCACHE=/workspace/scratch/96b8b6fbc8a7/toolchain/cache/go-build GOMAXPROCS=2 /workspace/scratch/96b8b6fbc8a7/toolchain/bin/go test -race -count=10 ./internal/renderers/basepolicy
ok  ngfw/agent/internal/renderers/basepolicy 1.069s
```

Executed tests include EIO after committed delete, unchanged rejection, confirmed compensation, unknown/failed/unconfirmed compensation, canceled original context with independent recovery, foreign-member preservation/no whole-set Element writes, add no-op/management rejection, config parser/symlink/mode checks and namespace projection tests. No real nft or VPP acceptance run. Whole lifecycle/product completion and lab acceptance: NOT RUN.

**Verdict: APPROVE WITH CHANGES (1 MAJOR): remove obsolete unsafe whole-set writer before merge; then narrow recheck. Element/config/projection helper scope otherwise approved.**
