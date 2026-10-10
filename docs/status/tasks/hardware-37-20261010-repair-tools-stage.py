#!/usr/bin/env python3
"""Stage reviewed finite-undo wrapper/runtime into existing RAM; no repair.

Only missing exact dpkg-verified tools/libs are copied; differing existing files
refuse. Mounted wrapper --preflight tests RAM undo storage then refuses BUSY3.
"""
import base64
import hashlib
import json
import os
import pathlib
import subprocess

HERE=pathlib.Path(__file__).resolve().parent
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
WRAPPER_DIGEST='0c76f5cba2cce5f088cb3485abee0ca45ed1fa2d0cfdae6091be2792c4927e8c'
V2_SOURCE_DIGEST='c5c0649f6150b869042e8890977bc84f5d2e8f18f736451cfe833cd0e5355216'
V2_BINARY_DIGEST='050316c810573137bd76d313415d9e5877d5dd92f841cdbcbbc09d671905d4e5'
REMOTE=r'''
import base64,glob,hashlib,json,os,pathlib,re,shutil,stat,subprocess
R=pathlib.Path('/run/ngfwrescue');reports=[]
assert os.stat('/').st_dev==2050 and os.stat(R).st_dev==51
assert os.stat('/proc/3940/root').st_dev==51 and not os.path.lexists('/run/nextroot')
def run(args,expected=0):
    p=subprocess.run(args,capture_output=True,text=True,timeout=30)
    reports.append({'command':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr})
    assert p.returncode==expected,'unexpected runtime/refusal exit'
    return p
main=run(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value']).stdout.strip()
assert main.isdigit() and int(main)>1 and os.stat('/proc/'+main+'/root').st_dev==51
tools={name:shutil.which(name) for name in ['df','tail','sha256sum']}
assert all(tools.values()),'required host tool absent'
checksums={}
for path in glob.glob('/var/lib/dpkg/info/*.md5sums'):
    for line in pathlib.Path(path).read_text().splitlines():
        pieces=line.split(None,1)
        if len(pieces)==2:checksums['/'+pieces[1].strip()]=pieces[0]
def copy_missing(source_path):
    source=pathlib.Path(source_path);content=source.read_bytes()
    expected=checksums.get(str(source)) or checksums.get(str(source.resolve()))
    assert expected and hashlib.md5(content).hexdigest()==expected,'source package MD5 differs/unavailable'
    target=R/str(source).lstrip('/')
    assert target.parent.is_dir() and os.stat(target.parent).st_dev==51
    existed=target.exists()
    if existed:
        assert target.is_file() and not target.is_symlink()
        assert target.read_bytes()==content,'refuse different existing RAM file'
    else:
        fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,stat.S_IMODE(source.stat().st_mode))
        with os.fdopen(fd,'wb') as out:out.write(content)
    assert os.stat(target).st_dev==51
    reports.append({'path':str(source),'source_package_MD5':True,'RAM_existed':existed,
                    'bytes':len(content),'sha256':hashlib.sha256(content).hexdigest()})
for name,source in tools.items():
    closure=run(['ldd',source]).stdout
    assert 'not found' not in closure
    copy_missing(source)
    for line in closure.splitlines():
        match=re.search(r'=>\s+(/\S+)',line) or re.match(r'\s*(/\S+)\s+\(',line)
        if match:copy_missing(match.group(1))
    run(['/usr/bin/chroot',str(R),source,'--version'])
parent=R/'root/recovery-private'
if parent.exists():
    st=parent.lstat();assert stat.S_ISDIR(st.st_mode) and st.st_uid==0
    assert stat.S_IMODE(st.st_mode)==0o700 and st.st_dev==51
else:
    assert os.stat(parent.parent).st_dev==51;parent.mkdir(mode=0o700)
for target,content,digest,mode in [(parent/'repair-wrapper.sh',WRAPPER_DATA,WRAPPER_DIGEST,0o700),
                                  (R/'usr/bin/raw-block-read-v2',V2_DATA,V2_BINARY_DIGEST,0o755)]:
    assert hashlib.sha256(content).hexdigest()==digest
    if target.exists():
        assert target.is_file() and not target.is_symlink() and target.read_bytes()==content
    else:
        fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,mode)
        with os.fdopen(fd,'wb') as out:out.write(content)
    assert os.stat(target).st_dev==51
run(['/usr/bin/chroot',str(R),'/usr/bin/raw-block-read-v2','invalid'],2)
run(['/usr/bin/chroot',str(R),'/bin/bash','--noprofile','--norc',
     '/root/recovery-private/repair-wrapper.sh','--preflight'],3)
assert not (parent/'root-repair.undo').exists() and not (parent/'undo-write-preflight.test').exists()
assert run(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value']).stdout.strip()==main
assert os.stat('/proc/3940/root').st_dev==51 and not os.path.lexists('/run/nextroot')
reports.append({'root_device':2050,'RAM_device':51,'rescue_MainPID':int(main),
                'held_shell_pid':3940,'nextroot_absent':True,'undo_created':False,
                'raw_block_capture':False,'repair_started':False})
print(json.dumps(reports,indent=2))
'''


def main():
    os.umask(0o077)
    out=PRIVATE/'ram-finite-undo-tools-stage-20261010.json'
    err=PRIVATE/'ram-finite-undo-tools-stage-20261010.stderr'
    assert not out.exists() and not err.exists()
    wrapper=(HERE/'hardware-37-20261010-repair-wrapper.sh').read_bytes()
    assert hashlib.sha256(wrapper).hexdigest()==WRAPPER_DIGEST
    assert hashlib.sha256((HERE/'hardware-37-20261010-raw-block-read-v2.c').read_bytes()).hexdigest()==V2_SOURCE_DIGEST
    binary=(PRIVATE/'raw-block-read-v2-static').read_bytes()
    assert len(binary)==821424 and hashlib.sha256(binary).hexdigest()==V2_BINARY_DIGEST
    fields='import base64\n'
    for name,value in [('WRAPPER_DIGEST',WRAPPER_DIGEST),('V2_BINARY_DIGEST',V2_BINARY_DIGEST)]:
        fields+=name+'='+repr(value)+'\n'
    for name,value in [('WRAPPER_DATA',wrapper),('V2_DATA',binary)]:
        fields+=name+'=base64.b64decode('+repr(base64.b64encode(value).decode())+')\n'
    cmd=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes',
         '-o','ConnectTimeout=15','root@172.30.126.37','python3','-']
    p=subprocess.run(cmd,input=(fields+REMOTE).encode(),capture_output=True,timeout=240)
    for path,data in [(out,p.stdout),(err,p.stderr)]:path.write_bytes(data);path.chmod(0o600)
    print(json.dumps({'ssh_exit':p.returncode,'stdout_bytes':len(p.stdout),
                      'stdout_sha256':hashlib.sha256(p.stdout).hexdigest(),
                      'stderr_bytes':len(p.stderr),'stderr_sha256':hashlib.sha256(p.stderr).hexdigest()}))
    raise SystemExit(p.returncode)


if __name__=='__main__':main()
