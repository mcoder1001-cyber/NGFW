import { readFileSync, writeFileSync, openSync } from 'node:fs';
import { createApp } from '/usr/lib/ngfw/api/dist/app.js';
import { CommitService } from '/usr/lib/ngfw/api/dist/commit/commit.service.js';
import { DatastoreService } from '/usr/lib/ngfw/api/dist/datastore/datastore.service.js';
import { AuditService } from '/usr/lib/ngfw/api/dist/audit/audit.service.js';
if (process.getuid() !== 0) throw new Error('root SSH maintenance only');
const work='/root/ngfw-half-resources-20261014';
const snapshot=process.argv[2] === 'snapshot';
const before=snapshot?null:JSON.parse(readFileSync(work+'/before.json','utf8'));
const plan=snapshot?null:JSON.parse(readFileSync(work+'/plan.json','utf8'));
const output=openSync(work+(snapshot?'/before.json':'/running.json'),snapshot?'wx':'w',0o600);
// Root authorizes the operation; use the existing service UID for PostgreSQL peer authentication.
process.setgid('ngfw'); process.setuid('ngfw');
const app = await createApp({docs:false,logger:['error']});
try {
 const ds=app.get(DatastoreService);
 const {revision,doc}=await ds.getRunning();
 if (process.argv[2] === 'snapshot') {
  writeFileSync(output,JSON.stringify({revision:revision?.id??null,doc},null,2),{mode:0o600,flag:'wx'});
  console.log(JSON.stringify({revision:revision?.id,dataplane:doc.dataplane,interfaces:doc.interfaces},null,2));
 } else {
  if (revision?.id !== before.revision) throw new Error('running revision changed; replan');
  const audit=app.get(AuditService);
  const entry={userId:null,username:'root (SSH maintenance)',sourceIp:process.env.SSH_CONNECTION?.split(' ')[0]??null,action:'appliance.dataplane.defaults',resource:'/dataplane',before:doc.dataplane,after:plan.dataplane,result:'failure',status:null};
  const id=await audit.begin(entry);
  try {
   const result=await app.get(CommitService).systemCommit(running=>{
    if(JSON.stringify(running)!==JSON.stringify(before.doc)) throw new Error('running changed under lock');
    return {...running,dataplane:plan.dataplane};
   },'Owner-approved root SSH: half CPU/RAM dataplane startup defaults, data NIC queues/rings; management excluded');
   if(result.status==='deferred') throw new Error(result.reason);
   await audit.finish(id,{...entry,result:'success',status:200});
   const updated=await ds.getRunning();
   writeFileSync(output,JSON.stringify(updated.doc,null,2),{mode:0o600});
   console.log(JSON.stringify({status:result.status,revision:updated.revision?.id,notApplied:result.notApplied,warnings:result.warnings}));
  } catch(e) {await audit.finish(id,{...entry,result:'failure',status:500}); throw e;}
 }
} finally {await app.close();}
