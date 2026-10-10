#!/usr/bin/env python3
"""Install only the immutable actual reviewed APT plan under preserved guards.

No firstboot, service activation, driver binding, hugepages or network/firewall
changes are requested. Original conffiles stay; policy101 and VPP mask remain.
"""
import argparse,datetime,hashlib,json,os,pathlib,re,runpy,subprocess,time
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
PLAN=PRIVATE/'install-input-solver-20261010T110641Z.json'
PLAN_SHA='07d88eb379ed4a9a813ac8c5cf6d0be9b31e0446ac9cbd5563afdbc49af035d8'
MARKER_SHA='146f46835119cb7341fce3974a40378bf5feab8159e419f962ded1a28d4ec792'
REMOTE=r'''
import hashlib,json,os,pathlib,re,shutil,stat,subprocess,time
os.umask(0o077)
def run(args):return subprocess.run(args,capture_output=True,text=True,timeout=60)
def output(args):
 p=run(args);assert p.returncode==0,(args,p.returncode);return p.stdout
def l3():
 return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(output(['ip','-j','addr']))},
 'routes4':json.loads(output(['ip','-j','-4','route','show','table','all'])),
 'routes6':json.loads(output(['ip','-j','-6','route','show','table','all'])),
 'rules4':json.loads(output(['ip','-j','-4','rule'])),
 'rules6':json.loads(output(['ip','-j','-6','rule']))}
assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
assert abs(time.time()-CONTROLLER_EPOCH)<60,'trusted time missing'
assert output(['findmnt','-no','FSTYPE','/run']).strip()=='tmpfs'
state=output(['tune2fs','-l','/dev/sda2']).splitlines()
assert [x.split(':',1)[1].strip() for x in state if x.startswith('Filesystem state:')]==['clean']
device=pathlib.Path('/sys/class/net/enp4s0/device')
assert device.resolve().name=='0000:04:00.0' and (device/'driver').resolve().name=='igc'
assert (device/'iommu_group').resolve().name=='28'
route=json.loads(output(['ip','-j','route','get','172.30.126.195']))[0]
assert route.get('dev')=='enp4s0' and route.get('prefsrc')=='172.30.110.211'
policy=pathlib.Path('/usr/sbin/policy-rc.d');s=policy.lstat()
assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o755
assert policy.read_bytes()==POLICY
mask=pathlib.Path('/etc/systemd/system/vpp.service')
assert mask.is_symlink() and os.readlink(mask)=='/dev/null' and mask.lstat().st_uid==0
assert output(['systemctl','show','vpp.service','-p','LoadState','--value']).strip()=='masked'
assert output(['systemctl','show','vpp.service','-p','ActiveState','--value']).strip() in ['inactive','failed']
marker=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-211-20261010/state.json');s=marker.lstat()
assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
assert (os.major(s.st_dev),os.minor(s.st_dev))==(8,2)
assert hashlib.sha256(marker.read_bytes()).hexdigest()==MARKER_SHA
before=l3();assert before==EXPECTED_NETWORK
# Actual needrestart is absent and absent from the reviewed plan. Refuse drift.
hook_refs=[]
for path in pathlib.Path('/etc/apt/apt.conf.d').iterdir():
 if path.is_file() and b'needrestart' in path.read_bytes().lower():hook_refs.append(str(path))
assert not hook_refs,'unexpected needrestart APT hook requires concrete review'
assert shutil.which('needrestart') is None and not pathlib.Path('/usr/lib/needrestart/apt-pinvoke').exists()
needrestart=run(['dpkg-query','-W','-f','${db:Status-Status}', 'needrestart'])
assert needrestart.stdout.strip()!='installed'
root=pathlib.Path(REMOTE_DIRECTORY);s=root.lstat()
assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o700
assert sorted(p.name for p in root.iterdir())==sorted(x['file'] for x in ARCHIVES)
for item in ARCHIVES:
 path=root/item['file'];assert path.is_file() and not path.is_symlink() and path.stat().st_size==item['bytes']
 assert hashlib.sha256(path.read_bytes()).hexdigest()==item['sha256']
args=['apt-get','--no-remove','--no-install-recommends','-o','Dir::Cache::pkgcache=',
      '-o','Dir::Cache::srcpkgcache=','-o','Dpkg::Options::=--force-confold',
      'install']+['./'+x['file'] for x in ARCHIVES]+EXTRA_PACKAGES
simulation=subprocess.run(args[:1]+['-s']+args[1:],cwd=root,capture_output=True,text=True,timeout=180)
changes=[x for x in simulation.stdout.splitlines() if re.match(r'^(Inst|Remv) ',x)]
assert simulation.returncode==0 and changes==EXPECTED_PLAN,'APT plan drift; no installation'
assert not any(x.startswith('Remv ') for x in changes)
assert not any(x.split()[1]=='needrestart' for x in changes)
# Automatic needrestart is absent; standard package starts are denied by exact101.
env=dict(os.environ,DEBIAN_FRONTEND='noninteractive')
logs={}
for suffix in ['stdout','stderr']:
 path=root/('apt-install.'+suffix);fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 logs[suffix]=(path,os.fdopen(fd,'w'))
p=subprocess.run(args[:1]+['-y']+args[1:],cwd=root,env=env,stdout=logs['stdout'][1],stderr=logs['stderr'][1])
for path,f in logs.values():f.flush();os.fsync(f.fileno());f.close()
after=l3()
result={'actual_command':args[:1]+['-y']+args[1:],'install_exit':p.returncode,
        'simulation_exit':simulation.returncode,'plan_exact':True,'plan_changes':changes,
        'stdout':logs['stdout'][0].read_text(),'stderr':logs['stderr'][0].read_text(),
        'network_before':before,'network_after':after,'network_equal':before==after,
        'policy_exact':policy.read_bytes()==POLICY,'persistent_mask':os.readlink(mask)=='/dev/null',
        'vpp_state':output(['systemctl','show','vpp.service','-p','LoadState','-p','ActiveState']),
        'needrestart_hook_refs':hook_refs,'needrestart_installed_before':False,
        'no_firstboot_activation':True,'no_driver_binding':True}
units=['vpp.service','ngfw-firstboot.service','ngfw-agent.service','ngfw-api.service','nginx.service','frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','chrony.service','postgresql.service','valkey-server.service','nftables.service','apply-executor.socket','ngfw-ra-openfile.socket']
result['service_states']={unit:output(['systemctl','show',unit,'-p','ActiveState','--value']).strip() for unit in units}
result['all_services_suppressed']=all(value in ['inactive','failed'] for value in result['service_states'].values())
print(json.dumps(result,indent=2))
passed=before==after and result['policy_exact'] and result['persistent_mask'] and result['all_services_suppressed'] and 'LoadState=masked' in result['vpp_state']
raise SystemExit(p.returncode if p.returncode else (0 if passed else 2))
'''
def save(path,data):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
 fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
 return {'file':str(path),'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest()}
def main():
 os.umask(0o077)
 p=argparse.ArgumentParser();p.add_argument('--install',action='store_true',required=True);p.parse_args()
 raw=PLAN.read_bytes();assert hashlib.sha256(raw).hexdigest()==PLAN_SHA
 plan=json.loads(raw);assert plan['exit']==0
 changes=[x for x in plan['stdout'].splitlines() if re.match(r'^(Inst|Remv) ',x)]
 assert len(changes)==114 and not any(x.startswith('Remv ') for x in changes)
 provider_path=pathlib.Path(__file__).with_name('hardware-211-20261010-package-input.py')
 assert hashlib.sha256(provider_path.read_bytes()).hexdigest()=='e5c7a1c927b03d0298ae122acc7f765edbd5f23b12cc2d8cea03500a91e6b346'
 provider=runpy.run_path(str(provider_path))
 archives=provider['local_archives']()
 network=json.loads((PRIVATE/'start-guards-prepare-20261010T110635Z.json').read_bytes())['network_after']
 fields={'ARCHIVES':[{k:x[k] for k in ['file','sha256','bytes']} for x in archives],
         'EXTRA_PACKAGES':provider['EXTRA_PACKAGES'],'POLICY':provider['POLICY'],
         'REMOTE_DIRECTORY':provider['REMOTE_DIRECTORY'],'EXPECTED_PLAN':changes,
         'EXPECTED_NETWORK':network,'MARKER_SHA':MARKER_SHA,'CONTROLLER_EPOCH':time.time()}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 # The tool caller remains resumable while actual APT runs; do not kill dpkg on a timer.
 result=subprocess.run(provider['SSH']+['python3 -'],input=code.encode(),capture_output=True)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 out=save(PRIVATE/('package-install-'+stamp+'.json'),result.stdout)
 err=save(PRIVATE/('package-install-'+stamp+'.stderr'),result.stderr)
 print(json.dumps({'SSH_exit':result.returncode,'stdout':out,'stderr':err,'no_firstboot_or_binding':True}))
 raise SystemExit(result.returncode)
if __name__=='__main__':main()
