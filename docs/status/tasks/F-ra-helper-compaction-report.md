# Two-helper compaction validation

Frozen product source: local 8b68426cad7f967451e48ed132f378d2dde5ae83 / remote a25d5a550d8da8bc737cfcf162d1cabd6f568138 / tree c339dd8089d378bbee8f75ca5eb7268aacc48719. Base 7af31dfb78c1681ca40c6e9929caf910cbb0cb0e. Contract published first at 4e366fe085424a2089009613e8865cca6e8bd370.

Only the two-helper prepare loop gains `-ldflags="-s -w"`, before unchanged SHA generation. Agent/startupgen/vppcheck/upgrade-probe build flags, CGO0/trimpath, Debian strip exclusions, units and all runtime guards/budgets remain unchanged. Staging regression records real invocation argument boundaries, checks all six builds and asserts only the two RA helpers receive the flags, with existing paired staged-byte checksums preserved.

## Actual public artifact evidence

Fresh owned paths: /root/ngfw-wt/ra-helper-compaction-20261006/artifacts/ra-helper-compaction/. No old artifacts read, modified or removed. Command from apps/agent for each helper: `CGO_ENABLED=0 GOMAXPROCS=2 ../../tools/heavy.sh go build -p=2 -trimpath -ldflags='-s -w' -o <fresh-owned-path> ./cmd/<helper>`. Toolchain `go1.26.0 linux/amd64`, shared caches retained. Helpers were not executed or installed.

| Helper | New bytes | Full SHA256 |
|---|---:|---|
| ngfw-ra-daemon | 15519906 | 5376f597406b2f8fe8559b1e579c318d301d4c8a533f5de28b28f8a9005b87fe |
| ngfw-ra-namespace-broker | 15847586 | f3bf43a5fa066e3c77da24be34974e07b3f0588211a14479fc8b074ef18bc159 |

Root-reported previous fresh artifacts: daemon22392731 and broker22851251 bytes. New artifacts are smaller by6872825 and7003665 bytes respectively (about30.7%). Previous artifacts were not rebuilt here for a controlled paired comparison; these are measurements against Root's supplied baseline, not a claim of identical provenance or old SHA identity. No timing or Ready claim follows.

Both new artifacts passed ELF magic checks, readelf section inspection (.go.buildinfo retained; .debug_info/.symtab absent), `go version -m` parsing Go version/module dependencies/CGO_ENABLED=0/-trimpath=true, and independently recomputed full SHA equality with fresh bare manifests. Go1.26's actual trimpath buildinfo output omits the linker flag field: an initial diagnostic assertion expecting that field failed and was corrected to the documented argv plus actual ELF section evidence. Runtime ReadBuildInfo was not executed; preserved embedded information and prior linker source inspection are the available evidence.

## Actual checks and limits

Staging3 tests passed (session3182); packaging15 tests passed (session62332). Initial staging assertion counted five Go commands; the unchanged upgrade-probe makes six, corrected before source freeze. Bash syntax and git diff whitespace checks passed. Repeated final staging session38128 EXIT0, 3 tests2.510s; bash syntax and whitespace checks EXIT0.

Unchanged race negatives passed: `GOMAXPROCS=2 ../../tools/heavy.sh go test -p=2 -race -count=1 ./internal/ra_vpn -run 'TestNumericPublisherHeldInstallationRejectsChanges|TestNumericPublisherStreamingHashAndCancellation|TestNumericPublisherDiagnosticTransportPinned|TestSourceAgentReferenceOwnershipAndProcessBinding|TestInstallationUnitDigestMatchesSource'`; session92986 EXIT0, RA1.450s. Covers replacement/content/mode/hardlink/symlink mutation, whole streamed SHA and cancellation, protected Source process binding, unit digest and original immutable budgets. Integration actual-engine preflight was intentionally not run: requires operational engine assets outside this assignment.

Independent P11 review pending at publication. No full quick gate or operational guest campaign run by this scoped author. Root owns final coherent integration, fresh installed-byte/Source identity and default-budget Ready acceptance. No caches, partial hashes or guard relaxation introduced.
