#!/usr/bin/python3
"""Finite private identity fixture; runs the unmodified actual acceptance ELF."""
import os,sys,subprocess,tempfile,shutil,runpy
from pathlib import Path
if os.environ.get('NGFW_MULTIWAN_PRIVATE_IDENTITY_CHILD') != '1':
 env=dict(os.environ,NGFW_MULTIWAN_PRIVATE_IDENTITY_CHILD='1',NGFW_MULTIWAN_HOST_UTS=os.readlink('/proc/self/ns/uts'))
 raise SystemExit(subprocess.call(['unshare','--mount','--uts','--propagation','private',sys.executable,__file__,*sys.argv[1:]],env=env))
assert os.readlink('/proc/self/ns/uts') != os.environ['NGFW_MULTIWAN_HOST_UTS']
root=Path(os.environ['NGFW_MULTIWAN_PRODUCT_ROOT']);binary=Path(os.environ['NGFW_MULTIWAN_ACTUAL_TEST_BIN']);assert binary.is_file() and binary.is_absolute()
with tempfile.TemporaryDirectory(prefix='multiwan-private-identity-') as directory:
 etc=Path(directory)/'etc';shutil.copytree('/etc',etc,symlinks=True)
 timezone=etc/'localtime'
 if timezone.is_symlink():
  target=Path('/etc/localtime').resolve(strict=True);assert target.is_relative_to('/usr/share/zoneinfo');timezone.unlink();timezone.symlink_to(str(target))
 for name in ('unbound','chrony','rsyslog.d','snmp','kea','frr'):
  target=etc/name
  if target.is_symlink():target.unlink()
  elif target.exists():shutil.rmtree(target)
  target.mkdir(mode=0o755)
 mounted=[]
 try:
  subprocess.run(['mount','--bind',str(etc),'/etc'],check=True);mounted.append('/etc')
  subprocess.run(['mount','-t','tmpfs','-o','mode=755,size=8m','tmpfs','/var/lib'],check=True);mounted.append('/var/lib')
  runpy.run_path(str(root/'deploy/debian/ngfw/assets/provision-system-identity.py'))['provision']()
  print('PRIVATE_EXACT_PACKAGE_IDENTITY_PROVISIONED PASS',flush=True)
  result=subprocess.call([str(binary),*sys.argv[1:]])
 finally:
  for target in reversed(mounted):subprocess.run(['umount',target],check=True)
 print('PRIVATE_IDENTITY_MOUNTS_REMOVED PASS',flush=True)
raise SystemExit(result)
