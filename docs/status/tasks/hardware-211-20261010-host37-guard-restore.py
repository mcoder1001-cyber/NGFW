#!/usr/bin/env python3
"""ROOT-only original37 guarded identity restoration; source preparation only."""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
OUTPUT=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
INPUT=OUTPUT/'recovery-private/host-37'
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
def verify_marker(data):
 st=MARKER_DIRECTORY.lstat();assert stat.S_ISDIR(st.st_mode) and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o700
 st=MARKER_PATH.lstat();assert stat.S_ISREG(st.st_mode) and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o600
 assert MARKER_PATH.read_bytes()==data,'refuse changed/unowned recovery record'

assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()=='c8d66ea9-afab-4228-a293-00c198745040'
assert hashlib.sha256(pathlib.Path('/usr/sbin/ngfw-agent').read_bytes()).hexdigest()=='a909ae56ecee2431921659d0d9d489a14768fb12a4d71628627659fecd463781'
before=preflight();assert before==BASELINE['network']==FIRSTBOOT['network_after']
assert hashlib.sha256(pathlib.Path('/etc/ngfw/api.env').read_bytes()).hexdigest()==FIRSTBOOT['files']['/etc/ngfw/api.env']['sha256']
assert int(pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip(),16)==6
for n in [10,11,13,14,15,16,17]:
 p=pathlib.Path('/sys/class/net/enp'+str(n)+'s0/device');pci='0000:'+format(n,'02x')+':00.0';g=(p/'iommu_group').resolve();assert p.resolve().name==pci and (p/'driver').resolve().name=='igc' and g.name!='58' and sorted(x.name for x in (g/'devices').iterdir())==[pci]
assert {k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in FIRSTBOOT['sysctls_after']}==FIRSTBOOT['sysctls_after']
assert SEED['mode']=='prepare' and SEED['network_equal'] and SEED['no_activation_revision_or_binding']
assert pathlib.Path('/etc/ngfw/agent.env').read_bytes()==b'NGFW_MGMT_IF=enp12s0\nNGFW_MGMT_PCI=0000:0c:00.0\n'
assert pathlib.Path('/etc/systemd/system/ngfw-api.service.d/10-hardware-seed.conf').read_bytes()==b'[Service]\nEnvironment=NGFW_SEED_DEFAULT_NICS=1\n'
assert {u:command(['systemctl','show',u,'-p','ActiveState','--value']).strip() for u in FIRSTBOOT['states']}==FIRSTBOOT['states']
original={'policy':snapshot(POLICY_PATH),'mask':snapshot(MASK_PATH)}
guarded={'policy':{'type':'regular','mode':0o755,'uid':0,'gid':0,'bytes':len(POLICY),'sha256':hashlib.sha256(POLICY).hexdigest(),'base64':base64.b64encode(POLICY).decode()},'mask':{'type':'symlink','mode':0o777,'uid':0,'gid':0,'link':'/dev/null'}}
record=marker_bytes(guarded);assert hashlib.sha256(record).hexdigest()=='e275e5061a30bb452c7f24537280394459e6d26e70c6b11cece7ae2db64d3e60';verify_marker(record)
assert all(q['type']=='absent' for q in BASELINE['original'].values())
for key,path in [('mask',MASK_PATH),('policy',POLICY_PATH)]:
 current=metadata(original[key]);prior=metadata(BASELINE['original'][key]);expected=metadata(guarded[key]);assert current in [prior,expected]
 if current!=prior:atomic(path,guarded[key],BASELINE['original'][key])
command(['systemctl','daemon-reload']);after=l3();assert after==before
assert all(snapshot(p,False)['type']=='absent' for p in [POLICY_PATH,MASK_PATH])
assert {u:command(['systemctl','show',u,'-p','ActiveState','--value']).strip() for u in FIRSTBOOT['states']}==FIRSTBOOT['states']
verify_marker(record);MARKER_PATH.unlink();syncdir(MARKER_DIRECTORY);MARKER_DIRECTORY.rmdir();syncdir(MARKER_PARENT)
print(json.dumps({'mode':'restore','original_guards_absent':True,'policy':snapshot(POLICY_PATH,False),'mask':snapshot(MASK_PATH,False),'network_before':before,'network_after':after,'network_equal':True,'guard_baseline_SHA':BASELINE_SHA256,'seed_inputs_SHA':SEED_SHA,'firstboot_SHA':FIRST_SHA,'removed_owned_marker_SHA':hashlib.sha256(record).hexdigest(),'sibling_seed_record_retained':pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-host37-seed-inputs/before.json').exists(),'no_activation_revision_or_binding':True},indent=2))
'''

def load(p,parent,sha):
 p=pathlib.Path(p);assert p.parent==parent and not p.is_symlink();s=p.stat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 raw=p.read_bytes();assert hashlib.sha256(raw).hexdigest()==sha;return json.loads(raw)
def save(p,b):
 assert p.parent==OUTPUT and not p.parent.is_symlink() and p.parent.stat().st_uid==0 and not p.parent.stat().st_mode&0o022;fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd);return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--seed-proof',required=True);p.add_argument('--seed-sha256',required=True);a=p.parse_args()
 baseline=load(INPUT/'start-guards-inspect-20261010T120209Z.json',INPUT,'f4378c2af96e669c09836a5183d98529d99e9962b6ea9e35be4c0bb4cd03527a')
 first=load(OUTPUT/'manager-firstboot37-apply-20261010T141814Z.json',OUTPUT,'1b75f2ee3d1489348b87218396d1f22c30207b967ba22eea22d484f448c8b15a')
 seed=load(a.seed_proof,OUTPUT,a.seed_sha256);assert seed['mode']=='prepare' and seed['network_equal'] and seed['no_activation_revision_or_binding']
 fields={'BASELINE':baseline,'BASELINE_SHA256':'f4378c2af96e669c09836a5183d98529d99e9962b6ea9e35be4c0bb4cd03527a','FIRSTBOOT':first,'FIRST_SHA':'1b75f2ee3d1489348b87218396d1f22c30207b967ba22eea22d484f448c8b15a','SEED':seed,'SEED_SHA':a.seed_sha256,'POLICY':POLICY}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 q=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37','python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 print(json.dumps({'SSH_exit':q.returncode,'stdout':save(OUTPUT/('manager-host37-guards-restore-'+stamp+'.json'),q.stdout),'stderr':save(OUTPUT/('manager-host37-guards-restore-'+stamp+'.stderr'),q.stderr)}));raise SystemExit(q.returncode)
if __name__=='__main__':main()
