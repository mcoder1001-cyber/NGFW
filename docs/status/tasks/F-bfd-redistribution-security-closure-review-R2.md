# F-bfd-redistribution — independent R2 bounded-conversion closure

Exact local source: `8f3d2f0789daa63a48d8254e6ee5df3dadea521f`; published source: `5a5c967cc088b235dc6379912bf7a576f010c572`; tree: `25359bb6d003ed24ae5347e5842933be721ea636`. Reviewer owns only review reports on `codex/review-r2-security-closures-20261005`. Product source was extracted with `git archive` into private executable RAM `/dev/shm/r2-bfd`; no source edits.

This review independently assesses the A3 bounded arithmetic/conversion correction, with explicit provenance of the previous full multihop/compensation R2 APPROVE on source `0912488609545fec12a329f6f801d00a7cd77231`, review checkpoint `1c71a8959640d5ace6d42acf5bbca8e6ffe34f6d`. Compared current BFD descriptor subtree against that source: unchanged. Other concurrently integrated main features are not covered by this targeted approval.

`BfdKeyID` rejects reversed endpoints before subtraction/modulo; the inclusive span is evaluated as uint64 and supports 2^32 without zero divisor. For valid endpoints, the remainder is less than the span, so addition stays within `last`; an explicit MaxUint32 check precedes conversion. FNV input and mapping remain unchanged for existing valid ranges. Caller handles the new error before emitting authentication objects. Detect multiplier and wire key ID bounds use the same local uint32 values later converted to uint8, rejecting 256 and MaxUint32 before object emission. Nil auth remains safe through generated getters; multiplier zero retains default three. Collision rejection, separate sealed-secret resolution, and ownership/recovery controls are preserved. Remaining changed renderer/subsystem lines are documentation or unused-variable cleanup.

Actual independent command in exact-source archive:

```
env TMPDIR=/dev/shm/r2-tmp GOTMPDIR=/dev/shm/r2-tmp GOCACHE=/dev/shm/r2-cache GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test ./internal/desired -run TestBfd -count=1
ok  ngfw/agent/internal/desired  0.095s
```

Tests include pinned normal mapping, full uint32 interval, upper-end pair, singleton/zero ranges, reversed ranges, nil-auth/default behavior, 255 acceptance and 256/MaxUint32 rejection with no emitted objects. Cold compilation used private RAM cache/temp and bounded concurrency; shared cache and services were untouched.

```
gitleaks detect --no-git -s /dev/shm/r2-bfd/apps/agent/internal/desired --config /dev/shm/r2-bfd/.github/gitleaks.toml --redact --no-banner
scanned ~801742 bytes (801.74 KB) in 862ms
no leaks found
```

No full quick gate, live multihop peer, shared VPP changes, HA fault or new RA engine was executed. Their acceptance and mandatory review/gate requirements remain separate. No BLOCKER or MAJOR found in the inspected security delta.

Verdict: **APPROVE** (R2 targeted closure).
