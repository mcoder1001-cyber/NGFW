#!/usr/bin/env python3
"""Stage only reviewed readonly audit helpers into the existing host37 RAM tree.

Does not transition, mount, stop services, change networking, or check/repair ext4.
Full process audit is intentionally reserved for the later offline phase.
"""
import base64
import hashlib
import json
import os
import pathlib
import subprocess

HERE = pathlib.Path(__file__).resolve().parent
PRIVATE = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
STATIC = PRIVATE / 'nsfs-check-static'
STATIC_DIGEST = '2907bb0345fb9dad199e48b2539ce9038b24fbafa00fa9672b4074239a6ac681'
SOURCE_DIGEST = '36e12485aa4e1ca3a85e0a1760fff89604c0d4b3f9b5b190aeee873bc45a66ee'
AUDIT_DIGEST = 'e5608afb712bd86251395800a4e27d9d7f1d8df75d75d0f2c41d827ada89affc'
GUARD_DIGEST = '02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c'
SSH = ['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes',
       '-o', 'ConnectTimeout=15', 'root@172.30.126.37', 'python3', '-']

REMOTE = r'''
import base64,hashlib,json,os,pathlib,subprocess
R=pathlib.Path('/run/ngfwrescue')
reports=[]
root=os.stat('/').st_dev
ram=os.stat(R).st_dev
assert (os.major(root),os.minor(root))==(8,2)
assert (os.major(os.stat('/dev/sda2').st_rdev),os.minor(os.stat('/dev/sda2').st_rdev))==(8,2)
assert (os.major(os.stat('/dev/sda').st_rdev),os.minor(os.stat('/dev/sda').st_rdev))==(8,0)
assert ram!=root and not os.path.lexists('/run/nextroot')
assert pathlib.Path('/proc/3940').exists(), 'held PTY absent'
def run(args,expected=0):
    p=subprocess.run(args,capture_output=True,text=True,timeout=30)
    reports.append({'command':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr})
    assert p.returncode==expected, 'unexpected exit for '+args[0]
    return p
main=run(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value']).stdout.strip()
assert main.isdigit() and int(main)>1
assert os.stat('/proc/'+main+'/root').st_dev==ram
assert run(['systemctl','is-active','ngfw-rescue.service']).stdout.strip()=='active'
for name in ['bash','cat','stat','readlink']:
    path=R/'usr/bin'/name
    assert path.is_file() and os.access(path,os.X_OK)
    run(['/usr/bin/chroot',str(R),'/usr/bin/'+name,'--version'])
guard=R/'usr/local/sbin/block-check'
assert hashlib.sha256(guard.read_bytes()).hexdigest()==GUARD_DIGEST
for relative,data,digest in [('usr/bin/nsfs-check',STATIC_DATA,STATIC_DIGEST),
                             ('usr/bin/offline-audit.sh',AUDIT_DATA,AUDIT_DIGEST),
                             ('usr/bin/block-check',guard.read_bytes(),GUARD_DIGEST)]:
    target=R/relative
    assert target.parent.is_dir() and os.stat(target.parent).st_dev==ram
    assert hashlib.sha256(data).hexdigest()==digest
    if target.exists():
        assert target.is_file() and not target.is_symlink()
        assert hashlib.sha256(target.read_bytes()).hexdigest()==digest, 'refuse different existing helper'
    else:
        fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o755)
        with os.fdopen(fd,'wb') as f: f.write(data)
    assert os.stat(target).st_dev==ram
    assert hashlib.sha256(target.read_bytes()).hexdigest()==digest
run(['/usr/bin/chroot',str(R),'/usr/bin/bash','-n','/usr/bin/offline-audit.sh'])
run(['/usr/bin/chroot',str(R),'/usr/bin/nsfs-check','/etc/os-release','8','2'],2)
selector=run(['/usr/bin/chroot',str(R),'/usr/bin/nsfs-check','/proc/self/ns/mnt','8','2'],3)
assert 'type=0x20000' in selector.stdout
assert 'oldroot_mounts=1' in selector.stdout and 'malformed=0' in selector.stdout
# Mounted-negative selector3 is expected; later selector0 alone is insufficient.
run(['/usr/bin/chroot',str(R),'/usr/bin/block-check','/dev/sda2','8','2'],3)
assert run(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value']).stdout.strip()==main
assert pathlib.Path('/proc/3940').exists() and not os.path.lexists('/run/nextroot')
reports.append({'root_major_minor':[os.major(root),os.minor(root)],
                'RAM_major_minor':[os.major(ram),os.minor(ram)],
                'rescue_MainPID':int(main),'held_shell_pid':3940,
                'nextroot_absent':True,'full_offline_audit_run':False})
print(json.dumps(reports,indent=2))
'''


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    os.umask(0o077)
    stdout = PRIVATE / 'ram-audit-stage-validated-20261010.json'
    stderr = PRIVATE / 'ram-audit-stage-validated-20261010.stderr'
    assert not stdout.exists() and not stderr.exists(), 'refuse overwrite of previous receipts'
    source = HERE / 'hardware-37-20261010-nsfs-check.c'
    audit = HERE / 'hardware-37-20261010-offline-audit.sh'
    assert digest(source.read_bytes()) == SOURCE_DIGEST
    assert digest(audit.read_bytes()) == AUDIT_DIGEST
    static = STATIC.read_bytes()
    assert len(static) == 1014992 and digest(static) == STATIC_DIGEST
    fields = 'import base64\n'
    for name, value in [('STATIC_DIGEST',STATIC_DIGEST),('AUDIT_DIGEST',AUDIT_DIGEST),
                        ('GUARD_DIGEST',GUARD_DIGEST)]:
        fields += name + '=' + repr(value) + '\n'
    for name, value in [('STATIC_DATA',static),('AUDIT_DATA',audit.read_bytes())]:
        fields += name + '=base64.b64decode(' + repr(base64.b64encode(value).decode()) + ')\n'
    result = subprocess.run(SSH,input=(fields+REMOTE).encode(),capture_output=True,timeout=180)
    for path, data in [(stdout,result.stdout),(stderr,result.stderr)]:
        assert not path.exists(), 'refuse overwrite of a previous receipt'
        path.write_bytes(data)
        os.chmod(path,0o600)
    print(json.dumps({'ssh_exit':result.returncode,'stdout_bytes':len(result.stdout),
                      'stdout_sha256':digest(result.stdout),'stderr_bytes':len(result.stderr),
                      'stderr_sha256':digest(result.stderr)}))
    raise SystemExit(result.returncode)


if __name__ == '__main__':
    main()
