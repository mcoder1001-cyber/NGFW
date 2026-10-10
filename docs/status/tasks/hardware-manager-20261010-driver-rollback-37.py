#!/usr/bin/env python3
"""Finite operational .37 driver/startup rollback candidate; ROOT MANAGER ONLY.

Default validates the exact private manifest and current stopped-unit scope.
--restore requires a separately reviewed manager phase; never a worker action.
No driverctl/new_id, management link change, network reload or service start.
"""
import argparse,fcntl,hashlib,json,os,pathlib,re,socket,stat,subprocess,time
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-data-37')
NICS={'enp10s0': ('0000:0a:00.0', 56, 'igc'), 'enp11s0': ('0000:0b:00.0', 57, 'igc'), 'enp13s0': ('0000:0d:00.0', 59, 'igc'), 'enp14s0': ('0000:0e:00.0', 60, 'igc'), 'enp15s0': ('0000:0f:00.0', 61, 'igc'), 'enp16s0': ('0000:10:00.0', 62, 'igc'), 'enp17s0': ('0000:11:00.0', 63, 'igc')}
OWNED={
 '/usr/local/libexec/ngfw-hardware-37-bind':0o755,
 '/etc/systemd/system/ngfw-hardware-37-bind.service':0o644,
 '/etc/systemd/system/vpp.service.d/20-hardware-37-bind.conf':0o644}
ORIGINAL_STARTUP_SHA='c1b121e410961cb64869909a2cd82448984c0472ada11ab0a32234897b86b3c8'
def checked(a):
 q=subprocess.run(a,capture_output=True,text=True,timeout=30)
 if q.returncode:raise RuntimeError('command refused/failed '+repr(a)+' exit '+str(q.returncode))
 return q.stdout
def trusted_parent(p):
 # Every traversed directory is opened without following symlinks. Only
 # root-owned directories without group/other write permissions are trusted.
 p=pathlib.Path(p);assert p.is_absolute();fd=os.open('/',os.O_RDONLY|os.O_DIRECTORY)
 try:
  for component in p.parts[1:]:
   child=os.open(component,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd)
   os.close(fd);fd=child;s=os.fstat(fd);assert s.st_uid==0 and not s.st_mode&0o022
  return fd
 except BaseException:os.close(fd);raise
def regular(p,mode):
 parent=trusted_parent(pathlib.Path(p).parent)
 try:fd=os.open(pathlib.Path(p).name,os.O_RDONLY|os.O_NOFOLLOW,dir_fd=parent)
 finally:os.close(parent)
 with os.fdopen(fd,'rb') as f:
  s=os.fstat(f.fileno());assert stat.S_ISREG(s.st_mode) and s.st_uid==s.st_gid==0 and stat.S_IMODE(s.st_mode)==mode
  assert (os.major(s.st_dev),os.minor(s.st_dev))==(8,2),'record/file must be on actual original root'
  return f.read(),s
def normalized_override(s):return '' if s.strip() in ['', '(null)'] else s.strip()
def syswrite(p,s):
 with open(p,'w') as f:f.write(s+'\n')
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def protected_network(d):
 current=l3();original=d['network_before']
 # The seven address-free data netdevs disappear while VFIO-bound.
 # Ignore only their empty address-map entries; every route/rule and every
 # other interface/address must equal the real prebinding baseline.
 for n in NICS:
  assert current['addresses'].get(n,[])==[] and original['addresses'].get(n)==[]
 current['addresses']={k:v for k,v in current['addresses'].items() if k not in NICS}
 expected=dict(original);expected['addresses']={k:v for k,v in original['addresses'].items() if k not in NICS}
 assert current==expected,'protected network or unexpected interface changed'
 dev=pathlib.Path('/sys/class/net/enp12s0/device')
 assert dev.resolve(strict=True).name=='0000:0c:00.0' and (dev/'driver').resolve(strict=True).name=='igc'
 g=(dev/'iommu_group').resolve(strict=True);assert g.name=='58' and sorted(x.name for x in (g/'devices').iterdir())==['0000:0c:00.0']
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
def validate_nics(d):
 assert set(d['data_nics'])==set(NICS)
 for name,(pci,group,driver) in NICS.items():
  q=d['data_nics'][name];assert (q['PCI'],q['IOMMU'],q['driver'])==(pci,str(group),driver) and q['group_members']==[pci]
  assert q['persistent_override']['type']=='absent' and normalized_override(q['sysfs_override'])==''
  assert not os.path.lexists('/etc/driverctl.d/pci-'+pci),'unknown persistent override'
  p=pathlib.Path('/sys/bus/pci/devices')/pci;g=(p/'iommu_group').resolve(strict=True)
  assert g.name==str(group) and sorted(x.name for x in (g/'devices').iterdir())==[pci]
  drv=(p/'driver').resolve(strict=True).name if (p/'driver').is_symlink() else None
  assert drv in [None,driver,'vfio-pci'] and normalized_override((p/'driver_override').read_text()) in ['',driver,'vfio-pci']
  master=q['bridge_master']
  if master:
   assert master not in NICS and master!='enp12s0' and re.fullmatch(r'br[0-9]+',master)
   assert (pathlib.Path('/sys/class/net')/master/'bridge').is_dir() and d['network_before']['addresses'].get(master)==[]
   assert not any(x.get('dev')==master for x in d['network_before']['routes4']+d['network_before']['routes6'])
def owned_files(d):
 assert set(d['owned_files'])==set(OWNED)
 for name,mode in OWNED.items():
  q=d['owned_files'][name];assert q['before_absent'] is True and q['mode']==mode and re.fullmatch('[0-9a-f]{64}',q['SHA'])
  if os.path.lexists(name):b,s=regular(name,mode);assert hashlib.sha256(b).hexdigest()==q['SHA'],'owned file changed'
