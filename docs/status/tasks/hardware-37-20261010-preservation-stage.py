#!/usr/bin/env python3
"""Prepare reviewed read helpers in existing RAM, never capture disk blocks.

No repair/transition/mount/service/network action. Existing e2fsprogs and their
loader closures are verified without replacing binaries, libraries or PID1.
"""
import base64
import hashlib
import json
import os
import pathlib
import subprocess

HERE = pathlib.Path(__file__).resolve().parent
PRIVATE = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
READERS = {
    'raw-block-read': (
        '1a6e5a23215376f7daa60c090c5b6ea26709db8d73ec58df5f55aa337c83535c',
        '3256798046a2f81d1b06b3a1b5e93837c7da35a1bcc472dcc095c8330f6a1ea2',821424),
    'kernel-read': (
        '78f74c244897cf6009f63570e11fa0c6e91b3da6bcc14dc7b89917b1ff3add4b',
        '3ebe9a3e308b2d05485035582c961eb272f826fa5d3c059080f82a85d13bdd2d',856288),
}
REMOTE = r'''
import base64,hashlib,json,os,pathlib,subprocess
R=pathlib.Path('/run/ngfwrescue')
root=os.stat('/').st_dev;ram=os.stat(R).st_dev
assert (os.major(root),os.minor(root))==(8,2) and root!=ram
for path,identity in [('/dev/sda2',(8,2)),('/dev/sda',(8,0))]:
    st=os.stat(path)
    assert (os.major(st.st_rdev),os.minor(st.st_rdev))==identity
assert not os.path.lexists('/run/nextroot') and pathlib.Path('/proc/3940').exists()
assert os.stat('/proc/3940/root').st_dev==ram
reports=[]
def run(args,expected=0):
    p=subprocess.run(args,capture_output=True,text=True,timeout=30)
    reports.append({'command':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr})
    assert p.returncode==expected, 'unexpected runtime selector exit'
    return p
main=run(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value']).stdout.strip()
assert main.isdigit() and int(main)>1 and os.stat('/proc/'+main+'/root').st_dev==ram
assert run(['systemctl','is-active','ngfw-rescue.service']).stdout.strip()=='active'
# Exact matching reviewed minimal-debugfs archive files already present: no extraction.
for relative,digest in [('usr/sbin/debugfs','864e1d7b445e7b5bfc831da78330dbcafc590fa82b89ea9de60b7527f989954f'),
                         ('usr/lib/x86_64-linux-gnu/libss.so.2','c8ecc8838857db598fd959bd2623730e9614e547a4f771f56f3a815a6b3ec251')]:
    path=R/relative
    assert path.is_file() and os.stat(path).st_dev==ram
    assert hashlib.sha256(path.read_bytes()).hexdigest()==digest
    reports.append({'existing_RAM_file':relative,'sha256':digest,'replaced':False})
loader='/lib64/ld-linux-x86-64.so.2'
assert (R/loader.lstrip('/')).is_file()
for executable in ['/usr/sbin/debugfs','/usr/sbin/e2image','/usr/sbin/e2undo','/usr/sbin/e2fsck']:
    run(['/usr/bin/chroot',str(R),loader,'--list',executable])
run(['/usr/bin/chroot',str(R),'/usr/sbin/debugfs','-V'])
run(['/usr/bin/chroot',str(R),'/usr/sbin/e2fsck','-V'])
for name,content,digest in PAYLOAD:
    path=R/'usr/bin'/name
    assert os.stat(path.parent).st_dev==ram and hashlib.sha256(content).hexdigest()==digest
    if path.exists():
        assert path.is_file() and not path.is_symlink()
        assert hashlib.sha256(path.read_bytes()).hexdigest()==digest, 'refuse different existing reader'
    else:
        fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o755)
        with os.fdopen(fd,'wb') as out: out.write(content)
    assert os.stat(path).st_dev==ram and hashlib.sha256(path.read_bytes()).hexdigest()==digest
    reports.append({'RAM_reader':name,'sha256':digest,'bytes':len(content)})
    # Invalid argument returns before reading any block/kernel ring bytes.
    run(['/usr/bin/chroot',str(R),'/usr/bin/'+name,'invalid'],2)
run(['/usr/bin/chroot',str(R),'/usr/bin/block-check','/dev/sda2','8','2'],3)
assert run(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value']).stdout.strip()==main
assert os.stat('/proc/3940/root').st_dev==ram and not os.path.lexists('/run/nextroot')
reports.append({'root_major_minor':[8,2],'RAM_major_minor':[os.major(ram),os.minor(ram)],
                'rescue_MainPID':int(main),'held_shell_pid':3940,'nextroot_absent':True,
                'disk_block_capture':False,'kernel_ring_capture':False,'ext4_repair':False})
print(json.dumps(reports,indent=2))
'''


def main():
    os.umask(0o077)
    out = PRIVATE / 'ram-preservation-readers-stage-20261010.json'
    err = PRIVATE / 'ram-preservation-readers-stage-20261010.stderr'
    assert not out.exists() and not err.exists()
    fields='import base64\nPAYLOAD=[]\n'
    for name,(source_digest,binary_digest,size) in READERS.items():
        source=HERE/('hardware-37-20261010-'+name+'.c')
        assert hashlib.sha256(source.read_bytes()).hexdigest()==source_digest
        content=(PRIVATE/(name+'-static')).read_bytes()
        assert len(content)==size and hashlib.sha256(content).hexdigest()==binary_digest
        fields+='PAYLOAD.append(('+repr(name)+',base64.b64decode('+repr(base64.b64encode(content).decode())+'),'+repr(binary_digest)+'))\n'
    cmd=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes',
         '-o','ConnectTimeout=15','root@172.30.126.37','python3','-']
    p=subprocess.run(cmd,input=(fields+REMOTE).encode(),capture_output=True,timeout=240)
    for path,data in [(out,p.stdout),(err,p.stderr)]:
        path.write_bytes(data)
        path.chmod(0o600)
    print(json.dumps({'ssh_exit':p.returncode,'stdout_bytes':len(p.stdout),
                      'stdout_sha256':hashlib.sha256(p.stdout).hexdigest(),
                      'stderr_bytes':len(p.stderr),
                      'stderr_sha256':hashlib.sha256(p.stderr).hexdigest()}))
    raise SystemExit(p.returncode)


if __name__ == '__main__':
    main()
