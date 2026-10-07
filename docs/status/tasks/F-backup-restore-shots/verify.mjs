import {readFileSync, writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {launchBrowser,newPage} from '../../../../apps/web/test/e2e/lib/browser.mjs';
import {login} from '../../../../apps/web/test/e2e/lib/auth.mjs';
import {createTranslator} from '../../../../apps/web/test/e2e/lib/locales.mjs';
import {shot} from '../../../../apps/web/test/e2e/lib/shot.mjs';
import {createChecklist} from '../../../../apps/web/test/e2e/lib/checklist.mjs';
const base='http://127.0.0.1:16600';
const secret=JSON.parse(readFileSync('/run/ngfw-test/w26browser/login.json','utf8'));
const t=createTranslator(resolve('apps/web'),['en','fa']);
const browser=await launchBrowser({args:['--no-sandbox']});
const errors=[];const network=[];const checks=createChecklist();
try {for (const lang of ['en','fa']) for (const mode of ['light','dark']) {
 const p=await newPage(browser,{lang,mode,persianDigits:lang==='fa'});let upgradeStatus;
 p.page.on('response',async r=>{if(new URL(r.url()).pathname==='/api/v1/actions/upgrade' && r.ok()) {const v=await r.json();upgradeStatus=JSON.parse(v.lines[0]);}});
 p.page.on('response',r=>{if(r.status()>=400)network.push({lang,mode,status:r.status(),path:new URL(r.url()).pathname});});
 p.page.on('requestfailed',r=>errors.push({lang,mode,path:new URL(r.url()).pathname,error:r.failure()?.errorText}));
 await p.page.goto(base+'/login');await login(p.page,t,lang,secret.username,secret.password);
 try {await p.page.waitForURL(u=>!u.pathname.includes('/login'),{timeout:20000});} catch(e) {console.log('LOGIN_RESPONSE',JSON.stringify(network));console.log('LOGIN_PAGE',await p.page.locator('body').innerText());throw e;}
 for(const [slug,title] of [['backup-restore','title'],['upgrade','upgrade']]) {
  await p.page.goto(base+'/system/'+slug);
  await p.page.getByRole('heading',{name:t(lang,'backup-restore:'+title),exact:true}).waitFor();
  if(slug==='backup-restore') {
   await p.page.getByText(t(lang,'backup-restore:noRuns'),{exact:true}).waitFor();
   checks.check(await p.page.getByRole('button',{name:t(lang,'backup-restore:download'),exact:true}).isDisabled(),'backup requires passphrase '+lang+'/'+mode);
   checks.check(await p.page.getByRole('button',{name:t(lang,'backup-restore:restore'),exact:true}).isDisabled(),'restore requires file and passphrase '+lang+'/'+mode);
  } else {
   await p.page.getByText(t(lang,'backup-restore:confirmed'),{exact:true}).waitFor();
   checks.check(Boolean(upgradeStatus),'real API upgrade status '+lang+'/'+mode);
   const disabled={uploadStage:true,activate:!upgradeStatus.staged_slot||Boolean(upgradeStatus.pending_slot),confirm:!upgradeStatus.pending_slot||upgradeStatus.active_slot!==upgradeStatus.pending_slot,rollback:!upgradeStatus.pending_slot};
   for (const key of Object.keys(disabled)) checks.check((await p.page.getByRole('button',{name:t(lang,'backup-restore:'+key),exact:true}).isDisabled())===disabled[key],'upgrade '+key+' matches real status '+lang+'/'+mode);
   console.log('STATUS',JSON.stringify(upgradeStatus));
  }
  await shot(p.page,resolve('docs/status/tasks/F-backup-restore-shots'),`${slug}-${lang}-${mode}-real`,{base,lang,check:checks.check});
 }
 p.assertNoPageErrors(lang+'/'+mode);await p.close();
}
writeFileSync(resolve('docs/status/tasks/F-backup-restore-shots/network.json'),JSON.stringify({network,requestFailures:errors,checks:checks.results},null,2));
console.log('NETWORK',JSON.stringify(network));console.log('REQUEST_FAILURES',JSON.stringify(errors));
} finally {await browser.close();}
