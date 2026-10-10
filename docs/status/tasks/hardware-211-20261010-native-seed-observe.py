#!/usr/bin/env python3
"""Read-only native API and protected-host observation around manager noPCI apply.

No service, startup, driver, candidate, database or revision mutation. Login
credentials travel only over SSH stdin; bearer tokens never enter the receipt.
"""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import hashlib,json,os,pathlib,re,socket,ssl,stat,subprocess,time,urllib.error,urllib.request
def run(a):
 p=subprocess.run(a,capture_output=True,text=True,timeout=30);return {'argv':a,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
def checked(a):
 q=run(a);assert q['exit']==0,(a,q['exit']);return q['stdout']
def value(u,k):return checked(['systemctl','show',u,'-p',k,'--value']).strip()
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def inventory():
 out={}
 for name,q in INVENTORY.items():
  p=pathlib.Path('/sys/class/net')/name/'device';group=(p/'iommu_group').resolve()
  out[name]={'PCI':p.resolve().name,'driver':(p/'driver').resolve().name,'group':group.name,'members':sorted(x.name for x in (group/'devices').iterdir())}
 return out
def sysctls():return {k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in SYSCTLS}
def dns():return {p:{'SHA':hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest(),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in DNS}
def foreign(d):return [x for x in d['nftables'] if 'metainfo' not in x and not any(isinstance(v,dict) and ((k=='table' and v.get('family')=='inet' and v.get('name')=='ngfw_base') or (k!='table' and v.get('family')=='inet' and v.get('table')=='ngfw_base')) for k,v in x.items())]
units=['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']
assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
assert pathlib.Path('/etc/ngfw/agent.env').read_bytes()==b'NGFW_MGMT_IF=enp4s0\nNGFW_MGMT_PCI=0000:04:00.0\n'
assert pathlib.Path('/etc/systemd/system/ngfw-api.service.d/10-hardware-seed.conf').read_bytes()==b'[Service]\nEnvironment=NGFW_SEED_DEFAULT_NICS=1\n'
assert hashlib.sha256(pathlib.Path('/etc/ngfw/api.env').read_bytes()).hexdigest()==API_ENV_SHA
dev=pathlib.Path('/sys/class/net/enp4s0/device');assert dev.resolve().name=='0000:04:00.0' and (dev/'driver').resolve().name=='igc' and (dev/'iommu_group').resolve().name=='28'
before=l3();assert before==NETWORK;before_inventory=inventory();assert before_inventory==INVENTORY
assert sysctls()==SYSCTLS and dns()==DNS
states={u:{k:value(u,k) for k in ['ActiveState','MainPID','NRestarts']} for u in units}
assert all(q['ActiveState']=='active' and q['NRestarts']=='0' for q in states.values())
if MODE=='before':assert states==UNITS
startup=pathlib.Path('/etc/vpp/startup.conf');raw=startup.read_bytes();st=startup.stat();sha=hashlib.sha256(raw).hexdigest();assert sha==STARTUP_SHA
text=raw.decode();assert '  no-pci\n' in text and 'blacklist 0000:04:00.0' in text and not any(re.match(r'\s*dev\s+(?!default(?:\s|\{))',x) for x in text.splitlines())
if MODE=='after':
 for p in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so']:assert 'plugin '+p+' { enable }' in text
with socket.create_connection(('172.30.126.195',22),timeout=5):pass
kernel_before=checked(['dmesg','--color=never']);io_before=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip()
result={'mode':MODE,'baseline_SHA':BASELINE_SHA,'startup':{'SHA':sha,'bytes':len(raw),'uid':st.st_uid,'gid':st.st_gid,'mode':oct(stat.S_IMODE(st.st_mode)),'text':text},'unit_states_before':states,'network_before':before,'inventory_before':before_inventory,'sysctls_before':sysctls(),'DNS_before':dns(),'nft_before':json.loads(checked(['nft','-j','list','ruleset'])),'VPP_commands':[],'read_only_no_service_driver_config_or_revision_mutation':True}
for a in [['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','version'],['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','bootid'],['vppctl','show','plugins']]:result['VPP_commands'].append(run(a))
result['required_plugins_loaded']=all(p in result['VPP_commands'][-1]['stdout'] for p in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so'])
result['manager_runtime_replaced']=states['vpp.service']['MainPID']!=UNITS['vpp.service']['MainPID']
ctx=ssl.create_default_context(cafile='/etc/ngfw/tls/server.crt')
def request(path,token=None,data=None):
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 req=urllib.request.Request('https://localhost'+path,data=None if data is None else json.dumps(data).encode(),headers=headers,method='GET' if data is None else 'POST')
 try:
  with urllib.request.urlopen(req,context=ctx,timeout=10) as r:
   b=r.read(2097153);assert len(b)<=2097152
   return {'status':r.status,'revision':r.headers.get('x-ngfw-revision'),'data':json.loads(b) if r.headers.get('Content-Type','').startswith('application/json') else None,'bytes':len(b)}
 except urllib.error.HTTPError as e:return {'status':e.code,'bytes':len(e.read(65536))}
login=request('/api/v1/auth/login',data={'username':ADMIN_USER,'password':ADMIN_PASSWORD});body=login.get('data') or {};token=body.get('accessToken');result['login']={'status':login['status'],'role':body.get('user',{}).get('role'),'token_received':isinstance(token,str) and len(token)>0};assert login['status']==200 and token and result['login']['role']=='admin'
result['seed_polls']=[]
for i in range(22 if MODE=='after' else 1):
 cfg=request('/api/v1/config',token);events=request('/api/v1/state/events?limit=100',token);result['running']=cfg;result['events']=events;result['seed_polls'].append({'attempt':i,'revision':cfg.get('revision'),'status':cfg['status'],'event_status':events['status']})
 if MODE=='before' or (cfg.get('revision')=='1' and any(e.get('code')=='system.seed-defaults' for e in (events.get('data') or {}).get('items',[]))):break
 if i!=21:time.sleep(3)
for k,p in {'candidate':'/api/v1/config/candidate','pending':'/api/v1/config/commit/pending','system':'/api/v1/state/system','interfaces':'/api/v1/state/interfaces','dataplane':'/api/v1/state/dataplane'}.items():result[k]=request(p,token)
doc=result['running'].get('data');result['candidate_equal_running']=result['candidate'].get('data')==doc;result['no_pending_commit']=(result['pending'].get('data') or {}).get('pending','missing') is None
result['state_RPC_HTTP_PASS']=all(result[k]['status']==200 for k in ['running','events','candidate','pending','system','interfaces','dataplane']) and (result['system'].get('data') or {}).get('agent',{}).get('reachable') is True
rows={name:row for name,row in (doc or {}).get('interfaces',{}).items() if row.get('physical')};dp=(doc or {}).get('dataplane',{});want={q['PCI'] for q in INVENTORY.values()}
result['seeded17_exact']=set(rows)==set(INVENTORY) and all(row['physical'].get('pci')==INVENTORY[name]['PCI'] and row['physical'].get('owner')=='dataplane' and row['physical'].get('builtIn') is True for name,row in rows.items()) and set(dp.get('pciWhitelist',[]))==want and set(dp.get('devices',{}))==want and all(dp['devices'][q['PCI']].get('name')==name for name,q in INVENTORY.items()) and dp.get('managementPci')==['0000:04:00.0']
result['revision_expectation_PASS']=(cfg.get('revision')=='0' and doc==EMPTY_DOCUMENT) if MODE=='before' else (cfg.get('revision')=='1' and result['seeded17_exact'] and any(e.get('code')=='system.seed-defaults' for e in (events.get('data') or {}).get('items',[])))
result['network_after']=l3();result['network_equal']=result['network_after']==NETWORK;result['inventory_after']=inventory();result['all17_still_kernel']=result['inventory_after']==INVENTORY;result['sysctls_after']=sysctls();result['sysctls_equal']=result['sysctls_after']==SYSCTLS;result['DNS_after']=dns();result['DNS_equal']=result['DNS_after']==DNS;result['nft_after']=json.loads(checked(['nft','-j','list','ruleset']));result['foreign_nft_unchanged']=foreign(result['nft_before'])==foreign(result['nft_after']);result['unit_states_after']={u:{k:value(u,k) for k in ['ActiveState','MainPID','NRestarts']} for u in units};result['unit_identity_stable']=result['unit_states_after']==states
kernel_after=checked(['dmesg','--color=never']);io_after=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();new=kernel_after.splitlines()[len(kernel_before.splitlines()):] if kernel_after.startswith(kernel_before) else None
bad=re.compile(r'(EXT4-fs error|Buffer I/O error|I/O error, dev sda|ata\d.*(hard resetting|failed command|error:.*\b(UNC|ICRC)\b))',re.I)
result.update(kernel_before=kernel_before,kernel_after=kernel_after,ioerr_before=io_before,ioerr_after=io_after,new_storage_errors=None if new is None else [x for x in new if bad.search(x)])
result['journal']=run(['journalctl','--no-pager','-u','ngfw-api.service','-u','ngfw-agent.service','-n','100'])
passed=all(result[k] for k in ['candidate_equal_running','no_pending_commit','state_RPC_HTTP_PASS','revision_expectation_PASS','network_equal','all17_still_kernel','sysctls_equal','DNS_equal','foreign_nft_unchanged','unit_identity_stable']) and result['new_storage_errors']==[] and io_before==io_after and all(q['exit']==0 for q in result['VPP_commands'])
if MODE=='after':passed=passed and result['required_plugins_loaded'] and result['manager_runtime_replaced']
result['observation_PASS']=passed;print(json.dumps(result,indent=2));raise SystemExit(0 if passed else 2)
'''
def save(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def read_private(p):
 assert p.parent==PRIVATE and not p.is_symlink() and p.stat().st_uid==0 and stat.S_IMODE(p.stat().st_mode)==0o600
 return p.read_bytes()
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--mode',choices=['before','after'],required=True);a=p.parse_args()
 q=PRIVATE/'initial-runtime-start-20261010T122029Z.json';raw=read_private(q);sha=hashlib.sha256(raw).hexdigest();assert sha=='98b516a17a02cc82a8b6e26b3ac81d3e1ea8b3cf28fb73b1c0a68bd41b9376fe';d=json.loads(raw)
 assert d['network_equal'] and d['all17_still_kernel'] and d['sysctls_equal'] and d['DNS_network_files_equal'] and d['foreign_nft_unchanged'] and d['new_storage_errors']==[]
 firstboot=read_private(PRIVATE/'firstboot-apply-20261010T120628Z.json');assert hashlib.sha256(firstboot).hexdigest()==d['firstboot_proof_SHA'];fb=json.loads(firstboot)
 c=json.loads(read_private(PRIVATE/'bootstrap-admin-211.json'));assert c['host']=='172.30.110.211'
 fields={'MODE':a.mode,'BASELINE_SHA':sha,'NETWORK':d['network_after'],'INVENTORY':d['inventory_after'],'UNITS':d['unit_states'],'SYSCTLS':d['sysctls_after'],'DNS':d['DNS_network_files_after'],'API_ENV_SHA':fb['files']['/etc/ngfw/api.env']['sha256'],'EMPTY_DOCUMENT':d['last_observed_config']['data'],'STARTUP_SHA':'c892394e36bfc45950407128a81c849fdaa1c66a1fbe64ce9154fa271f215b5e' if a.mode=='before' else '367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184','ADMIN_USER':c['username'],'ADMIN_PASSWORD':c['password']}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 r=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 print(json.dumps({'SSH_exit':r.returncode,'stdout':save(PRIVATE/('native-seed-'+a.mode+'-'+stamp+'.json'),r.stdout),'stderr':save(PRIVATE/('native-seed-'+a.mode+'-'+stamp+'.stderr'),r.stderr),'read_only':True}));raise SystemExit(r.returncode)
if __name__=='__main__':main()
