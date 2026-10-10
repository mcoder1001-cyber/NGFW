#!/usr/bin/env python3
"""Finite corrected-render retry rollback adapter; ROOT MANAGER ONLY.

Default validates the exact private manifest and current stopped-unit scope.
--restore requires a separately reviewed manager phase; never a worker action.
No driverctl/new_id, management link change, network reload or service start.
"""
import argparse,fcntl,hashlib,json,os,pathlib,re,socket,stat,subprocess,time
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-data-211')
RETRY=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-data-211-retry97ae')
ORIGINAL_MANIFEST_SHA='d9659df3792964c3a385eecaa2d1d0bb0f8e014dd34916d8f725625a44dc510e'
FAILED_RENDER_SHA='b1f977e8e8594f45047c4103c318390bd395cb50078949daa0d0f612572cb179'
DOC_SHA='ff37ed3cb8fc297915d2d85fbc68614dfcf651405b026d9e807e7b3d9d0d552d'
GENERATOR_SHA='55e490bad1c302505231df105a1d516cb074678b38b85e28740c399020aac619'
NICS={
 'enp1s0f0np0':('0000:01:00.0',16,'i40e'),'enp1s0f1np1':('0000:01:00.1',17,'i40e'),
 'enp1s0f2np2':('0000:01:00.2',18,'i40e'),'enp1s0f3np3':('0000:01:00.3',19,'i40e'),
 'enp2s0f0np0':('0000:02:00.0',20,'i40e'),'enp2s0f1np1':('0000:02:00.1',21,'i40e'),
 'enp2s0f2np2':('0000:02:00.2',22,'i40e'),'enp2s0f3np3':('0000:02:00.3',23,'i40e'),
 'enp3s0f0np0':('0000:03:00.0',24,'i40e'),'enp3s0f1np1':('0000:03:00.1',25,'i40e'),
 'enp3s0f2np2':('0000:03:00.2',26,'i40e'),'enp3s0f3np3':('0000:03:00.3',27,'i40e'),
 'enp5s0':('0000:05:00.0',29,'igc'),'enp6s0':('0000:06:00.0',30,'igc'),
 'enp7s0':('0000:07:00.0',31,'igc'),'enp8s0':('0000:08:00.0',32,'igc'),
 'enp9s0':('0000:09:00.0',33,'igc')}
OWNED={
 '/usr/local/libexec/ngfw-hardware-211-bind':0o755,
 '/etc/systemd/system/ngfw-hardware-211-bind.service':0o644,
 '/etc/systemd/system/vpp.service.d/20-hardware-211-bind.conf':0o644}
ORIGINAL_STARTUP_SHA='367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184'
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
 # The seventeen address-free data netdevs disappear while VFIO-bound.
 # Ignore only their empty address-map entries; every route/rule and every
 # other interface/address must equal the real prebinding baseline.
 for n in NICS:
  assert current['addresses'].get(n,[])==[] and original['addresses'].get(n)==[]
 current['addresses']={k:v for k,v in current['addresses'].items() if k not in NICS}
 expected=dict(original);expected['addresses']={k:v for k,v in original['addresses'].items() if k not in NICS}
 assert current==expected,'protected network or unexpected interface changed'
 dev=pathlib.Path('/sys/class/net/enp4s0/device')
 assert dev.resolve(strict=True).name=='0000:04:00.0' and (dev/'driver').resolve(strict=True).name=='igc'
 g=(dev/'iommu_group').resolve(strict=True);assert g.name=='28' and sorted(x.name for x in (g/'devices').iterdir())==['0000:04:00.0']
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
   assert master not in NICS and master!='enp4s0' and re.fullmatch(r'br[0-9]+',master)
   assert (pathlib.Path('/sys/class/net')/master/'bridge').is_dir() and d['network_before']['addresses'].get(master)==[]
   assert not any(x.get('dev')==master for x in d['network_before']['routes4']+d['network_before']['routes6'])
