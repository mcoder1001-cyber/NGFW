#!/usr/bin/env python3
"""Manager-only finite physical binding after a durable offhost original record.

Default inspects. --bind stops only API/agent/VPP, stages exactly three owned
files, executes the pinned finite binder, then starts the old noPCI VPP. A
failure is retained with actual phase evidence; original driver restoration is an
explicit root phase through the already recorded finite rollback helper.
"""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
REMOTE=r'''
import fcntl,hashlib,json,os,pathlib,socket,stat,subprocess
os.umask(0o077)
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-data-37')
result={'mode':'bind' if BIND else 'inspect','offhost_record_SHA':PROOF_SHA,'commands':[],'native_apply_launched':False}
def trusted(p):
 fd=os.open('/',os.O_DIRECTORY)
 try:
  for c in pathlib.Path(p).parts[1:]:
   child=os.open(c,os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd);os.close(fd);fd=child;s=os.fstat(fd);assert s.st_uid==0 and not s.st_mode&0o022
 finally:os.close(fd)
def read(p,mode):
 trusted(p.parent);fd=os.open(p,os.O_RDONLY|os.O_NOFOLLOW)
 with os.fdopen(fd,'rb') as f:
  s=os.fstat(f.fileno());assert stat.S_ISREG(s.st_mode) and s.st_uid==s.st_gid==0 and s.st_nlink==1 and stat.S_IMODE(s.st_mode)==mode and (os.major(s.st_dev),os.minor(s.st_dev))==(8,2);return f.read()
def checked(a,timeout=90):
 q=subprocess.run(a,capture_output=True,text=True,timeout=timeout);result['commands'].append({'argv':a,'exit':q.returncode,'stdout':q.stdout,'stderr':q.stderr});assert q.returncode==0,(a,q.returncode);return q.stdout.strip()
def state(u):return dict(x.split('=',1) for x in checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts']).splitlines())
def network():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def protected():
 now=network();old=MANIFEST['network_before'];names=set(MANIFEST['data_nics'])
 for n in names:assert now['addresses'].get(n,[])==old['addresses'][n]==[]
 now['addresses']={k:v for k,v in now['addresses'].items() if k not in names};old=dict(old);old['addresses']={k:v for k,v in old['addresses'].items() if k not in names};assert now==old
 m=pathlib.Path('/sys/class/net/enp12s0/device');g=(m/'iommu_group').resolve(strict=True);assert m.resolve().name=='0000:0c:00.0' and (m/'driver').resolve().name=='igc' and g.name=='58' and sorted(x.name for x in (g/'devices').iterdir())==['0000:0c:00.0']
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
 assert int(pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip(),16)==6
 assert hashlib.sha256(read(pathlib.Path('/etc/vpp/startup.conf'),0o644)).hexdigest()=='c1b121e410961cb64869909a2cd82448984c0472ada11ab0a32234897b86b3c8'
def fresh(p,b,mode):
 trusted(p.parent);fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,mode);os.fchmod(fd,mode)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
locks=[]
try:
 assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()=='c8d66ea9-afab-4228-a293-00c198745040'
 assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service')
 assert PROOF['record_created'] and PROOF['no_service_driver_or_startup_mutation'] and PROOF['network_equal']
 b=read(RECORD/'manifest.json',0o600);assert hashlib.sha256(b).hexdigest()==PROOF['manifest_SHA'];MANIFEST=json.loads(b);assert MANIFEST==PROOF['manifest']
 assert len(MANIFEST['data_nics'])==7 and MANIFEST['task']=='hardware-37-20261010' and MANIFEST['root_dev']==[8,2]
 before=json.loads(read(RECORD/'before.json',0o600));assert before==PROOF['record_before']
 for key,digest in PROOF['sources_SHA'].items():assert hashlib.sha256(read(RECORD/(key+'.source'),0o600)).hexdigest()==digest
 assert PROOF['sources_SHA']['binder']=='0de00a7e5f34097fe530fb8ae1dab3b3e541826facda704eb7a4c7511b13086d' and PROOF['sources_SHA']['rollback']=='00bed9a07c13f079eb56c384ce392a16b08083c4d101b872995139b50ae185dd'
 for u,old in before['units'].items():assert checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts','-p','UnitFileState','-p','ActiveEnterTimestampMonotonic'])==old
 assert network()==MANIFEST['network_before'];protected()
 links={x['ifname']:x for x in json.loads(checked(['ip','-j','-d','link']))}
 for name,q in MANIFEST['data_nics'].items():
  p=pathlib.Path('/sys/class/net')/name/'device';g=(p/'iommu_group').resolve(strict=True)
  assert links[name]==q['link'] and p.resolve().name==q['PCI'] and (p/'driver').resolve().name==q['driver'] and g.name==q['IOMMU'] and sorted(x.name for x in (g/'devices').iterdir())==q['group_members'] and (p/'driver_override').read_text()==q['sysfs_override']
 for p,q in MANIFEST['owned_files'].items():assert q['before_absent'] and not os.path.lexists(p)
 if BIND:
  for p in ['/run/lock/ngfw-vpp.lock','/run/lock/ngfw-lab.lock']:
   fd=os.open(p,os.O_RDWR|os.O_CREAT|os.O_NOFOLLOW|os.O_CLOEXEC,0o600);s=os.fstat(fd);assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and s.st_nlink==1 and not s.st_mode&0o022;fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB);locks.append(fd)
  protected();result['mutation_started']=True
  for u in ['ngfw-api.service','ngfw-agent.service','vpp.service']:checked(['systemctl','stop',u]);assert state(u)['ActiveState']=='inactive'
  protected();assert state('nginx.service')=={'ActiveState':'active','MainPID':before['expected_unit_PIDs']['nginx.service'],'NRestarts':'0'}
  mapping={'/usr/local/libexec/ngfw-hardware-37-bind':'binder','/etc/systemd/system/ngfw-hardware-37-bind.service':'unit','/etc/systemd/system/vpp.service.d/20-hardware-37-bind.conf':'dropin'}
  assert set(MANIFEST['owned_files'])==set(mapping)
  for target,key in mapping.items():
   p=pathlib.Path(target)
   if not p.parent.exists():trusted(p.parent.parent);p.parent.mkdir(mode=0o755);fd=os.open(p.parent.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
   b=read(RECORD/(key+'.source'),0o600);assert hashlib.sha256(b).hexdigest()==MANIFEST['owned_files'][target]['SHA'];fresh(p,b,MANIFEST['owned_files'][target]['mode'])
  checked(['systemctl','daemon-reload']);checked(['systemctl','start','ngfw-hardware-37-bind.service'],240);assert checked(['systemctl','show','ngfw-hardware-37-bind.service','-p','ActiveState','--value'])=='active'
  protected()
  for name,q in MANIFEST['data_nics'].items():
   p=pathlib.Path('/sys/bus/pci/devices')/q['PCI'];assert (p/'driver').resolve().name=='vfio-pci' and (p/'driver_override').read_text().strip()=='vfio-pci' and stat.S_ISCHR(pathlib.Path('/dev/vfio/'+q['IOMMU']).stat().st_mode)
  checked(['systemctl','start','vpp.service'],120);v=state('vpp.service');assert v['ActiveState']=='active' and v['NRestarts']=='0' and v['MainPID']!=before['expected_unit_PIDs']['vpp.service']
  for u in ['ngfw-agent.service','ngfw-api.service']:assert state(u)['ActiveState']=='inactive'
  checked(['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','version']);checked(['/usr/lib/ngfw/bin/ngfw-vppcheck','--timeout','10s','ifaces','local0']);protected()
  result.update(bound7=True,old_noPCI_VPP_started=v,agent_API_held=True,management_protected=True)
 result['PASS']=True
except Exception as e:result.update(PASS=False,failure={'type':type(e).__name__,'message':str(e)},rollback_required=bool(result.get('mutation_started')))
finally:
 for fd in reversed(locks):os.close(fd)
 print(json.dumps(result,indent=2),flush=True)
raise SystemExit(0 if result.get('PASS') else 2)
'''
def save(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd);return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--record-proof',required=True);p.add_argument('--record-sha',required=True);p.add_argument('--bind',action='store_true');a=p.parse_args();p=pathlib.Path(a.record_proof);assert p.parent==PRIVATE and not p.is_symlink() and p.stat().st_uid==0 and stat.S_IMODE(p.stat().st_mode)==0o600
 b=p.read_bytes();assert hashlib.sha256(b).hexdigest()==a.record_sha;proof=json.loads(b)
 code='BIND='+repr(a.bind)+'\nPROOF_SHA='+repr(a.record_sha)+'\nPROOF='+repr(proof)+'\n'+REMOTE
 q=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37','python3 -'],input=code.encode(),capture_output=True)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');out={'SSH_exit':q.returncode,'bind_requested':a.bind}
 for k,b in [('stdout',q.stdout),('stderr',q.stderr)]:out[k]=save(PRIVATE/('manager-physical-bind-37-'+stamp+'.'+k),b)
 print(json.dumps(out));raise SystemExit(q.returncode)
if __name__=='__main__':main()
