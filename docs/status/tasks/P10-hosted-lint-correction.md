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
