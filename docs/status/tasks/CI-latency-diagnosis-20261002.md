# Hosted quick gate latency — measured diagnosis

Read-only source diagnosis,2026-10-02 UTC, isolated sparse worktree
NGFW-ci-latency-diagnosis / task/CI-latency-diagnosis-20261002, actualmain
497b724bb0197c5e5eb50d388e0e4bdaba2c5c8e. D172 is the existing CI decision;
no new decision/task/board entry or optimization is applied here. No local build.

## Actual evidence

- [main run37061921326/job111020348098](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37061921326/job/111020348098): runner log begins20:38:56.452, gate20:39:38.843→20:55:12.526, explicitCI GATE PASSED. MainSHA497b724b. Gate15m34s; runner-log-start→PASS16m16s.
- [PR82 run37059936362/job111013825782](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37059936362/job/111013825782): log begins20:20:33.4345823, gate20:21:15.341→20:37:25.311, explicitCI GATE PASSED. Gate16m10s; runner-log-start→PASS16m52s.
- Retrieved both complete job logs through GitHub. Inspected main uploaded artifact11251770419, ci-logs-37061921326-1,9019537compressed bytes, published digestsha256:facdf7fd7ef3270fe17c31f68a536ed74a908ac60ff49c282a421a498f7df80d. Inspected03-pnpm-gen.log,07-turbo.log,08-agent.log and11-build-startupgen.log directly inside ZIP; no executables run/extraction/build.

| Measured phase (rounded job summary) | main497 | PR82 |
|---|---:|---:|
| runner-log-start to gate, checkout/setup/tools combined |42s|42s|
| pnpm frozen install |5s|5s|
| generation + committed output check |45s|46s|
| forbidden/trace/slot guards combined |6s|3s|
| Turbo lint/typecheck/tests/build |6m30s|6m52s|
| agent make lint/test/build |5m55s|6m06s|
| CLI make lint/test/build |13s|14s|
|19 Go test modules, unit mode |17s|18s|
| deploy shellcheck/fake-host harness |1m42s|1m45s|
| **complete quick** |**15m34s**|**16m10s**|

Rounding means rows are approximate; table reflects reported phase durations,
not hypothetical test-runner estimates. Turbo+agent consume about80% of both
gates. Network/package install is not the dominant measured phase. Queue time
before runner log begins, artifact upload/postcleanup afterPASS are not measured.

## Cache and dependency observations

Source .github/workflows/ci.yml disables setup-go cache explicitly; there is no
Actions cache restore/save for Go/pnpm/Turbo. Both installs download595 packages,
reuse0; --prefer-offline alone does not persist an ephemeral runner store. Go
module downloads occur in03-pnpm-gen.log. Node/Go tools also install per runner.
Turbo reports35/35 successful,6 cached, remote caching disabled. These6 hits do
not establish cross-run caching: earlier same-job generation built dependencies;
03-pnpm-gen.log itself reports13 tasks,0cached. Hosted limits are taskconcurrency2,
GOMAXPROCS2,GOFLAGS-p=2 and apply harness2 shards. Both harnesses execute149checks,
no persisted green-result marker hits. No claim of actual physical CPU count.

The agent artifact proves go vet, golangci-lint0issues, race test-count1 and build
all execute. Individual package durations include internal/agent30.874s and
subsystems16.131s in main; they are neither whole-phase times nor a serial sum.
The combined08-agent.log contains no per-command timestamps. Turbo errors-only
log has no per-task durations. Thus6min agent cannot yet be reliably divided
among cold compilation/vet/lint/race tests/final build, nor7min Turbo among build,
lint/typecheck and tests. Cold compilation is a plausible hypothesis, not measured
attribution. Tiny laterCLI phase is not a controlled warm-cache experiment.

## Safe next experiments, after current serial feature gates

1. Add timestamped per-command instrumentation and Turbo run-summary artifact on
   an isolated CI branch while preserving all assertions/tasks/exit propagation.
   Profile agent vet/lint/race/build separately before changing concurrency.
2. Evaluate persistent Go module/build cache with OS/arch/exactGo1.26.0 and all
   go.sum inputs; retain race/count1 and every lint/build/test. Measure cold/warm
   paired exact-tree hosted runs, cache restore/upload overhead and storage. No
   cached test PASS substitution or claim of estimated minutes saved.
3. pnpm store caching has only5s measured install opportunity, so lower priority.
   Avoid restoring Turbo test-result/output caches without independent complete
   input/env/output key review; current generation is intentionally uncached.
4. Do NOT simply overlap existingTurbo andagent. turbo.json depends on gen with
   cache:false; packages/proto/gen.sh removes/regenerates apps/agent/gen and runs
   go mod tidy. Concurrent Go readers could race generated source/go.mod. Any
   parallel proposal must establish a verified generation barrier and independent
   task ownership first. Likewise shared shell gate STEP_N/CUR_LOG/SUMMARY state
   needs isolation before backgrounding existing functions. Raise no timeout and
   skip no guard/test; changing capped parallelism needs measured resource proof.

No product/CI behavior changed; PR83 gates remain untouched. This is a useful
measurement baseline, not an optimized gate or24-hour worker guarantee. Local
quick previously stopped before Go on sandbox TypeScript failures; those runs
cannot profile hosted Go or prove changed-language validation. This diagnosis
uses completed hosted evidence instead. Report must be independently reviewed;
implementation, if authorized, needs its own unchanged full hosted validation.
