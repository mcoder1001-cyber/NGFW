# Setup wizard PPPoE completion

Branch `codex/setup-pppoe-20261008`, base `85a1626f`. Own setup input/builder/tests, API setup controller/tests, web setup component/tests, setup locales and feature documentation only.

Contract: additive WAN mode `pppoe` plus `wanPppoe { username, passwordRef }`, using the existing PPPoE username and password-reference schemas. The request contains no ISP password plaintext. DHCP/static modes reject retained PPPoE credentials. The wizard creates a separate deterministic logical `setup-pppoe` interface with an explicit exclusive physical parent, and attaches NAT/ACL/default-route behavior to the logical WAN. Collisions and modified/non-wizard reuse fail closed. Rerun removes only demonstrably wizard-owned logical state; unrelated references prevent deletion.

Implementation and focused schema/API/web en/fa tests pending. No CI triggered; final combined CI remains manager-owned. Native ISP and LAN acceptance remains laboratory work.

## Implementation checkpoint

Rebased onto manager's published combined base local `4683b9b6` / remote `fa5740fa92b40997b94a04af98cd1352f77aad02`; contract published `d5e791dac7ab64aa34933984284cf08dda900fa8` (local `77ca6cd3`).

Implemented distinct logical WAN, exclusive parent checks, unchanged-wizard ownership/collision guards, safe rerun cleanup and external-reference refusal; observed physical-parent verification in API preview/stage; en/fa reference-only PPPoE form with credential removal when switching modes; updated user guide. Existing atomic staging, current-password check, anti-lockout and confirmed-commit flow are retained.

Schema focused suite **12 PASS**; API setup suite **15 PASS**; schema/API/web typechecks and schema/API focused ESLint PASS. Initial web test execution under local Node24 failed before rendering all existing and new router tests because undici rejects jsdom's cross-realm AbortSignal. This is not a passed test or a waived acceptance. Preparing exact CI Node22 runtime for valid focused rerun; no test assertion or test environment has been weakened. Full CI not run.
