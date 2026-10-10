#!/usr/bin/env python3
"""Future RAM offline audit/native metadata/raw preservation; never repairs.

Requires independently released transition and exact-source review. No mounts,
network changes, reboot, SMART query or original-root write is performed.
"""
import argparse
import gzip
import hashlib
import json
import os
import pathlib
import shlex
import subprocess

PRIVATE = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
SSH = ['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes',
       '-o','HostKeyAlias=172.30.126.37','-o','ConnectTimeout=15','-p','2222',
       '-o','ServerAliveInterval=15','-o','ServerAliveCountMax=3',
       'root@172.30.126.37']
REMOTE_PARENT = '/root/recovery-private'
IMAGE = REMOTE_PARENT + '/root-before-repair.e2i'
BLOCKS = [15505493,15503361,15503362,15503363,15503874,15503875,15505492]
IDENTITY = r'''
import hashlib,json,os,pathlib,re,stat,subprocess,traceback
os.umask(0o077)
ENV=dict(os.environ,LC_ALL='C',PATH='/usr/sbin:/usr/bin:/sbin:/bin')
assert os.stat('/').st_dev==51 and os.stat('/proc/1/root').st_dev==51
assert os.stat('/proc/3940/root').st_dev==51
st=os.stat('/dev/sda2');assert stat.S_ISBLK(st.st_mode) and st.st_rdev==2050
assert int(pathlib.Path('/sys/class/block/sda2/size').read_text())*512==63510503424
assert subprocess.check_output(['systemctl','show','ngfw-rescue.service','-p','MainPID','--value'],text=True).strip()=='3866'
assert os.stat('/proc/3866/root').st_dev==51
for name,digest in [('/usr/bin/block-check','02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c'),
                    ('/usr/bin/nsfs-check','2907bb0345fb9dad199e48b2539ce9038b24fbafa00fa9672b4074239a6ac681'),
                    ('/usr/bin/offline-audit.sh','e5608afb712bd86251395800a4e27d9d7f1d8df75d75d0f2c41d827ada89affc'),
                    ('/usr/bin/kernel-read','3ebe9a3e308b2d05485035582c961eb272f826fa5d3c059080f82a85d13bdd2d'),
                    ('/usr/bin/raw-block-read-v3','d8d8aa8316e7fff7bc568d44979e70e66f12742d1a826c6111d6f109fad1c80e')]:
    p=pathlib.Path(name);assert p.is_file() and not p.is_symlink() and os.stat(p).st_dev==51
    assert hashlib.sha256(p.read_bytes()).hexdigest()==digest
parent=pathlib.Path(REMOTE_PARENT);st=parent.lstat()
assert stat.S_ISDIR(st.st_mode) and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o700 and st.st_dev==51
def run(args,timeout=180):
    return subprocess.run(args,capture_output=True,env=ENV,timeout=timeout)
def entry(args,p):
    return {'command':args,'exit':p.returncode,'stdout':p.stdout.decode(errors='replace'),
            'stderr':p.stderr.decode(errors='replace')}
def audit():
    args=['/bin/bash','--noprofile','--norc','/usr/bin/offline-audit.sh']
    p=run(args);report['audits'].append(entry(args,p))
    assert p.returncode==0 and b'FINAL_BLOCK_GUARD exit=0' in p.stdout,'offline audit/guard refused'
def checksum(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda:f.read(1024*1024),b''):h.update(chunk)
    return h.hexdigest()
def new_ram_file(path,data):
    assert not os.path.lexists(path) and os.stat(path.parent).st_dev==51
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
    return {'path':str(path),'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest()}
def health():
    p=run(['/usr/bin/kernel-read']);assert p.returncode==0 and not p.stderr
    counter=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip()
    return {'ioerr_counter':counter,'kernel_sha256':hashlib.sha256(p.stdout).hexdigest(),
            'kernel':p.stdout.decode(errors='replace')}
def storage_errors(text):
    pattern=r'(?i)(ata\d.*(error|failed|reset|timeout|unc)|scsi.*(error|failed|reset|timeout)|I/O error|Buffer I/O|blk_update_request|end_request|uncorrectable|critical medium error)'
    return [line for line in text.splitlines() if re.search(pattern,line)]
report={'audits':[],'commands':[],'files':[],'repair_started':False,'metadata_requires_review':True}
'''
REMOTE = r'''
try:
    audit()
    report['health_before']=health()
    if DIAGNOSE:
        args=['/usr/sbin/e2fsck','-f','-n','/dev/sda2']
        p=run(args,600);report['readonly_fsck']=entry(args,p)
        report['filesystem_consistent']=p.returncode==0
        assert p.returncode in [0,4,12],'unexpected readonly fsck exit; inspect retained diagnostics'
    if PRESERVE:
        assert not os.path.lexists(parent/'root-repair.undo') and not os.path.lexists(parent/'undo-write-preflight.test')
        assert not os.path.lexists(IMAGE)
        for block in BLOCKS:assert not os.path.lexists(parent/('root-block-'+str(block)+'.bin'))
        capacity=os.statvfs(parent)
        assert capacity.f_bavail*capacity.f_frsize>1610612736,'RAM capacity insufficient'
        for inode in [259594,259595,259596,259597,259598,259599,259600,259602,259603]:
            for selector in ['stat','blocks']:
                args=['/usr/sbin/debugfs','-R',selector+' <'+str(inode)+'>','/dev/sda2']
                p=run(args);report['commands'].append(entry(args,p));assert p.returncode==0
        args=['/usr/sbin/debugfs','-R','ncheck 259594 259595 259596 259597 259598 259599 259600 259602 259603','/dev/sda2']
        p=run(args);report['commands'].append(entry(args,p));assert p.returncode==0
        args=['/usr/sbin/e2image','/dev/sda2',IMAGE]
        p=run(args,600);report['commands'].append(entry(args,p));assert p.returncode==0,'native image failed; no fallback override'
        image=pathlib.Path(IMAGE);st=image.lstat()
        assert stat.S_ISREG(st.st_mode) and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o600 and st.st_dev==51
        assert 0<st.st_size<=1073741824,'unexpected native nominal image size'
        with image.open('rb') as f:os.fsync(f.fileno())
        report['files'].append({'path':IMAGE,'bytes':st.st_size,'allocated_bytes':st.st_blocks*512,
                                'sha256':checksum(image),'scope':'native core metadata; excludes user payload and directory/extent/EA/journal data blocks'})
        for block in BLOCKS:
            args=['/usr/bin/raw-block-read-v3',str(block)]
            p=run(args);assert p.returncode==0 and not p.stderr and len(p.stdout)==4096
            item=new_ram_file(parent/('root-block-'+str(block)+'.bin'),p.stdout)
            item.update({'block':block,'all_zero':not any(p.stdout)})
            report['files'].append(item)
        fd=os.open(parent,os.O_RDONLY|os.O_DIRECTORY)
        try:os.fsync(fd)
        finally:os.close(fd)
    report['health_after']=health()
    report['new_storage_error_lines']=[line for line in storage_errors(report['health_after']['kernel'])
                                        if line not in storage_errors(report['health_before']['kernel'])]
    assert report['health_after']['ioerr_counter']==report['health_before']['ioerr_counter'],'storage error counter changed'
    assert not report['new_storage_error_lines'],'new physical storage error'
    audit()
    report['status']='PASS_PRESERVATION' if PRESERVE else ('PASS_DIAGNOSTIC_CAPTURE' if DIAGNOSE else 'PASS_AUDIT_ONLY')
except Exception as exc:
    report['status']='REFUSE';report['failure_type']=type(exc).__name__;report['failure']=str(exc)
    report['traceback']=traceback.format_exc()
print(json.dumps(report,indent=2))
raise SystemExit(0 if report['status'].startswith('PASS_') else 1)
'''
STREAM = r'''
import os,pathlib,stat,sys
assert os.stat('/').st_dev==51 and os.stat('/proc/1/root').st_dev==51
p=pathlib.Path(PATH);st=p.lstat()
assert stat.S_ISREG(st.st_mode) and st.st_dev==51 and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o600
assert st.st_size==EXPECTED_SIZE
with p.open('rb') as f:
    for chunk in iter(lambda:f.read(1024*1024),b''):sys.stdout.buffer.write(chunk)
sys.stdout.buffer.flush()
'''


