import { createRequire } from 'node:module';
import { spawn } from 'node:child_process';
import { resolve } from 'node:path';
import { readFileSync } from 'node:fs';
const require=createRequire(import.meta.url);
const {chromium}=require('/root/.npm/_npx/e41f203b7505f1fb/node_modules/playwright-core');
let input='';for await(const chunk of process.stdin)input+=chunk;const {password}=JSON.parse(input);
const root=resolve('');const web=spawn('node',[root+'/apps/web/node_modules/vite/bin/vite.js','preview','--host','127.0.0.1','--port','15700','--strictPort'],{cwd:root+'/apps/web',env:{...process.env,NGFW_HTTP_PORT:'11700',NGFW_WEB_PORT:'15700'},stdio:'ignore'});
let browser;
try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:15700/')).ok)break;}catch{}await new Promise(r=>setTimeout(r,200));}
 browser=await chromium.launch({executablePath:'/tmp/fbr-ux-chrome/chrome-headless-shell-linux64/chrome-headless-shell',headless:true,args:['--no-sandbox'],env:{...process.env,LD_LIBRARY_PATH:'/tmp/fbr-ux-chrome/libroot/usr/lib/x86_64-linux-gnu'}});
 for(const lang of ['en','fa']){
  const context=await browser.newContext({viewport:{width:1440,height:1000}});await context.addInitScript(l=>localStorage.setItem('ngfw.ui.settings',JSON.stringify({lang:l,mode:'light',dense:true,persianDigits:false})),lang);
  const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto('http://127.0.0.1:15700/login');await page.locator('input[autocomplete="username"]').fill('admin');await page.locator('input[type="password"]').fill(password);
  await page.locator('button[type="submit"]').click();await page.waitForURL(url=>!url.pathname.includes('login'));
  await page.goto('http://127.0.0.1:15700/firewall/global-blocking');
  const row=page.getByRole('row').filter({has:page.getByText('proof',{exact:true})});
  await row.waitFor();await row.getByText('host-w17l0',{exact:true}).waitFor();
  if(!(await row.innerText()).includes('2'))throw new Error('real applied entry count missing');
  if(errors.length)throw new Error(errors.join('\n'));
  await page.screenshot({path:root+'/docs/status/tasks/lab-global-blocking-20261010-evidence/global-blocking-'+lang+'.png',fullPage:true});await context.close();
 }
 console.log('GLOBAL_BLOCKING_BROWSER_EN_FA_PASS');
}finally{if(browser)await browser.close();web.kill('SIGTERM');}
