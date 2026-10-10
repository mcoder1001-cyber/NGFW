#!/usr/bin/env python3
"""Stage exact reviewed V3 static reader in existing RAM; invalid selector only."""
import base64
import hashlib
import json
import os
import pathlib
import subprocess

HERE = pathlib.Path(__file__).resolve().parent
PRIVATE = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
SOURCE_DIGEST = 'b76411e2c3279fb3b7dc0fdfdc4f64c510491fe585e7a0ad00c0eb3d07facda9'
BINARY_DIGEST = 'd8d8aa8316e7fff7bc568d44979e70e66f12742d1a826c6111d6f109fad1c80e'
GUARD_DIGEST = '02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c'
REMOTE = r'''
import base64,hashlib,json,os,pathlib,stat,subprocess
R=pathlib.Path('/run/ngfwrescue')
assert os.stat('/').st_dev==2050 and os.stat(R).st_dev==51
assert os.stat('/proc/3940/root').st_dev==51 and not os.path.lexists('/run/nextroot')
device=os.stat('/dev/sda2');assert stat.S_ISBLK(device.st_mode) and device.st_rdev==2050
assert int(pathlib.Path('/sys/class/block/sda2/size').read_text())*512==63510503424
main=subprocess.check_output(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value'],text=True).strip()
assert main=='3866' and os.stat('/proc/'+main+'/root').st_dev==51
target=R/'usr/bin/raw-block-read-v3';parent=target.parent
assert parent.is_dir() and os.stat(parent).st_dev==51
assert hashlib.sha256(DATA).hexdigest()==BINARY_DIGEST and len(DATA)==821424
existed=target.exists()
if existed:
    st=target.lstat();assert stat.S_ISREG(st.st_mode) and st.st_uid==0 and not stat.S_IMODE(st.st_mode)&0o022
    assert target.read_bytes()==DATA,'refuse different existing V3 reader'
else:
    assert not target.is_symlink()
    fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o755)
    with os.fdopen(fd,'wb') as f:f.write(DATA)
assert os.stat(target).st_dev==51 and hashlib.sha256(target.read_bytes()).hexdigest()==BINARY_DIGEST
assert hashlib.sha256((R/'usr/bin/block-check').read_bytes()).hexdigest()==GUARD_DIGEST
checks=[]
for args,expected in [(['/usr/bin/chroot',str(R),'/usr/bin/raw-block-read-v3','invalid'],2),
                      (['/usr/bin/chroot',str(R),'/usr/bin/block-check','/dev/sda2','8','2'],3)]:
    p=subprocess.run(args,capture_output=True,text=True,timeout=30)
    checks.append({'command':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr})
    assert p.returncode==expected
    if expected==2:assert not p.stdout and not p.stderr
assert subprocess.check_output(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value'],text=True).strip()==main
assert os.stat('/proc/3940/root').st_dev==51 and not os.path.lexists('/run/nextroot')
print(json.dumps({'reader_bytes':len(DATA),'reader_sha256':BINARY_DIGEST,'RAM_file_existed':existed,
                  'RAM_device':51,'original_root_device':2050,'rescue_MainPID':int(main),
                  'held_shell_pid':3940,'nextroot_absent':True,'checks':checks,
                  'valid_block_capture':False,'repair_started':False},indent=2))
'''


def main():
    os.umask(0o077)
    out=PRIVATE/'ram-v3-reader-stage-20261010.json'
    err=PRIVATE/'ram-v3-reader-stage-20261010.stderr'
    assert not out.exists() and not err.exists()
    source=(HERE/'hardware-37-20261010-raw-block-read-v3.c').read_bytes()
    assert hashlib.sha256(source).hexdigest()==SOURCE_DIGEST
    binary=(PRIVATE/'raw-block-read-v3-static').read_bytes()
    assert len(binary)==821424 and hashlib.sha256(binary).hexdigest()==BINARY_DIGEST
    prefix='import base64\nDATA=base64.b64decode('+repr(base64.b64encode(binary).decode())+')\n'
    prefix+='BINARY_DIGEST='+repr(BINARY_DIGEST)+'\nGUARD_DIGEST='+repr(GUARD_DIGEST)+'\n'
    cmd=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15',
         'root@172.30.126.37','python3','-']
    p=subprocess.run(cmd,input=(prefix+REMOTE).encode(),capture_output=True,timeout=120)
    for path,data in [(out,p.stdout),(err,p.stderr)]:
        path.write_bytes(data);path.chmod(0o600)
    print(json.dumps({'ssh_exit':p.returncode,'stdout_bytes':len(p.stdout),
                      'stdout_sha256':hashlib.sha256(p.stdout).hexdigest(),
                      'stderr_bytes':len(p.stderr),'stderr_sha256':hashlib.sha256(p.stderr).hexdigest()}))
    raise SystemExit(p.returncode)


if __name__=='__main__':
    main()
