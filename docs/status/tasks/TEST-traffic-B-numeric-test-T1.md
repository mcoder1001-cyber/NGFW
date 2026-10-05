# TEST-traffic-B independent final T1

Verdict: **PASS** — complete unchanged quick, exit0, wall31m49s.

Product source8e1c41d59ef177e81dfec3ed095c317c20b36dd5, tree da4c9d727dde9981c923a49864974d659051d995; fresh main base4788c90f4adaf57737b0cb8394452eadd58d53a1. Own tested HEAD1b1c5e60ab8e4a1499e996e2bda3c6dacd29b4a4 differs from8e only three independent preflight report/envelope/WIP Markdown files. Compiled product, tests, tools and dependencies are exactly8e; D226 docs-only evidence carry applies. Own branchcodex/traffic-r1-numeric-20261005, worktree /root/ngfw-wt/traffic-r1-numeric-20261005. 2026-10-05, started19:52:08UTC, completed20:23:57UTC. Session51159 awaited exit0 before writing these reports. No source edits occurred during the run, no skip/guard/timeout changes, no live host/network campaign in this T1 role.

Exact command:

```sh
GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_CI_TASK_CONCURRENCY=2 NGFW_CI_APPLY_SHARDS=2 tools/heavy.sh tools/ci.sh quick --base 4788c90f4adaf57737b0cb8394452eadd58d53a1
```

Actual selected output:

```text
Tasks:    35 successful, 35 total Cached:    28 cached, 35 total Time:    1m2.394s
0 issues.
go test -race -count=1 ./...
apply-startup tests: 59 passed, 0 failed
apply-startup tests: 90 passed, 0 failed
apply-startup harness: green (2 shards; 149 checks passed in the parallel run)
mode quick · wall time 31m49s
CI GATE PASSED
```

All27 test/ Go modules completed gofmt, vet and unit mode tests, including actual DHCP guard tests. Agent vet/lint/race/build, CLI vet/lint/race/build, generators/clean-output, gitleaks1.04MB/no leaks, forbidden-pattern/shared-trace/classify guards, 212-task board validation, 964-port slot collision validation and five shellcheck files all passed. Startup harness executed fresh149 checks across two shards; it was not a cache hit. Unit-mode live integration tests are explicitly skipped with NGFW_INTEGRATION unset; live packet acceptance belongs to separate independent T3 and composed manager run. No real VPP restart was performed by this reviewer.

Full unabridged gate output: [numeric-full-quick-output.txt](TEST-traffic-B-numeric-full-quick-output.txt). Raw step directory /root/ngfw-wt/logs/ci/traffic-r1-numeric-20261005-20261005-195208-609669. Earlier focused independent guard/contract/race and Python17 PASS evidence is in numeric-preflight-R1.md. Earlier3e andeb interrupted gates remain NOT PASS and are preserved in their own archived branches/reports.

This T1 PASS does not claim the whole traffic task is Done: final source-pinned composed REST campaign, independent T3, source-specific failure/FLAKY provenance, final R7/evidence review, exact hosted CI, fresh expected-head merge and actual-main CI remain separately required.

## Subsequent readiness finding

Manager final unchanged8e composed campaign reproduced the post-restart DHCP rollback503 a second time. R1 is now BLOCK for that mandatory traffic readiness defect; this exact oldsource T1 quick stays PASS. The failure is NOT FLAKY and cannot be waived. A fresh code repair needs a new complete unchanged quick on its corrected immutable source. No final task completion is claimed.
