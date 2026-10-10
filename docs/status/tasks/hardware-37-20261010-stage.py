#!/usr/bin/env python3
"""Reviewed RAM-only staging for host .37. Never transitions or repairs a disk.

All explicit target writes are in /run tmpfs or the new RAM filesystem.
Credential bytes travel privately in SSH stdin and are never printed or hashed
individually. Execute only after manager/R7 review of this exact source.
"""
import base64
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import shlex
import time

PRIVATE = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
GUARD = PRIVATE.parent / 'shared/block-check'
GUARD_SHA = '02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c'
AUTH_SHA = '2a57558ae6a6c7c2694b2034249e301d209bb0b5b8bc69076e77710725cd45c9'
SSH = ['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes',
       '-o', 'ConnectTimeout=15', 'root@172.30.126.37']

REMOTE = r"""
import base64,glob,hashlib,json,os,pathlib,re,shutil,stat,subprocess,time
R=pathlib.Path('/run/ngfwrescue')
UNIT='ngfw-rescue.service'
reports=[]
def run(args,check=True):
    p=subprocess.run(args,capture_output=True,text=True,timeout=60)
    reports.append({'command':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr})
    if check and p.returncode: raise RuntimeError('Command failed: '+args[0]+' exit '+str(p.returncode))
    return p
def write(path,data,mode=0o644):
    path=pathlib.Path(path);path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(data);os.chmod(path,mode)
def network():
    commands=[['ip','-j','address','show'],['ip','-j','-4','route','show','table','all'],
              ['ip','-j','-6','route','show','table','all'],['ip','-j','-4','rule','show'],['ip','-j','-6','rule','show']]
    values=[json.loads(subprocess.check_output(args,text=True)) for args in commands]
    addresses=[]
    for iface in values[0]:
        addresses.append({'ifname':iface['ifname'],'addr_info':iface.get('addr_info',[])})
    values[0]=addresses
    return values
assert not os.path.lexists('/run/nextroot'), 'nextroot must remain absent'
assert not R.exists(), 'refuse overwrite of an existing rescue directory'
rootdev=os.stat('/').st_dev
assert (os.major(rootdev),os.minor(rootdev))==(8,2)
assert '259.5' in subprocess.check_output(['systemctl','--version'],text=True).splitlines()[0]
assert not subprocess.check_output(['ss','-H','-ltn','sport = :2222'],text=True).strip()
before=network()
required_names=['bash','dash','sshd','systemctl','systemd-analyze','chroot','e2fsck','e2image','e2undo','tune2fs',
       'dumpe2fs','debugfs','ip','mount','umount','findmnt','lsblk','stat','readlink','ls','cat','chmod','chown',
       'cp','mkdir','rm','sync','setsid','kill','ldd','getent','ss','python3','gzip','zcat','tar','sed','grep','awk']
resolved_tools={name:shutil.which(name) for name in required_names}
assert all(resolved_tools.values()), 'required tool absent before mutation'
for path in ['/usr/bin/chroot','/usr/bin/mkdir','/usr/bin/chmod','/usr/bin/chown','/usr/sbin/sshd','/usr/sbin/e2fsck','/usr/sbin/e2image','/usr/sbin/e2undo','/usr/bin/ldd','/usr/bin/python3','/usr/bin/bash','/usr/lib/systemd/systemd','/usr/lib/systemd/systemd-executor','/usr/lib/systemd/systemd-shutdown','/usr/lib/openssh/sshd-session','/usr/lib/openssh/sshd-auth']:
    assert pathlib.Path(path).is_file() and os.access(path,os.X_OK), 'required exact path absent: '+path
MOUNT='''[Unit]
Description=NGFW temporary RAM recovery filesystem
DefaultDependencies=no

[Mount]
What=tmpfs
Where=/run/ngfwrescue
Type=tmpfs
Options=rw,exec,nosuid,nodev,size=6G,mode=0755
'''
HOST_UNIT='''[Unit]
Description=NGFW temporary RAM recovery SSH
DefaultDependencies=no
SurviveFinalKillSignal=yes
IgnoreOnIsolate=yes
After=basic.target
Conflicts=reboot.target kexec.target poweroff.target halt.target rescue.target emergency.target
Before=shutdown.target rescue.target emergency.target

[Service]
Type=simple
ExecStart=/usr/bin/chroot /run/ngfwrescue /usr/sbin/sshd -D -e -f /etc/ssh/sshd_config
Restart=on-failure
RestartSec=2
KillMode=control-group
PrivateMounts=no
StandardOutput=append:/run/ngfwrescue/var/log/rescue-sshd.log
StandardError=append:/run/ngfwrescue/var/log/rescue-sshd.log
Environment=PATH=/usr/sbin:/usr/bin:/sbin:/bin
'''
RAM_UNIT='''[Unit]
Description=NGFW temporary RAM recovery SSH
DefaultDependencies=no
SurviveFinalKillSignal=yes
IgnoreOnIsolate=yes
After=basic.target ngfw-rescue-runtime.service
Conflicts=reboot.target kexec.target poweroff.target halt.target rescue.target emergency.target
Before=shutdown.target rescue.target emergency.target

[Service]
Type=simple
ExecStart=/usr/sbin/sshd -D -e -f /etc/ssh/sshd_config
Restart=on-failure
RestartSec=2
KillMode=control-group
PrivateMounts=no
StandardOutput=append:/var/log/rescue-sshd.log
StandardError=append:/var/log/rescue-sshd.log
Environment=PATH=/usr/sbin:/usr/bin:/sbin:/bin
'''
PREP_UNIT='''[Unit]
Description=Prepare transferred runtime for NGFW RAM SSH
DefaultDependencies=no
Before=ngfw-rescue.service

[Service]
Type=oneshot
ExecStart=/usr/bin/mkdir -p /run/sshd
ExecStart=/usr/bin/chmod 0755 /run/sshd
ExecStart=/usr/bin/chown 0:0 /run/sshd
RemainAfterExit=yes
'''
SSH_CONFIG='''Port 2222
ListenAddress 172.30.126.37
HostKey /etc/ssh/ssh_host_rsa_key
HostKey /etc/ssh/ssh_host_ecdsa_key
HostKey /etc/ssh/ssh_host_ed25519_key
PermitRootLogin prohibit-password
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
AuthenticationMethods publickey
UsePAM no
UseDNS no
StrictModes yes
AllowUsers root
AuthorizedKeysFile .ssh/authorized_keys .ssh/authorized_keys2
PidFile /run/ngfw-rescue-sshd.pid
Subsystem sftp internal-sftp
LogLevel VERBOSE
'''
for name in ['run-ngfwrescue.mount',UNIT]:
    assert not pathlib.Path('/run/systemd/system',name).exists(), 'runtime unit exists'
    for prefix in ['/etc/systemd/system','/run/systemd/transient','/etc/systemd/system.control','/run/systemd/system.control','/run/systemd/system']:
        assert not pathlib.Path(prefix,name).exists(), 'higher priority unit exists'
        assert not pathlib.Path(prefix,name+'.d').exists(), 'unit drop-in exists'
R.mkdir(mode=0o755)
write('/run/systemd/system/run-ngfwrescue.mount',MOUNT)
run(['systemctl','daemon-reload']);run(['systemctl','start','run-ngfwrescue.mount'])
mount=json.loads(run(['findmnt','-J','-n','-T',str(R),'-o','TARGET,SOURCE,FSTYPE,OPTIONS']).stdout)['filesystems'][0]
assert mount['target']==str(R) and mount['fstype']=='tmpfs' and 'noexec' not in mount['options']
ramdev=R.stat().st_dev
assert ramdev!=rootdev
for directory in ['usr/bin','usr/sbin','usr/lib','usr/lib64','usr/local/sbin','etc/ssh','etc/systemd/system',
                  'etc/systemd/system-generators','usr/lib/systemd/system-generators','root/.ssh',
                  'run/sshd','dev','proc','sys','var/log','var/recovery','tmp']:
    (R/directory).mkdir(parents=True,exist_ok=True)
for alias,target in [('bin','usr/bin'),('sbin','usr/sbin'),('lib','usr/lib'),('lib64','usr/lib64')]:
    (R/alias).symlink_to(target)
for name in ['root','root/.ssh','var/log','var/recovery']:os.chmod(R/name,0o700)
os.chmod(R/'tmp',0o1777)
package_md5={}
for filename in glob.glob('/var/lib/dpkg/info/*.md5sums'):
    for line in pathlib.Path(filename).read_text().splitlines():
        pieces=line.split(None,1)
        if len(pieces)==2:package_md5['/'+pieces[1].strip()]=pieces[0]
copied={}
def copy_file(path):
    path=str(path)
    if path in copied:return
    source=pathlib.Path(path);target=R/path.lstrip('/')
    real=str(source.resolve());data=source.read_bytes()
    expected=package_md5.get(path) or package_md5.get(real)
    actual=hashlib.md5(data).hexdigest()
    if expected:assert actual==expected, 'packaged file checksum differs: '+path
    target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data)
    os.chmod(target,stat.S_IMODE(source.stat().st_mode))
    copied[path]={'sha256':hashlib.sha256(data).hexdigest(),'bytes':len(data),'package_md5_verified':bool(expected)}
def copy_binary(path):
    copy_file(path)
    p=subprocess.run(['ldd',str(path)],capture_output=True,text=True,timeout=15)
    assert 'not found' not in p.stdout, 'missing executable closure library: '+str(path)
    for line in p.stdout.splitlines():
        m=re.search(r'=>\s+(/\S+)',line) or re.match(r'\s*(/\S+)\s+\(',line)
        if m:copy_file(m.group(1))
for name,path in resolved_tools.items():copy_binary(path)
for path in ['/usr/lib/systemd/systemd','/usr/lib/systemd/systemd-executor','/usr/lib/systemd/systemd-shutdown','/usr/lib/openssh/sshd-session','/usr/lib/openssh/sshd-auth']:
    assert pathlib.Path(path).exists();copy_binary(path)
for path in glob.glob('/usr/lib/x86_64-linux-gnu/libnss_*.so.2'):copy_binary(path)
pyversion=subprocess.check_output(['python3','-c','import sys; print(str(sys.version_info.major)+"."+str(sys.version_info.minor))'],text=True).strip()
pyroot=pathlib.Path('/usr/lib/python'+pyversion)
shutil.copytree(pyroot,R/str(pyroot).lstrip('/'),dirs_exist_ok=True)
for path in pyroot.glob('lib-dynload/*.so'):copy_binary(path)
(R/'usr/sbin/init').symlink_to('/usr/lib/systemd/systemd')
write(R/'etc/passwd','root:x:0:0:root:/root:/bin/bash\nsshd:x:990:65534:sshd:/run/sshd:/usr/sbin/nologin\n')
write(R/'etc/group','root:x:0:\nnogroup:x:65534:\n')
write(R/'etc/nsswitch.conf','passwd: files\ngroup: files\nshadow: files\nhosts: files dns\n')
write(R/'etc/hosts','127.0.0.1 localhost\n::1 localhost\n')
for path in ['/etc/machine-id','/etc/os-release']:
    source=pathlib.Path(path);target=R/path.lstrip('/')
    if source.exists():shutil.copyfile(source,target);os.chmod(target,0o644)
for path in glob.glob('/etc/ssh/ssh_host_*_key'):
    target=R/path.lstrip('/');shutil.copyfile(path,target);os.chmod(target,0o600)
keys=0
for path in ['/root/.ssh/authorized_keys','/root/.ssh/authorized_keys2']:
    if pathlib.Path(path).is_file():
        target=R/path.lstrip('/');shutil.copyfile(path,target);os.chmod(target,0o600);keys+=1
assert keys, 'no existing root authorized public key file'
write(R/'etc/ssh/sshd_config',SSH_CONFIG,0o600)
write(R/'etc/systemd/system'/UNIT,RAM_UNIT)
write(R/'etc/systemd/system/ngfw-rescue-runtime.service',PREP_UNIT)
write(R/'etc/systemd/system/default.target','[Unit]\nDescription=NGFW RAM recovery only\nDefaultDependencies=no\nRequires=ngfw-rescue-runtime.service '+UNIT+'\nAfter=ngfw-rescue-runtime.service '+UNIT+'\nAllowIsolate=yes\n')
write(R/'etc/systemd/system/basic.target','[Unit]\nDescription=RAM ordering anchor only\nDefaultDependencies=no\n')
guard=base64.b64decode(GUARD_B64);assert hashlib.sha256(guard).hexdigest()==GUARD_SHA
(R/'usr/local/sbin/block-check').write_bytes(guard);os.chmod(R/'usr/local/sbin/block-check',0o755)
auth=base64.b64decode(AUTH_B64);assert hashlib.sha256(auth).hexdigest()==AUTH_SHA
(R/'var/recovery/auth-return.tar').write_bytes(auth);os.chmod(R/'var/recovery/auth-return.tar',0o600)
for source,dest,recursive in [('/dev','dev',True),('/proc','proc',False),('/sys','sys',True)]:
    run(['mount','--rbind' if recursive else '--bind',source,str(R/dest)])
    run(['mount','--make-rprivate',str(R/dest)])
for binary in ['/usr/lib/systemd/systemd','/usr/lib/systemd/systemd-executor','/usr/sbin/sshd','/usr/lib/openssh/sshd-session','/usr/lib/openssh/sshd-auth','/usr/sbin/e2fsck','/usr/sbin/e2image','/usr/sbin/e2undo','/usr/bin/python3','/usr/bin/bash']:
    p=run(['chroot',str(R),'/usr/bin/ldd',binary]);assert 'not found' not in p.stdout
run(['chroot',str(R),'/usr/sbin/sshd','-t','-f','/etc/ssh/sshd_config'])
run(['chroot',str(R),'/usr/lib/systemd/systemd','--version'])
run(['chroot',str(R),'/usr/sbin/e2fsck','-V'])
verify=run(['systemd-analyze','verify','--root='+str(R),'ngfw-rescue.service','ngfw-rescue-runtime.service','default.target','basic.target'],check=False)
assert verify.returncode==0 and not verify.stderr.strip(), 'offline unit validation has output/failure'
probe=run([str(R/'usr/local/sbin/block-check'),'/dev/sda2','8','2'],check=False)
assert probe.returncode==3, 'mounted negative guard must return BUSY exit3'
write('/run/systemd/system/'+UNIT,HOST_UNIT)
run(['systemctl','daemon-reload']);run(['systemctl','start',UNIT])
props=run(['systemctl','show',UNIT,'-p','MainPID','-p','ControlGroup','-p','SurviveFinalKillSignal','-p','IgnoreOnIsolate','-p','DefaultDependencies','-p','PrivateMounts','-p','RootDirectory','-p','RequiresMountsFor','-p','Conflicts','-p','After','-p','Before','-p','FragmentPath','-p','DropInPaths','-p','WorkingDirectory']).stdout
propdict=dict(line.split('=',1) for line in props.splitlines() if '=' in line)
assert propdict['SurviveFinalKillSignal']=='yes' and propdict['IgnoreOnIsolate']=='yes' and propdict['DefaultDependencies']=='no'
assert propdict['PrivateMounts']=='no' and not propdict['RootDirectory'] and not propdict['RequiresMountsFor']
assert propdict['FragmentPath']=='/run/systemd/system/'+UNIT and not propdict['DropInPaths'] and not propdict['WorkingDirectory']
pid=int(propdict['MainPID']);assert pid>1
proc=pathlib.Path('/proc')/str(pid)
assert os.stat(proc/'root').st_dev==ramdev and os.stat(proc/'cwd').st_dev==ramdev and os.stat(proc/'exe').st_dev==ramdev
assert os.readlink(proc/'ns/mnt')==os.readlink('/proc/1/ns/mnt')
disk_maps=[]
for line in (proc/'maps').read_text().splitlines():
    fields=line.split()
    if len(fields)>4:
        major,minor=fields[3].split(':')
        if (int(major,16),int(minor,16))==(8,2):disk_maps.append(fields[:5])
assert not disk_maps, 'rescue process maps old disk'
disk_fds=[]
for fd in (proc/'fd').iterdir():
    try:
        s=fd.stat()
        if (stat.S_ISREG(s.st_mode) or stat.S_ISDIR(s.st_mode)) and s.st_dev==rootdev:disk_fds.append(fd.name)
    except FileNotFoundError:pass
assert not disk_fds, 'rescue process has file/directory old-disk fd'
after=network();assert before==after, 'network addresses/routes/rules changed'
assert not os.path.lexists('/run/nextroot')
manifest={'stage_only':True,'transition_executed':False,'repair_executed':False,'ram_root':str(R),
          'ram_major_minor':f'{os.major(ramdev)}:{os.minor(ramdev)}','main_pid':pid,'control_group':propdict['ControlGroup'],
          'same_pid1_mount_namespace':True,'root_cwd_exe_ram':True,'oldroot_maps':disk_maps,'oldroot_file_directory_fds':disk_fds,
          'network_exactly_unchanged':True,'nextroot_absent':True,'copied_nonsecret_files':copied,'service_properties':propdict,
          'units':{'mount':MOUNT,'runtime_service':HOST_UNIT,'ram_service':RAM_UNIT,'ram_runtime_prepare':PREP_UNIT},'reports':reports}
write(R/'var/recovery/stage-manifest.json',json.dumps(manifest,indent=2)+'\n',0o600)
print(json.dumps(manifest))
"""