def atomic_original(b):
 p=pathlib.Path('/etc/vpp/startup.conf');parent=trusted_parent(p.parent);temp='.startup.conf.hardware37-rollback-'+str(os.getpid())
 try:
  fd=os.open(temp,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600,dir_fd=parent)
  with os.fdopen(fd,'wb') as f:f.write(b);os.fchmod(f.fileno(),0o644);f.flush();os.fsync(f.fileno())
  os.rename(temp,p.name,src_dir_fd=parent,dst_dir_fd=parent);os.fsync(parent)
 finally:os.close(parent)
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--manifest-sha256',required=True);p.add_argument('--restore',action='store_true');a=p.parse_args()
 assert os.geteuid()==0 and (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert re.fullmatch('[0-9a-f]{64}',a.manifest_sha256)
 b,s=regular(RECORD/'manifest.json',0o600);assert hashlib.sha256(b).hexdigest()==a.manifest_sha256;d=json.loads(b)
 assert d['schema']==1 and d['task']=='hardware-37-20261010' and d['root_dev']==[8,2]
 for unit in ['vpp.service','ngfw-agent.service','ngfw-api.service']:assert checked(['systemctl','show',unit,'-p','ActiveState','--value']).strip()=='inactive','units must be stopped by manager before rollback'
 original,s=regular(RECORD/'startup.before',0o600);assert len(original)==735 and hashlib.sha256(original).hexdigest()==ORIGINAL_STARTUP_SHA
 assert re.fullmatch('[0-9a-f]{64}',d['new_startup_SHA'])
 live,s=regular('/etc/vpp/startup.conf',0o644);assert hashlib.sha256(live).hexdigest() in [ORIGINAL_STARTUP_SHA,d['new_startup_SHA']],'startup outside exact transaction'
 protected_network(d);validate_nics(d);owned_files(d)
 if not a.restore:
  print(json.dumps({'rollback_manifest_valid':True,'original_driver_count':7,'no_mutation':True}));return
 # Same lock order as the canonical startupapply; nonblocking refusal avoids
 # racing its still-running run/dead-man/holder. No arbitrary lock release.
 locks=[]
 try:
  for path in ['/run/lock/ngfw-vpp.lock','/run/lock/ngfw-lab.lock']:
   fd=os.open(path,os.O_RDWR|os.O_CREAT|os.O_NOFOLLOW|os.O_CLOEXEC,0o600)
   f=os.fdopen(fd,'a+');locks.append(f);s=os.fstat(f.fileno())
   assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and s.st_nlink==1 and not s.st_mode&0o022
   fcntl.flock(f.fileno(),fcntl.LOCK_EX|fcntl.LOCK_NB)
  protected_network(d);validate_nics(d);owned_files(d)
  for unit in ['vpp.service','ngfw-agent.service','ngfw-api.service']:assert checked(['systemctl','show',unit,'-p','ActiveState','--value']).strip()=='inactive'
  # Remove only the three unchanged transaction-created files; retain the
  # private immutable manifest and every recovery receipt for diagnosis.
  for name,mode in OWNED.items():
   if not os.path.lexists(name):continue
   b,s=regular(name,mode);assert hashlib.sha256(b).hexdigest()==d['owned_files'][name]['SHA'];parent=trusted_parent(pathlib.Path(name).parent)
   try:os.unlink(pathlib.Path(name).name,dir_fd=parent);os.fsync(parent)
   finally:os.close(parent)
  atomic_original(original);checked(['systemctl','daemon-reload'])
  for name,(pci,group,driver) in NICS.items():
   protected_network(d);q=d['data_nics'][name];dev=pathlib.Path('/sys/bus/pci/devices')/pci
   current=(dev/'driver').resolve(strict=True).name if (dev/'driver').is_symlink() else None
   if current!=driver:
    syswrite(dev/'driver_override',driver)
    if current:assert current=='vfio-pci';syswrite(dev/'driver/unbind',pci)
    # Original i40e/igc module must still be present; do not load unknown
    # module options or invoke global dynamic-ID mechanisms during rollback.
    assert (pathlib.Path('/sys/bus/pci/drivers')/driver).is_dir()
    syswrite(pathlib.Path('/sys/bus/pci/drivers_probe'),pci)
   assert (dev/'driver').resolve(strict=True).name==driver
   syswrite(dev/'driver_override',normalized_override(q['sysfs_override']))
   for attempt in range(100):
    link=pathlib.Path('/sys/class/net')/name/'device'
    if link.exists() and link.resolve(strict=True).name==pci:break
    if attempt==99:raise RuntimeError('original predictable netdev did not return: '+name)
    time.sleep(0.1)
   master=pathlib.Path('/sys/class/net')/name/'master';wanted=q['bridge_master']
   actual=master.resolve(strict=True).name if master.is_symlink() else None
   assert actual in [None,wanted],'unknown data master; refusing broad cleanup'
   if wanted and actual!=wanted:checked(['ip','link','set','dev',name,'master',wanted])
   checked(['ip','link','set','dev',name,'up' if 'UP' in q['link'].get('flags',[]) else 'down'])
   assert normalized_override((dev/'driver_override').read_text())==normalized_override(q['sysfs_override'])
   protected_network(d)
  assert l3()==d['network_before'],'whole original L3 did not return'
  assert hashlib.sha256(regular('/etc/vpp/startup.conf',0o644)[0]).hexdigest()==ORIGINAL_STARTUP_SHA
  print(json.dumps({'restored_original_driver_count':7,'restored_original_startup':True,'removed_only_owned_unchanged_files':True,'management_protected':True,'services_remain_stopped':True}))
 finally:
  for f in reversed(locks):f.close()
if __name__=='__main__':main()
