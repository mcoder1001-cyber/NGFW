# P10 hosted lint correction

PR67 full hosted gate run37033151184/job110924731099 failed in agent lint.
This is a real source integration failure, not an environment deferral.

On isolated task/P10-punt-sync-20261002 atop360bbde9, checkpoint4d54f7cd
added accurate exported API comments to descriptor.go and punt.go, without
suppressions. The subsequent full pinned golangci-lint2.13.2 run ./... returned
exit1 with five additional findings: readonly configuration file Close unchecked,
negative fixture chmod0644 flagged by gosec, and missing ProductConfig,
ParseConfig and EnvBasePolicy comments.

Checkpoint228d116ab03f70f5f972c17f0f3f34feefa07c40 resolves those findings.
Readonly Close is explicitly discarded in deferred cleanup after bounded reads;
no file writes depend on Close. The negative permission fixture still adds
public-read bits to its isolated nonsecret file, derived from its existing mode;
the unsafe-mode rejection assertion remains intact. All missing exported
comments were added. No nolint, analyzer configuration or gate changes.

Actual validation on228d116a: full apps/agent golangci-lint2.13.2 run ./...
returned **exit0, 0 issues**. Basepolicy package: **17 tests PASS, 0.010s**.
Whitespace check passed. Logs from these runs are
/tmp/p10-golangci-lint.log and /tmp/p10-golangci-lint-r2.log in the execution
workspace. Earlier full local Go environment failures remain documented;
this lint success does not establish a full unit/quick pass.

The packaging-fixture CI branch was not modified. Publish this correction and
repeat unchanged full hosted quick on the final corrected integration head
before merging PR67. Port these source fixes into the separate fixture-gate
integration after feature source is finalized; do not publish a stale product
integration tree. Target appliance traffic/boot cases remain NOT RUN.

## Recovery lesson

The earlier local mandatory quick run stopped at TypeScript/API tests because
Unix sockets and foreign-owner changes were restricted. Its Go stages were
NOT REACHED; that is missing verification, not evidence that Go lint/tests or
builds pass. Before integrating a change in a language whose full gate stage did
not run, separately execute that language's complete pinned lint and meaningful
tests/builds. Keep any actual environment failures explicit and still require
the unchanged hosted full quick on the exact final integration commit.

For this correction the exact full lint command was:
`source ../toolchain/env.sh; cd apps/agent; golangci-lint run ./...`.
Version2.13.2 exited0 with0issues;17basepolicy tests passed. The read-only
configuration file Close result is explicitly discarded in deferred cleanup
(the same prior behavior), not propagated or claimed to establish additional
I/O durability. The negative fixture still has actual0644 public-read permission;
only its expression now derives those bits from the existing temporary mode.
No gate configuration, nolint annotation or analyzer suppression changed.
