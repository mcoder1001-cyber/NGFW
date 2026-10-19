# RA reviewed delta recovery on actual main

Branch codex/ra-union-source-20261007, base f6ae6e555, initial remotely published envelope 0fc5520ae, product f01180f6e58aee1c30c190ac0eb744179106a44b, draft PR202. This is source recovery, not operational closure.

## Exact provenance and scope

Reviewed predecessor b0367efb48339808359c4c82e2675060d8c1ddd4 was merged by PR193. Reviewed subsequent union 711e18e12ac2776ad4fc15066f811b60599e63f2 remains published on codex/integrate-ra-final-20261005. Applied only 45 product/test/module/helper paths using a clean three-way delta; all runtime/RPC/renderer/RA UI/localization source matches that union. Unrelated current-main code, Interfaces and wizard hotfixes, contracts/generated source, installer scripts and board are unchanged. No copied old full tree, history rewrite, dependency installation completion or main edit.

Recovered: authenticated root PID1 native property readback with pinned godbus5.2.2 and protected held Unix identity; exact frame/type/16KiB/rights/cancel bounds; ordered role pipeline and Id barriers; terminal and closed failure-checkpoint diagnostics; shared1024 session limit and100-row paging; lazy template initialization; observer template digest; lossless counters and localized initial loading; build-time -s -w RA helpers before SHA attestations; exact reviewed vm15 coordinator fixture literals. No caller-deadline/proof relaxation or security-model change. Native implementation decision exists at reviewed union docs/decisions/DEC-ra-pid1-property-readback-20261006.md and is recommended for manager provenance integration.

## Actual checks

Offline targeted command in apps/agent: TMPDIR=/rau GOMAXPROCS=4 GOTOOLCHAIN=local GOPROXY=off go test -race -count=1 ./internal/ra_vpn ./internal/renderers/strongswan ./internal/agent: PASS, runtime3.279s, renderer7.102s, agent76.803s. Genuine gated integration skips are not graded operational success.

Scoped go vet same three packages and go build ./cmd/ngfw-ra-daemon ./cmd/ngfw-ra-namespace-broker: PASS. Python preparation unit suite3/3 PASS2.581s; coordinator ast.parse PASS without execution. git diff --check PASS.

Initial RA UI unchanged file10/11 PASS; first initial lazy heading lookup timed out. Full unchanged11-case rerun PASS11/11,28.23s tests/39.09s runner; original failed run retained. No test/source/deadline change. Initial web tsc dependency-resolution failure is not product acceptance: own packages lacked existing dependency dirs, resulting in missing openapi-fetch and downstream never types. Linked existing immutable dependency directories in own worktree; no installation required, whole-web typecheck rerun PASS exit0 using existing dependency directories. Initial scoped eslint invocation used absent web-local binary; corrected root binary scoped lint PASS exit0 (existing MODULE_TYPELESS informational warning). pnpm exec attempted auto-install lockfile/policy startup, canceled before completion; not graded a successful installation or test.

Logs: /rau/targeted-race.log, /rau/targeted-ui-direct.log, /rau/targeted-ui-rerun.log, /rau/targeted-typecheck.log, /rau/targeted-typecheck-fixeddeps.log, /rau/targeted-eslint-root.log. Whole aggregate CI not run under current owner waiver. Independent current-main review pending; draft PR202 reviewable at frozen product f01180f6e. All targeted author checks complete.

## Original negatives and remaining acceptance

Original Boot36 exited/reaped but failed readiness within caller deadline; post-ACK Source identity revalidation checkpoint15 native query canceled after parent expiry/HUP. Its final negative receipt6c27a9f64b6263ecf4c5399f96aac39f38ea745d retains source605bb/4fb5/tree3d409, producer20 all3 binaries/all9 original units and raw-log SHA cb9d790539b52b6b86c0cb0deb2d453cc3435c7dea91f1f93cf59719a2441328. Critical-path audit established no latency dominance. Boot37 stale bare SHA fixture refusal receipt2ad6956a04cb0ac8849becc36ad292848b7dfcc2 is preserved; Boot38 result unverified. Source recovery does not infer supplier READY, real EAP/TLS, DNS/ESP/ACL packets, disconnect/restart identity, physical rollback, daemon isolation, production HTTP licensing/PKI/secrets/roles/audits, second-pool semantic rejection, appliance or four browser language/theme modes. No original predicate, deadline, fixture-profile or host privilege boundary changed. No host services/shared VPP modified.
