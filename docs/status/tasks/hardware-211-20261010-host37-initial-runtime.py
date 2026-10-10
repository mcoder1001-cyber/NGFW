#!/usr/bin/env python3
"""Start proved no-PCI runtime and observe the real FIRST API NIC seed.

ROOT-only execution; worker source preparation. Separate parent/reviewer release required. No startup apply, PCI binding,
candidate edit, manual revision, DB shortcut, service restart or reboot.
"""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37']
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
  group=(p/'iommu_group').resolve();members=sorted(x.name for x in (group/'devices').iterdir());assert members==[pci] and group.name!='58'
  out[name]={'PCI':pci,'driver':(p/'driver').resolve().name,'group':group.name,'members':members}
 return out
units=['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']
assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()=='c8d66ea9-afab-4228-a293-00c198745040'
assert hashlib.sha256(pathlib.Path('/usr/sbin/ngfw-agent').read_bytes()).hexdigest()=='a909ae56ecee2431921659d0d9d489a14768fb12a4d71628627659fecd463781'
packages=checked(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-meta','ngfw-web']);assert len(packages.splitlines())==4 and all(x.split('\t')[1:]==['0.1.0~dev+ee2025007293','installed'] for x in packages.splitlines())
assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
assert pathlib.Path('/var/lib/ngfw/firstboot-complete').read_bytes()==b'completed\n' and not os.path.lexists('/etc/ngfw/bootstrap.env')
assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service') and not os.path.lexists('/var/lib/ngfw-install-recovery/hardware-37-20261010/state.json')
assert all(value(u,'ActiveState')=='inactive' for u in units if u!='vpp.service')
if RESUME_PID:
 assert value('vpp.service','ActiveState')=='active' and value('vpp.service','MainPID')==RESUME_PID and value('vpp.service','NRestarts')=='0'
else:assert value('vpp.service','ActiveState')=='inactive'
assert pathlib.Path('/etc/ngfw/agent.env').read_bytes()==b'NGFW_MGMT_IF=enp12s0\nNGFW_MGMT_PCI=0000:0c:00.0\n'
assert pathlib.Path('/etc/systemd/system/ngfw-api.service.d/10-hardware-seed.conf').read_bytes()==b'[Service]\nEnvironment=NGFW_SEED_DEFAULT_NICS=1\n'
assert 'NGFW_SEED_DEFAULT_NICS=1' in value('ngfw-api.service','Environment').split()
assert hashlib.sha256(pathlib.Path('/etc/ngfw/api.env').read_bytes()).hexdigest()==API_ENV_SHA
startup=pathlib.Path('/etc/vpp/startup.conf').read_bytes();assert hashlib.sha256(startup).hexdigest()==STARTUP_SHA
text=startup.decode();assert not any(re.match(r'\s*dev\s+(?!default(?:\s|\{))',x) for x in text.splitlines()) and ('  no-pci\n' in text or 'plugin dpdk_plugin.so { disable }' in text)
assert 'blacklist 0000:0c:00.0' in text or 'plugin dpdk_plugin.so { disable }' in text
assert all('plugin '+n+' { enable }' in text for n in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so'])
assert pathlib.Path('/proc/sys/vm/nr_hugepages').read_text().strip()=='1024'
dev=pathlib.Path('/sys/class/net/enp12s0/device');assert dev.resolve().name=='0000:0c:00.0' and (dev/'driver').resolve().name=='igc' and (dev/'iommu_group').resolve().name=='58'
assert sorted(x.name for x in (dev/'iommu_group/devices').iterdir())==['0000:0c:00.0']
assert {u:value(u,'ActiveState') for u in STATE_BASELINE if u not in units}=={u:q for u,q in STATE_BASELINE.items() if u not in units}
before=l3();assert before==NETWORK_BASELINE;before_inventory=inventory();sys_before=sysctls();assert sys_before==SYSCTL_BASELINE;DNS_before=DNS_network_files();nft_before=nft()
with socket.create_connection(('172.30.126.195',22),timeout=5):pass
kernel_before=checked(['dmesg','--color=never']);ioerr_before=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();assert int(ioerr_before,16)==6
result={'sysctls_before':sys_before,'DNS_network_files_before':DNS_before,'nft_before':nft_before,'firstboot_proof_SHA':PROOF_SHA,'network_before':before,'inventory_before':before_inventory,'preflight':True,'commands':[],'no_manual_revision_driver_binding_or_startup_apply':True,'guard_restore_proof_SHA':RESTORE_SHA,'seed_inputs_proof_SHA':SEED_SHA}
if not APPLY:print(json.dumps(result,indent=2))
else:
 failed=None
 for unit in units:
  if unit=='vpp.service' and RESUME_PID:
   result['resumed_existing_VPP_PID']=RESUME_PID
  else:
   d=run(['systemctl','start',unit]);result['commands'].append(d)
   if d['exit']!=0:failed=unit;break
  if unit=='vpp.service':
   expected_pid=value(unit,'MainPID');deadline=time.monotonic()+45;result['VPP_readiness']=[]
   while True:
    state={'MainPID':value(unit,'MainPID'),'NRestarts':value(unit,'NRestarts'),'ActiveState':value(unit,'ActiveState'),'api_socket':os.path.exists('/run/vpp/api.sock')};result['VPP_readiness'].append(state)
    if state['MainPID']!=expected_pid or state['NRestarts']!='0' or state['ActiveState']!='active':failed='VPP identity changed during readiness';break
    if state['api_socket']:break
    if time.monotonic()>=deadline:failed='VPP API socket readiness deadline';break
    time.sleep(0.5)
   if failed:break
   for args in [['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','version'],['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','bootid']]:
    q=run(args,15);result['commands'].append(q)
    if q['exit']!=0:failed='VPP binary API';break
   if failed:break
  if unit=='ngfw-agent.service':
   deadline=time.monotonic()+45;expected_pid=value(unit,'MainPID');result['agent_readiness']=[]
   while True:
    props={k:value(unit,k) for k in ['ActiveState','MainPID','NRestarts']};ready=props['ActiveState']=='active' and props['NRestarts']=='0' and props['MainPID']==expected_pid and os.path.exists('/run/ngfw/agent.sock');result['agent_readiness'].append(dict(props,socket_present=os.path.exists('/run/ngfw/agent.sock')))
    if props['MainPID']!=expected_pid or props['NRestarts']!='0' or props['ActiveState']!='active':failed='agent identity changed during readiness';break
    if ready:break
    if time.monotonic()>=deadline:failed='agent socket readiness deadline';break
    time.sleep(0.5)
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
  # Type=simple API can return from systemctl before listening. Probe only
  # an unauthenticated protected GET; no repeated administrator logins.
  deadline=time.monotonic()+45;result['HTTPS_readiness']=[]
  while True:
   try:q=request('/api/v1/state/system');state={'status':q['status']}
   except (urllib.error.URLError,TimeoutError,OSError) as e:state={'transport_error':type(e).__name__}
   result['HTTPS_readiness'].append(state)
   if state.get('status') in [200,401,403]:break
   if time.monotonic()>=deadline:failed='HTTPS API readiness deadline';break
   time.sleep(1)
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
  dp=doc.get('dataplane',{});want=set(EXPECTED_NICS.values());result['seeded7_exact']=set(rows)==set(EXPECTED_NICS) and all(row['physical'].get('pci')==EXPECTED_NICS[name] and row['physical'].get('owner')=='dataplane' and row['physical'].get('builtIn') is True for name,row in rows.items()) and set(dp.get('pciWhitelist',[]))==want and set(dp.get('devices',{}))==want and all(row.get('name')==name for name,pci in EXPECTED_NICS.items() for row in [dp.get('devices',{}).get(pci,{})]) and dp.get('managementPci')==['0000:0c:00.0']
  result['candidate']=request('/api/v1/config/candidate',token=token);result['pending']=request('/api/v1/config/commit/pending',token=token);result['system']=request('/api/v1/state/system',token=token);result['interfaces']=request('/api/v1/state/interfaces',token=token);result['dataplane']=request('/api/v1/state/dataplane',token=token)
  result['candidate_equal_running']=result['candidate'].get('data')==doc
  result['no_pending_commit']=(result['pending'].get('data') or {}).get('pending','missing') is None
  result['state_RPC_HTTP_PASS']=all(result[k]['status']==200 for k in ['candidate','pending','system','interfaces','dataplane']) and (result['system'].get('data') or {}).get('agent',{}).get('reachable') is True and (result['system'].get('data') or {}).get('sync',{}).get('state')=='in-sync'
  if not result['seeded7_exact'] or not result['candidate_equal_running'] or not result['no_pending_commit'] or not result['state_RPC_HTTP_PASS']:failed='seed/config identity mismatch'
 after=l3();after_inventory=inventory();assert dev.resolve().name=='0000:0c:00.0' and (dev/'driver').resolve().name=='igc' and (dev/'iommu_group').resolve().name=='58';result['network_after']=after;result['network_equal']=before==after;result['inventory_after']=after_inventory;result['all7_still_kernel']=before_inventory==after_inventory;result['sysctls_after']=sysctls();result['sysctls_equal']=result['sysctls_after']==sys_before;result['DNS_network_files_after']=DNS_network_files();result['DNS_network_files_equal']=result['DNS_network_files_after']==DNS_before;result['nft_after']=nft();result['foreign_nft_unchanged']=foreign(result['nft_after'])==foreign(nft_before)
 result['loaded_plugins']=run(['vppctl','show','plugins']);result['required_plugins_loaded']=result['loaded_plugins']['exit']==0 and all(re.search(r'\b'+re.escape(n)+r'\b',result['loaded_plugins']['stdout']) for n in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so'])
 result['buffer_query']=run(['vppctl','show','buffers']);pools=re.findall(r'^\s*default-numa-0\s+\d+\s+0\s+\d+\s+\d+\s+(\d+)\s+',result['buffer_query']['stdout'],re.M);result['actual_pool_total']=int(pools[0]) if result['buffer_query']['exit']==0 and len(pools)==1 else None
 result['buffer_budget7_PASS']=result['actual_pool_total'] is not None and result['actual_pool_total']>=7*1024
 result['remaining_unit_states']={u:value(u,'ActiveState') for u in STATE_BASELINE if u not in units};result['remaining_units_unchanged']=result['remaining_unit_states']=={u:q for u,q in STATE_BASELINE.items() if u not in units}
 result['unit_states']={u:{k:value(u,k) for k in ['ActiveState','MainPID','NRestarts']} for u in units};result['journal']=run(['journalctl','--no-pager','-u','vpp.service','-u','ngfw-agent.service','-u','ngfw-api.service','-u','nginx.service','-n','200'])
 kernel_after=checked(['dmesg','--color=never']);ioerr_after=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();new_lines=kernel_after.splitlines()[len(kernel_before.splitlines()):] if kernel_after.startswith(kernel_before) else None
 bad=re.compile(r'(EXT4-fs error|Buffer I/O error|I/O error, dev sda|ata\d.*(hard resetting|failed command|error:.*\b(UNC|ICRC)\b))',re.I)
 result.update(failure=failed,kernel_before=kernel_before,kernel_after=kernel_after,ioerr_before=ioerr_before,ioerr_after=ioerr_after,new_storage_errors=None if new_lines is None else [x for x in new_lines if bad.search(x)])
 success=failed is None and result['remaining_units_unchanged'] and result['required_plugins_loaded'] and result['buffer_budget7_PASS'] and result['sysctls_equal'] and result['DNS_network_files_equal'] and result['foreign_nft_unchanged'] and before==after and before_inventory==after_inventory and ioerr_before==ioerr_after and result['new_storage_errors']==[] and all(s['ActiveState']=='active' and s['NRestarts']=='0' for s in result['unit_states'].values())
 result['native_seed7_PASS']=success;print(json.dumps(result,indent=2));raise SystemExit(0 if success else 2)
'''
def save(p,raw):
 assert p.parent==PRIVATE and not p.parent.is_symlink() and p.parent.stat().st_uid==0 and not p.parent.stat().st_mode&0o022
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--proof',required=True);p.add_argument('--proof-sha256',required=True);p.add_argument('--start-and-seed',action='store_true');p.add_argument('--seed-proof',required=True);p.add_argument('--seed-sha256',required=True);p.add_argument('--restore-proof',required=True);p.add_argument('--restore-sha256',required=True);p.add_argument('--resume-from');p.add_argument('--resume-sha256');a=p.parse_args()
 proof=pathlib.Path(a.proof).resolve();assert proof.parent==PRIVATE and not pathlib.Path(a.proof).is_symlink() and stat.S_IMODE(proof.stat().st_mode)==0o600 and proof.stat().st_uid==0;raw=proof.read_bytes();assert hashlib.sha256(raw).hexdigest()==a.proof_sha256=='1b75f2ee3d1489348b87218396d1f22c30207b967ba22eea22d484f448c8b15a';d=json.loads(raw)
 def linked(p,sha):
  p=pathlib.Path(p);assert p.parent==PRIVATE and not p.is_symlink() and p.stat().st_uid==0 and stat.S_IMODE(p.stat().st_mode)==0o600;b=p.read_bytes();assert hashlib.sha256(b).hexdigest()==sha;return json.loads(b)
 seed=linked(a.seed_proof,a.seed_sha256);restored=linked(a.restore_proof,a.restore_sha256);assert seed['mode']=='prepare' and seed['network_equal'] and seed['no_activation_revision_or_binding'] and restored['mode']=='restore' and restored['original_guards_absent'] and restored['network_equal'] and restored['seed_inputs_SHA']==a.seed_sha256 and restored['firstboot_SHA']==a.proof_sha256
 assert d['firstboot']['exit']==0 and d['network_equal'] and d['safe_initial_noPCI'] and d['owned_table_only'] and d['bootstrap_removed'] and d['guard_error'] is None and d['sysctls_only_expected_nr_change'] and d['new_storage_errors']==[]
 expected={'enp'+str(n)+'s0':'0000:'+format(n,'02x')+':00.0' for n in [10,11,13,14,15,16,17]};assert len(expected)==7
 credentials={}
 resume_pid=None
 if a.resume_from:
  q=pathlib.Path(a.resume_from).resolve();assert q.parent==PRIVATE and q.stat().st_uid==0 and stat.S_IMODE(q.stat().st_mode)==0o600
  b=q.read_bytes();assert hashlib.sha256(b).hexdigest()==a.resume_sha256;prior=json.loads(b)
  assert prior['failure']=='VPP binary API' and prior['firstboot_proof_SHA']==a.proof_sha256 and len(prior['commands'])==2 and prior['commands'][0]['argv']==['systemctl','start','vpp.service'] and prior['commands'][0]['exit']==0
  assert prior['network_equal'] and prior['all7_still_kernel'] and prior['sysctls_equal'] and prior['DNS_network_files_equal'] and prior['foreign_nft_unchanged'] and prior['new_storage_errors']==[] and prior['ioerr_before']==prior['ioerr_after']
  states=prior['unit_states'];assert states['vpp.service']['ActiveState']=='active' and states['vpp.service']['NRestarts']=='0' and all(states[u]['ActiveState']=='inactive' for u in ['ngfw-agent.service','ngfw-api.service','nginx.service'])
  resume_pid=states['vpp.service']['MainPID'];assert resume_pid.isdigit() and int(resume_pid)>1
 else:assert a.resume_sha256 is None
 if a.start_and_seed:
  c=PRIVATE/'manager-bootstrap-admin-37.json';assert not c.is_symlink() and stat.S_ISREG(c.stat().st_mode) and stat.S_IMODE(c.stat().st_mode)==0o600 and c.stat().st_uid==0;credentials=json.loads(c.read_bytes());assert credentials['host']=='172.30.126.37'
 fields={'STATE_BASELINE':d['states'],'RESTORE_SHA':a.restore_sha256,'SEED_SHA':a.seed_sha256,'APPLY':a.start_and_seed,'RESUME_PID':resume_pid,'PROOF_SHA':a.proof_sha256,'NETWORK_BASELINE':d['network_after'],'SYSCTL_BASELINE':d['sysctls_after'],'EXPECTED_NICS':expected,'API_ENV_SHA':d['files']['/etc/ngfw/api.env']['sha256'],'STARTUP_SHA':d['files']['/etc/vpp/startup.conf']['sha256'],'ADMIN_USER':credentials.get('username',''),'ADMIN_PASSWORD':credentials.get('password','')};code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 r=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');mode='start' if a.start_and_seed else 'inspect'
 out=save(PRIVATE/('manager-host37-initial-runtime-'+mode+'-'+stamp+'.json'),r.stdout);err=save(PRIVATE/('manager-host37-initial-runtime-'+mode+'-'+stamp+'.stderr'),r.stderr)
 print(json.dumps({'SSH_exit':r.returncode,'stdout':out,'stderr':err,'no_startup_apply_driverbind_manualrevision':True}));raise SystemExit(r.returncode)
if __name__=='__main__':main()