def owned_files(d):
 assert set(d['owned_files'])==set(OWNED)
 for name,mode in OWNED.items():
  q=d['owned_files'][name];assert q['before_absent'] is True and q['mode']==mode and re.fullmatch('[0-9a-f]{64}',q['SHA'])
  if os.path.lexists(name):b,s=regular(name,mode);assert hashlib.sha256(b).hexdigest()==q['SHA'],'owned file changed'
def atomic_original(b):
 p=pathlib.Path('/etc/vpp/startup.conf');parent=trusted_parent(p.parent);temp='.startup.conf.hardware211-rollback-'+str(os.getpid())
 try:
  fd=os.open(temp,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600,dir_fd=parent)
  with os.fdopen(fd,'wb') as f:f.write(b);os.fchmod(f.fileno(),0o644);f.flush();os.fsync(f.fileno())
  os.rename(temp,p.name,src_dir_fd=parent,dst_dir_fd=parent);os.fsync(parent)
 finally:os.close(parent)
def supplemental(expected):
 assert re.fullmatch('[0-9a-f]{64}',expected)
 b,s=regular(RETRY/'manifest.json',0o600);assert hashlib.sha256(b).hexdigest()==expected
 q=json.loads(b);assert q['schema']==1 and q['task']=='hardware-211-20261010-retry97ae' and q['original_manifest_SHA']==ORIGINAL_MANIFEST_SHA and q['original_startup_SHA']==ORIGINAL_STARTUP_SHA and q['failed_render_SHA']==FAILED_RENDER_SHA
 assert q['source']=='97ae88ee5b6aaf304f78547abed39e80bbeac5e1' and q['version']=='0.1.0~dev+97ae88ee5b6a' and q['generator_SHA']==GENERATOR_SHA and q['document_SHA']==DOC_SHA
 b,s=regular(RETRY/'physical.doc.json',0o600);assert hashlib.sha256(b).hexdigest()==DOC_SHA
 b,s=regular(RETRY/'physical.rendered.conf',0o600);assert len(b)==q['new_render_bytes'] and hashlib.sha256(b).hexdigest()==q['new_render_SHA'] and q['new_render_SHA'] not in [ORIGINAL_STARTUP_SHA,FAILED_RENDER_SHA]
 text=b.decode();assert not re.search(r'\bblacklist\b',text) and re.findall(r'^\s*dev\s+(0000:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7])\s*\{',text,re.M)==sorted(pci for pci,group,driver in NICS.values())
 return q
