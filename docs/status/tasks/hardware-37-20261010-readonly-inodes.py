#!/usr/bin/env python3
"""Private live mounted ext4 metadata diagnostics. Not an offline snapshot.

debugfs is read-only (no -w); stat/blocks/ncheck expose metadata/path names only,
never dump file contents. No repair, mount, transition, or service/network action.
"""
import hashlib
import json
import os
import pathlib
import subprocess

PRIVATE = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
REMOTE = r'''
import json,os,subprocess,time
dev=os.stat('/').st_dev
assert (os.major(dev),os.minor(dev))==(8,2)
assert not os.path.lexists('/run/nextroot')
assert os.path.exists('/proc/3940')
inodes=[259596,259597,259598,259599,259602]
reports=[]
commands=[['/usr/sbin/debugfs','-R',kind+' <'+str(inode)+'>','/dev/sda2']
          for inode in inodes for kind in ['stat','blocks']]
commands.append(['/usr/sbin/debugfs','-R','ncheck '+' '.join(map(str,inodes)),'/dev/sda2'])
for args in commands:
    start=time.monotonic()
    try:
        p=subprocess.run(args,capture_output=True,text=True,timeout=60)
        reports.append({'command':args,'exit':p.returncode,'stdout':p.stdout,
                        'stderr':p.stderr,'monotonic_seconds':time.monotonic()-start})
    except subprocess.TimeoutExpired as e:
        reports.append({'command':args,'timeout_seconds':60,
                        'stdout':(e.stdout or b'').decode(errors='replace'),
                        'stderr':(e.stderr or b'').decode(errors='replace')})
print(json.dumps({'mounted_root_major_minor':[8,2],'offline_snapshot':False,
                  'target_mutation':False,'records':reports},indent=2))
'''


def main():
    os.umask(0o077)
    out = PRIVATE / 'readonly-inode-mappings-20261010.json'
    err = PRIVATE / 'readonly-inode-mappings-20261010.stderr'
    assert not out.exists() and not err.exists()
    cmd = ['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes',
           '-o','ConnectTimeout=15','root@172.30.126.37','python3','-']
    p = subprocess.run(cmd,input=REMOTE.encode(),capture_output=True,timeout=720)
    for path,data in [(out,p.stdout),(err,p.stderr)]:
        path.write_bytes(data)
        path.chmod(0o600)
    report = {'ssh_exit':p.returncode,'stdout_bytes':len(p.stdout),
              'stdout_sha256':hashlib.sha256(p.stdout).hexdigest(),
              'stderr_bytes':len(p.stderr),
              'stderr_sha256':hashlib.sha256(p.stderr).hexdigest()}
    if p.returncode == 0:
        records=json.loads(p.stdout)['records']
        report['command_exits']=[x.get('exit','timeout') for x in records]
    print(json.dumps(report))
    raise SystemExit(p.returncode)


if __name__ == '__main__':
    main()
