# T2 independent API/SQL acceptance and concurrency

SHA c4ad647cf6df3dd951630b843e738fc4d60ac7fe. Exclusive fixture slotw28 assigned/released by manager. PostgreSQL18.6; Valkey logicaldb10 shared by key prefix ngfw:w28:e2e: only. Never reused w26browser, never flushed shared DB, never host upgrade/VPP operation.

```text
NGFW_TEST_PREFIX=w28 NGFW_VALKEY_DB=10 pnpm --filter @ngfw/api exec vitest run --config vitest.e2e.config.ts test/e2e/backup-restore.e2e.test.ts
Tests 6 passed (6)
FBR recovered document SHA256 ad27929269906644d14d795f096a6dd441780cbe241acf04821e88df696974e4; inactive secret preserved before commit
```

Actual output `/tmp/fbr-correctness-e2e.log`; cases: recovery/authentication/inactive pins/discard/confirm/current accounts/audit refusal; templates/injection; durable scheduled local export;10MiBstream and >8MiBJSONrestore; oversized history; local retention. Source tests share staged fixture sequence; separate independent race probes each start a fresh harness.

```text
NGFW_TEST_PREFIX=w28 NGFW_VALKEY_DB=10 pnpm --filter @ngfw/api exec vitest run --config /tmp/ngfw-fbr-r1-race/vitest.config.ts
```

| Probe | Expected | Observed | Outcome |
|---|---|---|---|
| committed account between snapshot user read and import transaction | new account remains | before admin,race-account; after admin | FAIL twice, race1/race2 logs |
| newer restore staged during held earlier commit response | payload and pins retained | payload retained; pins {password/r1pin:2} -> null | FAIL twice, pins2/pins3 logs |
| first pin probe setup | JSON config body accepted | scalar injected body415 | reviewer fixture error; corrected objectPUT, excluded from product grade |

Each run removed its w28 database and role and4/5 Valkey prefix keys; logs verify `ok nothing named ngfw_w28 / ngfw_w28 remains`. No persistent fixture remains after teardown.

Two source bugs fixed by author9c1f1f3a9; current reviewer source1fa4d24ab includes them, independently fixed-source rerun pending. Account reproduction below has been adapted to intercept importCandidate, preserving the same deterministic race boundary even when fixed code removes the old external user read. This avoids a false pass because an obsolete getRunning spy no longer fires.

## Independent authored-fix closure

Verified source49393f40a668b4acd90e3acbfe009b6211820e4e; product/test paths identical root243f44f23591c83cb0f7dffb7bc8b88f1f6b1746. Exact same commands on this source:

```text
Test Files1 passed (1)
Tests8 passed (8)
Accounts before restore commit: admin,race-account
Accounts after restore commit: admin,race-account
Candidate pins before unrelated promotion: {"password/r1pin":2}
Candidate pins after unrelated promotion: {"password/r1pin":2}
Test Files1 passed (1)
Tests2 passed (2)
e2e teardown: deleted8 Valkey keys ngfw:w28:e2e:* in db10
drop database ngfw_w28; drop role ngfw_w28
ok nothing named ngfw_w28 / ngfw_w28 remains
```

Logs `/tmp/fbr-correctness-final-e2e.log`, `/tmp/fbr-correctness-final-race.log`. No original failing race excluded; adapted import-entry account hook executes actual account creation with fixed service. Pin test observes nonempty SQL map before/after actual API promotion.

Verdict: **PASS (T2)** for verified product tree; final quick/contract review separate.

## Durable external recovery probe

