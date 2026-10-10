"""Exact original finite inventory broker assets for current-source MultiWAN acceptance."""
import fcntl,hashlib,json,os,shutil,stat,subprocess,sys
from pathlib import Path
ROOT=Path(os.environ.get('NGFW_MULTIWAN_PRODUCT_ROOT','/root/ngfw-wt/lab-wan-20261010'))
assert os.geteuid()==0 and ROOT.is_absolute() and ROOT.is_dir() and not ROOT.is_symlink()
assert int(os.environ['NGFW_MULTIWAN_SLOT'])==20
inventory='ngp-'+hashlib.sha256(b'inventory\0w20').hexdigest()[:12]
unit='ngfw-pppoe-broker@'+inventory+'.service'
assets={Path('/usr/lib/ngfw/pppoe-carrier.py'):ROOT/'scripts/pppoe-kernel-carrier.py',Path('/run/systemd/system/ngfw-pppoe-broker@.service'):ROOT/'scripts/pppoe-carrier-assets/ngfw-pppoe-broker@.service'}
assert all(not os.path.lexists(p) for p in assets)
receipts={};created_dirs=[];loaded=False
asset_lock=open('/run/lock/ngfw-wan-w20-inventory-assets.lock','a');fcntl.flock(asset_lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
assert not Path('/run/netns/ns-w20-mw-router').exists()
baseline_frames={}
for area in ('requests','results'):
 path=Path('/run/ngfw/pppoe-broker')/area/(inventory+'.json')
 if os.path.lexists(path):
  info=path.lstat();assert stat.S_ISREG(info.st_mode) and info.st_uid==0 and info.st_nlink==1 and not info.st_mode&0o077
  baseline_frames[area]=(info.st_dev,info.st_ino,hashlib.sha256(path.read_bytes()).hexdigest())
shared=subprocess.run(['systemctl','show','vpp','-p','MainPID','-p','NRestarts'],check=True,capture_output=True,text=True).stdout
try:
 for dest,source in assets.items():
  for parent in (source,*source.parents):
   info=parent.lstat();assert info.st_uid==0 and not info.st_mode&0o022 and not stat.S_ISLNK(info.st_mode)
   if parent==ROOT:break
  assert source.is_file() and source.stat().st_nlink==1
  for parent in reversed(dest.parents):
   if not parent.exists():parent.mkdir(mode=0o755);created_dirs.append(parent)
   info=parent.lstat();assert stat.S_ISDIR(info.st_mode) and info.st_uid==0 and not info.st_mode&0o022
  body=source.read_bytes();fd=os.open(dest,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o644 if dest.suffix=='.service' else 0o755)
  with os.fdopen(fd,'wb') as stream:stream.write(body)
  info=dest.lstat();receipts[dest]=(info.st_dev,info.st_ino,hashlib.sha256(body).hexdigest())
  print('TEMP_ORIGINAL_INVENTORY_ASSET '+str(dest)+' sha256='+receipts[dest][2],flush=True)
 subprocess.run(['systemctl','daemon-reload'],check=True);loaded=True
 result=subprocess.call([sys.executable,str(ROOT/'test/topology/multiwan-host-acceptance/run.py')])
finally:
 # Inventory is a finite list operation; only this slot's original broker unit is touched.
 if loaded:
  assert not Path('/run/netns/ns-w20-mw-router').exists()
  subprocess.run(['systemctl','stop',unit],check=True,capture_output=True)
  values=dict(line.split('=',1) for line in subprocess.run(['systemctl','show',unit,'-p','MainPID','-p','ActiveState','-p','FragmentPath'],check=True,capture_output=True,text=True).stdout.splitlines())
  assert values['MainPID']=='0' and values['ActiveState'] in ('inactive','failed')
  assert values['FragmentPath']==str(next(p for p in assets if p.suffix=='.service'))
  if values['ActiveState']=='failed':subprocess.run(['systemctl','reset-failed',unit],check=True,capture_output=True)
  assert subprocess.run(['systemctl','is-active',unit],capture_output=True,text=True).stdout.strip()=='inactive'
  frames={}
  for area in ('requests','results'):
   path=Path('/run/ngfw/pppoe-broker')/area/(inventory+'.json')
   if not os.path.lexists(path):continue
   for parent in path.parents:
    info=parent.lstat();assert stat.S_ISDIR(info.st_mode) and info.st_uid==0 and not info.st_mode&0o022
   info=path.lstat();assert stat.S_ISREG(info.st_mode) and info.st_uid==0 and info.st_nlink==1 and not info.st_mode&0o077 and info.st_size<=1048576
   data=path.read_bytes();digest=hashlib.sha256(data).hexdigest();record=json.loads(data)
   assert record['token']==inventory and type(record['nonce']) is str and len(record['nonce'])==32
   assert all(c in '0123456789abcdef' for c in record['nonce'])
   frames[area]=(path,info.st_dev,info.st_ino,digest,record)
  if frames:
   assert 'requests' in frames
   request=frames['requests'][4];assert request['request']=={'op':'list','owner':'w20'}
   if 'results' in frames:
    response=frames['results'][4]
    assert all(response[key]==request[key] for key in ('token','nonce','boot','expires','request_sha256'))
   for area,(path,device,inode,digest,record) in frames.items():
    if baseline_frames.get(area)==(device,inode,digest):continue
    current=path.lstat();assert (current.st_dev,current.st_ino)==(device,inode) and hashlib.sha256(path.read_bytes()).hexdigest()==digest
    path.unlink()
 for dest,(device,inode,digest) in reversed(list(receipts.items())):
  info=dest.lstat();assert stat.S_ISREG(info.st_mode) and info.st_uid==0 and info.st_nlink==1 and not info.st_mode&0o022 and (info.st_dev,info.st_ino)==(device,inode) and hashlib.sha256(dest.read_bytes()).hexdigest()==digest
  dest.unlink()
 for parent in reversed(created_dirs):parent.rmdir()
 subprocess.run(['systemctl','daemon-reload'],check=True)
 assert subprocess.run(['systemctl','show','vpp','-p','MainPID','-p','NRestarts'],check=True,capture_output=True,text=True).stdout==shared
 print('TEMP_ORIGINAL_INVENTORY_ASSETS_REMOVED_SHARED_VPP_UNCHANGED PASS',flush=True)
raise SystemExit(result)
