# F-bfd-redistribution independent R3 contracts/API review

Current source:97f67a2cd3a3584fea37f7956bb302cd722b0a52. Current verdict:APPROVE (2026-10-05 focused carry/actual API retest).

Date:2026-10-05 UTC. Reviewer:/root/review_r3_api_contracts, no product authorship.
Exact local source:8f3d2f0789daa63a48d8254e6ee5df3dadea521f. Snapshot:/dev/shm/r3-021ue61v/bfd.
Reference main:492c0156e38ee73acca235527d7979b447067bb8.

## Findings

BLOCKER:0. MAJOR:0. MINOR:0.

BFD profiles/auth/multihop are additive, with optional/defaulted fields; OSPF6 continues rejecting unsupported BFD rather than inheriting OSPFv2's new profile. Proto fields remain stable; new BfdSession8/9, BfdConfig2, OspfInterface10, IsisInterface8 and EventKind22 use allocated additions. Auth carries keyRef/keyId only; decrypted material is confined to the existing sealed operational secret bundle. Existing legacy stored configuration and projection fixtures retain their field names and defaults. FRR millisecond receive/transmit values become uint32 microseconds with overflow rejection; standalone VPP intervals remain microseconds. Redistribution optional uint64 counts become nullable decimal strings, preserving unknown versus zero. The two live-state routes require authentication through Protected; agent RPC requests carry configured owner and translate the sibling unary errors consistently. Config mutations still use existing transactional commit/audit machinery; no direct state mutation endpoint was introduced. BFD event22 maps to the additive subscribed bus topic. No pagination contract is removed; these bounded desired-session/matrix lists follow sibling aggregate state endpoints.

Both source histories contain the required contract-prefixed commits and task contract notes. Node uses agent gRPC, never VPP directly. Generated artifacts were produced normally, without manual edits.

## Actual independent verification

Commands ran with private executable RAM TMPDIR/GOTMPDIR/GOCACHE and private RAM pnpm store/Turbo cache; heavy semaphore preserved.

- `tools/heavy.sh pnpm install --frozen-lockfile --store-dir /dev/shm/r3-021ue61v/store --prefer-offline`:PASS.
- `tools/heavy.sh pnpm exec turbo run build --filter=@ngfw/api^... --concurrency=2`:6 successful tasks.
- `tools/heavy.sh pnpm exec turbo run gen --filter=@ngfw/api-client --concurrency=2`:9 successful tasks.
- Independent git blob-hash comparison of normally regenerated tracked Go/TS proto, API-client types and YANG outputs:31 of31 matched the exact source.
- Focused schema tests:6 passed.

BFD DesiredState/parsed-document projection suites:112 passed. BFD sealed-secret delivery tests:2 passed.

The generator emits existing unrelated OIDC callback OpenAPI response warnings; generated tracked bytes nevertheless match. Complete quick CI remains the manager/T1 gate, not claimed here. T2 uses actual isolated PostgreSQL/Valkey with a clearly identified fake agent; data-plane execution is R4/T3 scope.

**Verdict:APPROVE.**

## Actual output excerpts

```text
Tests6 passed(6)
Duration4.91s
Tests112 passed(112)
Duration11.14s
Tests2 passed(2)
Duration16.95s
Tasks9 successful,9 total
Time1m38.647s
BFD normally regenerated tracked artifacts:31 matched:31 differences:[]
```

Real private PostgreSQL/Valkey BFD plus config regression:17passed; actual simultaneous distinct-principal writes:1passed. APIrestart supplemental test hit a reviewer setup415before reaching restart because the reviewer supplied a scalar string without JSON media type. Corrected reviewer fixture now writes an object to `/config/system`; no rerun per manager stop instruction. Actual APIrestart remains unverified, without a product finding.

## Current source97f67a2cd carry and actual API verification

The delta from8f3d2f078 touches seven production Go files plus Gotests/docs. Independently ran:
`git diff --quiet 8f3d2f078..97f67a2cd -- apps/api packages/schema packages/proto packages/api-client apps/agent/gen` →exit0. Contract definitions, APIconsumer, generatedbindings and APIroutes remain byte-identical; earlier normal generation31/31 and schema/proto/securityboundary results carry for R3. Descriptor/subsystem lifecycle changes are R1/R4/R5scope; fake-agent APItesting does not prove native runtime.

Actual private PostgreSQL18/Valkey API/config regression17cases passed on current snapshot (18passed plus one reviewer restart-response parsing error in the combined19case run). Corrected independent fixture queries the existing whole system object rather than trying to JSON.parse a raw scalar leaf. Then restarted the actual APIapplication against its retained private PostgreSQL/Valkey and fake-agent socket, observed the committed hostname and authenticated BFD live-state route after restart, and checked actual simultaneous different-operator candidate writes.

```text
Tests2 passed(2)
reloads committed PostgreSQL configuration and authenticated state after API restart:passed
arbitrates actual simultaneous distinct-principal candidate writes:passed
Duration30.70s
ok nothing named ngfw_w10/ngfw_w10 remains
```

Current independent T2 coverage:19unique cases passed across the existing17-case regression and corrected2-case lifecycle/concurrency run. Reviewer fixtures are evidence-only; no tracked product/test changes.

**Current R3 verdict:APPROVE.**
