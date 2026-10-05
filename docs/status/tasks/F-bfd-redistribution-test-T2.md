# F-bfd-redistribution independent T2 API test

Current exact source:97f67a2cd3a3584fea37f7956bb302cd722b0a52. Current verdict:PASS. The original evidence below is preserved as history.

Exact source:8f3d2f078 (full SHA recorded in R3report). Date:2026-10-05 UTC. Assigned slot10, actual exclusive `/run/lock/ngfw-slot-10.lock`. Real PostgreSQL18 fixture entirely RAM on127.0.0.1:4002 and private Valkey on4001; unchanged API plus clearly identified in-process fake agent. Shared PostgreSQL/Valkey/VPP unchanged. Unrelated w10sysident/swan-stock directories and foreign sharedValkey10 keys untouched.

Reviewer-only Vitest wrapper mocks only harness environment selection to pass `NGFW_VALKEY_URL` for the private instance, then imports unchanged product BFD/config e2e suites; it does not mock API routes, controllers, validation, RBAC, database or commit machinery. The original global setup's shared teardown is replaced by private-DB lifecycle; only fresh owned private ngfw_w10role/database is dropped.

Command in owned source snapshot `/dev/shm/r3-021ue61v/bfd/apps/api`, with privatePGhost/port and TMPDIR:
`../../tools/heavy.sh pnpm exec vitest run -c .reviewer-r3/config.ts`

```text
Test Files1 passed(1)
Tests17 passed(17)
Duration35.16s
create role ngfw_w10
create database ngfw_w10(owner ngfw_w10)
drop database ngfw_w10
drop role ngfw_w10
ok nothing named ngfw_w10/ngfw_w10 remains
```

|Scenario|Observed|Result|
|---|---|---|
|Standalone BFD candidate/commit/live API|200and explicit fake observed Down state|PASS|
|Conflicting same-AF FRR/VPP BFD|400problem-json with `/routing/bgp/neighbors/10.14.2.2/bfd` pointer|PASS|
|Existing config regression15cases|Transactional commit/rollback, confirmed auto-revert, 400/404pointers,403roles,409singlewriter, secret output protection, stale-lock security, audit, agent503|PASS|
|Actual simultaneous different-operator writes|Exactly200/409; conflict problem-json; no source modification|PASS|
|Actual APIrestart|Reviewer fixture scalar-body415before restart; corrected fixture object payload prepared, not rerun under manager instruction|UNVERIFIED|

Supplement command: `... pnpm exec vitest run -c .reviewer-r3/config.ts .reviewer-r3/restart.test.ts`
```text
Tests1 failed|1 passed(2)
restart setup:expected415to be200
concurrent-writer arbitration:passed
Duration28.36s
OWN fixture processes stopped; shared services untouched.
```

No product failure found in executed BFD/API cases. Existing schema6/proto112/sealed-secret2also passed independently, normal generated31/31artifacts match source. Native multihop packet runtime and a real agent are T3scope, not proved by this fake-agent T2fixture.

**Verdict:PASS for executed feature/config/concurrent-writer scenarios; APIrestart acceptance remains unverified and needs the corrected fixture run before claiming complete T2coverage.**

## Current retest and restart closure

On source97f67a2cd, existing BFD/config17cases again passed; simultaneous-write case passed. Restart setup correctly wrote object config and recreated the actual API against persistent private DB/Valkey, but the reviewer incorrectly JSON.parsed an existing raw scalar GET leaf. This was fixture-only; no product failure. The corrected test reads whole `/api/v1/config/system` and asserts its hostname, preserving the persisted-config and authenticated live-state requirements.

Commands:unchanged private-fixture wrapper `pnpm exec vitest run -c .reviewer-r3/config.ts`, then corrected targeted `pnpm exec vitest run -c .reviewer-r3/config.ts .reviewer-r3/restart.test.ts`.
```text
First combined wave:18passed/1reviewer-fixture failure(19)
Corrected targeted wave:Tests2passed(2)
Duration30.70s
bfd T2exit0
```

Actual restart and simultaneous-write acceptance now PASS. Nineteen unique scenarios passed on this exact source across the two commands; no native BFD packet execution claim.

**Current T2 verdict:PASS.**
