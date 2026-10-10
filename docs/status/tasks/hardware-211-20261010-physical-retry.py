#!/usr/bin/env python3
"""ROOT-only corrected EAL retry: inspect/observe read-only; explicit stage/launch."""
import argparse,ast,datetime,hashlib,json,os,pathlib,re,stat,subprocess
OUTPUT=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
ROLLBACK_SHA='d1ca0e834fef7d00c265ff64e4f1c6a42c834d96eecbe9bbdeb4db21f82afbaa'
REMOTE=r'''
import fcntl,hashlib,json,os,pathlib,re,socket,stat,subprocess,time
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-data-211')
OLD='367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184'
RETRY=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-data-211-retry97ae')
FAILED='b1f977e8e8594f45047c4103c318390bd395cb50078949daa0d0f612572cb179'
GENERATOR='55e490bad1c302505231df105a1d516cb074678b38b85e28740c399020aac619'
NEW=None
DOC='ff37ed3cb8fc297915d2d85fbc68614dfcf651405b026d9e807e7b3d9d0d552d'
result={'mode':'observe' if WORK else 'launch' if LAUNCH else 'stage' if STAGE else 'inspect','record_proof_SHA':RECORD_SHA,'bind_proof_SHA':BIND_SHA,'upgrade_proof_SHA':UPGRADE_SHA,'commands':[],'native_apply_launched':False,'no_worker_apply':True}
def trusted(p):
 fd=os.open('/',os.O_DIRECTORY)
 try:
  for c in pathlib.Path(p).parts[1:]:
   n=os.open(c,os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd);os.close(fd);fd=n;s=os.fstat(fd);assert s.st_uid==0 and not s.st_mode&0o022
 finally:os.close(fd)
def read(p,mode=None,limit=134217728):
 trusted(p.parent);fd=os.open(p,os.O_RDONLY|os.O_NOFOLLOW)
 with os.fdopen(fd,'rb') as f:
  s=os.fstat(f.fileno());assert stat.S_ISREG(s.st_mode) and s.st_uid==s.st_gid==0 and s.st_nlink==1 and (os.major(s.st_dev),os.minor(s.st_dev))==(8,2)
  assert mode is None or stat.S_IMODE(s.st_mode)==mode
  b=f.read(limit+1);assert len(b)<=limit;return b
def syncdir(p):
 fd=os.open(p,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
 try:os.fsync(fd)
 finally:os.close(fd)
def fresh(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 syncdir(p.parent)
def digest(b):return hashlib.sha256(b).hexdigest()
def run(a,timeout=30,input=None,pass_fds=()):
 q=subprocess.run(a,input=input,capture_output=True,text=True,timeout=timeout,pass_fds=pass_fds);d={'argv':a,'exit':q.returncode,'stdout':q.stdout,'stderr':q.stderr};result['commands'].append(d);return d
def checked(a,timeout=30,input=None):
 q=run(a,timeout,input);assert q['exit']==0,'command failed '+repr(a);return q['stdout'].strip()
def state(u):return dict(x.split('=',1) for x in checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts']).splitlines())
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def nft(d):
 if isinstance(d,dict):return {k:nft({a:b for a,b in v.items() if a not in ['packets','bytes']}) if k=='counter' and isinstance(v,dict) else nft(v) for k,v in d.items() if k!='metainfo'}
 if isinstance(d,list):return [nft(x) for x in d if not(isinstance(x,dict) and 'metainfo' in x)]
 return d
def protected(startup):
 assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()=='3a609803-be4e-46eb-bfc6-7dcfe385cf50'
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 now=l3();old=MANIFEST['network_before'];names=set(MANIFEST['data_nics'])
 for n in names:assert old['addresses'][n]==[] and now['addresses'].get(n,[])==[]
 actual=dict(now);actual['addresses']={k:v for k,v in now['addresses'].items() if k not in names};expected=dict(old);expected['addresses']={k:v for k,v in old['addresses'].items() if k not in names};assert actual==expected,'unexpected management/L3 delta'
 p=pathlib.Path('/sys/class/net/enp4s0/device');g=(p/'iommu_group').resolve(strict=True);assert p.resolve().name=='0000:04:00.0' and (p/'driver').resolve().name=='igc' and g.name=='28' and sorted(x.name for x in (g/'devices').iterdir())==['0000:04:00.0']
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
 assert pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode').read_text().strip()=='N'
 ids=pathlib.Path('/sys/module/vfio_pci/parameters/ids');status={'exposed':ids.exists(),'value':ids.read_text().strip() if ids.exists() else None};assert not status['exposed'] or not status['value']
 devices={}
 for name,q in MANIFEST['data_nics'].items():
  p=pathlib.Path('/sys/bus/pci/devices')/q['PCI'];g=(p/'iommu_group').resolve(strict=True);assert (p/'driver').resolve().name=='vfio-pci' and (p/'driver_override').read_text().strip()=='vfio-pci' and g.name==q['IOMMU'] and sorted(x.name for x in (g/'devices').iterdir())==[q['PCI']] and stat.S_ISCHR(pathlib.Path('/dev/vfio/'+q['IOMMU']).stat().st_mode);devices[name]={'PCI':q['PCI'],'driver':'vfio-pci','group':g.name}
 assert digest(read(pathlib.Path('/etc/vpp/startup.conf'),0o644))==startup
 # Configuration/cache/crypto inodes and bytes remain exactly as the native
 # upgrade-restoration proof, except the intentionally replaced startup.
 for name,q in UPGRADE['after']['files'].items():
  if name=='/etc/vpp/startup.conf':continue
  p=pathlib.Path(name);s=p.lstat();actual={'uid':s.st_uid,'gid':s.st_gid,'mode':stat.S_IMODE(s.st_mode),'inode':s.st_ino}
  if stat.S_ISLNK(s.st_mode):actual.update(type='symlink',link=os.readlink(p))
  else:
   assert stat.S_ISREG(s.st_mode);actual.update(type='regular',bytes=s.st_size,SHA=digest(p.read_bytes()))
  assert actual==q,'tracked cache/config/crypto identity drift '+name
 for name,q in UPGRADE['after']['units'].items():
  if name=='vpp.service':continue
  actual=dict(x.split('=',1) for x in checked(['systemctl','show',name,'-p','ActiveState','-p','MainPID','-p','NRestarts','-p','ActiveEnterTimestampMonotonic']).splitlines());assert actual==q,'non-VPP unit identity drift '+name
 assert pathlib.Path('/proc/sys/vm/nr_hugepages').read_text().strip()=='1024' and int(pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip(),16)==6
 assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service')
 for u in ['ngfw-agent.service','ngfw-api.service']:assert state(u)['ActiveState']=='inactive'
 assert state('nginx.service')=={'ActiveState':'active','MainPID':'9281','NRestarts':'0'}
 assert checked(['systemctl','show','ngfw-hardware-211-bind.service','-p','ActiveState','--value'])=='active'
 for p,q in MANIFEST['owned_files'].items():assert digest(read(pathlib.Path(p),q['mode']))==q['SHA']
 sysctls={k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in BASELINE['sysctls']};assert sysctls==BASELINE['sysctls']
 dns={p:{'SHA':digest(pathlib.Path(p).read_bytes()),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in BASELINE['DNS']};assert dns==BASELINE['DNS']
 rules=json.loads(checked(['nft','-j','list','ruleset']));assert nft(rules)==nft(BASELINE['nft'])
 return {'sysctls':sysctls,'DNS':dns,'nft':rules,'network':now,'allowed_empty_data_map_removals':sorted(names-set(now['addresses'])),'VFIO_devices':devices,'global_ids':status,'storage_ioerr':'0x6','startup_SHA':startup}
def seal(w):
 h=hashlib.sha256()
 for n in ['settings','doc.json','gen-args','bin/ngfw-startupgen','bin/ngfw-vppcheck','bin/apply-startup.sh','gate']:h.update(read(w/n))
 actual=read(w/'plan.sha256').decode().strip();assert re.fullmatch('[0-9a-f]{64}',actual) and h.hexdigest()==actual;return actual
def workpath(s):
 assert re.fullmatch(r'/var/lib/ngfw/startup-apply/[0-9]{8}-[0-9]{6}-[0-9]+',s);w=pathlib.Path(s);trusted(w);return w
try:
 assert PROOF['record_created'] and PROOF['network_equal'] and PROOF['no_service_driver_or_startup_mutation']
 assert UPGRADE['mode']=='restore' and UPGRADE['host']=='211' and UPGRADE['protected_unchanged'] and UPGRADE['no_new_storage_errors'] and UPGRADE['original_guards_absent'] and UPGRADE['recovery_record_retained']
 assert UPGRADE['manifest_SHA']=='b2f3f6a78014ea76995c783919a375f4398ea4bf7ccbf56bfc32b3ae1472b893'
 packages=checked(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']);assert {x.split('\t')[0]:x.split('\t')[1:] for x in packages.splitlines()}=={p:['0.1.0~dev+97ae88ee5b6a','installed'] for p in ['ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']}
 assert digest(read(pathlib.Path('/usr/sbin/ngfw-agent'),0o755))=='b3c7cc10671abcab6d11c7dce12323f284c64822a15cfda1c46ba78803283744' and digest(read(pathlib.Path('/usr/lib/ngfw/bin/ngfw-startupgen'),0o755))==GENERATOR
 assert FAILURE['markers']['rolled-back'] is not None and FAILURE['markers']['committed'] is None
 assert BIND['PASS'] and BIND['bound17'] and BIND['agent_API_held'] and BIND['management_protected'] and BIND['offhost_record_SHA']==RECORD_SHA and not BIND['native_apply_launched']
 b=read(RECORD/'manifest.json',0o600);assert digest(b)==PROOF['manifest_SHA'];MANIFEST=json.loads(b);assert MANIFEST==PROOF['manifest'] and len(MANIFEST['data_nics'])==17 and MANIFEST['task']=='hardware-211-20261010' and MANIFEST['root_dev']==[8,2]
 assert json.loads(read(RECORD/'before.json',0o600))==PROOF['record_before']
 for k,v in PROOF['sources_SHA'].items():assert digest(read(RECORD/(k+'.source'),0o600))==v
 for name,size,sha in [('startup.before',735,OLD),('physical.doc.json',6930,DOC),('physical.rendered.conf',1462,FAILED)]:
  b=read(RECORD/name,0o600);assert len(b)==size and digest(b)==sha
 failed_work=pathlib.Path('/var/lib/ngfw/startup-apply/20261010-142900-71835')
 assert os.path.lexists(failed_work/'rolled-back') and not os.path.lexists(failed_work/'committed') and not os.path.lexists(failed_work/'console-needed')
 kernel=checked(['dmesg','--color=never']);result['kernel_before']=kernel
 if WORK or LAUNCH:
  assert RETRY_PROOF['record_created'] and RETRY_PROOF['no_service_driver_or_startup_mutation'] and RETRY_PROOF['upgrade_proof_SHA']==UPGRADE_SHA
  raw=read(RETRY/'manifest.json',0o600);assert digest(raw)==RETRY_PROOF['retry_manifest_SHA'];supp=json.loads(raw);assert supp==RETRY_PROOF['supplemental_manifest']
  assert supp['original_manifest_SHA']==PROOF['manifest_SHA'] and supp['document_SHA']==DOC and supp['generator_SHA']==GENERATOR and supp['original_startup_SHA']==OLD and supp['failed_render_SHA']==FAILED
  NEW=supp['new_render_SHA'];assert re.fullmatch('[0-9a-f]{64}',NEW) and NEW not in [OLD,FAILED]
  for name,size,sha in [('physical.doc.json',6930,DOC),('startup.before',735,OLD),('physical.rendered.conf',supp['new_render_bytes'],NEW),('rollback.source',len(ROLLBACK_SOURCE),ROLLBACK_SHA)]:
   raw=read(RETRY/name,0o600);assert len(raw)==size and digest(raw)==sha
 else:
  assert state('vpp.service')=={'ActiveState':'active','MainPID':'72599','NRestarts':'0'}
  result['before']=protected(OLD)
  rendered=run(['/usr/lib/ngfw/bin/ngfw-startupgen','--current','/etc/vpp/startup.conf','--mgmt-if','enp4s0','--mgmt-pci','0000:04:00.0','-'],input=read((RETRY if LAUNCH or WORK else RECORD)/'physical.doc.json',0o600).decode());assert rendered['exit']==0
  NEW=digest(rendered['stdout'].encode());result['render']=rendered;result['new_render_SHA']=NEW;result['new_render_bytes']=len(rendered['stdout'].encode());assert NEW not in [OLD,FAILED]
 text=read(RETRY/'physical.rendered.conf',0o600).decode() if WORK or LAUNCH else rendered['stdout']
 pcis=re.findall(r'^\s*dev\s+(0000:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7])\s*\{',text,re.M);assert pcis==sorted(q['PCI'] for q in MANIFEST['data_nics'].values()) and len(pcis)==17 and '0000:04:00.0' not in pcis
 assert not re.search(r'\bblacklist\b',text) and re.search(r'buffers-per-numa\s+65536\b',text)
 assert all(re.search(re.escape(n)+r'\s*\{\s*enable\s*\}',text) for n in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so'])
 result['closed_allow17_no_blacklist']=True;result['new_render_SHA']=NEW;result['document_SHA']=DOC
 if WORK:
  w=workpath(WORK);result['work']=str(w);result['plan_SHA']=seal(w)
  assert digest(read(w/'doc.json'))==DOC and digest(read(w/'new.conf'))==NEW and digest(read(w/'backup.conf'))==OLD
  unit=read(w/'run-unit').decode().strip();assert unit=='ngfw-startup-apply-'+w.name;result['native_unit']=unit
  result['markers']={n:read(w/n).decode() if os.path.lexists(w/n) else None for n in ['committed','rolled-back','console-needed','superseded','deadman-fired','installed']}
  result['native_log']=read(w/'log').decode();result['identity_reads']=read(w/'ident.reads').decode();assert len(result['identity_reads'].splitlines())>=2
  assert result['markers']['committed'] is not None and result['markers']['installed'] is not None and all(result['markers'][n] is None for n in ['rolled-back','console-needed','superseded','deadman-fired'])
  result['before']=protected(NEW);v=state('vpp.service');assert v['ActiveState']=='active' and v['NRestarts']=='0' and v['MainPID']!='72599'
  checked(['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','version']);checked(['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','bootid']);checked(['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','ifaces']+sorted(MANIFEST['data_nics']))
  plugins=checked(['vppctl','show','plugins']);assert all(re.search(r'\b'+re.escape(n)+r'\b',plugins) for n in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so'])
  buffers=checked(['vppctl','show','buffers']);totals=re.findall(r'^\s*default-numa-0\s+\d+\s+0\s+\d+\s+\d+\s+(\d+)\s+',buffers,re.M);assert len(totals)==1 and int(totals[0])>=65536;result['actual_pool_total']=int(totals[0])
  result['hardware']={n:checked(['vppctl','show','hardware-interfaces',n]) for n in sorted(MANIFEST['data_nics'])}
  for n,q in MANIFEST['data_nics'].items():
   text=result['hardware'][n];m=re.search(r'address\s+([0-9a-f]{4}):([0-9a-f]{2}):([0-9a-f]{2})\.([0-9a-f]{1,2})',text,re.I);assert m and ':'.join(m.group(i).lower() for i in [1,2,3])+'.'+str(int(m.group(4),16))==q['PCI']
  timer=read(w/'deadman-unit').decode().strip();assert re.fullmatch(r'ngfw-startup-apply-deadman-[0-9]{8}-[0-9]{6}-[0-9]+',timer)
  t=run(['systemctl','show',timer+'.timer','-p','LoadState','-p','ActiveState','-p','Result']);props=dict(x.split('=',1) for x in t['stdout'].splitlines() if '=' in x);assert props.get('ActiveState')=='inactive','dead-man timer not observed inactive'
  time.sleep(2);assert state('vpp.service')==v;result['stable_VPP']=v;result['after']=protected(NEW)
  terminal=dict(x.split('=',1) for x in checked(['systemctl','show',unit,'-p','LoadState','-p','ActiveState','-p','Result','-p','ExecMainStatus']).splitlines())
  result['native_terminal_unit']=terminal
  assert terminal.get('ActiveState')=='inactive' and terminal.get('Result')=='success' and terminal.get('ExecMainStatus')=='0','native run not observed successfully terminal'
  # Existing files only, no create or write; exclusive nonblocking flock
  # proves that the native holder/run/dead-man released both canonical locks.
  locks=[];result['canonical_lock_checks']=[]
  try:
   for name in ['/run/lock/ngfw-vpp.lock','/run/lock/ngfw-lab.lock']:
    trusted(pathlib.Path(name).parent);fd=os.open(name,os.O_RDONLY|os.O_NOFOLLOW|os.O_CLOEXEC);locks.append(fd);s=os.fstat(fd)
    assert stat.S_ISREG(s.st_mode) and s.st_uid==s.st_gid==0 and s.st_nlink==1 and not s.st_mode&0o022
    fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
    result['canonical_lock_checks'].append({'path':name,'dev':s.st_dev,'inode':s.st_ino,'mode':stat.S_IMODE(s.st_mode),'exclusive_nonblocking_acquired':True,'no_file_create_or_write':True})
   result['canonical_locks_released']=True
  finally:
   for fd in reversed(locks):os.close(fd)
  result['native_committed17_PASS']=True
 else:
  result['before']=protected(OLD);assert state('vpp.service')=={'ActiveState':'active','MainPID':'72599','NRestarts':'0'}
  checked(['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','version']);checked(['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','bootid']);checked(['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','ifaces','local0'])
  rendered=run(['/usr/lib/ngfw/bin/ngfw-startupgen','--current','/etc/vpp/startup.conf','--mgmt-if','enp4s0','--mgmt-pci','0000:04:00.0','-'],input=read((RETRY if LAUNCH or WORK else RECORD)/'physical.doc.json',0o600).decode());assert rendered['exit']==0 and digest(rendered['stdout'].encode())==NEW
  docfd=os.memfd_create('ngfw-retry-document',os.MFD_CLOEXEC);os.write(docfd,read((RETRY if LAUNCH else RECORD)/'physical.doc.json',0o600));os.lseek(docfd,0,os.SEEK_SET)
  argv=['/usr/lib/ngfw/apply-startup.sh','--mode','product','--doc',str(RETRY/'physical.doc.json') if LAUNCH else '/proc/'+str(os.getpid())+'/fd/'+str(docfd),'--approve-rendering',NEW,'--expect-sha256',OLD,'--expect-new-sha256',NEW,'--mgmt-if','enp4s0','--mgmt-peer','172.30.126.195','--mgmt-probe','tcp:172.30.126.195:22']
  result['dryrun']=run(argv,120);os.close(docfd);assert result['dryrun']['exit']==0;result['after_dryrun']=protected(OLD);assert state('vpp.service')=={'ActiveState':'active','MainPID':'72599','NRestarts':'0'}
  if STAGE:
   assert not os.path.lexists(RETRY);trusted(RETRY.parent);RETRY.mkdir(mode=0o700);syncdir(RETRY.parent)
   supp={'schema':1,'task':'hardware-211-20261010-retry97ae','source':'97ae88ee5b6aaf304f78547abed39e80bbeac5e1','version':'0.1.0~dev+97ae88ee5b6a','original_manifest_SHA':PROOF['manifest_SHA'],'original_record_proof_SHA':RECORD_SHA,'failed_transaction_proof_SHA':FAILURE_SHA,'upgrade_proof_SHA':UPGRADE_SHA,'original_startup_SHA':OLD,'failed_render_SHA':FAILED,'document_SHA':DOC,'generator_SHA':GENERATOR,'new_render_SHA':NEW,'new_render_bytes':len(rendered['stdout'].encode()),'rollback_source_SHA':ROLLBACK_SHA,'before':result['before']}
   for name,b in [('startup.before',read(RECORD/'startup.before',0o600)),('physical.doc.json',read(RECORD/'physical.doc.json',0o600)),('physical.rendered.conf',rendered['stdout'].encode()),('rollback.source',ROLLBACK_SOURCE),('manifest.json',json.dumps(supp,sort_keys=True).encode())]:fresh(RETRY/name,b)
   result.update(record_created=True,no_service_driver_or_startup_mutation=True,retry_record=str(RETRY),supplemental_manifest=supp,retry_manifest_SHA=digest(read(RETRY/'manifest.json',0o600)),immutable_original_manifest_retained=True)
  if LAUNCH:
   result['launch_started']=True;launch=run(argv+['--apply'],120);result['launch']=launch;assert launch['exit']==0
   matches=re.findall(r'started detached unit (ngfw-startup-apply-([0-9]{8}-[0-9]{6}-[0-9]+)) ',launch['stdout']);assert len(matches)==1
   unit,stamp=matches[0];w=workpath('/var/lib/ngfw/startup-apply/'+stamp);result['native_unit']=unit;result['work']=str(w);assert read(w/'run-unit').decode().strip()==unit and digest(read(w/'doc.json'))==DOC
   result['plan_SHA']=seal(w);result['native_apply_launched']=True;result['COMMITTED_not_yet_observed']=True
 result['kernel_after']=checked(['dmesg','--color=never']);after=result['kernel_after'];window=LAUNCH_KERNEL if WORK else kernel;result['kernel_window_source']='immutable-launch-proof' if WORK else 'current-phase';new=after.splitlines()[len(window.splitlines()):] if after.startswith(window) else None;bad=re.compile(r'(EXT4-fs error|Buffer I/O error|I/O error, dev sda|ata\d.*(hard resetting|failed command|error:.*\b(UNC|ICRC)\b))',re.I);result['new_storage_errors']=None if new is None else [x for x in new if bad.search(x)];assert result['new_storage_errors']==[];result['PASS']=True
except Exception as e:result.update(PASS=False,failure={'type':type(e).__name__,'message':str(e)},explicit_finite_driver_rollback_required_if_terminal_failure=bool(LAUNCH or WORK))
finally:print(json.dumps(result,indent=2),flush=True)
raise SystemExit(0 if result.get('PASS') else 2)
'''
def private(p,sha):
 p=pathlib.Path(p);assert p.parent==OUTPUT and not p.is_symlink();s=p.stat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 b=p.read_bytes();assert re.fullmatch('[0-9a-f]{64}',sha) and hashlib.sha256(b).hexdigest()==sha;return json.loads(b)