```ts
import {describe,it,expect,vi} from 'vitest';
import {startHarness} from '/root/ngfw-wt/f-backup-correctness-review-20261005/apps/api/test/support/harness.ts';
import {DatastoreService} from '/root/ngfw-wt/f-backup-correctness-review-20261005/apps/api/src/datastore/datastore.service.ts';
import {BackupRestoreService} from '/root/ngfw-wt/f-backup-correctness-review-20261005/apps/api/src/features/backup-restore/backup-restore.service.ts';
import {AgentClient} from '/root/ngfw-wt/f-backup-correctness-review-20261005/apps/api/src/agent/agent.client.ts';
import {configCandidate,appUser} from '/root/ngfw-wt/f-backup-correctness-review-20261005/apps/api/src/db/schema.ts';
describe('R1 account preservation race',()=>{
 it('preserves account committed between restore snapshot read and candidate transaction',async()=>{
  const h=await startHarness();
  try{
   const token=await h.login('admin',h.adminPassword);
   await h.createUsers(token,[]);
   const backup=h.app.get(BackupRestoreService);
   const bytes=await backup.backup({id:1,username:'admin',role:'admin',via:'jwt'},'NGFW_TEST_PSK_R1_BACKUP');
   const ds=h.app.get(DatastoreService), original=ds.importCandidate.bind(ds);
   const read=vi.spyOn(ds,'importCandidate').mockImplementationOnce(async(...args)=>{
    await h.createUsers(token,[{username:'race-account',role:'operator',password:'NGFW_TEST_PSK_R1_ACCOUNT'}]);
    return original(...args);
   });
   const restored=await h.call(token,'POST','/api/v1/actions/restore',{passphrase:'NGFW_TEST_PSK_R1_BACKUP',archive:bytes.toString('base64')});
   expect(restored.status).toBe(200);
   read.mockRestore();
   const usersBefore=await h.db.select({username:appUser.username}).from(appUser);
   console.log('Accounts before restore commit:',usersBefore.map(x=>x.username).join(','));
   expect((await h.call(token,'POST','/api/v1/config/commit')).status).toBe(200);
   const usersAfter=await h.db.select({username:appUser.username}).from(appUser);
   console.log('Accounts after restore commit:',usersAfter.map(x=>x.username).join(','));
   expect(usersAfter).toEqual(usersBefore);
  }finally{vi.restoreAllMocks();await h.close();}
 });
});

describe('R1 concurrent candidate restore',()=>{
 it('preserves restored secret pins across promotion of a different in-flight commit',async()=>{
  const h=await startHarness();
  try{
   const token=await h.login('admin',h.adminPassword);
   await h.createUsers(token,[]);
   await h.call(token,'POST','/api/v1/secrets',{kind:'password',name:'r1pin',value:'NGFW_TEST_PSK_R1_PIN'});
   expect((await h.call(token,'PUT','/api/v1/config/management/backup',{enabled:false,passphraseRef:'password/r1pin'})).status).toBe(200);
   expect((await h.call(token,'POST','/api/v1/config/commit')).status).toBe(200);
   const bytes=await h.app.get(BackupRestoreService).backup({id:1,username:'admin',role:'admin',via:'jwt'},'NGFW_TEST_PSK_R1_BACKUP');
   expect((await h.call(token,'PUT','/api/v1/config/system',{hostname:'in-flight'})).status).toBe(200);
   const agent=h.app.get(AgentClient), original=agent.apply.bind(agent);
   let started!:()=>void, release!:()=>void;
   const applied=new Promise<void>(resolve=>{started=resolve;});
   const held=new Promise<void>(resolve=>{release=resolve;});
   vi.spyOn(agent,'apply').mockImplementationOnce(async(...args)=>{const response=await original(...args);started();await held;return response;});
   const commit=h.call(token,'POST','/api/v1/config/commit');
   await applied;
   const restored=await h.call(token,'POST','/api/v1/actions/restore',{passphrase:'NGFW_TEST_PSK_R1_BACKUP',archive:bytes.toString('base64')});
   expect(restored.status).toBe(200);
   const [before]=await h.db.select().from(configCandidate);
   console.log('Candidate pins before unrelated promotion:',JSON.stringify(before.restoreSecrets));
   release();
   expect((await commit).status).toBe(200);
   const [after]=await h.db.select().from(configCandidate);
   console.log('Candidate pins after unrelated promotion:',JSON.stringify(after.restoreSecrets));
   expect(after.payload).toEqual(before.payload);
   expect(after.restoreSecrets).toEqual(before.restoreSecrets);
  }finally{vi.restoreAllMocks();await h.close();}
 });
});
```

```ts
import {defineConfig} from 'vitest/config';
import swc from 'unplugin-swc';
export default defineConfig({test:{include:['/tmp/ngfw-fbr-r1-race/race.e2e.test.ts'],environment:'node',globalSetup:['/root/ngfw-wt/f-backup-correctness-review-20261005/apps/api/test/support/global-setup.ts'],fileParallelism:false,testTimeout:30000,hookTimeout:60000},plugins:[swc.vite({module:{type:'es6'}})]});
```

## Final integration recheck — PASS

Source 4c8640695c8ac9877819817506979e361e801216; tree 79c94d41e9a5e0fc71b7a8837ed9956d661258fd. Commands, all exit0:

```sh
NGFW_TEST_PREFIX=w28 NGFW_VALKEY_DB=10 pnpm --filter @ngfw/api exec vitest run --config vitest.e2e.config.ts test/e2e/backup-restore.e2e.test.ts
NGFW_TEST_PREFIX=w28 NGFW_VALKEY_DB=10 pnpm --filter @ngfw/api exec vitest run --config /tmp/ngfw-fbr-r1-race/vitest.config.ts
NGFW_TEST_PREFIX=w28 NGFW_VALKEY_DB=10 pnpm --filter @ngfw/api exec vitest run --config /tmp/ngfw-fbr-r2-final/vitest.config.ts
pnpm audit --prod --json
```

Authored real PostgreSQL9/9PASS including100000 history rows uses generated descending index; original external race2/2PASS, current account preserved and pins{"password/r1pin":2} identical; independent audit refusal/unauthorized raw upload1/1PASS. Production audit0 advisories, all severity counts0. Logs respectively /tmp/fbr-correctness-integration-final-e2e.log, /tmp/fbr-correctness-integration-final-race.log, /tmp/fbr-r2-integration-final-security.log, /tmp/fbr-r2-integration-final-audit.json. All three harnesses created/dropped private ngfw_w28 DB/role sequentially; cleaned only ngfw:w28:e2e:* keys in ValkeyDB10, no flush; no live host upgrade/VPP mutation. Final verdict **PASS**. Earlier intermediate findings/pending evidence retained above for recovery history.
