# D172 Go cache options — read-only source assessment

Own isolated sparse NGFW-ci-go-cache-options / task/CI-go-cache-options-20261002,
base actualmain337cbef881ada5bc5ba20aafe396684c4cefd009.2026-10-02 UTC.
No workflow/cache/product/board/decision/main/PR87 mutation, publication or build.
Baseline corrected4c8b/4e9 report measured complete hosted quick15–16min; agent
combined phase5m55–6m06, not measured cold compiler time. InstrumentedPR87head472,
run37072992341 is pending; no per-command outcome or speedup invented here.

## Pinned primary implementation inspected

Fetched official actions/setup-go at exact existing pin
40f1582b2485089dde7abd97c1529aa768e1baff through GitHub connector:
[README](https://github.com/actions/setup-go/blob/40f1582b2485089dde7abd97c1529aa768e1baff/README.md),
[action.yml](https://github.com/actions/setup-go/blob/40f1582b2485089dde7abd97c1529aa768e1baff/action.yml),
[main](https://github.com/actions/setup-go/blob/40f1582b2485089dde7abd97c1529aa768e1baff/src/main.ts),
[restore](https://github.com/actions/setup-go/blob/40f1582b2485089dde7abd97c1529aa768e1baff/src/cache-restore.ts),
[save](https://github.com/actions/setup-go/blob/40f1582b2485089dde7abd97c1529aa768e1baff/src/cache-save.ts),
[paths](https://github.com/actions/setup-go/blob/40f1582b2485089dde7abd97c1529aa768e1baff/src/package-managers.ts).
Runtime entrypoints are bundled dist setup/post-save, action Node20. This report
traces checked-in source, not a proof that every bundle byte matches TypeScript.

| Property | Exact pinned source behavior | Implication for proposed experiment |
|---|---|---|
| Inputs | cache:true supported/default; repo explicitlyfalse today | Same action pin, no action upgrade needed |
| Paths | go env GOMODCACHE and GOCACHE | Dependencies/compiler artifacts, not Turbo outputs/GOBIN/agentbin |
| Dependency input | explicit glob hashed by actions/glob; fallback requires rootgo.sum | Repo lacks rootgo.sum, so explicit all-repo go.sum glob required |
| Key | setup-go-OS-NodeArch-LinuxImageOS-go-ActualVersion-FileHash | NativeUbuntu24.04/Go1.26.0 segregated; process.arch is Nodehost, not crosscompileGOARCH |
| Go version | parsed actual go version after install, not merely input range | Retain exact1.26.0,GOTOOLCHAINlocal; no latest/range change |
| Restore | one primary key passed to cache toolkit, no caller restore-key list | No explicit broad fallback configured by setup-go; toolkit/server matching still applies |
| Save | post-if success(); exact-key hit skips upload, otherwise save requested | Existing same-key archive is not refreshed with later source-only outputs |
| Failure | cache errors warn; full validation still executes | Miss/failure must be ordinary cold fallback, never acceptance bypass |

At337 the committed tree has17go.sum files and22go.mod modules. Proposed glob
**/go.sum covers agent, CLI, SDKTerraform and test module sums; modules with no
sum still have their source compiled/tested by applicable existing commands.
An unrelated sum change invalidates the combined archive, a conservative
performance cost. hashFiles only demands a nonempty aggregate; it does not prove
every module has a sum. No dependency files are synthesized or omitted here.

The key does NOT explicitly include sourceSHA, workflow/tool pins, go.mod,
CGO/GOFLAGS or race flags. Therefore it is a restore bucket, not proof of identical
source/environment or a valid binary/PASS. Go compiler reuse must retain its own
validation; all original vet/lint/build commands and race/count1 tests still run.
[Official Go test docs](https://pkg.go.dev/cmd/go#hdr-Test_packages) document that
-count=1 disables test result reuse. RestoringGOCACHE is not permission to remove
that flag or condition any command on cache-hit. Source-only changes can produce
new compiler objects but exact-key save is skipped, so one archive is not an
incrementally refreshed build cache. Scope/epoch/key strategy needs measurement
rather than a promise of repeated zero-cost compilation.

## Trust boundaries and storage

[Current official cache policy](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching), checked2026-10-02:
PR caches are merge-ref scoped and unavailable to base/otherPRs; PRs can read base
caches. Same-branch workflows can share entries. Thus contents:read is not cache
write isolation; trigger/ref and runtime cache tokens matter. Default-branch
low-trust triggers are read-only unless explicitly overridden; preserve that
boundary, never add write override or pull_request_target execution here.
Caches are unsigned/unverified and readable by eligiblePRs. Cache no secrets,
credentials or private modules not intended for those readers. Repository actual
cache-mode/settings/token behavior must be observed, not assumed from a key.
Default retention is7days idle and10GB aggregate, with last-access eviction;
configured limit can differ, and extra paid storage needs separate owner choice.

Inference for current pushmain/pull_request CI: ordinary forkPR writes cannot
replace main-scope entry solely by selecting its key, but a compromised trusted
writer can poison cached compiler/module bytes. Keys are not an authentication
boundary. Cache content cannot be treated as release provenance. Any future
privileged release workflow must reassess its own scope, not inherit this option.
No such new workflow/trust model implemented or approved here.

## Options for root, not implementation approval

| Option | Benefit/limit | Required next evidence |
|---|---|---|
| Keepfalse until timings complete | Preserve measured baseline and serial merge queue | PR87 vet/lint/race/build timestamps |
| Isolated exact-pin cachetrue + allsum experiment | Small config change, whole gate remains; combined archive can be large/stale | paired exact-tree cold/warm hosted jobs, actual keys/paths/ref/cachemode |
| Separate module/build caches with reviewed epoch/keys | More control over mutable performance bucket and storage | new pinned action/source/trust review; not a quick approved substitute |

Prefer finish instrumentation, then a separately reviewed experiment. Record
restore/decompress and post-upload wall time, archive bytes, hits/misses/warnings,
Go command timings, all full-gate results and ref isolation. Measure all tasks,
not a subset or local sandbox quick that never reached Go. Eviction/cancellation/
network fallback and cache pressure are unmeasured. No cache quota read/deletion,
seeding/upload/download benchmark, workflow dispatch or source changes performed.
No estimated speedup, cold-time attribution, new cost or24-hour agent guarantee.
ParallelTurbo/agent remains unsafe while generation rewrites Go source/go.mod;
this option does not change task DAG/concurrency/flags/testcount or acceptance.
