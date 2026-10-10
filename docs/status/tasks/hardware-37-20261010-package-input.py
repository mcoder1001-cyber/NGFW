#!/usr/bin/env python3
"""Check exact11 archives locally; later upload+simulate only after clean return.

Target mode requires reviewed post-repair originalroot, management+clock and
existing start-policy/VPP-mask safeguards. It never installs/activates packages,
creates/restores those safeguards, changes firewall/network/SSH, or binds NICs.
"""
import argparse
import datetime
import hashlib
import json
import os
import pathlib
import re
import shlex
import subprocess
import time

BASE = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
PRIVATE = BASE / 'recovery-private/host-37'
SOURCE = '2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c'
PRODUCT_VERSION = '0.1.0~dev+2045ab8b3d2f'
VPP_VERSION = '26.06-release+ngfw3'
EXPECTED_PRODUCTS = {'ngfw-agent','ngfw-api','ngfw-web','ngfw-meta'}
EXPECTED_VPP = {'libvppinfra','python3-vpp-api','vpp-crypto-engines','vpp-drivers',
                'vpp-plugin-core','vpp-plugin-dpdk','vpp'}
SSH = ['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes',
       '-o','ConnectTimeout=15','root@172.30.126.37']
REMOTE_DIRECTORY = '/run/ngfw-hardware-37-install-input'
POLICY = b'#!/bin/sh\nexit 101\n'
# Actual post-return tool preflight found these required commands absent.
EXTRA_PACKAGES = ['jq','pciutils','driverctl','curl','nftables']

PREFLIGHT = r'''
import hashlib,json,os,pathlib,stat,subprocess,time
assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
assert abs(time.time()-EXPECTED_CONTROLLER_EPOCH)<60, 'trusted target time prerequisite missing'
def output(args):return subprocess.check_output(args,text=True,timeout=30)
assert output(['findmnt','-no','FSTYPE','/run']).strip()=='tmpfs'
state=output(['tune2fs','-l','/dev/sda2'])
matches=[line.split(':',1)[1].strip() for line in state.splitlines() if line.startswith('Filesystem state:')]
assert matches==['clean'], 'filesystem not clean; no upload'
route=json.loads(output(['ip','-j','route','get','172.30.126.195']))
assert route[0].get('dev')=='enp12s0' and route[0].get('prefsrc')=='172.30.126.37'
device=pathlib.Path('/sys/class/net/enp12s0/device')
assert device.resolve().name=='0000:0c:00.0' and (device/'driver').resolve().name=='igc'
policy=pathlib.Path('/usr/sbin/policy-rc.d');st=policy.lstat()
assert stat.S_ISREG(st.st_mode) and st.st_uid==0 and not st.st_mode&0o022
assert policy.read_bytes()==POLICY, 'exact reviewed start-policy101 guard missing'
mask=pathlib.Path('/etc/systemd/system/vpp.service')
assert mask.is_symlink() and os.readlink(mask)=='/dev/null', 'persistent VPP mask missing'
load=output(['systemctl','show','vpp.service','-p','LoadState','--value']).strip()
active=output(['systemctl','show','vpp.service','-p','ActiveState','--value']).strip()
assert load=='masked' and active in ['inactive','failed'], 'VPP must be suppressed'
root=pathlib.Path(REMOTE_DIRECTORY)
assert not root.exists(), 'refuse an existing upload directory'
root.mkdir(mode=0o700)
assert os.stat(root).st_dev==os.stat('/run').st_dev
print(json.dumps({'filesystem_state':'clean','original_root':[8,2],
                  'management_interface':'enp12s0','management_PCI':'0000:0c:00.0',
                  'policy101_exact':True,'VPP_persistent_mask':True,'VPP_active':active,
                  'trusted_clock_offset_seconds':time.time()-EXPECTED_CONTROLLER_EPOCH,
                  'upload_directory':str(root),'package_install':False}))
'''

SIMULATE = r'''
import hashlib,json,os,pathlib,subprocess
root=pathlib.Path(REMOTE_DIRECTORY)
assert root.is_dir() and not root.is_symlink() and os.stat(root).st_uid==0
assert sorted(p.name for p in root.iterdir())==sorted(x['file'] for x in EXPECTED_ARCHIVES)
for item in EXPECTED_ARCHIVES:
    path=root/item['file'];assert path.is_file() and not path.is_symlink()
    assert path.stat().st_size==item['bytes']
    assert hashlib.sha256(path.read_bytes()).hexdigest()==item['sha256']
assert pathlib.Path('/usr/sbin/policy-rc.d').read_bytes()==POLICY
assert os.readlink('/etc/systemd/system/vpp.service')=='/dev/null'
args=['apt-get','-s','--no-remove','--no-install-recommends',
      '-o','Dir::Cache::pkgcache=','-o','Dir::Cache::srcpkgcache=',
      'install']+['./'+item['file'] for item in EXPECTED_ARCHIVES]+EXTRA_PACKAGES
p=subprocess.run(args,cwd=root,capture_output=True,text=True,timeout=180)
print(json.dumps({'command':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr,
                  'package_install':False,'service_activation':False,
                  'requires_plan_review_before_install':True,'additional_named_packages':EXTRA_PACKAGES},indent=2))
raise SystemExit(p.returncode)
'''


def checksum(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda:f.read(1024*1024),b''):h.update(block)
    return h.hexdigest()


