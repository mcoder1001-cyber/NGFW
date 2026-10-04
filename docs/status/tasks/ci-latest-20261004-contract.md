# Contract review rationale

No schema/protobuf shape or generated bindings change. Correct group-a OSPF example to use the process VRF. TS parsed-document guard now mirrors the existing Go drift guard exclusion of read-only API-owned /system/setup and rejects accidental propagation with an added negative test. All other missing-leaf checks remain active.

Independent ci_review APPROVE on local source fcdaacbe, conditional on unchanged complete hosted quick. Reviewer ran parsed-documents: 52 PASS and diff check clean. Manager local schema 1637 PASS; proto 108 PASS; focused ESLint exit0. Initial hosted run37189626029 failed the mandatory contract commit naming check before tests; this checkpoint records the required contract classification. Full gate pending.