def save(p,b):
 assert p.parent==OUTPUT and not p.parent.is_symlink() and p.parent.stat().st_uid==0 and not p.parent.stat().st_mode&0o022
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd);return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--upgrade-proof',required=True);p.add_argument('--upgrade-sha',required=True)
 for key in ['retry-proof','retry-sha','launch-proof','launch-sha']:p.add_argument('--'+key)
 g=p.add_mutually_exclusive_group();g.add_argument('--stage',action='store_true');g.add_argument('--launch',action='store_true');g.add_argument('--observe-work')
 p.add_argument('--validate-inputs-only',action='store_true');a=p.parse_args()
 record_sha='73fd6e164aa7843d39e7b31cd22b7e06f51e29ad8088368abc6a4402c16d0b0a';bind_sha='c4ed7d5cfde4131bd62f6c85048eb501d7975f8e5dd201bb5596bc0086b1f1ca';failure_sha='f64ad15bd241d4bbed2e3219900b6836138dcaf13be17ce2f916df2bc9559de1'
 record=private(OUTPUT/'manager-physical-record-211-20261010T141803Z.stdout',record_sha);bound=private(OUTPUT/'manager-physical-bind-211-20261010T142004Z.stdout',bind_sha);failure=private(OUTPUT/'manager-physical-failure211-20261010T143031Z.stdout',failure_sha)
 upgrade=private(a.upgrade_proof,a.upgrade_sha);assert upgrade['mode']=='restore' and upgrade['host']=='211' and upgrade['original_guards_absent'] and upgrade['protected_unchanged'] and upgrade['no_new_storage_errors']
 resource=private(OUTPUT/'manager-resource211-commit-20261010T141048Z.json','b3fcfa09afc1d79bd0c76ca237015fb18bf61802cc88cdc642646c7ff835c8f9');assert resource['native_resource2_PASS'] and resource['PASS'] and resource['running']['revision']=='2'
 rollback=pathlib.Path(__file__).with_name('hardware-211-20261010-driver-rollback-retry.py').read_bytes();assert hashlib.sha256(rollback).hexdigest()==ROLLBACK_SHA
 retry=None;launch=None
 if a.launch or a.observe_work:
  retry=private(a.retry_proof,a.retry_sha);assert retry['mode']=='stage' and retry['record_created'] and retry['PASS'] and retry['new_storage_errors']==[] and retry['upgrade_proof_SHA']==a.upgrade_sha
 if a.observe_work:
  launch=private(a.launch_proof,a.launch_sha);assert launch['native_apply_launched'] and launch['PASS'] and launch['work']==a.observe_work and launch['upgrade_proof_SHA']==a.upgrade_sha and launch['new_render_SHA']==retry['new_render_SHA']
 fields={'BASELINE':resource['after'],'PROOF':record,'BIND':bound,'FAILURE':failure,'FAILURE_SHA':failure_sha,'UPGRADE':upgrade,'UPGRADE_SHA':a.upgrade_sha,'RECORD_SHA':record_sha,'BIND_SHA':bind_sha,'ROLLBACK_SOURCE':rollback,'ROLLBACK_SHA':ROLLBACK_SHA,'RETRY_PROOF':retry,'LAUNCH_KERNEL':launch['kernel_before'] if launch else None,'LAUNCH':a.launch,'STAGE':a.stage,'WORK':a.observe_work};code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 if a.validate_inputs_only:
  ast.parse(code);print(json.dumps({'controller_only':True,'target_contacted':False,'generated_AST_PASS':True,'original_manifest_SHA':record['manifest_SHA'],'rollback_source_SHA':ROLLBACK_SHA,'upgrade_SHA':a.upgrade_sha}));return
 q=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211','python3 -'],input=code.encode(),capture_output=True)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');mode='observe' if a.observe_work else 'launch' if a.launch else 'stage' if a.stage else 'inspect';out={'SSH_exit':q.returncode,'ROOT_only_launch':a.launch,'ROOT_only_stage':a.stage}
 for k,b in [('stdout',q.stdout),('stderr',q.stderr)]:out[k]=save(OUTPUT/('manager-physical-retry211-'+mode+'-'+stamp+'.'+k),b)
 print(json.dumps(out));raise SystemExit(q.returncode)
if __name__=='__main__':main()