AUDIT = r"""
import json,os,pathlib,stat,subprocess
R=pathlib.Path('/run/ngfwrescue');unit='ngfw-rescue.service'
rootdev=os.stat('/').st_dev;ramdev=R.stat().st_dev
props=dict(x.split('=',1) for x in subprocess.check_output(['systemctl','show',unit,'-p','ControlGroup','-p','MainPID'],text=True).splitlines())
cg=pathlib.Path('/sys/fs/cgroup')/props['ControlGroup'].lstrip('/')
pids=sorted(set(int(x) for file in cg.rglob('cgroup.procs') for x in file.read_text().split()))
assert len(pids)>=2, 'authenticated child must be present in rescue service cgroup'
rows=[]
for pid in pids:
 p=pathlib.Path('/proc')/str(pid)
 try:
  assert all(os.stat(p/name).st_dev==ramdev for name in ['root','cwd','exe']), 'non-RAM process reference'
  assert os.readlink(p/'ns/mnt')==os.readlink('/proc/1/ns/mnt'), 'hidden mount namespace'
  oldmaps=[];oldfds=[]
  for line in (p/'maps').read_text().splitlines():
   f=line.split()
   if len(f)>4:
    a,b=f[3].split(':')
    if (int(a,16),int(b,16))==(8,2):oldmaps.append(f[:5])
  for fd in (p/'fd').iterdir():
   try:
    st=fd.stat()
    if (stat.S_ISREG(st.st_mode) or stat.S_ISDIR(st.st_mode)) and st.st_dev==rootdev:oldfds.append(fd.name)
   except FileNotFoundError:pass
  assert not oldmaps and not oldfds, 'old disk reference found'
  rows.append({'pid':pid,'exe':os.readlink(p/'exe'),'root_cwd_exe_ram':True,'same_pid1_mount_namespace':True,'oldroot_maps':oldmaps,'oldroot_file_directory_fds':oldfds})
 except FileNotFoundError:raise RuntimeError('child disappeared before proof')
assert not os.path.lexists('/run/nextroot')
print(json.dumps({'unit':unit,'control_group':props['ControlGroup'],'pids':rows,'authenticated_child_cgroup_proven':True,'nextroot_absent':True}))
"""

