#!/usr/bin/env python3
"""ROOT physical native acceptance; default READONLY, root-only agent/API resume."""
import argparse,datetime,hashlib,json,os,pathlib,re,stat,subprocess
OUTPUT=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
PRIVATE=OUTPUT/'recovery-private/host-211'
REMOTE=r'''
import hashlib,json,os,pathlib,re,socket,ssl,stat,subprocess,time,urllib.error,urllib.request
result={'mode':'resume-runtime' if RESUME else 'observe','committed_proof_SHA':COMMIT_SHA,'seed1_proof_SHA':SEED_SHA,'resource2_proof_SHA':RESOURCE_SHA,'expected_startup_SHA':NEW,'expected_agent_SHA':AGENT_SHA,'expected_package_version':VERSION,'commands':[],'no_startup_driver_cache_network_sysctl_package_DB_or_reboot_operation':True,'stage':'begin'}
if POSTBOOT:result.update(mode='observe-postboot',postboot_proof_SHA=BOOT_SHA,preboot_native_proof_SHA=PREBOOT_SHA,storage_counter_epoch='fresh-boot',expected_boot_id=BOOT['after']['boot_id'])
EXPECTED_BOOT=BOOT['after']['boot_id'] if POSTBOOT else '3a609803-be4e-46eb-bfc6-7dcfe385cf50'
EXPECTED_IOERR=int(BOOT['after']['storage_ioerr'],16) if POSTBOOT else 6
def run(a,timeout=30):
 q=subprocess.run(a,capture_output=True,text=True,timeout=timeout);d={'argv':a,'exit':q.returncode,'stdout':q.stdout,'stderr':q.stderr};result['commands'].append(d);return d
def checked(a,timeout=30):
 q=run(a,timeout);assert q['exit']==0,'command failed '+repr(a);return q['stdout'].strip()
def trusted(p):
 fd=os.open('/',os.O_DIRECTORY)
 try:
  for c in pathlib.Path(p).parts[1:]:
   n=os.open(c,os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd);os.close(fd);fd=n;s=os.fstat(fd);assert s.st_uid==0 and not s.st_mode&0o022
 finally:os.close(fd)
def raw(p,mode=None,limit=134217728):
 p=pathlib.Path(p);trusted(p.parent);fd=os.open(p,os.O_RDONLY|os.O_NOFOLLOW)
 with os.fdopen(fd,'rb') as f:
  s=os.fstat(f.fileno());assert stat.S_ISREG(s.st_mode) and s.st_uid==s.st_gid==0 and s.st_nlink==1 and (os.major(s.st_dev),os.minor(s.st_dev))==(8,2) and (mode is None or stat.S_IMODE(s.st_mode)==mode)
  b=f.read(limit+1);assert len(b)<=limit;return b
def digest(b):return hashlib.sha256(b).hexdigest()
def state(u,timeout=30):return dict(x.split('=',1) for x in checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts'],timeout).splitlines())
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def nft(d):
 if isinstance(d,dict):return {k:nft({a:b for a,b in v.items() if a not in ['packets','bytes']}) if k=='counter' and isinstance(v,dict) else nft(v) for k,v in d.items() if k!='metainfo' and not(POSTBOOT and k=='handle')}
 if isinstance(d,list):return [nft(x) for x in d if not(isinstance(x,dict) and 'metainfo' in x)]
 return d
def protect():
 assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()==EXPECTED_BOOT
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2) and not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service')
 assert digest(raw('/usr/sbin/ngfw-agent',0o755))==AGENT_SHA and digest(raw('/etc/vpp/startup.conf',0o644))==NEW
 packages=checked(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']);assert len(packages.splitlines())==4 and all(x.split('\t')[1:]==[VERSION,'installed'] for x in packages.splitlines())
 now=l3();names=set(RECORD['manifest']['data_nics']);old=RECORD['manifest']['network_before']
 for n in names:assert old['addresses'][n]==[] and now['addresses'].get(n,[])==[]
 actual=dict(now);actual['addresses']={k:v for k,v in now['addresses'].items() if k not in names};expected=dict(old);expected['addresses']={k:v for k,v in old['addresses'].items() if k not in names};assert actual==expected
 p=pathlib.Path('/sys/class/net/enp4s0/device');g=(p/'iommu_group').resolve(strict=True);assert p.resolve().name=='0000:04:00.0' and (p/'driver').resolve().name=='igc' and g.name=='28' and sorted(x.name for x in (g/'devices').iterdir())==['0000:04:00.0']
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
 assert pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode').read_text().strip()=='N'
 ids=pathlib.Path('/sys/module/vfio_pci/parameters/ids');status={'exposed':ids.exists(),'value':ids.read_text().strip() if ids.exists() else None};assert not status['exposed'] or not status['value']
 devices={}
 for name,q in RECORD['manifest']['data_nics'].items():
  p=pathlib.Path('/sys/bus/pci/devices')/q['PCI'];g=(p/'iommu_group').resolve(strict=True);assert (p/'driver').resolve().name=='vfio-pci' and (p/'driver_override').read_text().strip()=='vfio-pci' and g.name==q['IOMMU'] and sorted(x.name for x in (g/'devices').iterdir())==[q['PCI']];devices[name]={'PCI':q['PCI'],'driver':'vfio-pci','group':g.name}
 for p,q in RECORD['manifest']['owned_files'].items():assert digest(raw(p,q['mode']))==q['SHA']
 sysctls={k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in BASELINE['sysctls']};assert sysctls==BASELINE['sysctls']
 dns={p:{'SHA':digest(pathlib.Path(p).read_bytes()),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in BASELINE['DNS']};assert dns==BASELINE['DNS']
 rules=json.loads(checked(['nft','-j','list','ruleset']));assert nft(rules)==nft(BASELINE['nft']) and int(pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip(),16)==EXPECTED_IOERR
 if POSTBOOT:
  for u in ['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']:
   expected={k:BOOT['after']['units'][u][k] for k in ['ActiveState','MainPID','NRestarts']};assert state(u)==expected
 else:assert state('vpp.service')==COMMITTED['stable_VPP'] and state('nginx.service')=={'ActiveState':'active','MainPID':'9281','NRestarts':'0'}
 return {'boot_id':EXPECTED_BOOT,'network':now,'allowed_empty_data_map_removals':sorted(names-set(now['addresses'])),'VFIO_devices':devices,'global_ids':status,'sysctls':sysctls,'DNS':dns,'nft':rules,'ioerr':hex(EXPECTED_IOERR)}
ctx=ssl.create_default_context(cafile='/etc/ngfw/tls/server.crt')
def request(path,method='GET',data=None,token=None,timeout=15):
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 req=urllib.request.Request('https://localhost'+path,data=None if data is None else json.dumps(data).encode(),headers=headers,method=method)
 try:
  with urllib.request.urlopen(req,context=ctx,timeout=timeout) as r:
   b=r.read(4194305);assert len(b)<=4194304;return {'status':r.status,'revision':r.headers.get('x-ngfw-revision'),'data':json.loads(b) if r.headers.get('Content-Type','').startswith('application/json') else None,'bytes':len(b)}
 except urllib.error.HTTPError as e:return {'status':e.code,'bytes':len(e.read(65536))}
def interface_rows(response,expected,rows):
 assert response['status']==200
 items=response['data']['items'];assert isinstance(items,list);mapped={q['name']:q for q in items};assert len(mapped)==len(items),'duplicate native interface name'
 admin={}
 for name,d in expected.items():
  row=rows[name];p=row['physical'];assert p['pci']==d['PCI'] and p['owner']=='dataplane' and p['builtIn'] is True and isinstance(row['enabled'],bool)
  q=mapped[name];assert q['state'] is not None and q['state']['vppName']==name and q.get('awaitingDataplane') is False and q.get('inventoryOnly') is False and q.get('hasPendingChange') is False and q['physical']['pci']==d['PCI'] and q['builtIn'] is True
  assert isinstance(q['state']['adminUp'],bool);admin[name]=q['state']['adminUp']
 return mapped,admin
def vpp_admin_rows(text,expected):
 states={}
 for line in text.splitlines():
  m=re.match(r'^\s*(\S+)\s+\d+\s+(up|down)\s+',line)
  if m:
   name=m.group(1);assert name not in states,'duplicate VPP interface name';states[name]=m.group(2)=='up'
 assert set(expected)<=set(states),'missing VPP interface administrative-state row'
 return {n:states[n] for n in expected}
def main():
 assert COMMITTED['native_committed17_PASS'] and COMMITTED['PASS'] and COMMITTED['new_storage_errors']==[] and COMMITTED['actual_pool_total']>=65536
 assert re.fullmatch(r'/var/lib/ngfw/startup-apply/[0-9]{8}-[0-9]{6}-[0-9]+',COMMITTED['work']);w=pathlib.Path(COMMITTED['work']);trusted(w)
 assert digest(raw(w/'doc.json'))=='ff37ed3cb8fc297915d2d85fbc68614dfcf651405b026d9e807e7b3d9d0d552d' and digest(raw(w/'new.conf'))==NEW and digest(raw(w/'backup.conf'))=='367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184'
 assert (w/'committed').exists() and (w/'installed').exists() and all(not os.path.lexists(w/n) for n in ['rolled-back','console-needed','superseded','deadman-fired']);result['committed_marker']=raw(w/'committed').decode()
 h=hashlib.sha256()
 for n in ['settings','doc.json','gen-args','bin/ngfw-startupgen','bin/ngfw-vppcheck','bin/apply-startup.sh','gate']:h.update(raw(w/n))
 assert h.hexdigest()==raw(w/'plan.sha256').decode().strip()==COMMITTED['plan_SHA'];result['native_plan_SHA']=h.hexdigest();result['before']=protect();kernel=checked(['dmesg','--color=never']);result['kernel_before']=kernel;result['stage']='protected-commit-verified'
 if POSTBOOT:assert kernel.startswith(BOOT['kernel_after']),'fresh-boot kernel baseline rotated or changed'
 units=['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service'];result['unit_states_before']={u:state(u) for u in units}
 if RESUME:
  assert all(result['unit_states_before'][u]['ActiveState']=='inactive' for u in ['ngfw-agent.service','ngfw-api.service'])
  for u in ['ngfw-agent.service','ngfw-api.service']:
   checked(['systemctl','start',u],120);result['stage']=u+'-started';expected=state(u);assert expected['ActiveState']=='active' and expected['NRestarts']=='0' and int(expected['MainPID'])>1
   if u=='ngfw-agent.service':
    deadline=time.monotonic()+45;result['agent_readiness']=[]
    while True:
     now=state(u);assert now==expected;present=os.path.exists('/run/ngfw/agent.sock');result['agent_readiness'].append(dict(now,socket_present=present))
     if present:break
     assert time.monotonic()<deadline,'agent socket not ready';time.sleep(0.5)
 else:assert all(x['ActiveState']=='active' and x['NRestarts']=='0' for x in result['unit_states_before'].values())
 result['unit_states_after_start']={u:state(u) for u in units};assert all(x['ActiveState']=='active' and x['NRestarts']=='0' for x in result['unit_states_after_start'].values())
 deadline=time.monotonic()+45;result['HTTPS_readiness']=[]
 while True:
  try:q=request('/api/v1/state/system');r={'status':q['status']}
  except (urllib.error.URLError,TimeoutError,OSError) as e:r={'transport_error':type(e).__name__}
  result['HTTPS_readiness'].append(r)
  if r.get('status')==401:break
  assert time.monotonic()<deadline,'protected HTTPS endpoint not ready';time.sleep(1)
 login=request('/api/v1/auth/login','POST',{'username':ADMIN_USER,'password':ADMIN_PASSWORD});b=login.get('data') or {};token=b.get('accessToken');result['login']={'status':login['status'],'role':b.get('user',{}).get('role'),'token_received':isinstance(token,str) and bool(token)};assert login['status']==200 and token and result['login']['role']=='admin';result['stage']='authenticated'
 for k,path in [('running','/api/v1/config'),('candidate','/api/v1/config/candidate'),('pending','/api/v1/config/commit/pending'),('revision1','/api/v1/config/revisions/1'),('revision2','/api/v1/config/revisions/2'),('events','/api/v1/state/events?limit=500'),('system','/api/v1/state/system'),('interfaces','/api/v1/state/interfaces'),('dataplane','/api/v1/state/dataplane')]:
  result[k]=request(path,token=token);assert result[k]['status']==200
 assert result['running']['revision']=='2' and result['running']['data']==DOCUMENT and result['candidate']['data']==DOCUMENT and result['pending']['data']=={'pending':None}
 assert result['revision1']['data']['id']==1 and result['revision1']['data']['kind']=='system' and result['revision1']['data']['payload']==SEED_DOCUMENT
 assert result['revision2']['data']['id']==2 and result['revision2']['data']['parentId']==1 and result['revision2']['data']['kind']=='commit' and result['revision2']['data']['payload']==DOCUMENT
 assert any(e.get('code')=='system.seed-defaults' for e in result['events']['data']['items']) and result['system']['data']['agent']['reachable'] is True and result['system']['data']['sync']['state']=='in-sync'
 expected=RECORD['manifest']['data_nics'];rows={n:q for n,q in DOCUMENT['interfaces'].items() if q.get('physical')};assert set(rows)==set(expected)
 desired={n:rows[n]['enabled'] for n in expected};assert all(isinstance(v,bool) for v in desired.values());result['desired_physical_admin']=desired
 deadline=time.monotonic()+45;result['physical_admin_poll']=[]
 while True:
  current={}
  for u in units:
   remaining=deadline-time.monotonic();assert remaining>0,'physical administrative state deadline reached';current[u]=state(u,min(5,remaining))
  assert current==result['unit_states_after_start'],'runtime identity changed while polling administrative state'
  remaining=deadline-time.monotonic();assert remaining>0,'physical administrative state deadline reached'
  response=request('/api/v1/state/interfaces',token=token,timeout=min(15,remaining));result['interfaces']=response;mapped,admin=interface_rows(response,expected,rows)
  result['physical_admin_poll'].append({'status':response['status'],'adminUp':admin,'matches_desired':admin==desired})
  assert time.monotonic()<=deadline,'physical administrative state deadline reached'
  if admin==desired:break
  assert time.monotonic()<deadline,'physical administrative state did not converge';time.sleep(1)
 result['interface_acceptance']={}
 for name,d in expected.items():
  q=mapped[name];result['interface_acceptance'][name]={'PCI':d['PCI'],'desired_enabled':desired[name],'state':q['state'],'link_and_counters_recorded_no_packet_throughput_claim':True}
 result['VPP_interface_table']=checked(['vppctl','show','interface']);result['VPP_physical_admin']=vpp_admin_rows(result['VPP_interface_table'],expected);assert result['VPP_physical_admin']==desired,'VPP administrative state differs from desired document';result['physical_admin_converged']=True
 result['plugins']=checked(['vppctl','show','plugins']);assert all(re.search(r'\b'+re.escape(n)+r'\b',result['plugins']) for n in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so'])
 result['buffers']=checked(['vppctl','show','buffers']);pools=re.findall(r'^\s*default-numa-0\s+\d+\s+0\s+\d+\s+\d+\s+(\d+)\s+',result['buffers'],re.M);assert len(pools)==1 and int(pools[0])>=65536;result['actual_pool_total']=int(pools[0]);total_rx=0;result['hardware']={}
 for name,d in expected.items():
  text=checked(['vppctl','show','hardware-interfaces',name]);result['hardware'][name]=text;m=re.search(r'address\s+([0-9a-f]{4}):([0-9a-f]{2}):([0-9a-f]{2})\.([0-9a-f]{1,2})',text,re.I);assert m and ':'.join(m.group(i).lower() for i in [1,2,3])+'.'+str(int(m.group(4),16))==d['PCI']
  r=re.search(r'rx:\s+queues\s+(\d+)\s+\(max\s+\d+\),\s+desc\s+(\d+)',text);assert r;total_rx+=int(r.group(1))*int(r.group(2))
 assert total_rx<=result['actual_pool_total'];result['total_RX_descriptor_requirement']=total_rx;result['after']=protect();time.sleep(2);result['unit_states_after']={u:state(u) for u in units};assert result['unit_states_after']==result['unit_states_after_start'];result['unit_identity_stable']=True
 after=checked(['dmesg','--color=never']);result['kernel_after']=after;new=after.splitlines()[len(kernel.splitlines()):] if after.startswith(kernel) else None;bad=re.compile(r'(EXT4-fs error|Buffer I/O error|I/O error, dev sda|ata\d.*(hard resetting|failed command|error:.*\b(UNC|ICRC)\b))',re.I);result['new_storage_errors']=None if new is None else [x for x in new if bad.search(x)];assert result['new_storage_errors']==[]
 if POSTBOOT:
  bad_boot=re.compile(r'EXT4-fs error|I/O error|Buffer I/O|blk_update_request|\bUNC\b|hard resetting link|failed command|ata\d.*(?:error|reset)|sd\s+\S+.*(?:error|fail)',re.I);result['fresh_boot_storage_errors']=[x for x in after.splitlines() if bad_boot.search(x)];assert result['fresh_boot_storage_errors']==[];result['native2_physical17_postboot_PASS']=True
 result.update(native2_physical17_PASS=True,stage='complete',PASS=True)
try:main()
except Exception as e:result.update(PASS=False,failure={'type':type(e).__name__,'message':str(e)})
finally:print(json.dumps(result,indent=2),flush=True)
raise SystemExit(0 if result.get('PASS') else 2)
'''
def load(p,parent,sha):
 p=pathlib.Path(p);assert p.parent==parent and not p.is_symlink();s=p.stat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 b=p.read_bytes();assert re.fullmatch('[0-9a-f]{64}',sha) and hashlib.sha256(b).hexdigest()==sha;return json.loads(b)
