#!/usr/bin/env python3
"""ROOT-only native resource revision after immutable real initial17 seed.

Default is read-only inspect. Worker prepares source; ROOT alone may execute
--commit under a separate actual phase release. No service/driver/startup ops.
"""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import copy,hashlib,json,os,pathlib,re,socket,ssl,stat,subprocess,urllib.error,urllib.request
result={'mode':'commit' if COMMIT else 'inspect','seed1_proof_SHA':SEED_SHA,'compiled_source':'ee20250072938a46407c5ff541e61e1afb7db2d5','API_steps':[],'no_service_startup_driver_network_sysctl_cache_package_or_reboot_operation':True,'stage':'begin'}
def run(a):
 q=subprocess.run(a,capture_output=True,text=True,timeout=30);return {'argv':a,'exit':q.returncode,'stdout':q.stdout,'stderr':q.stderr}
def checked(a):
 q=run(a);assert q['exit']==0,'read-only command failed';return q['stdout']
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def inventory():
 out={}
 for name in INVENTORY:
  p=pathlib.Path('/sys/class/net')/name/'device';g=(p/'iommu_group').resolve();out[name]={'PCI':p.resolve().name,'driver':(p/'driver').resolve().name,'group':g.name,'members':sorted(x.name for x in (g/'devices').iterdir())}
 return out
def nft(d):
 if isinstance(d,dict):return {k:nft({a:b for a,b in v.items() if a not in ['packets','bytes']}) if k=='counter' and isinstance(v,dict) else nft(v) for k,v in d.items() if k!='metainfo'}
 if isinstance(d,list):return [nft(x) for x in d if not(isinstance(x,dict) and 'metainfo' in x)]
 return d
def pool_total(q):
 assert q['exit']==0,'actual buffer query failed'
 rows=re.findall(r'^\s*default-numa-0\s+\d+\s+0\s+\d+\s+\d+\s+(\d+)\s+',q['stdout'],re.M);assert len(rows)==1,'unexpected actual buffer format';return int(rows[0])
