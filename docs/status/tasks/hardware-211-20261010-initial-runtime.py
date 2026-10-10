#!/usr/bin/env python3
"""Start proved no-PCI runtime and observe the real FIRST API NIC seed.

Separate parent/reviewer release required. No startup apply, PCI binding,
candidate edit, manual revision, DB shortcut, service restart or reboot.
"""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import hashlib,json,os,pathlib,re,socket,ssl,stat,subprocess,time,urllib.error,urllib.request
os.umask(0o077)
def run(args,timeout=130):
 try:p=subprocess.run(args,capture_output=True,text=True,timeout=timeout);return {'argv':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
 except subprocess.TimeoutExpired as e:return {'argv':args,'exit':124,'stdout':(e.stdout or b'').decode() if isinstance(e.stdout,bytes) else (e.stdout or ''),'stderr':(e.stderr or b'').decode() if isinstance(e.stderr,bytes) else (e.stderr or ''),'timed_out':True}
def checked(args):
 d=run(args);assert d['exit']==0,(args,d['exit']);return d['stdout']
def value(u,k):return checked(['systemctl','show',u,'-p',k,'--value']).strip()
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def sysctls():return {k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in SYSCTL_BASELINE}
def DNS_network_files():return {p:{'SHA':hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest(),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in ['/etc/resolv.conf','/etc/netplan/90-ngfw-management.yaml']}
def nft():return json.loads(checked(['nft','-j','list','ruleset']))
def foreign(doc):return [x for x in doc['nftables'] if 'metainfo' not in x and not any(isinstance(v,dict) and ((k=='table' and v.get('family')=='inet' and v.get('name')=='ngfw_base') or (k!='table' and v.get('family')=='inet' and v.get('table')=='ngfw_base')) for k,v in x.items())]
def inventory():
 out={}
 for name,pci in EXPECTED_NICS.items():
  p=pathlib.Path('/sys/class/net')/name/'device';assert p.resolve().name==pci
  group=(p/'iommu_group').resolve();members=sorted(x.name for x in (group/'devices').iterdir());assert members==[pci] and group.name!='28'
  out[name]={'PCI':pci,'driver':(p/'driver').resolve().name,'group':group.name,'members':members}
 return out
units=['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']
assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
assert pathlib.Path('/var/lib/ngfw/firstboot-complete').read_bytes()==b'completed\n' and not os.path.lexists('/etc/ngfw/bootstrap.env')
assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service') and not os.path.lexists('/var/lib/ngfw-install-recovery/hardware-211-20261010/state.json')
assert all(value(u,'ActiveState')=='inactive' for u in units)
assert pathlib.Path('/etc/ngfw/agent.env').read_bytes()==b'NGFW_MGMT_IF=enp4s0\nNGFW_MGMT_PCI=0000:04:00.0\n'
assert pathlib.Path('/etc/systemd/system/ngfw-api.service.d/10-hardware-seed.conf').read_bytes()==b'[Service]\nEnvironment=NGFW_SEED_DEFAULT_NICS=1\n'
assert 'NGFW_SEED_DEFAULT_NICS=1' in value('ngfw-api.service','Environment').split()
assert hashlib.sha256(pathlib.Path('/etc/ngfw/api.env').read_bytes()).hexdigest()==API_ENV_SHA
startup=pathlib.Path('/etc/vpp/startup.conf').read_bytes();assert hashlib.sha256(startup).hexdigest()==STARTUP_SHA
text=startup.decode();assert not any(re.match(r'\s*dev\s+(?!default(?:\s|\{))',x) for x in text.splitlines()) and ('  no-pci\n' in text or 'plugin dpdk_plugin.so { disable }' in text)
assert 'blacklist 0000:04:00.0' in text or 'plugin dpdk_plugin.so { disable }' in text
assert pathlib.Path('/proc/sys/vm/nr_hugepages').read_text().strip()=='1024'
dev=pathlib.Path('/sys/class/net/enp4s0/device');assert dev.resolve().name=='0000:04:00.0' and (dev/'driver').resolve().name=='igc' and (dev/'iommu_group').resolve().name=='28'
before=l3();assert before==NETWORK_BASELINE;before_inventory=inventory();sys_before=sysctls();assert sys_before==SYSCTL_BASELINE;DNS_before=DNS_network_files();nft_before=nft()
with socket.create_connection(('172.30.126.195',22),timeout=5):pass
kernel_before=checked(['dmesg','--color=never']);ioerr_before=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip()
result={'sysctls_before':sys_before,'DNS_network_files_before':DNS_before,'nft_before':nft_before,'firstboot_proof_SHA':PROOF_SHA,'network_before':before,'inventory_before':before_inventory,'preflight':True,'commands':[],'no_manual_revision_driver_binding_or_startup_apply':True}
if not APPLY:print(json.dumps(result,indent=2))
else:
 failed=None
 for unit in units:
  d=run(['systemctl','start',unit]);result['commands'].append(d)
  if d['exit']!=0:failed=unit;break
  if unit=='vpp.service':
   for args in [['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','version'],['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','bootid']]:
    q=run(args,15);result['commands'].append(q)
    if q['exit']!=0:failed='VPP binary API';break
   if failed:break
 ctx=ssl.create_default_context(cafile='/etc/ngfw/tls/server.crt')
 def request(path,method='GET',data=None,token=None):
  headers={'Content-Type':'application/json'}
  if token:headers['Authorization']='Bearer '+token
  req=urllib.request.Request('https://localhost'+path,data=None if data is None else json.dumps(data).encode(),headers=headers,method=method)
  try:
   with urllib.request.urlopen(req,context=ctx,timeout=10) as r:
    raw=r.read(2097153);assert len(raw)<=2097152
    return {'status':r.status,'revision':r.headers.get('x-ngfw-revision'),'data':json.loads(raw) if r.headers.get('Content-Type','').startswith('application/json') else None,'bytes':len(raw)}
  except urllib.error.HTTPError as e:return {'status':e.code,'bytes':len(e.read(65536))}
 token=None;result['seed_polls']=[]
 if failed is None:
  login=request('/api/v1/auth/login','POST',{'username':ADMIN_USER,'password':ADMIN_PASSWORD});body=login.get('data') or {};token=body.get('accessToken');result['login']={'status':login['status'],'role':body.get('user',{}).get('role'),'token_received':isinstance(token,str) and len(token)>0}
  if login['status']!=200 or not token or result['login']['role']!='admin':failed='TLS login'
 if failed is None:
  for attempt in range(22):
   cfg=request('/api/v1/config',token=token);events=request('/api/v1/state/events?limit=100',token=token);result['last_observed_config']=cfg;result['last_observed_events']=events
   result['seed_polls'].append({'attempt':attempt,'status':cfg['status'],'revision':cfg.get('revision'),'event_status':events['status']})
   if cfg.get('revision')=='1' and any(x.get('code')=='system.seed-defaults' for x in (events.get('data') or {}).get('items',[])):
    result['running']=cfg;result['events']=events;break
   if attempt!=21:time.sleep(3)
  else:failed='actual system.seed-defaults revision1 not observed'
 if failed is None:
  doc=result['running']['data'];rows={name:row for name,row in doc.get('interfaces',{}).items() if row.get('physical')}
  dp=doc.get('dataplane',{});want=set(EXPECTED_NICS.values());result['seeded17_exact']=set(rows)==set(EXPECTED_NICS) and all(row['physical'].get('pci')==EXPECTED_NICS[name] and row['physical'].get('owner')=='dataplane' and row['physical'].get('builtIn') is True for name,row in rows.items()) and set(dp.get('pciWhitelist',[]))==want and set(dp.get('devices',{}))==want and all(row.get('name')==name for name,pci in EXPECTED_NICS.items() for row in [dp.get('devices',{}).get(pci,{})]) and dp.get('managementPci')==['0000:04:00.0']
  result['candidate']=request('/api/v1/config/candidate',token=token);result['pending']=request('/api/v1/config/commit/pending',token=token);result['system']=request('/api/v1/state/system',token=token);result['interfaces']=request('/api/v1/state/interfaces',token=token);result['dataplane']=request('/api/v1/state/dataplane',token=token)
  result['candidate_equal_running']=result['candidate'].get('data')==doc
  result['no_pending_commit']=(result['pending'].get('data') or {}).get('pending','missing') is None
  result['state_RPC_HTTP_PASS']=all(result[k]['status']==200 for k in ['candidate','pending','system','interfaces','dataplane']) and (result['system'].get('data') or {}).get('agent',{}).get('reachable') is True
  if not result['seeded17_exact'] or not result['candidate_equal_running'] or not result['no_pending_commit'] or not result['state_RPC_HTTP_PASS']:failed='seed/config identity mismatch'
 after=l3();after_inventory=inventory();assert dev.resolve().name=='0000:04:00.0' and (dev/'driver').resolve().name=='igc' and (dev/'iommu_group').resolve().name=='28';result['network_after']=after;result['network_equal']=before==after;result['inventory_after']=after_inventory;result['all17_still_kernel']=before_inventory==after_inventory;result['sysctls_after']=sysctls();result['sysctls_equal']=result['sysctls_after']==sys_before;result['DNS_network_files_after']=DNS_network_files();result['DNS_network_files_equal']=result['DNS_network_files_after']==DNS_before;result['nft_after']=nft();result['foreign_nft_unchanged']=foreign(result['nft_after'])==foreign(nft_before)
 result['unit_states']={u:{k:value(u,k) for k in ['ActiveState','MainPID','NRestarts']} for u in units};result['journal']=run(['journalctl','--no-pager','-u','vpp.service','-u','ngfw-agent.service','-u','ngfw-api.service','-u','nginx.service','-n','200'])
 kernel_after=checked(['dmesg','--color=never']);ioerr_after=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();new_lines=kernel_after.splitlines()[len(kernel_before.splitlines()):] if kernel_after.startswith(kernel_before) else None
 bad=re.compile(r'(EXT4-fs error|Buffer I/O error|I/O error, dev sda|ata\d.*(hard resetting|failed command|error:.*(UNC|ICRC)))',re.I)
 result.update(failure=failed,kernel_before=kernel_before,kernel_after=kernel_after,ioerr_before=ioerr_before,ioerr_after=ioerr_after,new_storage_errors=None if new_lines is None else [x for x in new_lines if bad.search(x)])
 print(json.dumps(result,indent=2))
 success=failed is None and result['sysctls_equal'] and result['DNS_network_files_equal'] and result['foreign_nft_unchanged'] and before==after and before_inventory==after_inventory and ioerr_before==ioerr_after and result['new_storage_errors']==[] and all(s['ActiveState']=='active' and s['NRestarts']=='0' for s in result['unit_states'].values())
 raise SystemExit(0 if success else 2)
'''
def save(p,raw):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--proof',required=True);p.add_argument('--proof-sha256',required=True);p.add_argument('--start-and-seed',action='store_true');a=p.parse_args()
 proof=pathlib.Path(a.proof).resolve();assert proof.parent==PRIVATE and stat.S_IMODE(proof.stat().st_mode)==0o600 and proof.stat().st_uid==0;raw=proof.read_bytes();assert hashlib.sha256(raw).hexdigest()==a.proof_sha256;d=json.loads(raw)
 assert d['firstboot']['exit']==0 and d['network_equal'] and d['safe_initial_noPCI'] and d['owned_table_only'] and d['bootstrap_removed'] and d['guard_error'] is None and d['sysctls_only_expected_nr_change'] and d['new_storage_errors']==[]
 nics=json.loads(pathlib.Path('docs/status/tasks/hardware-211-20261010-pci-groups.json').read_bytes())['physical_nics'];expected={n['netdev']:n['pci'] for n in nics if n['pci']!='0000:04:00.0'};assert len(expected)==17
 credentials={}
 if a.start_and_seed:
  c=PRIVATE/'bootstrap-admin-211.json';assert stat.S_IMODE(c.stat().st_mode)==0o600 and c.stat().st_uid==0;credentials=json.loads(c.read_bytes());assert credentials['host']=='172.30.110.211'
 fields={'APPLY':a.start_and_seed,'PROOF_SHA':a.proof_sha256,'NETWORK_BASELINE':d['network_after'],'SYSCTL_BASELINE':d['sysctls_after'],'EXPECTED_NICS':expected,'API_ENV_SHA':d['files']['/etc/ngfw/api.env']['sha256'],'STARTUP_SHA':d['files']['/etc/vpp/startup.conf']['sha256'],'ADMIN_USER':credentials.get('username',''),'ADMIN_PASSWORD':credentials.get('password','')};code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 r=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');mode='start' if a.start_and_seed else 'inspect'
 out=save(PRIVATE/('initial-runtime-'+mode+'-'+stamp+'.json'),r.stdout);err=save(PRIVATE/('initial-runtime-'+mode+'-'+stamp+'.stderr'),r.stderr)
 print(json.dumps({'SSH_exit':r.returncode,'stdout':out,'stderr':err,'no_startup_apply_driverbind_manualrevision':True}));raise SystemExit(r.returncode)
if __name__=='__main__':main()