def save(p,b):
 assert p.parent==OUTPUT and not p.parent.is_symlink() and p.parent.stat().st_uid==0 and not p.parent.stat().st_mode&0o022
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd);return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--commit-proof',required=True);p.add_argument('--commit-sha',required=True);p.add_argument('--expected-startup-sha',required=True);p.add_argument('--expected-agent-sha',required=True);p.add_argument('--expected-version',required=True);p.add_argument('--resume-runtime',action='store_true')
 for key in ['postboot-proof','postboot-sha','preboot-proof','preboot-sha']:p.add_argument('--'+key)
 a=p.parse_args();postboot=bool(a.postboot_proof or a.postboot_sha or a.preboot_proof or a.preboot_sha);boot=preboot=None
 if postboot:
  assert not a.resume_runtime and all([a.postboot_proof,a.postboot_sha,a.preboot_proof,a.preboot_sha]),'postboot requires four immutable inputs and READONLY mode'
  preboot=load(a.preboot_proof,OUTPUT,a.preboot_sha);boot=load(a.postboot_proof,OUTPUT,a.postboot_sha)
  assert preboot['native2_physical17_PASS'] and preboot['physical_admin_converged'] is True and preboot['PASS'] and preboot['stage']=='complete' and preboot['unit_identity_stable'] and preboot['new_storage_errors']==[]
  assert preboot.get('mode')!='observe-postboot' and preboot['committed_proof_SHA']==a.commit_sha and preboot['expected_startup_SHA']==a.expected_startup_sha and preboot['expected_agent_SHA']==a.expected_agent_sha and preboot['expected_package_version']==a.expected_version
  assert preboot['seed1_proof_SHA']=='bc1262d1f2d5b35cb21a7e2ff2b007d6faacc33ab13fe286f684d8f7ca19f138' and preboot['resource2_proof_SHA']=='b3fcfa09afc1d79bd0c76ca237015fb18bf61802cc88cdc642646c7ff835c8f9'
  assert boot['mode']=='observe-boot' and boot['host']=='211' and boot['PASS'] and boot['stage']=='complete' and boot['new_boot_observed'] and boot['storage_counter_epoch']=='fresh-boot' and boot['new_storage_errors']==[] and boot['native_proof_SHA']==a.preboot_sha
  newboot=boot['after']['boot_id'];assert re.fullmatch(r'[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}',newboot) and newboot==boot['before']['boot_id'] and newboot!='3a609803-be4e-46eb-bfc6-7dcfe385cf50'
  assert boot['before']['units']==boot['after']['units'] and len(boot['after']['units'])==21 and boot['after']['startup_SHA']==a.expected_startup_sha
  for u in ['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']:
   q=boot['after']['units'][u];assert q['ActiveState']=='active' and q['NRestarts']=='0' and int(q['MainPID'])>1
  assert all(re.fullmatch(r'(?:0x)?[0-9a-fA-F]+',boot[k]['storage_ioerr']) for k in ['before','after']) and int(boot['before']['storage_ioerr'],16)==int(boot['after']['storage_ioerr'],16)
  assert isinstance(boot['kernel_before'],str) and boot['kernel_before'] and isinstance(boot['kernel_after'],str) and boot['kernel_after'].startswith(boot['kernel_before'])
 for v in [a.expected_startup_sha,a.expected_agent_sha]:assert re.fullmatch('[0-9a-f]{64}',v)
 assert a.expected_startup_sha not in ['b1f977e8e8594f45047c4103c318390bd395cb50078949daa0d0f612572cb179','367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184']
 assert re.fullmatch(r'0\.1\.0~dev\+[0-9a-f]{12}',a.expected_version)
 committed=load(a.commit_proof,OUTPUT,a.commit_sha);assert committed['native_committed17_PASS'] and committed['PASS'] and committed['new_storage_errors']==[] and committed['actual_pool_total']>=65536 and committed['after']['startup_SHA']==a.expected_startup_sha
 record=load(OUTPUT/'manager-physical-record-211-20261010T141803Z.stdout',OUTPUT,'73fd6e164aa7843d39e7b31cd22b7e06f51e29ad8088368abc6a4402c16d0b0a')
 resource=load(OUTPUT/'manager-resource211-commit-20261010T141048Z.json',OUTPUT,'b3fcfa09afc1d79bd0c76ca237015fb18bf61802cc88cdc642646c7ff835c8f9');assert resource['native_resource2_PASS'] and resource['PASS']
 seed=load(PRIVATE/'native-seed-after-20261010T135602Z.json',PRIVATE,'bc1262d1f2d5b35cb21a7e2ff2b007d6faacc33ab13fe286f684d8f7ca19f138');assert seed['observation_PASS'] and seed['seeded17_exact']
 cpath=PRIVATE/'bootstrap-admin-211.json';assert not cpath.is_symlink() and cpath.stat().st_uid==0 and stat.S_IMODE(cpath.stat().st_mode)==0o600;c=json.loads(cpath.read_bytes());assert c['host']=='172.30.110.211'
 fields={'RESUME':a.resume_runtime,'POSTBOOT':postboot,'BOOT':boot,'BOOT_SHA':a.postboot_sha,'PREBOOT_SHA':a.preboot_sha,'COMMITTED':committed,'COMMIT_SHA':a.commit_sha,'NEW':a.expected_startup_sha,'AGENT_SHA':a.expected_agent_sha,'VERSION':a.expected_version,'RECORD':record,'BASELINE':resource['after'],'DOCUMENT':resource['running']['data'],'SEED_DOCUMENT':seed['running']['data'],'SEED_SHA':'bc1262d1f2d5b35cb21a7e2ff2b007d6faacc33ab13fe286f684d8f7ca19f138','RESOURCE_SHA':'b3fcfa09afc1d79bd0c76ca237015fb18bf61802cc88cdc642646c7ff835c8f9','ADMIN_USER':c['username'],'ADMIN_PASSWORD':c['password']}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE;q=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211','python3 -'],input=code.encode(),capture_output=True)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');mode='postboot' if postboot else 'resume' if a.resume_runtime else 'observe';print(json.dumps({'SSH_exit':q.returncode,'ROOT_only_resume':a.resume_runtime,'postboot_readonly':postboot,'stdout':save(OUTPUT/('manager-physical-native211-'+mode+'-'+stamp+'.json'),q.stdout),'stderr':save(OUTPUT/('manager-physical-native211-'+mode+'-'+stamp+'.stderr'),q.stderr)}));raise SystemExit(q.returncode)
if __name__=='__main__':main()
