# PR87 hosted timing outcome independent review

**APPROVE bounded hosted timing outcome** exact PR87 source head `47221bcf9f688a2ef5d382da8b06477959fb4c9f`, independently checked2026-10-02. This outcome report is committed separately from frozen candidate; it changes no candidate/main/board source or review history.

Independently fetched run37072992341 metadata: exact head47221bcf, completed/success. Retrieved complete job111056517154 log: explicit `CI GATE PASSED` at2026-10-02T22:48:42.0236694Z, quick15m57s, agent6m01s. This is actual full hosted gate evidence at the intended published source head, not inferred from artifact presence.

Read ZIP bytes only, no extraction/execution. SHA256 independently matches `2b3a8ba603e0b1961485f6d27a86721fc97c2de319fc92389bb621e850331416`. Exact actual agent entry is `NGFW-20261002-223245-3846/09-agent.log`, not historical expected08. Eight markers appear in exact vet/lint-dispatch/race-test/build BEGIN/END order. Independently calculated durations from UTC timestamps:

| Command phase | Seconds |
|---|---:|
| vet |50.716961|
| lint dispatch |45.925938|
| race-test |229.934969|
| build |35.222453|

Agent marker span is about361.830s (6m01.830s); tiny gaps/rounding explain sum/summary differences. Artifact contains actual `go vet ./...`, `go test -race -count=1 ./...` and original trimpath/ldflags/output build command. golangci reports0issues; no FAIL/SKIP/cached-result/missing-linter line appears in this agent log. Nonverbose Go output cannot prove absence of every internal t.Skip, so this is not a claim all optional integration/lab tests executed; full gate remains intentionally unit mode.

Race-test duration includes any compilation/linking and test execution within that command, not measured pure race runtime or cold-compile attribution. Single-run timing is not a cold/warm controlled experiment or measured speedup; cache remains disabled and all original commands execute. No heavy local rerun or real host/lab operation performed. Previously independently approved current-main composition/source failure semantics remain required context; manager still owns expected-head merge and post-main verification.
