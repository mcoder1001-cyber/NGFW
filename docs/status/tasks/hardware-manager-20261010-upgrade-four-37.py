#!/usr/bin/env python3
"""Prepared four-NGFW native upgrade; explicit parent release before execution.

Inspect is read-only. Prepare records the existing installation safeguards;
install consumes a separately reviewed exact four-package simulation. All
guarded services remain inactive. No firstboot/driver/cache writes.
"""
import argparse,datetime,hashlib,json,os,pathlib,re,shlex,stat,subprocess,tarfile
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37']
SOURCE='ee20250072938a46407c5ff541e61e1afb7db2d5'
VERSION='0.1.0~dev+ee2025007293'
AGENT_SHA='a909ae56ecee2431921659d0d9d489a14768fb12a4d71628627659fecd463781'
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
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-upgrade-37-ee202500')
INPUT=pathlib.Path('/run/ngfw-hardware-manager-37-upgrade-ee202500')
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
def observe():
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 assert abs(time.time()-(CONTROLLER_EPOCH+time.monotonic()-MONOTONIC_START))<90
 assert [x.split(':',1)[1].strip() for x in checked(['tune2fs','-l','/dev/sda2']).splitlines() if x.startswith('Filesystem state:')]==['clean']
 d=pathlib.Path('/sys/class/net/enp12s0/device');g=(d/'iommu_group').resolve(strict=True)
 assert d.resolve(strict=True).name=='0000:0c:00.0' and (d/'driver').resolve(strict=True).name=='igc' and g.name=='58' and sorted(x.name for x in (g/'devices').iterdir())==['0000:0c:00.0']
 network=l3();assert network==NETWORK
 for name,q in INVENTORY.items():
  p=pathlib.Path('/sys/class/net')/name/'device';group=(p/'iommu_group').resolve(strict=True)
  assert p.resolve(strict=True).name==q['PCI'] and (p/'driver').resolve(strict=True).name==q['driver'] and group.name==q['group'] and sorted(x.name for x in (group/'devices').iterdir())==q['members']
 states={}
 inactive=['ngfw-agent.service','ngfw-api.service','frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','snmpd.service','keepalived.service','chrony.service','rsyslog.service','apply-executor.socket','ngfw-ra-openfile.socket','ngfw-ra-namespace-broker.socket']
 active=['vpp.service','nginx.service','ngfw-firstboot.service','ngfw-firewall-bootstrap.service','nftables.service','postgresql.service','postgresql@18-main.service','valkey-server.service']
 for u in active+inactive:
  states[u]=dict(x.split('=',1) for x in checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts','-p','ActiveEnterTimestampMonotonic']).splitlines())
 assert all(states[u]['ActiveState']=='inactive' and (u.endswith('.socket') or (states[u]['MainPID']=='0' and states[u]['NRestarts']=='0')) for u in active+inactive)
 files={p:metadata(p) for p in ['/etc/vpp/startup.conf','/etc/ngfw/api.env','/etc/ngfw/agent.env','/etc/systemd/system/ngfw-api.service.d/10-hardware-seed.conf','/var/lib/ngfw/firstboot-complete','/var/lib/ngfw/secret.key','/etc/ngfw/tls/server.crt','/etc/ngfw/tls/server.key','/etc/resolv.conf','/etc/netplan/90-ngfw-management.yaml','/var/lib/ngfw/agent/auto-block.json']}
 assert files['/etc/vpp/startup.conf']['SHA']=='d96ec489a11e2de6c0ac6beec2134946f1210cfa38a077848b3d057e898bb5be'
 for p in ['/etc/ngfw/api.env','/var/lib/ngfw/firstboot-complete','/var/lib/ngfw/secret.key','/etc/ngfw/tls/server.crt','/etc/ngfw/tls/server.key','/var/lib/ngfw/agent/auto-block.json']:
  assert files[p]=={'type':'absent'},'unexpected initialized runtime'
 assert not os.path.lexists('/etc/ngfw/bootstrap.env')
 keys=[]
 sysctls={k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in SYSCTL_KEYS};assert sysctls['vm.nr_hugepages']=='0'
 assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()=='c8d66ea9-afab-4228-a293-00c198745040'
 assert hashlib.sha256(pathlib.Path('/var/lib/ngfw-install-recovery/hardware-37-20261010/state.json').read_bytes()).hexdigest()=='e275e5061a30bb452c7f24537280394459e6d26e70c6b11cece7ae2db64d3e60'
 assert POLICY_PATH.read_bytes()==POLICY and MASK.is_symlink() and os.readlink(MASK)=='/dev/null'
 assert checked(['systemctl','show','vpp.service','-p','LoadState','--value']).strip()=='masked'
 assert hashlib.sha256(pathlib.Path('/usr/bin/deb-systemd-invoke').read_bytes()).hexdigest()=='92eadae89f4df4cd6088f6316f5390b685faf8d0e312a4e5af3a507caaf81bdb'
 assert checked(['dpkg-query','-W','-f','${Version}','init-system-helpers']).strip()=='1.69'
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
 ioerr=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();assert int(ioerr,16)==6
 packages=run(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta'])
 return {'network':network,'units':states,'files':files,'api_env_keys':keys,'sysctls':sysctls,'DNS_hashes':{p:hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest() for p in ['/etc/resolv.conf','/etc/netplan/90-ngfw-management.yaml']},'storage_ioerr':ioerr,'nft':json.loads(checked(['nft','-j','list','ruleset'])),'native_packages':packages}
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
 d=json.loads(p.read_bytes());assert d['task']=='hardware-manager-20261010-upgrade-37-ee202500' and d['baseline_SHA']==BASELINE_SHA
 return d
def guards(d):
 assert metadata(POLICY_PATH)==d['policy'] and metadata(MASK)==d['mask']
 assert POLICY_PATH.read_bytes()==POLICY and MASK.is_symlink() and os.readlink(MASK)=='/dev/null'
 assert checked(['systemctl','show','vpp.service','-p','LoadState','--value']).strip()=='masked'
before=observe();kernel_before=checked(['dmesg','--color=never']);result={'mode':MODE,'before':before,'no_firstboot_cache_driver_or_startup_change_requested':True}
# A partial failure still emits its accumulated private receipt; the original
# traceback is retained separately, and no repair/retry is attempted.
original_excepthook=sys.excepthook
def failure_hook(t,v,tb):
 result.update(partial_failure_type=t.__name__);print(json.dumps(result,indent=2));original_excepthook(t,v,tb)
sys.excepthook=failure_hook
if MODE=='inspect':
 assert POLICY_PATH.read_bytes()==POLICY and MASK.is_symlink() and os.readlink(MASK)=='/dev/null'
 assert before['native_packages']['exit']==0 and {line.split('\t')[0]:line.split('\t')[1:] for line in before['native_packages']['stdout'].splitlines()}=={p:['0.1.0~dev+2045ab8b3d2f','installed'] for p in ['ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']}
 result.update(original_policy=metadata(POLICY_PATH),original_mask=metadata(MASK),read_only=True)
elif MODE=='prepare':
 assert equivalent(before,BASELINE['before']) and metadata(POLICY_PATH)==BASELINE['original_policy'] and metadata(MASK)==BASELINE['original_mask']
 assert not os.path.lexists(RECORD)
 parent=RECORD.parent;s=parent.lstat();assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o700 and (os.major(s.st_dev),os.minor(s.st_dev))==(8,2)
 RECORD.mkdir(mode=0o700);syncdir(parent)
 d={'task':'hardware-manager-20261010-upgrade-37-ee202500','baseline_SHA':BASELINE_SHA,'policy':metadata(POLICY_PATH),'mask':metadata(MASK),'original_guard_files_preserved':True}
 fresh_file(RECORD/'guards.json',json.dumps(d,indent=2).encode());guards(d)
 after=observe();assert equivalent(before,after)
 result.update(after=after,protected_unchanged=True,guard_record=d,no_guard_mutation=True)
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
  after=observe();guards(d);result.update(after=after,protected_unchanged=equivalent(before,after),**kernel_health(kernel_before));print(json.dumps(result,indent=2));raise SystemExit(q.returncode if q.returncode else (0 if result['protected_unchanged'] and result['exact_four_configured'] and result['fixed_agent_binary_matches'] and result['dpkg_audit']['exit']==0 and result['dpkg_audit']['stdout']=='' and result['no_new_storage_errors'] else 2))
result.update(**kernel_health(kernel_before));assert result['no_new_storage_errors']
print(json.dumps(result,indent=2))
'''
def save(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def private_read(p):
 p=pathlib.Path(p);assert p.parent==PRIVATE and not p.is_symlink() and p.stat().st_uid==0 and stat.S_IMODE(p.stat().st_mode)==0o600
 return p.read_bytes()
def spool_info(p):
 b=private_read(p)
 return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('mode',choices=['inspect','prepare','upload','simulate','install']);p.add_argument('--baseline');p.add_argument('--baseline-sha256');p.add_argument('--manifest');p.add_argument('--manifest-sha256');p.add_argument('--plan');p.add_argument('--plan-sha256');a=p.parse_args()
 original=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37/firstboot-installed-inspect-20261010T121917Z.json')
 raw=original.read_bytes();assert not original.is_symlink() and stat.S_IMODE(original.stat().st_mode)==0o600 and hashlib.sha256(raw).hexdigest()=='6cf4eaed57b65f4f2ee6e999d11c53f67f8ae1240c5386b9665bd28e0620b3ea'
 by={tuple(x['argv']):x['stdout'] for x in json.loads(raw)['commands']}
 network={'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(by[('ip','-j','addr')])},'routes4':json.loads(by[('ip','-j','-4','route','show','table','all')]),'routes6':json.loads(by[('ip','-j','-6','route','show','table','all')]),'rules4':json.loads(by[('ip','-j','-4','rule')]),'rules6':json.loads(by[('ip','-j','-6','rule')])}
 inventory={n:{'PCI':pci,'driver':'igc','group':str(g),'members':[pci]} for n,pci,g in [('enp10s0','0000:0a:00.0',56),('enp11s0','0000:0b:00.0',57),('enp13s0','0000:0d:00.0',59),('enp14s0','0000:0e:00.0',60),('enp15s0','0000:0f:00.0',61),('enp16s0','0000:10:00.0',62),('enp17s0','0000:11:00.0',63)]}
 syskeys=['vm.nr_hugepages','vm.hugetlb_shm_group','kernel.shmmax','net.ipv4.ip_forward','net.ipv6.conf.all.forwarding','net.ipv4.conf.all.rp_filter','net.ipv4.conf.default.rp_filter','net.ipv4.conf.enp12s0.rp_filter','net.ipv4.conf.enp12s0.forwarding','net.ipv4.conf.all.accept_redirects','net.ipv4.conf.enp12s0.accept_redirects','net.ipv6.conf.all.disable_ipv6','net.ipv6.conf.default.disable_ipv6','net.ipv6.conf.enp12s0.disable_ipv6','net.ipv6.conf.all.accept_ra','net.ipv6.conf.enp12s0.accept_ra']
 baseline=None;archives=[];plan=None
 if a.mode!='inspect':
  raw=private_read(a.baseline);assert hashlib.sha256(raw).hexdigest()==a.baseline_sha256;baseline=json.loads(raw);assert baseline['mode']=='inspect' and baseline['read_only']
 if a.mode in ['upload','simulate','install']:
  m=pathlib.Path(a.manifest);assert not m.is_symlink() and m.stat().st_uid==0;raw=m.read_bytes();assert hashlib.sha256(raw).hexdigest()==a.manifest_sha256;j=json.loads(raw)
  assert j['source_sha']==SOURCE and j['version']==VERSION and len(j['packages'])==4 and {x['Package'] for x in j['packages']}==PACKAGES and j['agent_binary_sha256']==AGENT_SHA
  assert len({x['file'] for x in j['packages']})==4
  for x in j['packages']:
   assert re.fullmatch(r'[A-Za-z0-9.+~_-]+\.deb',x['file']);f=m.parent/x['file'];assert f.is_file() and not f.is_symlink();raw=f.read_bytes();assert hashlib.sha256(raw).hexdigest()==x['sha256']
   fields={k:subprocess.check_output(['dpkg-deb','-f',str(f),k],text=True).strip() for k in ['Package','Version','Architecture']};assert fields['Package']==x['Package'] and fields['Version']==VERSION and fields['Architecture'] in ['amd64','all']
   archives.append({'file':x['file'],'sha256':x['sha256'],'bytes':len(raw),**fields})
 if a.mode=='install':
  raw=private_read(a.plan);assert hashlib.sha256(raw).hexdigest()==a.plan_sha256;q=json.loads(raw);assert q['mode']=='simulate' and q['simulation']['exit']==0;plan=q['plan_changes']
 fields={'MODE':a.mode,'NETWORK':network,'INVENTORY':inventory,'SYSCTL_KEYS':syskeys,'POLICY':POLICY,'BASELINE':baseline,'BASELINE_SHA':a.baseline_sha256,'ARCHIVES':archives,'PLAN':plan,'VERSION':VERSION,'AGENT_SHA':AGENT_SHA,'CONTROLLER_EPOCH':datetime.datetime.now(datetime.timezone.utc).timestamp()}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');paths={k:PRIVATE/('manager-upgrade37-'+a.mode+'-'+stamp+s) for k,s in [('stdout','.json'),('stderr','.stderr')]}
 if a.mode=='upload':
  streams={k:os.fdopen(os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600),'wb') for k,p in paths.items()}
  try:
   q=subprocess.Popen(SSH+['python3 -c '+shlex.quote(UPLOAD_BOOTSTRAP)],stdin=subprocess.PIPE,stdout=streams['stdout'],stderr=streams['stderr'])
   try:
    framed=code.encode();q.stdin.write(format(len(framed),'016x').encode());q.stdin.write(framed);q.stdin.flush()
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
   fd=os.open(PRIVATE,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
  receipts={k:spool_info(p) for k,p in paths.items()}
 else:
  q=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);receipts={k:save(p,b) for (k,p),b in zip(paths.items(),[q.stdout,q.stderr])}
 print(json.dumps({'SSH_exit':q.returncode,**receipts,'no_firstboot_service_guard_or_driver_change':True}));raise SystemExit(q.returncode)
if __name__=='__main__':main()
