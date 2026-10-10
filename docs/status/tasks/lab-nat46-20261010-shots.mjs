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
  await page.goto('http://127.0.0.1:15700/firewall/nat?tab=nat46');await page.getByRole('tab',{name:'NAT46',exact:true}).waitFor();
  const locale=JSON.parse(readFileSync(root+'/apps/web/src/locales/'+lang+'/nat46.json'));
  await page.getByLabel(locale.client.ipv4,{exact:true}).fill('10.17.1.2');await page.getByRole('button',{name:locale.client.show,exact:true}).click();
  await page.getByTestId('nat46-client').filter({hasText:'fd00:11:4646::a11:102'}).waitFor();
  if(errors.length)throw new Error(errors.join('\n'));
  await page.screenshot({path:root+'/docs/status/tasks/lab-nat46-20261010-evidence/nat46-'+lang+'.png',fullPage:true});await context.close();
 }
 console.log('NAT46_BROWSER_EN_FA_PASS');
}finally{if(browser)await browser.close();web.kill('SIGTERM');}
