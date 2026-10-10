#!/usr/bin/env python3
"""Preserve and temporarily suppress package starts; no install or activation.

Inspect is read-only. Prepare/restore require explicit matching private baseline.
Only policy-rc.d, the persistent VPP mask, daemon-reload and wallclock are changed.
NTP settings, RTC, network, firewall, SSH, boot and device bindings are untouched.
"""
import argparse,datetime,hashlib,json,os,pathlib,shlex,subprocess,time
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37']
POLICY=b'#!/bin/sh\nexit 101\n'
REMOTE=r'''
import base64,hashlib,json,os,pathlib,stat,subprocess,tempfile,time
POLICY_PATH=pathlib.Path('/usr/sbin/policy-rc.d')
MASK_PATH=pathlib.Path('/etc/systemd/system/vpp.service')
MARKER_PARENT=pathlib.Path('/var/lib/ngfw-install-recovery')
MARKER_DIRECTORY=MARKER_PARENT/'hardware-37-20261010'
MARKER_PATH=MARKER_DIRECTORY/'state.json'
def command(args):
 p=subprocess.run(args,capture_output=True,text=True,timeout=30)
 assert p.returncode==0, (args,p.returncode)
 return p.stdout
def snapshot(path,contents=True):
 if not os.path.lexists(path):return {'type':'absent'}
 s=path.lstat();d={'mode':stat.S_IMODE(s.st_mode),'uid':s.st_uid,'gid':s.st_gid}
 assert s.st_uid==0,'refuse non-root safeguard'
 if stat.S_ISLNK(s.st_mode):
  d.update(type='symlink',link=os.readlink(path));return d
 assert stat.S_ISREG(s.st_mode) and s.st_size<=65536 and not s.st_mode&0o022,'unsupported/unsafe safeguard file'
 data=path.read_bytes();d.update(type='regular',bytes=len(data),sha256=hashlib.sha256(data).hexdigest())
 if contents:d['base64']=base64.b64encode(data).decode()
 return d
def metadata(d):return {k:v for k,v in d.items() if k!='base64'}
def l3():
 return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(command(['ip','-j','addr']))},
 'routes4':json.loads(command(['ip','-j','-4','route','show','table','all'])),
 'routes6':json.loads(command(['ip','-j','-6','route','show','table','all'])),
 'rules4':json.loads(command(['ip','-j','-4','rule'])),
 'rules6':json.loads(command(['ip','-j','-6','rule']))}
def preflight():
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 lines=command(['tune2fs','-l','/dev/sda2']).splitlines()
 assert [x.split(':',1)[1].strip() for x in lines if x.startswith('Filesystem state:')]==['clean']
 route=json.loads(command(['ip','-j','route','get','172.30.126.195']))[0]
 assert route.get('dev')=='enp12s0' and route.get('prefsrc')=='172.30.126.37'
 dev=pathlib.Path('/sys/class/net/enp12s0/device')
 assert dev.resolve().name=='0000:0c:00.0' and (dev/'driver').resolve().name=='igc'
 assert (dev/'iommu_group').resolve().name=='58'
 assert command(['systemctl','show','vpp.service','-p','ActiveState','--value']).strip() in ['inactive','failed']
 return l3()
def atomic(path,old,new):
 assert metadata(snapshot(path,False))==metadata(old),'safeguard changed since backup'
 if new['type']=='absent':
  if os.path.lexists(path):path.unlink()
 elif new['type']=='symlink':
  tmp=path.with_name('.ngfw-37-'+path.name+'.link')
  assert not os.path.lexists(tmp);os.symlink(new['link'],tmp)
  assert metadata(snapshot(path,False))==metadata(old)
  os.replace(tmp,path)
 else:
  fd,name=tempfile.mkstemp(prefix='.ngfw-37-',dir=path.parent)
  try:
   data=base64.b64decode(new['base64'],validate=True)
   assert hashlib.sha256(data).hexdigest()==new['sha256']
   with os.fdopen(fd,'wb') as f:
    f.write(data);f.flush();os.fchown(f.fileno(),new['uid'],new['gid']);os.fchmod(f.fileno(),new['mode']);os.fsync(f.fileno())
   assert metadata(snapshot(path,False))==metadata(old)
   os.replace(name,path)
  finally:
   if os.path.lexists(name):os.unlink(name)
 fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
def syncdir(path):
 fd=os.open(path,os.O_RDONLY|os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
def marker_bytes(guarded):
 return json.dumps({'task':'hardware-37-20261010','baseline_sha256':BASELINE_SHA256,
                    'original':BASELINE['original'],
                    'guarded':{k:metadata(v) for k,v in guarded.items()}},sort_keys=True).encode()
def create_marker(data):
 if os.path.lexists(MARKER_PARENT):
  st=MARKER_PARENT.lstat();assert stat.S_ISDIR(st.st_mode) and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o700
 else:
  MARKER_PARENT.mkdir(mode=0o700);syncdir(MARKER_PARENT.parent)
 assert (os.major(MARKER_PARENT.stat().st_dev),os.minor(MARKER_PARENT.stat().st_dev))==(8,2),'recovery record must be durable original-root storage'
 assert not os.path.lexists(MARKER_DIRECTORY),'refuse existing recovery marker directory'
 MARKER_DIRECTORY.mkdir(mode=0o700);syncdir(MARKER_PARENT)
 fd=os.open(MARKER_PATH,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
 syncdir(MARKER_DIRECTORY)
def verify_marker(data):
 st=MARKER_DIRECTORY.lstat();assert stat.S_ISDIR(st.st_mode) and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o700
 st=MARKER_PATH.lstat();assert stat.S_ISREG(st.st_mode) and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o600
 assert MARKER_PATH.read_bytes()==data,'refuse changed/unowned recovery record'
before=preflight()
original={'policy':snapshot(POLICY_PATH),'mask':snapshot(MASK_PATH)}
clock={'epoch':time.time(),'timedatectl':command(['timedatectl','show','-p','NTP','-p','NTPSynchronized','-p','TimeUSec','-p','RTCTimeUSec'])}
if MODE=='inspect':
 result={'original':original,'network':before,'clock':clock,'target_mutated':False}
else:
 assert BASELINE['network']==before,'L3 drift since baseline'
 guarded={'policy':{'type':'regular','mode':0o755,'uid':0,'gid':0,'bytes':len(POLICY),'sha256':hashlib.sha256(POLICY).hexdigest(),'base64':base64.b64encode(POLICY).decode()},
          'mask':{'type':'symlink','mode':0o777,'uid':0,'gid':0,'link':'/dev/null'}}
 record=marker_bytes(guarded)
 if MODE=='prepare':
  assert {k:metadata(v) for k,v in original.items()}=={k:metadata(v) for k,v in BASELINE['original'].items()}
  create_marker(record)
  atomic(POLICY_PATH,original['policy'],guarded['policy'])
  atomic(MASK_PATH,original['mask'],guarded['mask'])
  command(['systemctl','daemon-reload'])
  assert command(['systemctl','show','vpp.service','-p','LoadState','--value']).strip()=='masked'
  # Explicit operator UTC baseline, only wallclock; no hwclock or NTP changes.
  command(['date','--set=@'+str(CONTROLLER_EPOCH),'-u'])
  assert abs(time.time()-CONTROLLER_EPOCH)<30
 else:
  verify_marker(record)
  for key,path in [('mask',MASK_PATH),('policy',POLICY_PATH)]:
   current=metadata(original[key]);prior=metadata(BASELINE['original'][key]);expected=metadata(guarded[key])
   assert current in [prior,expected],'refuse unknown safeguard state during partial rollback'
   if current!=prior:atomic(path,guarded[key],BASELINE['original'][key])
  command(['systemctl','daemon-reload'])
 after=l3();assert after==before,'L3 changed during safeguard operation'
 if MODE=='restore':
  assert {k:metadata(snapshot(v,False)) for k,v in [('policy',POLICY_PATH),('mask',MASK_PATH)]}=={k:metadata(v) for k,v in BASELINE['original'].items()}
  verify_marker(record);MARKER_PATH.unlink();syncdir(MARKER_DIRECTORY)
  MARKER_DIRECTORY.rmdir();syncdir(MARKER_PARENT)
 result={'mode':MODE,'before_clock':clock,'after_epoch':time.time(),
         'policy':snapshot(POLICY_PATH,False),'mask':snapshot(MASK_PATH,False),
         'network_before':before,'network_after':after,'network_equal':True,
         'no_package_install':True,'no_service_activation':True,
         'time_restoration':False,'RTC_NTP_unchanged':True,
         'recovery_marker_present':os.path.lexists(MARKER_PATH),
         'recovery_marker_sha256':hashlib.sha256(record).hexdigest()}
print(json.dumps(result,indent=2))
'''
def save(path,data):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
 fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
 return {'file':str(path),'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest()}
def main():
 os.umask(0o077)
 p=argparse.ArgumentParser();p.add_argument('mode',choices=['inspect','prepare','restore']);p.add_argument('--baseline');a=p.parse_args()
 baseline=None;baseline_sha=None
 if a.mode!='inspect':
  assert a.baseline,'private baseline required'
  path=pathlib.Path(a.baseline).resolve();assert path.parent==PRIVATE
  st=path.stat();assert st.st_uid==0 and st.st_mode&0o777==0o600
  raw=path.read_bytes();baseline=json.loads(raw);baseline_sha=hashlib.sha256(raw).hexdigest();assert 'original' in baseline
 fields={'MODE':a.mode,'BASELINE':baseline,'POLICY':POLICY,'CONTROLLER_EPOCH':time.time(),'BASELINE_SHA256':baseline_sha}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 run=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True,timeout=180)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 out=save(PRIVATE/('start-guards-'+a.mode+'-'+stamp+'.json'),run.stdout)
 err=save(PRIVATE/('start-guards-'+a.mode+'-'+stamp+'.stderr'),run.stderr)
 print(json.dumps({'mode':a.mode,'SSH_exit':run.returncode,'stdout':out,'stderr':err,'no_package_install':True,'no_service_activation':True}))
 raise SystemExit(run.returncode)
if __name__=='__main__':main()
