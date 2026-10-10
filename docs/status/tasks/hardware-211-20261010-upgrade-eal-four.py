#!/usr/bin/env python3
"""ROOT-only same-campaign four-package EAL upgrade; explicit per-phase release.

Closed host211/37; default phase selection is explicit. No runtime starts,
firstboot, cache recovery, driver/config/startup apply or reboot. Host37's
separate hold-runtime stops only existing API then agent after native proof.
"""
import argparse,ast,datetime,hashlib,json,os,pathlib,re,shlex,stat,subprocess,tarfile
OUTPUT=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
PRIVATE=OUTPUT/'recovery-private/host-211'
SOURCE='97ae88ee5b6aaf304f78547abed39e80bbeac5e1'
VERSION='0.1.0~dev+97ae88ee5b6a'
OLD_VERSION='0.1.0~dev+ee2025007293'
OLD_AGENT_SHA='a909ae56ecee2431921659d0d9d489a14768fb12a4d71628627659fecd463781'
MANIFEST_SHA='b2f3f6a78014ea76995c783919a375f4398ea4bf7ccbf56bfc32b3ae1472b893'
AGENT_SHA='b3c7cc10671abcab6d11c7dce12323f284c64822a15cfda1c46ba78803283744'
STARTUPGEN_SHA='55e490bad1c302505231df105a1d516cb074678b38b85e28740c399020aac619'
PACKAGES={'ngfw-agent','ngfw-api','ngfw-web','ngfw-meta'}
POLICY=b'#!/bin/sh\nexit 101\n'
UPLOAD_BOOTSTRAP=r'''
import sys
source_stream=sys.stdin.buffer
def read_exact(n):
 out=bytearray()
 while len(out)<n:
  block=source_stream.read(min(n-len(out),65536))
  if not block:raise EOFError('incomplete framed source')
  out.extend(block)
 return bytes(out)
header=read_exact(16)
if any(x not in b'0123456789abcdef' for x in header):raise ValueError('invalid source length header')
length=int(header,16)
if not 0<length<=8*1024**2:raise ValueError('source length outside bound')
source=read_exact(length).decode('utf-8')
# Preserve this same binary stream, including its buffered unread tar bytes.
exec(compile(source,'<controller-upload>','exec'))
'''
REMOTE=r'''
import grp,hashlib,json,os,pathlib,pwd,re,shutil,socket,stat,subprocess,sys,tarfile,time
os.umask(0o077)
MONOTONIC_START=time.monotonic()
TASK='hardware-manager-20261010-eal-upgrade-'+HOST
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery')/TASK
INPUT=pathlib.Path('/run')/('ngfw-eal-upgrade-'+HOST+'-97ae')
POLICY_PATH=pathlib.Path('/usr/sbin/policy-rc.d');MASK=pathlib.Path('/etc/systemd/system/vpp.service')
def run(a,timeout=30):
 q=subprocess.run(a,capture_output=True,text=True,timeout=timeout);return {'argv':a,'exit':q.returncode,'stdout':q.stdout,'stderr':q.stderr}
def checked(a):
 q=run(a);assert q['exit']==0,(a,q['exit']);return q['stdout']
def syncdir(p):
 fd=os.open(p,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
 try:os.fsync(fd)
 finally:os.close(fd)
def fresh_file(p,b,mode=0o600):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,mode)
 os.fchmod(fd,mode)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 syncdir(p.parent)
def metadata(p):
 p=pathlib.Path(p)
 if not os.path.lexists(p):return {'type':'absent'}
 s=p.lstat()
 if p==pathlib.Path('/var/lib/ngfw/secret.key'):
  # Canonical firstboot explicitly owns the 32-byte private key as ngfw.
  assert s.st_uid==pwd.getpwnam('ngfw').pw_uid and s.st_gid==grp.getgrnam('ngfw').gr_gid and stat.S_ISREG(s.st_mode) and stat.S_IMODE(s.st_mode)==0o600 and s.st_size==32,'canonical secret key metadata'
 else:assert s.st_uid==0,str(p)
 d={'uid':s.st_uid,'gid':s.st_gid,'mode':stat.S_IMODE(s.st_mode),'inode':s.st_ino}
 if stat.S_ISLNK(s.st_mode):d.update(type='symlink',link=os.readlink(p))
 else:
  assert stat.S_ISREG(s.st_mode) and not s.st_mode&0o022
  d.update(type='regular',bytes=s.st_size,SHA=hashlib.sha256(p.read_bytes()).hexdigest())
 return d
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def observe(active_pair=False):
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()==BOOT_ID
 assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 assert abs(time.time()-(CONTROLLER_EPOCH+time.monotonic()-MONOTONIC_START))<90
 assert [x.split(':',1)[1].strip() for x in checked(['tune2fs','-l','/dev/sda2']).splitlines() if x.startswith('Filesystem state:')]==['clean']
 d=pathlib.Path('/sys/class/net')/MGMT_IF/'device';g=(d/'iommu_group').resolve(strict=True)
 assert d.resolve(strict=True).name==MGMT_PCI and (d/'driver').resolve(strict=True).name=='igc' and g.name==MGMT_GROUP and sorted(x.name for x in (g/'devices').iterdir())==[MGMT_PCI]
 network=l3();assert network==NETWORK,'complete protected L3 drift'
 data_devices={}
 for name,q in INVENTORY.items():
  p=pathlib.Path('/sys/bus/pci/devices')/q['PCI'];group=(p/'iommu_group').resolve(strict=True)
  driver=(p/'driver').resolve(strict=True).name;members=sorted(x.name for x in (group/'devices').iterdir())
  assert q['PCI']!=MGMT_PCI and group.name!=MGMT_GROUP and group.name==q['group'] and members==q['members']
  override=(p/'driver_override').read_text();persist=metadata('/etc/driverctl.d/pci-'+q['PCI'])
  if HOST=='211':
   assert driver=='vfio-pci' and override.strip()=='vfio-pci' and not os.path.lexists(pathlib.Path('/sys/class/net')/name)
  else:
   assert driver==q['driver']=='igc' and override.strip() in ['', '(null)'] and (pathlib.Path('/sys/class/net')/name/'device').resolve(strict=True).name==q['PCI']
  assert persist=={'type':'absent'}
  data_devices[name]={'PCI':q['PCI'],'driver':driver,'group':group.name,'members':members,'override':override,'persistent_override':persist}
 if HOST=='211':
  assert pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode').read_text().strip()=='N'
  ids=pathlib.Path('/sys/module/vfio_pci/parameters/ids')
  if ids.exists():assert ids.read_text().strip()=='','global VFIO IDs'
  exposed_ids={'exposed':ids.exists(),'status':'empty' if ids.exists() else 'unexposed-not-claimed-empty'}
 else:exposed_ids={'status':'no-module-operation'}
 states={}
 inactive=['ngfw-agent.service','ngfw-api.service','frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','snmpd.service','keepalived.service','chrony.service','rsyslog.service','apply-executor.socket','ngfw-ra-openfile.socket','ngfw-ra-namespace-broker.socket']
 active=['vpp.service','nginx.service','ngfw-firstboot.service','ngfw-firewall-bootstrap.service','nftables.service','postgresql.service','postgresql@18-main.service','valkey-server.service']
 for u in active+inactive:
  states[u]=dict(x.split('=',1) for x in checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts','-p','ActiveEnterTimestampMonotonic']).splitlines())
 assert states['vpp.service']['ActiveState']=='active' and states['vpp.service']['MainPID']==VPP_PID and states['vpp.service']['NRestarts']=='0'
 assert states['nginx.service']['ActiveState']=='active' and states['nginx.service']['MainPID']==NGINX_PID and states['nginx.service']['NRestarts']=='0'
 if active_pair:
  assert HOST=='37' and MODE=='hold-runtime'
  for u,pid in [('ngfw-agent.service','7848'),('ngfw-api.service','7887')]:assert states[u]['ActiveState']=='active' and states[u]['MainPID']==pid and states[u]['NRestarts']=='0'
 else:
  assert all(states[u]['ActiveState']=='inactive' and (u.endswith('.socket') or states[u]['MainPID']=='0') for u in inactive)
 if active_pair:assert all(states[u]['ActiveState']=='inactive' and (u.endswith('.socket') or states[u]['MainPID']=='0') for u in inactive if u not in ['ngfw-agent.service','ngfw-api.service'])
 assert all(states[u]['ActiveState']=='active' for u in active)
 files={p:metadata(p) for p in ['/etc/vpp/startup.conf','/etc/ngfw/api.env','/etc/ngfw/agent.env','/etc/systemd/system/ngfw-api.service.d/10-hardware-seed.conf','/var/lib/ngfw/firstboot-complete','/var/lib/ngfw/secret.key','/etc/ngfw/tls/server.crt','/etc/ngfw/tls/server.key','/etc/resolv.conf','/etc/netplan/90-ngfw-management.yaml','/var/lib/ngfw/agent/auto-block.json']+list(OWNED_FILES)}
 for name,q in OWNED_FILES.items():assert files[name]['SHA']==q['SHA'] and files[name]['mode']==q['mode']
 assert files['/etc/vpp/startup.conf']['SHA']==STARTUP_SHA
 assert files['/etc/ngfw/api.env']['SHA']==API_ENV_SHA and files['/etc/ngfw/api.env']['mode']==0o600
 keys=[x.split('=',1)[0] for x in pathlib.Path('/etc/ngfw/api.env').read_text().splitlines() if x and not x.startswith('#')]
 assert sorted(keys)==['NGFW_DATABASE_URL','NGFW_JWT_SECRET','NGFW_SECRET_KEY_FILE']
 assert pathlib.Path('/var/lib/ngfw/firstboot-complete').read_bytes()==b'completed\n'
 assert not os.path.lexists('/etc/ngfw/bootstrap.env')
 cache=json.loads(pathlib.Path('/var/lib/ngfw/agent/auto-block.json').read_bytes());assert isinstance(cache,dict) and cache.get('owner')=='ngfw','preserve valid owner-bound cache; no recovery in upgrade'
 sysctls={k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in SYSCTLS};assert sysctls==SYSCTLS and sysctls['vm.nr_hugepages']=='1024'
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
 ioerr=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();assert int(ioerr,16)==6
 packages=run(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta'])
 assert packages['exit']==0
 installed={line.split('\t')[0]:line.split('\t')[1:] for line in packages['stdout'].splitlines()}
 assert installed in [{p:[v,'installed'] for p in ['ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']} for v in [OLD_VERSION,VERSION]],'exact native package set'
 agent=metadata('/usr/sbin/ngfw-agent')
 assert agent['SHA']==(OLD_AGENT_SHA if installed['ngfw-agent'][0]==OLD_VERSION else AGENT_SHA),'exact installed native agent'
 wrapper=pathlib.Path('/usr/bin/deb-systemd-invoke');wrapper_bytes=wrapper.read_bytes()
 assert hashlib.sha256(wrapper_bytes).hexdigest()=='92eadae89f4df4cd6088f6316f5390b685faf8d0e312a4e5af3a507caaf81bdb','exact reviewed policy101 all-actions wrapper'
 stop_policy={'SHA':hashlib.sha256(wrapper_bytes).hexdigest(),'package':run(['dpkg-query','-W','-f','${Version}','init-system-helpers']),'all_actions_policy101_review_required':True}
 assert stop_policy['package']['exit']==0 and stop_policy['package']['stdout']=='1.69'
 return {'data_devices':data_devices,'loaded_ids':exposed_ids,'stop_policy':stop_policy,'network':network,'units':states,'files':files,'api_env_keys':keys,'sysctls':sysctls,'DNS_hashes':{p:hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest() for p in ['/etc/resolv.conf','/etc/netplan/90-ngfw-management.yaml']},'storage_ioerr':ioerr,'nft':json.loads(checked(['nft','-j','list','ruleset'])),'native_packages':packages}
def kernel_health(before):
 after=checked(['dmesg','--color=never']);old=set(before.splitlines())
 new=[x for x in after.splitlines() if x not in old]
 errors=[x for x in new if re.search(r'EXT4-fs error|I/O error|Buffer I/O|blk_update_request|\bUNC\b|hard resetting link|failed command|ata\d.*(?:error|reset)|sd\s+\S+.*(?:error|fail)',x,re.I)]
 return {'kernel_before':before,'kernel_after':after,'new_storage_errors':errors,'no_new_storage_errors':not errors}
def nft_semantic(d):
 if isinstance(d,list):return [nft_semantic(x) for x in d]
 if not isinstance(d,dict):return d
 return {k:({a:b for a,b in v.items() if a not in ['packets','bytes']} if k=='counter' and isinstance(v,dict) else nft_semantic(v)) for k,v in d.items()}
def equivalent(a,b):
 # Preserve complete native snapshots while excluding only volatile packet/
 # byte counters from rule comparison. Every tracked file inode/content/mode,
 # firstboot execution time and VPP/nginx PID must remain exact.
 return {k:v for k,v in a.items() if k not in ['nft','native_packages']}=={k:v for k,v in b.items() if k not in ['nft','native_packages']} and nft_semantic(a['nft'])==nft_semantic(b['nft'])
def marker():
 s=RECORD.lstat();assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o700
 p=RECORD/'guards.json';s=p.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600 and (os.major(s.st_dev),os.minor(s.st_dev))==(8,2)
 d=json.loads(p.read_bytes());assert d['task']==TASK and d['baseline_SHA']==BASELINE_SHA
 return d
def guards(d):
 assert metadata(POLICY_PATH)==d['policy'] and metadata(MASK)==d['mask']
 assert POLICY_PATH.read_bytes()==POLICY and MASK.is_symlink() and os.readlink(MASK)=='/dev/null'
 assert checked(['systemctl','show','vpp.service','-p','LoadState','--value']).strip()=='masked'
before=observe(active_pair=MODE=='hold-runtime');kernel_before=checked(['dmesg','--color=never']);result={'mode':MODE,'host':HOST,'manifest_SHA':MANIFEST_SHA,'native_input_proof_SHA':NATIVE_PROOF_SHA,'before':before,'no_firstboot_cache_driver_or_startup_change_requested':True}
# A partial failure still emits its accumulated private receipt; the original
# traceback is retained separately, and no repair/retry is attempted.
original_excepthook=sys.excepthook
def failure_hook(t,v,tb):
 result.update(partial_failure_type=t.__name__);print(json.dumps(result,indent=2));original_excepthook(t,v,tb)
sys.excepthook=failure_hook
if MODE=='hold-runtime':
 assert HOST=='37' and metadata(POLICY_PATH)==metadata(MASK)=={'type':'absent'}
 result['stop_commands']=[]
 for unit in ['ngfw-api.service','ngfw-agent.service']:
  q=run(['systemctl','stop',unit],60);result['stop_commands'].append(q);assert q['exit']==0
 after=observe();a=dict(before);b=dict(after)
 a['units']={k:v for k,v in a['units'].items() if k not in ['ngfw-api.service','ngfw-agent.service']}
 b['units']={k:v for k,v in b['units'].items() if k not in ['ngfw-api.service','ngfw-agent.service']}
 assert equivalent(a,b),'unintended state change during exact runtime hold'
 result.update(after=after,protected_unchanged=True,only_API_agent_stopped=True)
elif MODE=='inspect':
 assert metadata(POLICY_PATH)==metadata(MASK)=={'type':'absent'}
 assert before['native_packages']['exit']==0 and {line.split('\t')[0]:line.split('\t')[1:] for line in before['native_packages']['stdout'].splitlines()}=={p:[OLD_VERSION,'installed'] for p in ['ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']}
 result.update(original_policy=metadata(POLICY_PATH),original_mask=metadata(MASK),read_only=True)
elif MODE=='prepare':
 assert equivalent(before,BASELINE['before']) and BASELINE['original_policy']==BASELINE['original_mask']=={'type':'absent'}
 assert not os.path.lexists(POLICY_PATH) and not os.path.lexists(MASK) and not os.path.lexists(RECORD)
 parent=RECORD.parent;s=parent.lstat();assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o700 and (os.major(s.st_dev),os.minor(s.st_dev))==(8,2)
 RECORD.mkdir(mode=0o700);syncdir(parent)
 # Durable original/owned identities BEFORE either guard rename. The staged
 # files are on each final filesystem so replacement is atomic.
 policy_temp=POLICY_PATH.with_name('.ngfw-eal-'+HOST+'-upgrade-policy');mask_temp=MASK.with_name('.ngfw-eal-'+HOST+'-upgrade-vpp.link')
 assert not os.path.lexists(policy_temp) and not os.path.lexists(mask_temp)
 fresh_file(policy_temp,POLICY,0o755);os.symlink('/dev/null',mask_temp);syncdir(mask_temp.parent)
 d={'task':TASK,'baseline_SHA':BASELINE_SHA,'original_policy':{'type':'absent'},'original_mask':{'type':'absent'},'policy':metadata(policy_temp),'mask':metadata(mask_temp)}
 fresh_file(RECORD/'guards.json',json.dumps(d,sort_keys=True).encode())
 assert not os.path.lexists(POLICY_PATH) and not os.path.lexists(MASK)
 os.rename(policy_temp,POLICY_PATH);syncdir(POLICY_PATH.parent);os.rename(mask_temp,MASK);syncdir(MASK.parent)
 checked(['systemctl','daemon-reload']);guards(d);after=observe();assert equivalent(before,after)
 result.update(after=after,protected_unchanged=True,recovery_record=str(RECORD/'guards.json'),guards_SHA=hashlib.sha256((RECORD/'guards.json').read_bytes()).hexdigest())
elif MODE=='restore':
 assert INSTALL_PROOF['host']==HOST and INSTALL_PROOF['mode']=='install' and INSTALL_PROOF['upgrade_exit']==0 and INSTALL_PROOF['exact_four_configured'] and INSTALL_PROOF['fixed_agent_binary_matches'] and INSTALL_PROOF['fixed_startupgen_binary_matches'] and INSTALL_PROOF['protected_unchanged'] and INSTALL_PROOF['no_new_storage_errors'] and INSTALL_PROOF['manifest_SHA']==MANIFEST_SHA
 d=marker();assert equivalent(before,BASELINE['before'])
 for p,key in [(MASK,'mask'),(POLICY_PATH,'policy')]:
  current=metadata(p);assert current in [{'type':'absent'},d[key]],'unowned safeguard state'
  if current!={'type':'absent'}:p.unlink();syncdir(p.parent)
 checked(['systemctl','daemon-reload']);assert not os.path.lexists(POLICY_PATH) and not os.path.lexists(MASK)
 after=observe();assert equivalent(before,after)
 result.update(after=after,protected_unchanged=True,original_guards_absent=True,recovery_record_retained=True)
elif MODE=='upload':
 d=marker();guards(d);assert equivalent(before,BASELINE['before'])
 assert checked(['findmnt','-no','FSTYPE','/run']).strip()=='tmpfs' and not os.path.lexists(INPUT)
 assert len(ARCHIVES)==4 and sum(x['bytes'] for x in ARCHIVES)<1024**3
 v=os.statvfs('/run');assert v.f_bavail*v.f_frsize>sum(x['bytes'] for x in ARCHIVES)+64*1024**2
 INPUT.mkdir(mode=0o700);syncdir(INPUT.parent);expected={x['file']:x for x in ARCHIVES};seen=set()
 with tarfile.open(fileobj=sys.stdin.buffer,mode='r|') as archive:
  for member in archive:
   assert member.name in expected and member.name not in seen and member.isreg() and member.size==expected[member.name]['bytes']
   seen.add(member.name);p=INPUT/member.name;fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600);h=hashlib.sha256()
   stream=archive.extractfile(member);assert stream is not None;remaining=member.size
   with os.fdopen(fd,'wb') as f:
    while remaining:
     block=stream.read(min(1024**2,remaining));assert block;remaining-=len(block);h.update(block);f.write(block)
    f.flush();os.fsync(f.fileno())
   assert h.hexdigest()==expected[member.name]['sha256']
 assert seen==set(expected);syncdir(INPUT);after=observe();guards(d);assert equivalent(before,after)
 result.update(after=after,protected_unchanged=True,archives_uploaded=4,input_tmpfs=True,no_install=True)
else:
 d=marker();guards(d);assert equivalent(before,BASELINE['before'])
 root=INPUT;s=root.lstat();assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o700 and os.stat(root).st_dev==os.stat('/run').st_dev
 for item in ARCHIVES:
  p=root/item['file'];s=p.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600 and s.st_size==item['bytes'] and hashlib.sha256(p.read_bytes()).hexdigest()==item['sha256']
 assert {p.name for p in root.iterdir()}=={x['file'] for x in ARCHIVES},'unexpected upload directory member'
 assert len(ARCHIVES)==4 and {x['Package'] for x in ARCHIVES}=={'ngfw-agent','ngfw-api','ngfw-web','ngfw-meta'}
 assert all(x['Version']==VERSION for x in ARCHIVES)
 args=['apt-get','--no-remove','--no-install-recommends','--only-upgrade','-o','Dir::Cache::pkgcache=','-o','Dir::Cache::srcpkgcache=','-o','Dpkg::Options::=--force-confold','install']+[str(root/x['file']) for x in ARCHIVES]
 q=run(args[:1]+['-s']+args[1:],180);changes=[x for x in q['stdout'].splitlines() if re.match(r'^(Inst|Remv) ',x)]
 assert q['exit']==0 and len(changes)==4 and {x.split()[1] for x in changes}=={x['Package'] for x in ARCHIVES} and not any(x.startswith('Remv ') for x in changes)
 for line in changes:
  assert OLD_VERSION in line and VERSION in line,'exact old/new upgrade versions required'
 result.update(simulation=q,plan_changes=changes,guards_retained=True)
 if MODE=='install':
  assert changes==PLAN,'actual upgrade plan drift'
  assert before['native_packages']==BASELINE['before']['native_packages'],'native installed package drift'
  # No VPP package is included; retain native skip as bounded transaction env.
  assert shutil.which('needrestart') is None and not pathlib.Path('/usr/lib/needrestart/apt-pinvoke').exists()
  assert not any(p.is_file() and b'needrestart' in p.read_bytes().lower() for p in pathlib.Path('/etc/apt/apt.conf.d').iterdir())
  env=dict(os.environ,DEBIAN_FRONTEND='noninteractive',VPP_INSTALL_SKIP_SYSCTL='1',NEEDRESTART_MODE='l')
  logs={}
  for key in ['stdout','stderr']:
   p=INPUT/('native-upgrade.'+key);fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600);logs[key]=(p,os.fdopen(fd,'w'))
  q=subprocess.run(args[:1]+['-y']+args[1:],env=env,stdout=logs['stdout'][1],stderr=logs['stderr'][1])
  for p,f in logs.values():f.flush();os.fsync(f.fileno());f.close()
  syncdir(INPUT)
  result.update(upgrade_exit=q.returncode,stdout=logs['stdout'][0].read_text(),stderr=logs['stderr'][0].read_text(),dpkg_audit=run(['dpkg','--audit']),packages=run(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']))
  expected={x['Package']:[VERSION,'installed'] for x in ARCHIVES}
  actual={line.split('\t')[0]:line.split('\t')[1:] for line in result['packages']['stdout'].splitlines()}
  result['exact_four_configured']=result['packages']['exit']==0 and actual==expected
  result['installed_agent_binary']=metadata('/usr/sbin/ngfw-agent');result['fixed_agent_binary_matches']=result['installed_agent_binary']['SHA']==AGENT_SHA
  result['installed_startupgen_binary']=metadata('/usr/lib/ngfw/bin/ngfw-startupgen');result['fixed_startupgen_binary_matches']=result['installed_startupgen_binary']['SHA']==STARTUPGEN_SHA
  after=observe();guards(d);result.update(after=after,protected_unchanged=equivalent(before,after),**kernel_health(kernel_before));print(json.dumps(result,indent=2));raise SystemExit(q.returncode if q.returncode else (0 if result['protected_unchanged'] and result['exact_four_configured'] and result['fixed_agent_binary_matches'] and result['fixed_startupgen_binary_matches'] and result['dpkg_audit']['exit']==0 and result['dpkg_audit']['stdout']=='' and result['no_new_storage_errors'] else 2))
result.update(**kernel_health(kernel_before));assert result['no_new_storage_errors']
print(json.dumps(result,indent=2))
'''

