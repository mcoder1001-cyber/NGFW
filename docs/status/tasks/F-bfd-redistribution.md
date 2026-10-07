Integration refresh 2026-10-05: actual main ba17cb2b86d5a1672dd5a2f36291f2785fc96945 includes external PR182 board reconciliation. Preserved its P12 blocker and actual P11 closure. Product bytes unchanged from archived38d53a14; independent complete quick on38 is historical proof only. New frozen current-main full local/hosted gates remain mandatory before merge.

# Current integration review state

Manager-owned codex/integrate-bfd-final-20261005 in /dev/shm/ngfw-integrate-bfd-final-20261005, final parentf445f57c after actual P11 merge; frozen current-main complete gates remain required. Productsource97f67a2c includes fresh assigned scale/history correction; UI2a5abf9 includes assigned localization/observation repair. Reviewed history archived remotely before final rewrite. Current R3/T2 APPROVE/PASS19API scenarios and corrected actual API restart (remotece852bcf); independent R5 verify APPROVE exact97 and six-package race PASS. Fresh changed-scope R1 closure19311b07, whole R2 security74ba755d, whole R4 ownership52fd2795 and R6/T4 closure4ca796dd all APPROVE/PASS; their actual reports adjacent. Previous R8 unchanged packaging scope retained. Fresh whole R1 APPROVE remote6631a42f and final integration R7 APPROVE remotecebc94fb are now recorded; R7 scoped-ledger minor addressed. Independent full T1 on the frozen final current-main tree and hosted complete quick remain required. No complete final quick PASS claimed yet. Native multihop real peer acceptance NOTRUN; source/durable compensation/scale/runtime observation code and mandatory gates are not deferred.

Historical chronology follows, superseded where dated.

# Current integration state

Manager-owned branch codex/bfd-final-gate-20261005, pinned parent492c0156; original091 repair preserved locally/remotely. Root fullquick417 FAILED16mandatorylint issues. Fresh independent A3 remote7213ead2 permits one bounded correction with independent closure, not merge or waiver. Checked range arithmetic and SAME bounded protobuf locals, comments/registration justification and unused callback correction now pass whole-agent vet/lint0issues and focused TestBfd0.107s. New complete quick and changed-source R1/R2/R4 plus remaining panels pending; no native multihop peer runtime proof claimed. Historical source/test chronology follows.

# F-bfd-redistribution — standalone BFD and redistribution operator views

Branch `codex/ready-bfd-20261005`, isolated worktree `/root/ngfw-wt/ready-bfd-20261005`, base `e5dba658`. Slot14, host-independent work. Draft PR175: https://github.com/mcoder1001-cyber/NGFW/pull/175 (attached by manager). Local source checkpoint20360d35, actual published remote5b8d2b7e4c058a222d816fc61e220214aaa9681f. Connector checkpoints published successfully; CLI push403.

## Delivered support

| Capability | Source implementation | Actual evidence |
|---|---|---|
| VPP UDP single-hop | Projection, authenticated keys, retrieved state and scheduler registration | Fake VPP service apply/live Down/retrieve/persisted restart/remove test passes |
| SHA1 authentication |1–20-byte key refs, version-pinned existing sealed channel, scoped stable key IDs, collision/byte-ID validation, resolver-error redaction | API sealed delivery2/2 tests; projection/auth retrieval tests; descriptor redaction test |
| VPP multihop | Globals-owner API-only implementation | Durable boot-bound exact tuple claims; lifecycle/restart/readback/removal and event filtering tested; no laboratory acceptance claimed |
| FRR BFD | Profiles/order700, existing protocol dynamic peers, profile attachments, live peer reader | Timer/precision and empty profile renderer tests pass; lab Up not executed |
| Port ownership | Per-box AF/hop exclusion, admin-down occupancy, peer-group inheritance, exact bfd pointers | Focused semantic/examples21/21 tests pass |
| Redistribution | Configured source/target/VRF/map/metric edges, counts from scoped per-protocol summary, absent counts remain unknown, advisory loop warning | Edge tests and semantic advisory test pass; no full-RIB dump |
| API | Read-only sessions and matrix routes/RPCs; events22 on dedicated topic; existing pointer mutation/commit audit infrastructure | Real fake handlers; E2E source added, DB execution deferred |
| UI | Existing BFD/editor reuse, observed state/errors, event updates, matrix protocol edit links, policy sequence reorder and where-used links, en/fa | Observed Down/error UI test1/1; focused UI lint passes |

No policy hit-counter endpoint is offered: a reliable scoped FRR counter reader was not verified. Last flap is reported only after an observed VPP state transition; unknown observations stay absent. VPP multihop global enable remains one-way until VPP restart; non-owner activation fails closed. Browser screenshot capture was unavailable (no Chromium/Chrome installed); no screenshots claimed.

## Shared hunks

- `routing.ts`: anchored auth/multihop/profiles/profile keys and mandatory OSPFv3 inherited-profile exclusion; `index.ts` profile export; semantic registry import/registration.
- `dataplane.proto`: allocated session8/9, config2, OSPF10, ISIS8, event22, owner-scoped RPCs/messages; generated Go/TS only through generator.
- Subsystems registration, Connected/AfterResync event lifecycle; projection builder/assembler/handled BFD warning; FRRDoc profiles-only copy; one established sealed-cache source call in agent startup.
- API agent client/real fake wiring, feature controller module, bus topic and relay event case; narrow existing secret-delivery selection/kind/length logic.
- UI route/nav/nav expected items/i18n namespace registrations. Generated client/CLI/YANG outputs via existing generators.
- Manager-authorized shrink-only reachability maintenance and historical unsupported-BFD expected-output updates; conflicting group-a example now selects only VPP IPv4 BFD.

## Verification and remaining acceptance

Actual focused output:
```
Schema BFD semantic/examples: Test Files2 passed; Tests21 passed.
Sealed delivery: Test Files1 passed; Tests2 passed.
UI observed state: Test Files1 passed; Tests1 passed.
go test ./internal/agent -run 'TestBfd|TestP12RoutingWarningTable|TestLispProjectionWarns'
ok ngfw/agent/internal/agent 0.165s
go test ./internal/desired -run TestBfd
ok ngfw/agent/internal/desired 0.114s
```
The complete unchanged quick gate is being rerun on the frozen final tree. Previous attempts correctly failed for unstaged generator output and historical conflicting fixture/JSX i18n issues; fixes preserve every check. No complete green quick or hosted CI acceptance is claimed until recorded below.

Laboratory-only acceptance: real af_packet peer Up/down event≤2s, FRR bfdd over linux-cp manager window, static viaFrr→OSPF peer advertisement and VPP route exclusion, daemon restart and real rollback. No host daemon/package/boot/VPP state was changed, no lab process or interface created. Independent review and a newly tested current-main integration tree remain manager responsibilities. Do not merge a stale product tree.
