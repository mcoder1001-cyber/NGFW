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
const p=await newPage(browser,{lang:'en',mode:'light'});const replies=[];
p.page.on('response',async r=>{const path=new URL(r.url()).pathname;if(path.includes('config-templates')&&r.request().method()!=='GET') replies.push({path,status:r.status(),body:await r.json()});});
try {
 await p.page.goto(base+'/login');await login(p.page,t,'en',secret.username,secret.password);await p.page.waitForURL(u=>!u.pathname.includes('/login'),{timeout:20000});
 await p.page.goto(base+'/system/backup-restore');await p.page.getByLabel(t('en','backup-restore:name'),{exact:true}).fill('w26browser_ux');
 await p.page.getByLabel(t('en','backup-restore:description'),{exact:true}).fill('Browser review template');
 await p.page.getByLabel(t('en','backup-restore:parameterDefinition'),{exact:true}).fill(JSON.stringify({banner:{type:'string',required:true}}));
 await p.page.getByLabel(t('en','backup-restore:patch'),{exact:true}).fill(JSON.stringify({system:{banner:{motd:'$'+'{banner}'}}}));
 await p.page.getByRole('button',{name:t('en','backup-restore:saveTemplate'),exact:true}).click();
 await p.page.getByText(t('en','backup-restore:staged'),{exact:true}).waitFor({timeout:15000});
 await p.page.getByLabel('banner',{exact:false}).fill('w26browser review candidate only');
 await p.page.getByRole('button',{name:t('en','backup-restore:apply'),exact:true}).click();
 await p.page.waitForTimeout(1500);p.assertNoPageErrors();
 console.log('CANDIDATE_RESPONSES',JSON.stringify(replies));
 if(replies.length!==2 || replies.some(r=>r.status!==200)||replies[1].body.staged!==true || !replies[1].body.diff.changes.some(c=>c.pointer==='/system/banner/motd'&&c.to==='w26browser review candidate only')) throw new Error('candidate staging did not return expected real responses');
 await shot(p.page,resolve('docs/status/tasks/F-backup-restore-shots'),'backup-restore-en-light-template-candidate',{base,lang:'en'});
} finally {await browser.close();}