def local_archives():
    product=json.loads((BASE/'runtime-fixed/manifest.json').read_text())
    vpp=json.loads((BASE/'vpp-runtime/INSTALL-SET.json').read_text())
    assert product['source_sha']==SOURCE and product['version']==PRODUCT_VERSION
    assert {x['Package'] for x in product['packages']}==EXPECTED_PRODUCTS
    assert {x['package'] for x in vpp['packages']}==EXPECTED_VPP
    records=[]
    for directory,items in [('runtime-fixed',product['packages']),('vpp-runtime',vpp['packages'])]:
        for item in items:
            name=item['file'];assert re.fullmatch(r'[A-Za-z0-9.+~_-]+\.deb',name)
            path=BASE/directory/name;assert path.is_file() and not path.is_symlink()
            digest=checksum(path);assert digest==item['sha256']
            fields={key:subprocess.check_output(['dpkg-deb','-f',str(path),key],text=True).strip()
                    for key in ['Package','Version','Architecture']}
            assert fields['Package']==item.get('Package',item.get('package'))
            assert fields['Version']==(PRODUCT_VERSION if directory=='runtime-fixed' else VPP_VERSION)
            assert fields['Architecture'] in ['amd64','all']
            records.append({'file':name,'sha256':digest,'bytes':path.stat().st_size,
                            'path':str(path),'control':fields})
    assert len(records)==11 and len({x['file'] for x in records})==11
    return records


def remote(script,fields):
    code='\n'.join(name+'='+repr(value) for name,value in fields.items())+'\n'+script
    return subprocess.run(SSH+['python3 -c '+shlex.quote(code)],capture_output=True,timeout=240)


def save(name,data):
    path=PRIVATE/name;assert not os.path.lexists(path)
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
    fd=os.open(PRIVATE,os.O_RDONLY|os.O_DIRECTORY)
    try:os.fsync(fd)
    finally:os.close(fd)
    return {'file':str(path),'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest()}


def main():
    os.umask(0o077)
    parser=argparse.ArgumentParser()
    modes=parser.add_mutually_exclusive_group(required=True)
    modes.add_argument('--check-local',action='store_true')
    modes.add_argument('--upload-simulate-after-recovery',action='store_true')
    mode=parser.parse_args()
    stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    archives=local_archives()
    receipt=save('install-input-local-'+stamp+'.json',json.dumps(archives,indent=2).encode())
    report={'local_archives':11,'local_receipt':receipt,'target_contacted':False,
            'additional_named_packages':EXTRA_PACKAGES}
    if mode.check_local:
        print(json.dumps(report));return
    # Manager-reviewed recovery/return and start-policy/mask setup are external prerequisites.
    fields={'EXPECTED_CONTROLLER_EPOCH':time.time(),'POLICY':POLICY,
            'REMOTE_DIRECTORY':REMOTE_DIRECTORY}
    p=remote(PREFLIGHT,fields)
    report['preflight_exit']=p.returncode
    report['preflight_stdout']=save('install-input-preflight-'+stamp+'.json',p.stdout)
    report['preflight_stderr']=save('install-input-preflight-'+stamp+'.stderr',p.stderr)
    report['target_contacted']=True
    if p.returncode:
        print(json.dumps(report));raise SystemExit(p.returncode)
    upload=['scp','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes',
            '-o','ConnectTimeout=15']+[x['path'] for x in archives]+['root@172.30.126.37:'+REMOTE_DIRECTORY+'/']
    p=subprocess.run(upload,capture_output=True,timeout=240)
    report['upload_exit']=p.returncode
    report['upload_stdout']=save('install-input-upload-'+stamp+'.stdout',p.stdout)
    report['upload_stderr']=save('install-input-upload-'+stamp+'.stderr',p.stderr)
    if p.returncode:
        print(json.dumps(report));raise SystemExit(p.returncode)
    p=remote(SIMULATE,{'REMOTE_DIRECTORY':REMOTE_DIRECTORY,'POLICY':POLICY,
                       'EXPECTED_ARCHIVES':[{k:x[k] for k in ['file','sha256','bytes']} for x in archives],
                       'EXTRA_PACKAGES':EXTRA_PACKAGES})
    report['solver_exit']=p.returncode
    report['solver_stdout']=save('install-input-solver-'+stamp+'.json',p.stdout)
    report['solver_stderr']=save('install-input-solver-'+stamp+'.stderr',p.stderr)
    if p.stdout:
        try:
            plan=json.loads(p.stdout).get('stdout','')
            changes=[line for line in plan.splitlines() if re.match(r'^(Inst|Remv) ',line)]
            critical=[line for line in changes if re.match(
                r'^(Inst|Remv) (systemd[^ ]*|libsystemd[^ ]*|libc6|libssl[^ ]*|openssh[^ ]*|netplan.io|'
                r'iproute2|network-manager|udev|grub[^ ]*|shim[^ ]*|linux-image[^ ]*|nftables)(?:[: ]|$)',line)]
            report['planned_change_count']=len(changes)
            report['critical_change_count']=len(critical)
            report['critical_plan']=save('install-input-critical-plan-'+stamp+'.json',
                                         json.dumps(critical,indent=2).encode())
        except (ValueError,AttributeError):
            report['solver_plan_parse_failed']=True
    # All planned Inst/Remv lines, particularly management/system libraries, require human/manager review.
    report['requires_plan_review_before_install']=True
    report['package_install']=False
    print(json.dumps(report))
    raise SystemExit(p.returncode)


if __name__=='__main__':
    main()