def sync_dir():
    fd=os.open(PRIVATE,os.O_RDONLY|os.O_DIRECTORY)
    try:os.fsync(fd)
    finally:os.close(fd)


def save(name,data):
    path=PRIVATE/name;assert not os.path.lexists(path)
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
    sync_dir()
    return {'file':str(path),'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest()}


def code(fields,body):
    return '\n'.join(name+'='+repr(value) for name,value in fields.items())+'\n'+body


def transfer(item):
    name=pathlib.Path(item['path']).name
    path=PRIVATE/(name+'.gz' if item['path']==IMAGE else name)
    assert not os.path.lexists(path)
    body=code({'PATH':item['path'],'EXPECTED_SIZE':item['bytes']},STREAM)
    p=subprocess.Popen(SSH+['python3 -B -c '+shlex.quote(body)],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    total=0;source_hash=hashlib.sha256()
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    try:
        with os.fdopen(fd,'wb') as output:
            writer=gzip.GzipFile(fileobj=output,mode='wb',mtime=0) if item['path']==IMAGE else output
            try:
                for chunk in iter(lambda:p.stdout.read(1024*1024),b''):
                    total+=len(chunk);assert total<=item['bytes'],'transfer exceeds declared size'
                    source_hash.update(chunk);writer.write(chunk)
            finally:
                if writer is not output:writer.close()
            output.flush();os.fsync(output.fileno())
        stderr=p.stderr.read();status=p.wait(timeout=30);sync_dir()
    except BaseException:
        # Only the controller subprocess created above is stopped on failure.
        if p.poll() is None:
            p.terminate()
            try:p.wait(timeout=10)
            except subprocess.TimeoutExpired:p.kill();p.wait(timeout=10)
        raise
    assert status==0 and not stderr and total==item['bytes'] and source_hash.hexdigest()==item['sha256'],'incomplete transfer'
    read_hash=hashlib.sha256();read_size=0
    opener=gzip.open if item['path']==IMAGE else open
    with opener(path,'rb') as f:
        for chunk in iter(lambda:f.read(1024*1024),b''):read_hash.update(chunk);read_size+=len(chunk)
    assert read_size==item['bytes'] and read_hash.hexdigest()==item['sha256'],'offhost readback differs'
    stored_hash=hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda:f.read(1024*1024),b''):stored_hash.update(chunk)
    return {'file':str(path),'stored_bytes':path.stat().st_size,'stored_sha256':stored_hash.hexdigest(),
            'nominal_bytes':read_size,'nominal_sha256':read_hash.hexdigest(),'mode':'0600','fsynced':True}


def main():
    os.umask(0o077)
    parser=argparse.ArgumentParser()
    mode=parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--audit-only',action='store_true')
    mode.add_argument('--diagnose-after-transition',action='store_true')
    mode.add_argument('--preserve-after-transition',action='store_true')
    args=parser.parse_args()
    assert os.geteuid()==0 and PRIVATE.stat().st_mode&0o777==0o700
    capacity=os.statvfs(PRIVATE)
    assert capacity.f_bfree*capacity.f_frsize>1610612736,'controller space gate retained'
    name=('offline-preservation-20261010' if args.preserve_after_transition else
          ('offline-diagnostic-first-20261010' if args.diagnose_after_transition else 'offline-audit-first-20261010'))
    assert not os.path.lexists(PRIVATE/(name+'.json')) and not os.path.lexists(PRIVATE/(name+'.stderr'))
    fields={'REMOTE_PARENT':REMOTE_PARENT,'IMAGE':IMAGE,'BLOCKS':BLOCKS,'PRESERVE':args.preserve_after_transition,
            'DIAGNOSE':args.diagnose_after_transition}
    p=subprocess.run(SSH+['python3 -B -'],input=code(fields,IDENTITY+REMOTE).encode(),capture_output=True,timeout=1000)
    receipt=save(name+'.json',p.stdout);error=save(name+'.stderr',p.stderr)
    public={'ssh_exit':p.returncode,'receipt':receipt,'stderr':error,'repair_started':False}
    if p.returncode:
        print(json.dumps(public));raise SystemExit(p.returncode)
    report=json.loads(p.stdout);assert report['status'].startswith('PASS_')
    if args.preserve_after_transition:
        transfers=[transfer(item) for item in report['files']]
        public['offhost_receipt']=save('offline-preservation-transfers-20261010.json',json.dumps(transfers,indent=2).encode())
        public['verified_offhost_files']=len(transfers)
    print(json.dumps(public))


if __name__=='__main__':
    main()
