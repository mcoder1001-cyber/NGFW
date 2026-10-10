"""Finite genuine PPP runtime provisioning on .250; never replace existing assets."""
import hashlib,os,stat,subprocess,sys
from pathlib import Path
BASE=Path('/tmp/ngfw-lab-wan250-20261010')
ROOT=BASE/'repo'
TOKEN='ngp-'+hashlib.sha256(b'w20\0w20ppp').hexdigest()[:12]
assert os.geteuid()==0
for path in (BASE,ROOT):
 assert path.is_absolute() and str(path).find('\n')<0
 for parent in (path,*path.parents):
  if parent==Path('/tmp'):break
  info=parent.lstat();assert stat.S_ISDIR(info.st_mode) and info.st_uid==0 and not info.st_mode&0o022
regular={Path('/usr/sbin/pppd'):BASE/'bin/pppd',Path('/usr/sbin/pppoe-server'):BASE/'bin/pppoe-server',Path('/usr/lib/pppd/2.5.2/pppoe.so'):BASE/'bin/pppoe.so'}
alias=Path('/usr/lib/pppd/2.5.2/rp-pppoe.so')
created_dirs=[];receipts={}
def safe_parent(path):
 for parent in reversed(path.parents):
  if not parent.exists():
   parent.mkdir(mode=0o755);created_dirs.append(parent)
  info=parent.lstat()
  assert stat.S_ISDIR(info.st_mode) and info.st_uid==0 and not info.st_mode&0o022
assert all(not os.path.lexists(p) for p in [*regular,alias])
try:
 for dest,source in regular.items():
  assert source.is_relative_to(BASE)
  for parent in source.parents:
   info=parent.lstat();assert stat.S_ISDIR(info.st_mode) and info.st_uid==0 and not info.st_mode&0o022
   if parent==BASE:break
  info=source.lstat();assert stat.S_ISREG(info.st_mode) and info.st_uid==0 and info.st_nlink==1 and not info.st_mode&0o022
  body=source.read_bytes();safe_parent(dest)
  fd=os.open(dest,os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o755)
  with os.fdopen(fd,'wb') as stream:stream.write(body);stream.flush();os.fsync(stream.fileno());os.fchmod(stream.fileno(),0o755)
  info=dest.lstat();digest=hashlib.sha256(body).hexdigest();receipts[dest]=(info.st_dev,info.st_ino,digest)
  print('TEMP_GENUINE_RUNTIME',dest,digest,flush=True)
 safe_parent(alias);os.symlink('pppoe.so',alias);info=alias.lstat();receipts[alias]=(info.st_dev,info.st_ino,'pppoe.so')
 print('TEMP_GENUINE_ALIAS',alias,'pppoe.so',flush=True)
 env=dict(os.environ,NGFW_WAN_NATIVE_ROOT=str(ROOT),NGFW_WAN_NATIVE_BASE=str(BASE))
 result=subprocess.call([sys.executable,str(ROOT/'docs/status/tasks/lab-wan-20261010-carrier-live.py')],env=env)
finally:
 subprocess.run(['systemctl','stop','ngfw-pppoe-carrier@'+TOKEN+'.service'],check=False,capture_output=True)
 active=subprocess.run(['systemctl','is-active','ngfw-pppoe-carrier@'+TOKEN+'.service'],capture_output=True,text=True).stdout.strip()
 assert active!='active'
 assert not os.path.lexists('/run/netns/'+TOKEN),'owned carrier namespace still active'
 for dest,(device,inode,digest) in reversed(list(receipts.items())):
  info=dest.lstat();assert info.st_uid==0 and (info.st_dev,info.st_ino)==(device,inode)
  if dest==alias:assert stat.S_ISLNK(info.st_mode) and os.readlink(dest)==digest
  else:assert stat.S_ISREG(info.st_mode) and info.st_nlink==1 and not info.st_mode&0o022 and hashlib.sha256(dest.read_bytes()).hexdigest()==digest
  dest.unlink()
 for directory in reversed(created_dirs):directory.rmdir()
 print('TEMP_GENUINE_RUNTIME_REMOVED PASS',flush=True)
raise SystemExit(result)
