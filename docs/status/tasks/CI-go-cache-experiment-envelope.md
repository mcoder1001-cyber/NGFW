# D172 Go cache experiment preparation

Own branch task/CI-go-cache-experiment-20261003, isolated
NGFW-ci-go-cache-experiment, exact main base78218e6da8b623debf2542bebbe15af2fcc5a809.
Own new inert patch, bounded source policy controls, this envelope and WIP.
No existing workflow, tools/ci.sh, Go source, module sum, agent Makefile, board,
decision log, PR90 or candidate integration changes. D172 remains root-owned.
Reviewed options source: NGFW-ci-go-cache-options-final/
docs/status/tasks/CI-go-cache-options-20261002.md, published32fe5da1; measured
latency diagnosis remains historical15–16minute baseline, not compiler coldtime.

## Concrete inactive delta

CI-go-cache-experiment.patch proposes exactly two setup-go inputs: cache:false
becomes true, cache-dependency-path becomes '**/go.sum'. The actual ci.yml stays
cache:false. Same existing setup-go40f1582b2485089dde7abd97c1529aa768e1baff,
Go1.26.0, GOTOOLCHAINlocal, permissions, trigger/ref, command order and complete
quick remain byte-for-byte identical otherwise. All17sum files/22modules are
covered; no rootgo.sum exists. No Turbo/harness/PASS/test-result artifact cache,
parallelism, cache-hit conditional gate, count1 removal or trust/permission override.

[Official pinned paths source](https://github.com/actions/setup-go/blob/40f1582b2485089dde7abd97c1529aa768e1baff/src/package-managers.ts)
(blob3547d33661ece962c747d107108c1bf61f3f0cd4) confirms only go env GOMODCACHE
and GOCACHE. [Pinned restore](https://github.com/actions/setup-go/blob/40f1582b2485089dde7abd97c1529aa768e1baff/src/cache-restore.ts)
uses OS/Nodearch/LinuxImageOS/actualGo version/sum hash. Key/ref scope is not
source provenance; archives immutable, same-key exact hit skips later upload.
No archive of repository checkout/workspace, credentials, GOBIN or result markers
is proposed. Resolved actual cache directories remain UNOBSERVED, not asserted
safe merely from conventional directory names.

## Required constraints before an experiment

Repository/server cache mode and actual fork token/write behavior are UNKNOWN.
Pinned action source does not set or prove server mode. Current
[official cache policy](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching)
is the authority for defaults: PR entries are merge-ref scoped, base cache can
be read by eligible PRs, low-trust default-branch writes follow server policy.
contents:read alone does not isolate cache writes. No cache-mode/trusted-writer/
permission override is permitted in this proposal; do not silently infer the
repository configuration. Independent R7 must establish suitable observed server
policy or leave hosted experiment blocked. Untrusted/compromised writer bytes
are not authenticated by cache key, compiler output or test success.

Before activation: verify current server/default policy, eligible readers and
trusted writers; confirm no private modules/secrets in cache; log actual go env
GOMODCACHE/GOCACHE and reject symlink/broad workspace/root/credential directories.
Go tooling version/OS/native architecture/ref scope and cache keys must be logged.
No runtime path change, credential inclusion or privileged release acceptance.
Missing evidence must stop only this experiment, not other product development.
Do not add overrides to manufacture a safe-default claim.

After serial PR90 merge, root may assign a reviewed, exact current-main
integration for paired SAME-TREE hosted runs. First must show an actual miss,
second an actual hit with identical workflow/source/tool/sum/ref inputs. Do not
call existing-hit run cold, delete unrelated caches or change tree between pair.
Record restore/decompression/post-upload time, archive bytes, actual key/ref,
miss/hit/warnings, instrumented vet/lint/race/count1/build timings, full quick
CI GATE PASSED and all other phase times. Miss/eviction/error remains normal
cold fallback; never skip validation. Real paths/key/hit/storage/overhead/coldwarm
are all NOT RUN/unmeasured. No speedup or cold-compiler attribution claimed.
This preparation is not approval for rollout or merge of active cache settings.