def save(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def read_json(p,expected,parent=OUTPUT):
 p=pathlib.Path(p);s=p.lstat();assert p.parent==parent and stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 b=p.read_bytes();assert hashlib.sha256(b).hexdigest()==expected;return json.loads(b)
def spool_info(p):
 s=p.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 b=p.read_bytes();return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser()
 p.add_argument('--host',choices=['211','37'],required=True);p.add_argument('mode',choices=['hold-runtime','inspect','prepare','upload','simulate','install','restore'])
 p.add_argument('--validate-inputs-only',action='store_true',help='controller-only input/control/AST verification; no target contact or ROOT output writes')
 for x in ['baseline','baseline-sha256','manifest','manifest-sha256','plan','plan-sha256','install-proof','install-proof-sha256']:p.add_argument('--'+x)
 a=p.parse_args();assert OUTPUT.lstat().st_uid==0 and stat.S_ISDIR(OUTPUT.lstat().st_mode)
 assert a.mode!='hold-runtime' or a.host=='37'
 if a.host=='211':
  native_sha='b3fcfa09afc1d79bd0c76ca237015fb18bf61802cc88cdc642646c7ff835c8f9'
  prior=read_json(OUTPUT/'manager-resource211-commit-20261010T141048Z.json',native_sha);assert prior['PASS'] and prior['native_resource2_PASS']
  original=read_json(OUTPUT/'manager-physical-record-211-20261010T141803Z.stdout','73fd6e164aa7843d39e7b31cd22b7e06f51e29ad8088368abc6a4402c16d0b0a')
  network=json.loads(json.dumps(prior['after']['network']));inventory={n:{'PCI':q['PCI'],'driver':q['driver'],'group':q['IOMMU'],'members':q['group_members']} for n,q in original['manifest']['data_nics'].items()}
  assert len(inventory)==17
  for n in inventory:assert network['addresses'].pop(n)==[]
  sysctls=prior['after']['sysctls'];owned=original['manifest']['owned_files']
  firstboot=read_json(PRIVATE/'firstboot-apply-20261010T120628Z.json','9134aaf1e3fae358d154bb3af6b69cf3602c1bc0e169fc2fcffa9471930045f8',PRIVATE)
  expected={'host':'172.30.110.211','BOOT_ID':'3a609803-be4e-46eb-bfc6-7dcfe385cf50','MGMT_IF':'enp4s0','MGMT_PCI':'0000:04:00.0','MGMT_GROUP':'28','VPP_PID':'72599','NGINX_PID':'9281','STARTUP_SHA':'367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184'}
 else:
  native_sha='ba5439fcb0a7b2ccce59c1cf3b09f177e76e8de8cb1fe4e75d12840743b659ea'
  prior=read_json(OUTPUT/'manager-host37-initial-runtime-start-20261010T144323Z.json',native_sha);assert prior['native_seed7_PASS'] and prior['seeded7_exact'] and prior['all7_still_kernel']
  network=prior['network_after'];inventory=prior['inventory_after'];sysctls=prior['sysctls_after'];owned={};assert len(inventory)==7
  firstboot=read_json(OUTPUT/'manager-firstboot37-apply-20261010T141814Z.json','1b75f2ee3d1489348b87218396d1f22c30207b967ba22eea22d484f448c8b15a')
  expected={'host':'172.30.126.37','BOOT_ID':'c8d66ea9-afab-4228-a293-00c198745040','MGMT_IF':'enp12s0','MGMT_PCI':'0000:0c:00.0','MGMT_GROUP':'58','VPP_PID':'7820','NGINX_PID':'9669','STARTUP_SHA':'c1b121e410961cb64869909a2cd82448984c0472ada11ab0a32234897b86b3c8'}
 baseline=None;archives=[];plan=None;installed=None;m=None
 if a.mode not in ['inspect','hold-runtime']:
  baseline=read_json(a.baseline,a.baseline_sha256);assert baseline['mode']=='inspect' and baseline['host']==a.host and baseline['read_only'] and baseline['native_input_proof_SHA']==native_sha
 if a.mode in ['upload','simulate','install'] or a.validate_inputs_only and a.manifest:
  m=pathlib.Path(a.manifest);assert a.manifest_sha256==MANIFEST_SHA
  manifest=read_json(m,MANIFEST_SHA,OUTPUT/'runtime-eal-97ae');assert manifest['source_sha']==SOURCE and manifest['version']==VERSION and manifest['agent_binary_sha256']==AGENT_SHA and manifest['startupgen_binary_sha256']==STARTUPGEN_SHA
  assert len(manifest['packages'])==4 and {x['Package'] for x in manifest['packages']}==PACKAGES and len({x['file'] for x in manifest['packages']})==4
  for x in manifest['packages']:
   assert re.fullmatch(r'[A-Za-z0-9.+~_-]+\.deb',x['file']);f=m.parent/x['file'];s=f.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
   b=f.read_bytes();assert hashlib.sha256(b).hexdigest()==x['sha256']
   fields={k:subprocess.check_output(['dpkg-deb','-f',str(f),k],text=True).strip() for k in ['Package','Version','Architecture']}
   assert fields['Package']==x['Package'] and fields['Version']==VERSION and fields['Architecture']==x['Architecture'] and fields['Architecture'] in ['amd64','all']
   archives.append({'file':x['file'],'sha256':x['sha256'],'bytes':len(b),**fields})
 if a.mode=='install':
  q=read_json(a.plan,a.plan_sha256);assert q['mode']=='simulate' and q['host']==a.host and q['manifest_SHA']==MANIFEST_SHA and q['simulation']['exit']==0 and q['no_new_storage_errors'];plan=q['plan_changes']
 if a.mode=='restore':installed=read_json(a.install_proof,a.install_proof_sha256)
 fields={**{k:v for k,v in expected.items() if k!='host'},'MODE':a.mode,'HOST':a.host,'NETWORK':network,'INVENTORY':inventory,'OWNED_FILES':owned,'SYSCTLS':sysctls,'API_ENV_SHA':firstboot['files']['/etc/ngfw/api.env']['sha256'],'POLICY':POLICY,'BASELINE':baseline,'BASELINE_SHA':a.baseline_sha256,'ARCHIVES':archives,'PLAN':plan,'INSTALL_PROOF':installed,'VERSION':VERSION,'OLD_VERSION':OLD_VERSION,'OLD_AGENT_SHA':OLD_AGENT_SHA,'AGENT_SHA':AGENT_SHA,'STARTUPGEN_SHA':STARTUPGEN_SHA,'MANIFEST_SHA':MANIFEST_SHA,'NATIVE_PROOF_SHA':native_sha,'CONTROLLER_EPOCH':datetime.datetime.now(datetime.timezone.utc).timestamp()}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 if a.validate_inputs_only:
  ast.parse(code);ast.parse(UPLOAD_BOOTSTRAP)
  print(json.dumps({'host':a.host,'mode':a.mode,'target_contacted':False,'controller_output_written':False,'generated_AST_PASS':True,'data_devices':len(inventory),'archives_verified':len(archives),'source':SOURCE,'manifest_SHA':MANIFEST_SHA,'native_input_proof_SHA':native_sha,'generated_source_bytes':len(code.encode())}));return
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 paths={key:OUTPUT/('manager-eal-upgrade'+a.host+'-'+a.mode+'-'+stamp+suffix) for key,suffix in [('stdout','.json'),('stderr','.stderr')]}
 ssh=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@'+expected['host']]
 if a.mode=='upload':
  streams={key:os.fdopen(os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600),'wb') for key,path in paths.items()}
  try:
   payload=code.encode('utf-8');assert 0<len(payload)<=8*1024**2
   q=subprocess.Popen(ssh+['python3 -c '+shlex.quote(UPLOAD_BOOTSTRAP)],stdin=subprocess.PIPE,stdout=streams['stdout'],stderr=streams['stderr'])
   try:
    q.stdin.write(format(len(payload),'016x').encode('ascii'));q.stdin.write(payload)
    with tarfile.open(fileobj=q.stdin,mode='w|') as archive:
     for x in archives:
      info=tarfile.TarInfo(x['file']);info.size=x['bytes'];info.mode=0o600
      with (m.parent/x['file']).open('rb') as f:archive.addfile(info,f)
   except BrokenPipeError:pass
   finally:
    try:q.stdin.close()
    except BrokenPipeError:pass
    q.wait()
  finally:
   for stream in streams.values():stream.flush();os.fsync(stream.fileno());stream.close()
   fd=os.open(OUTPUT,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
  receipts={key:spool_info(path) for key,path in paths.items()}
 else:
  q=subprocess.run(ssh+['python3 -'],input=code.encode(),capture_output=True)
  receipts={'stdout':save(paths['stdout'],q.stdout),'stderr':save(paths['stderr'],q.stderr)}
 print(json.dumps({'host':a.host,'phase':a.mode,'SSH_exit':q.returncode,**receipts,'no_runtime_start_firstboot_driver_bind_cache_recovery_or_apply':True}));raise SystemExit(q.returncode)
if __name__=='__main__':main()
