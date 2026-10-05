# Combined independent hardening panel

Frozen reviewedsource726d92141402459505ee27fdc6fafc7aeef94295 / remote4707bffb. All product changes carried unchanged onto pinnedmain492c0156; packaging3-way merged cleanly. History archived locally refs/archive/F-hardening-reviewed-20261005 and remotely codex/archive-F-hardening-reviewed-20261005 before singlecommit integration. Fresh final integration gate and hosted exacthead gate remain mandatory; final gate receipts are published in reviewer evidence/PR checks without rewriting frozen source.

| Aspect | Verdict | BLOCKER/MAJOR/MINOR | Evidence |
| --- | --- | --- | --- |
| R1 correctness | APPROVE |0/0/0|F-hardening-lite-review-R1.md|
| R2 security | APPROVE |0/0/0|F-hardening-lite-review-R2.md and F-hardening-lite-immutable-review-R2.md; remote1c71a895 finaldelta/fc1e6c08 original|
| R4 agent/topology | APPROVE |0/0/0|F-hardening-lite-review-R4.md; remote24c15483|
| R7 evidence/docs | APPROVE |0/0/1|F-hardening-lite-review-R7.md; remote505025a8; currentstate/T1 minor addressed by integration headers|
| R8 packaging | APPROVE |0/0/0|F-hardening-lite-review-R8.md; remotee5e57674|

Combined source verdict APPROVE. IndependentT1 exact726 fullquick PASS36m12 (remote88f63715); T3 actual10fixture cases PASS1.560s. No contract/API/UI/timer changes select additionalR3/R5/R6. Final singlecommit parent/current-main product integration must pass unchanged complete quick and hosted gates at exacthead before expected-head sequentialmerge. No merge asserted here.

Actual installed daemon compatibility, signed package install/key rotation and appliance fullboot NOTRUN: packagepool/appliance unavailable. Profiles remain opt-in/inactive. These genuine runtime prerequisites are explicit in installguide and deferredledger; source/test failures never deferred. SharedVPP/hostservices unchanged.
