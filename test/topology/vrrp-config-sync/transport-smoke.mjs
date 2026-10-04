// Host-independent transport acceptance: disposable TLS server, generated test key, no product/lab mutation.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash, X509Certificate } from 'node:crypto';
import { once } from 'node:events';
import { mkdtempSync,readFileSync,rmSync } from 'node:fs';
import { createServer } from 'node:https';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { sendPinned,validSignature } from '../../../apps/api/dist/features/vrrp-config-sync/transport.js';
const dir=mkdtempSync(join(tmpdir(),'ngfw-ha-tls-'));
const key='NGFW_TEST_PSK_cluster_auth_fixture';
let server;
try {
 execFileSync('openssl',['req','-x509','-newkey','rsa:2048','-nodes','-keyout',join(dir,'key'),'-out',join(dir,'cert'),'-days','1','-subj','/CN=127.0.0.1'],{stdio:'ignore'});
 const certificate=readFileSync(join(dir,'cert')),pin=createHash('sha256').update(new X509Certificate(certificate).raw).digest('hex');
 let received=0;
 server=createServer({key:readFileSync(join(dir,'key')),cert:certificate},(req,res)=>{
  received++; const parts=[];req.on('data',p=>parts.push(p));req.on('end',()=>{
   const raw=Buffer.concat(parts).toString(),body=JSON.parse(raw);
   assert.equal(validSignature(JSON.stringify(body),key,req.headers['x-ngfw-cluster-signature']),true);
   assert.equal(body.origin,'node-a');assert.equal(body.document.management,undefined);
   res.writeHead(201,{'content-type':'application/json'});res.end(JSON.stringify({revision:{id:2},role:'backup'}));
  });
 });
 server.listen(0,'127.0.0.1');await once(server,'listening');const port=server.address().port;
 assert.deepEqual(await sendPinned('127.0.0.1',port,pin,key,'node-a',1,{routing:{}},new AbortController().signal),{revision:2,role:'backup'});
 assert.equal(received,1);
 await assert.rejects(sendPinned('127.0.0.1',port,'0'.repeat(64),key,'node-a',1,{},new AbortController().signal),/pin mismatch/);
 assert.equal(received,1,'wrong certificate pin must prevent any HTTP request');
 console.log('PASS: real disposable HTTPS; HMAC authenticated; matching pin delivers; wrong pin sends zero HTTP requests');
} finally {
 if(server) await new Promise(resolve=>server.close(resolve));
 rmSync(dir,{recursive:true,force:true});
}
