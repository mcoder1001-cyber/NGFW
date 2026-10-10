"""Remove only previously owned, packaged hook leftovers before new remote fixture."""
import hashlib,json,os,stat,subprocess
from pathlib import Path
BASE=Path('/tmp/ngfw-lab-wan250-20261010');ROOT=BASE/'repo'
TOKEN='ngp-'+hashlib.sha256(b'w20\0w20ppp').hexdigest()[:12]
root=Path('/var/lib/ngfw/agent/pppoe-carrier')/TOKEN
assert os.geteuid()==0
for parent in (root,*root.parents):
 info=parent.lstat();assert stat.S_ISDIR(info.st_mode) and info.st_uid==0 and not info.st_mode&0o022
identity=(root.stat().st_dev,root.stat().st_ino)
assert identity==(2050,268536),'previously recorded remote token replaced'
assert not os.path.lexists('/run/netns/'+TOKEN) and not os.path.lexists('/run/ngfw-pppoe-carrier/'+TOKEN+'.json')
def no_live_references():
 roots=(str(root),'/run/netns/'+TOKEN)
 def references(value):return any(value==owned or value.startswith(owned+'/') for owned in roots)
 for proc in Path('/proc').iterdir():
  if not proc.name.isdigit() or int(proc.name)==os.getpid():continue
  try:
   before=(proc/'stat').read_text().split(') ',1)[1].split()
   if before[0]=='Z':continue
   cwd=os.readlink(proc/'cwd').removesuffix(' (deleted)')
   argv=[item.decode() for item in (proc/'cmdline').read_bytes().split(b'\0') if item]
   assert not references(cwd) and not any(references(arg) for arg in argv),'live process references owned token'
   assert not any(references(row.split()[4]) for row in (proc/'mountinfo').read_text().splitlines()),'live process retains owned token mount'
   after=(proc/'stat').read_text().split(') ',1)[1].split()
   assert after[19]==before[19],'PID changed during admission'
  except (FileNotFoundError,ProcessLookupError):
   assert not proc.exists(),'unreadable live process reference'
 assert not any(references(row.split()[4]) for row in Path('/proc/self/mountinfo').read_text().splitlines())
no_live_references()
unit='ngfw-pppoe-carrier@'+TOKEN+'.service'
def properties():
 out=subprocess.run(['systemctl','show',unit,'-p','MainPID','-p','ActiveState','-p','FragmentPath'],check=True,capture_output=True,text=True).stdout
 return dict(line.split('=',1) for line in out.splitlines())
before=properties();assert before['MainPID']=='0' and before['ActiveState'] in ('inactive','failed')
assert before['FragmentPath']=='' and not os.path.lexists('/run/systemd/system/ngfw-pppoe-carrier@.service')
assert properties()==before
subprocess.run(['systemctl','stop',unit],check=True,capture_output=True)
subprocess.run(['systemctl','reset-failed',unit],check=True,capture_output=True)
after=properties();assert after['MainPID']=='0' and after['ActiveState']=='inactive'
directories={'ppp','ppp/ip-up.d','ppp/ip-down.d','ppp/ipv6-up.d','ppp/ipv6-down.d','ppp/peers'}
files={f'ppp/{name}':hashlib.sha256((ROOT/'scripts/pppoe-carrier-assets'/name).read_bytes()).hexdigest() for name in ('ip-up','ip-down','ipv6-up','ipv6-down')}
files['ppp/resolv.conf']=hashlib.sha256(b'').hexdigest()
snapshots=[]
for path in root.rglob('*'):
 rel=str(path.relative_to(root));info=path.lstat()
 assert info.st_uid==0 and not info.st_mode&0o022 and not stat.S_ISLNK(info.st_mode)
 if stat.S_ISDIR(info.st_mode):assert rel in directories;digest=None
 else:
  assert stat.S_ISREG(info.st_mode) and info.st_nlink==1 and rel in files
  digest=hashlib.sha256(path.read_bytes()).hexdigest();assert digest==files[rel]
 snapshots.append((path,info.st_dev,info.st_ino,digest))
print('REMOTE_OWNED_RENDER_LEFTOVERS '+json.dumps([{'relative':str(p.relative_to(root)),'dev':d,'ino':i,'sha256':h} for p,d,i,h in snapshots]),flush=True)
for path,device,inode,digest in sorted(snapshots,key=lambda row:(row[3] is None,-len(row[0].parts))):
 no_live_references()
 info=path.lstat();assert not path.is_symlink() and (info.st_dev,info.st_ino)==(device,inode)
 if digest is None:path.rmdir()
 else:assert hashlib.sha256(path.read_bytes()).hexdigest()==digest;path.unlink()
no_live_references()
assert (root.stat().st_dev,root.stat().st_ino)==identity and properties()['MainPID']=='0' and not os.path.lexists('/run/netns/'+TOKEN)
root.rmdir();print('REMOTE_OWNED_RENDER_LEFTOVERS_REMOVED PASS',flush=True)