STOP_START = r"""
import json,os,pathlib,subprocess
unit='ngfw-rescue.service'
def net():
 return [subprocess.check_output(x,text=True) for x in [['ip','-j','address','show'],['ip','-j','-4','route','show','table','all'],['ip','-j','-6','route','show','table','all'],['ip','-j','-4','rule','show'],['ip','-j','-6','rule','show']]]
before=net();pid=int(subprocess.check_output(['systemctl','show',unit,'-p','MainPID','--value'],text=True))
assert pid>1
subprocess.run(['systemctl','stop',unit],check=True)
assert not pathlib.Path('/proc',str(pid)).exists()
assert not subprocess.check_output(['ss','-H','-ltn','sport = :2222'],text=True).strip()
assert subprocess.check_output(['ss','-H','-ltn','sport = :22'],text=True).strip()
subprocess.run(['systemctl','start',unit],check=True)
newpid=int(subprocess.check_output(['systemctl','show',unit,'-p','MainPID','--value'],text=True));assert newpid>1 and newpid!=pid
assert before==net() and not os.path.lexists('/run/nextroot')
print(json.dumps({'owned_rescue_only_stop_start':True,'stopped_exact_main_pid':pid,'restarted_main_pid':newpid,'original_22_still_listening':True,'network_exactly_unchanged':True,'nextroot_absent':True}))
"""