def observe():
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service')
 p=pathlib.Path('/sys/class/net/enp4s0/device');assert p.resolve().name=='0000:04:00.0' and (p/'driver').resolve().name=='igc' and (p/'iommu_group').resolve().name=='28'
 assert sorted(x.name for x in (p/'iommu_group/devices').iterdir())==['0000:04:00.0']
 assert hashlib.sha256(pathlib.Path('/usr/sbin/ngfw-agent').read_bytes()).hexdigest()=='a909ae56ecee2431921659d0d9d489a14768fb12a4d71628627659fecd463781'
 packages=checked(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta'])
 assert len(packages.splitlines())==4 and all(x.split('\t')[1:]==['0.1.0~dev+ee2025007293','installed'] for x in packages.splitlines())
 raw=pathlib.Path('/etc/vpp/startup.conf').read_bytes();assert hashlib.sha256(raw).hexdigest()=='367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184' and len(raw)==735
 assert hashlib.sha256(pathlib.Path('/etc/ngfw/api.env').read_bytes()).hexdigest()==API_ENV_SHA
 states={u:{k:checked(['systemctl','show',u,'-p',k,'--value']).strip() for k in ['ActiveState','MainPID','NRestarts']} for u in UNITS}
 assert states==UNITS and all(x['ActiveState']=='active' and x['NRestarts']=='0' for x in states.values())
 network=l3();devices=inventory();sysctls={k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in SYSCTLS};dns={p:{'SHA':hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest(),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in DNS}
 assert network==NETWORK and devices==INVENTORY and sysctls==SYSCTLS and dns==DNS
 assert pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip()=='0x6'
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
 return {'network':network,'inventory':devices,'units':states,'sysctls':sysctls,'DNS':dns,'startup_SHA':hashlib.sha256(raw).hexdigest(),'nft':json.loads(checked(['nft','-j','list','ruleset'])),'storage_ioerr':'0x6','VPP_buffers':run(['vppctl','show','buffers'])}
ctx=ssl.create_default_context(cafile='/etc/ngfw/tls/server.crt')
def request(method,path,token=None,data=None):
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 req=urllib.request.Request('https://localhost'+path,data=None if data is None else json.dumps(data).encode(),headers=headers,method=method)
 try:
  with urllib.request.urlopen(req,context=ctx,timeout=30) as r:
   b=r.read(4194305);assert len(b)<=4194304
   return {'status':r.status,'revision':r.headers.get('x-ngfw-revision'),'data':json.loads(b) if r.headers.get('Content-Type','').startswith('application/json') else None,'bytes':len(b)}
 except urllib.error.HTTPError as e:
  b=e.read(65536);return {'status':e.code,'bytes':len(b),'data':json.loads(b) if e.headers.get('Content-Type','').startswith('application/') else None}
def api(method,path,data=None):
 q=request(method,path,TOKEN,data);result['API_steps'].append({'method':method,'path':path,'response':q});assert q['status']==200,'native API request refused';return q

def main():
 global TOKEN
 result['before']=observe();assert pool_total(result['before']['VPP_buffers'])==16784;kernel=checked(['dmesg','--color=never']);result['kernel_before']=kernel
 login=request('POST','/api/v1/auth/login',data={'username':ADMIN_USER,'password':ADMIN_PASSWORD});body=login.get('data') or {};TOKEN=body.get('accessToken');result['login']={'status':login['status'],'role':body.get('user',{}).get('role'),'token_received':isinstance(TOKEN,str) and bool(TOKEN)};assert login['status']==200 and TOKEN and result['login']['role']=='admin'
 running=api('GET','/api/v1/config');candidate=api('GET','/api/v1/config/candidate');pending=api('GET','/api/v1/config/commit/pending');lock=api('GET','/api/v1/config/lock');rev1=api('GET','/api/v1/config/revisions/1');events=api('GET','/api/v1/state/events?limit=500');state=api('GET','/api/v1/state/system')
 assert state['data']['agent']['reachable'] is True and state['data']['sync']['state']=='in-sync'
 assert running['revision']=='1' and running['data']==SEED_DOCUMENT and candidate['data']==SEED_DOCUMENT and pending['data']=={'pending':None} and lock['data']['locked'] is False
 assert rev1['data']['id']==1 and rev1['data']['kind']=='system' and rev1['data']['payload']==SEED_DOCUMENT
 assert any(e.get('code')=='system.seed-defaults' for e in events['data']['items'])
 assert SEED_DOCUMENT['dataplane'].get('buffersPerNuma') is None,'initial resource field unexpectedly present'
 target=copy.deepcopy(SEED_DOCUMENT);target['dataplane']['buffersPerNuma']=65536
 result['target_document']=target;result['target_document_SHA']=hashlib.sha256(json.dumps(target,separators=(',',':'),sort_keys=True).encode()).hexdigest();result['stage']='seed1-verified'
 if COMMIT:
  api('PATCH','/api/v1/config/dataplane',{'buffersPerNuma':65536});result['stage']='candidate-patched'
  current=api('GET','/api/v1/config/candidate');held=api('GET','/api/v1/config/lock');diff=api('GET','/api/v1/config/diff')
  assert current['data']==target and held['data']['locked'] and held['data']['owner']==ADMIN_USER and held['data']['ownerKeyId'] is None
  changes=diff['data'];assert changes['baseRevision']==1 and len(changes['changes'])==1
  change=changes['changes'][0];assert change['pointer']=='/dataplane/buffersPerNuma' and change['op']=='add' and change['to']==65536
  validated=api('POST','/api/v1/config/validate',{});v=validated['data'];assert v['ok'] is True and v['notApplied']==['dataplane']
  assert all(x['op']=='noop' for x in v['plan']),'supported runtime plan unexpectedly changes objects'
  result['validation_not_enforced_domains']=v['notApplied'];result['stage']='validated-resource-not-enforced'
  commit=api('POST','/api/v1/config/commit?confirm=180&comment=hardware-211-20261010-buffers65536',{});c=commit['data'];assert c['status']=='pending' and c['notApplied']==['dataplane'] and c['txnId'];result['stage']='pending-resource'
  pending=api('GET','/api/v1/config/commit/pending');assert pending['data']['pending']['txnId']==c['txnId']
  # Recheck protected host before committing a native revision. No startup
  # change occurs here; a failed check leaves native confirmed-commit rollback.
  midpoint=observe();assert {k:v for k,v in midpoint.items() if k not in ['nft','VPP_buffers']}=={k:v for k,v in result['before'].items() if k not in ['nft','VPP_buffers']};assert nft(midpoint['nft'])==nft(result['before']['nft'])
  confirmed=api('POST','/api/v1/config/commit/confirm',{});c2=confirmed['data'];assert c2['status']=='confirmed' and c2['txnId']==c['txnId'] and c2['revision']['id']==2 and c2['revision']['parentId']==1 and c2['revision']['kind']=='commit';result['stage']='resource-confirmed'
  final=api('GET','/api/v1/config');candidate=api('GET','/api/v1/config/candidate');pending=api('GET','/api/v1/config/commit/pending');lock=api('GET','/api/v1/config/lock');rev1=api('GET','/api/v1/config/revisions/1');rev2=api('GET','/api/v1/config/revisions/2');events=api('GET','/api/v1/state/events?limit=500');state=api('GET','/api/v1/state/system')
  assert final['revision']=='2' and final['data']==target and candidate['data']==target and pending['data']=={'pending':None} and lock['data']['locked'] is False
  assert rev1['data']['kind']=='system' and rev1['data']['payload']==SEED_DOCUMENT
  assert rev2['data']['id']==2 and rev2['data']['parentId']==1 and rev2['data']['kind']=='commit' and rev2['data']['payload']==target and rev2['data']['txnId']==c['txnId']
  assert any(e.get('code')=='COMMIT_CONFIRMED' and (e.get('data') or {}).get('txnId')==c['txnId'] for e in events['data']['items']),'native confirmation event missing'
  assert state['data']['agent']['reachable'] is True and state['data']['sync']['state']=='in-sync'
  result.update(running=final,candidate=candidate,pending=pending,revision1=rev1,revision2=rev2,events=events,system=state,native_resource2_PASS=True)
 result['after']=observe();assert pool_total(result['after']['VPP_buffers'])==16784;result['current_VPP_pool_total_unchanged']=16784;assert {k:v for k,v in result['before'].items() if k not in ['nft','VPP_buffers']}=={k:v for k,v in result['after'].items() if k not in ['nft','VPP_buffers']};assert nft(result['before']['nft'])==nft(result['after']['nft'])
 after=checked(['dmesg','--color=never']);new=after.splitlines()[len(kernel.splitlines()):] if after.startswith(kernel) else None;bad=re.compile(r'(EXT4-fs error|Buffer I/O error|I/O error, dev sda|ata\d.*(hard resetting|failed command|error:.*\b(UNC|ICRC)\b))',re.I)
 result.update(kernel_after=after,new_storage_errors=None if new is None else [x for x in new if bad.search(x)],protected_unchanged=True,current_startup_unchanged=True,resource_stored_not_runtime_enforced=COMMIT)
 assert result['new_storage_errors']==[];result['PASS']=True;result['stage']='complete'
try:main()
except Exception as e:
 result['PASS']=False;result['failure']={'type':type(e).__name__,'message':str(e)}
finally:print(json.dumps(result,indent=2),flush=True)
raise SystemExit(0 if result.get('PASS') else 2)
'''
def private_read(p):
 p=pathlib.Path(p);assert p.parent==PRIVATE and not p.is_symlink();s=p.stat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 return p.read_bytes()
def save(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 d=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY);os.fsync(d);os.close(d);return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--seed-proof',required=True);p.add_argument('--seed-sha256',required=True);p.add_argument('--commit',action='store_true',help='ROOT-only separately released native confirmed resource commit');a=p.parse_args()
 raw=private_read(a.seed_proof);assert hashlib.sha256(raw).hexdigest()==a.seed_sha256;s=json.loads(raw)
 assert s['mode']=='after' and s['running']['revision']=='1' and all(s[k] for k in ['observation_PASS','seeded17_exact','revision_expectation_PASS','candidate_equal_running','no_pending_commit','required_plugins_loaded','manager_runtime_replaced','state_RPC_HTTP_PASS','network_equal','all17_still_kernel','sysctls_equal','DNS_equal','foreign_nft_unchanged','unit_identity_stable']) and s['new_storage_errors']==[] and s['ioerr_before']==s['ioerr_after']=='0x6'
 creds=json.loads(private_read(PRIVATE/'bootstrap-admin-211.json'));assert creds['host']=='172.30.110.211'
 old=private_read(PRIVATE/'initial-runtime-start-20261010T122029Z.json');assert hashlib.sha256(old).hexdigest()=='98b516a17a02cc82a8b6e26b3ac81d3e1ea8b3cf28fb73b1c0a68bd41b9376fe';baseline=json.loads(old);first=json.loads(private_read(PRIVATE/'firstboot-apply-20261010T120628Z.json'))
 fields={'COMMIT':a.commit,'SEED_SHA':a.seed_sha256,'SEED_DOCUMENT':s['running']['data'],'NETWORK':s['network_after'],'INVENTORY':s['inventory_after'],'SYSCTLS':s['sysctls_after'],'DNS':s['DNS_after'],'UNITS':s['unit_states_after'],'API_ENV_SHA':first['files']['/etc/ngfw/api.env']['sha256'],'ADMIN_USER':creds['username'],'ADMIN_PASSWORD':creds['password']}
 assert baseline['firstboot_proof_SHA']==hashlib.sha256(private_read(PRIVATE/'firstboot-apply-20261010T120628Z.json')).hexdigest()
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 q=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');mode='commit' if a.commit else 'inspect'
 print(json.dumps({'SSH_exit':q.returncode,'stdout':save(PRIVATE/('resource-'+mode+'-'+stamp+'.json'),q.stdout),'stderr':save(PRIVATE/('resource-'+mode+'-'+stamp+'.stderr'),q.stderr),'manager_only_commit':a.commit}));raise SystemExit(q.returncode)
if __name__=='__main__':main()