def terminal(work,q):
 assert work and re.fullmatch(r'/var/lib/ngfw/startup-apply/[0-9]{8}-[0-9]{6}-[0-9]+',work)
 w=pathlib.Path(work);parent=trusted_parent(w);os.close(parent)
 def read(name):
  path=w/name;s=path.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==s.st_gid==0 and stat.S_IMODE(s.st_mode) in [0o600,0o640,0o644,0o700,0o750,0o755] and s.st_nlink==1
  return regular(path,stat.S_IMODE(s.st_mode))[0]
 h=hashlib.sha256()
 for name in ['settings','doc.json','gen-args','bin/ngfw-startupgen','bin/ngfw-vppcheck','bin/apply-startup.sh','gate']:h.update(read(name))
 seal=read('plan.sha256').decode().strip();assert re.fullmatch('[0-9a-f]{64}',seal) and h.hexdigest()==seal
 assert hashlib.sha256(read('doc.json')).hexdigest()==DOC_SHA and hashlib.sha256(read('new.conf')).hexdigest()==q['new_render_SHA'] and hashlib.sha256(read('backup.conf')).hexdigest()==ORIGINAL_STARTUP_SHA and hashlib.sha256(read('bin/ngfw-startupgen')).hexdigest()==GENERATOR_SHA
 markers={n:read(n).decode() if os.path.lexists(w/n) else None for n in ['committed','rolled-back','console-needed','superseded']}
 assert sum(markers[n] is not None for n in ['committed','rolled-back'])==1 and markers['console-needed'] is None and markers['superseded'] is None,'native work is not safely terminal'
 unit=read('run-unit').decode().strip();assert unit=='ngfw-startup-apply-'+w.name
 timer=read('deadman-unit').decode().strip();assert re.fullmatch(r'ngfw-startup-apply-deadman-[0-9]{8}-[0-9]{6}-[0-9]+',timer)
 for name in [unit,timer+'.timer',timer+'.service']:
  assert checked(['systemctl','show',name,'-p','ActiveState','--value']).strip()=='inactive','native run/deadman still active'
 return {'work':work,'plan_SHA':seal,'markers':markers}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--manifest-sha256',required=True);p.add_argument('--retry-sha256',required=True);p.add_argument('--work');p.add_argument('--restore',action='store_true');a=p.parse_args()
 assert os.geteuid()==0 and (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert a.manifest_sha256==ORIGINAL_MANIFEST_SHA
 retry=supplemental(a.retry_sha256)
 # ROOT invokes the exact staged private source, never a different copy.
 assert pathlib.Path(__file__).resolve(strict=True)==RETRY/'rollback.source'
 self_bytes,self_stat=regular(RETRY/'rollback.source',0o600);assert hashlib.sha256(self_bytes).hexdigest()==retry['rollback_source_SHA']
 b,s=regular(RECORD/'manifest.json',0o600);assert hashlib.sha256(b).hexdigest()==a.manifest_sha256;d=json.loads(b)
 assert d['schema']==1 and d['task']=='hardware-211-20261010' and d['root_dev']==[8,2]
 for unit in ['vpp.service','ngfw-agent.service','ngfw-api.service']:assert checked(['systemctl','show',unit,'-p','ActiveState','--value']).strip()=='inactive','units must be stopped by manager before rollback'
 original,s=regular(RECORD/'startup.before',0o600);assert len(original)==735 and hashlib.sha256(original).hexdigest()==ORIGINAL_STARTUP_SHA
 assert d['new_startup_SHA']==FAILED_RENDER_SHA
 # Only this in-memory view accepts the separately sealed new render. The
 # original immutable manifest bytes/inodes are never changed.
 d=dict(d);d['new_startup_SHA']=retry['new_render_SHA']
 live,s=regular('/etc/vpp/startup.conf',0o644);assert hashlib.sha256(live).hexdigest() in [ORIGINAL_STARTUP_SHA,d['new_startup_SHA']],'startup outside exact transaction'
 protected_network(d);validate_nics(d);owned_files(d)
 if not a.restore:
  print(json.dumps({'rollback_manifest_valid':True,'original_driver_count':17,'no_mutation':True}));return
 terminal_state=terminal(a.work,retry)
 # Same lock order as the canonical startupapply; nonblocking refusal avoids
 # racing its still-running run/dead-man/holder. No arbitrary lock release.
 locks=[]
 try:
  for path in ['/run/lock/ngfw-vpp.lock','/run/lock/ngfw-lab.lock']:
   fd=os.open(path,os.O_RDWR|os.O_CREAT|os.O_NOFOLLOW|os.O_CLOEXEC,0o600)
   f=os.fdopen(fd,'a+');locks.append(f);s=os.fstat(f.fileno())
   assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and s.st_nlink==1 and not s.st_mode&0o022
   fcntl.flock(f.fileno(),fcntl.LOCK_EX|fcntl.LOCK_NB)
  terminal_state=terminal(a.work,retry)
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
  print(json.dumps({'restored_original_driver_count':17,'restored_original_startup':True,'removed_only_owned_unchanged_files':True,'management_protected':True,'services_remain_stopped':True,'terminal_native_work':terminal_state,'immutable_original_manifest_retained':True,'supplemental_manifest_SHA':a.retry_sha256}))
 finally:
  for f in reversed(locks):f.close()
if __name__=='__main__':main()