def protected_run(name,args,payload=None,timeout=60):
 stdout=PRIVATE/(name+'.json');stderr=PRIVATE/(name+'.stderr')
 with stdout.open('wb') as out,stderr.open('wb') as err:
  os.chmod(stdout,0o600);os.chmod(stderr,0o600)
  p=subprocess.run(args,input=payload,stdout=out,stderr=err,timeout=timeout)
 if p.returncode:raise RuntimeError(name+' exit '+str(p.returncode)+'; private stderr bytes '+str(stderr.stat().st_size))
 return json.loads(stdout.read_text())

def pty_test(sequence):
 # Keep an authenticated terminal child alive while global PID1 namespace is audited.
 code='import json,os,sys; print(json.dumps({"authenticated": True, "uid": os.getuid(), "stdin_pty":os.isatty(0), "stdout_pty":os.isatty(1), "pid":os.getpid(), "root_device":os.stat("/").st_dev}),flush=True); sys.stdin.readline()'
 command='/usr/bin/python3 -c '+shlex.quote(code)
 outpath=PRIVATE/('ram-pty-'+sequence+'.json');errpath=PRIVATE/('ram-pty-'+sequence+'.stderr')
 args=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','HostKeyAlias=172.30.126.37','-o','ConnectTimeout=15','-p','2222','-tt','root@172.30.126.37',command]
 with outpath.open('wb') as out,errpath.open('wb') as err:
  os.chmod(outpath,0o600);os.chmod(errpath,0o600)
  p=subprocess.Popen(args,stdin=subprocess.PIPE,stdout=out,stderr=err)
  try:
   deadline=time.monotonic()+30
   result=None
   while time.monotonic()<deadline:
    if p.poll() is not None:raise RuntimeError('PTY exited before authenticated proof')
    try:result=json.loads(outpath.read_text().strip())
    except (ValueError,FileNotFoundError):time.sleep(0.2);continue
    break
   assert result and result['authenticated'] and result['uid']==0 and result['stdin_pty'] and result['stdout_pty']
   audit=protected_run('ram-cgroup-'+sequence,SSH+['python3 -'],AUDIT.encode())
   assert any(row['pid']==result['pid'] for row in audit['pids'])
   p.stdin.write(b'\n');p.stdin.flush();p.wait(timeout=30)
   assert p.returncode==0
   return {'pty':result,'audit':audit,'ssh_exit':p.returncode}
  finally:
   if p.poll() is None:p.terminate();p.wait(timeout=15)

def main():
 if sys.argv[1:] not in [['--stage'],['--test']]:
  raise SystemExit('Usage: hardware-37-20261010-stage.py --stage|--test (manager/R7 reviewed source only)')
 private_copy=PRIVATE/'stage37-reviewed.py'
 private_copy.write_bytes(pathlib.Path(__file__).read_bytes());os.chmod(private_copy,0o600)
 if sys.argv[1]=='--test':
  first=pty_test('before-restart')
  restart=protected_run('ram-rescue-owned-restart',SSH+['python3 -'],STOP_START.encode())
  second=pty_test('after-restart')
  result={'authenticated_pty_twice':True,'before_restart':first,'owned_restart':restart,'after_restart':second,'transition_executed':False,'repair_executed':False}
  receipt=PRIVATE/'ram-functional-tests-20261010.json';receipt.write_text(json.dumps(result,indent=2)+'\n');os.chmod(receipt,0o600)
  print('Authenticated PTY, RAM references, child cgroup, owned service stop/restart: PASS; original22 listening; no transition/repair')
  return
 guard=GUARD.read_bytes();auth=(PRIVATE/'auth-return-private-20261010.tar').read_bytes()
 assert hashlib.sha256(guard).hexdigest()==GUARD_SHA
 assert hashlib.sha256(auth).hexdigest()==AUTH_SHA
 payload='GUARD_SHA='+repr(GUARD_SHA)+'\nAUTH_SHA='+repr(AUTH_SHA)+'\nGUARD_B64='+repr(base64.b64encode(guard).decode())+'\nAUTH_B64='+repr(base64.b64encode(auth).decode())+'\n'+REMOTE
 manifest=protected_run('ram-stage-result-20261010',SSH+['python3 -'],payload.encode(),timeout=300)
 print(json.dumps({k:manifest[k] for k in ['stage_only','main_pid','control_group','same_pid1_mount_namespace','root_cwd_exe_ram','network_exactly_unchanged','nextroot_absent']}))

if __name__=='__main__':main()
