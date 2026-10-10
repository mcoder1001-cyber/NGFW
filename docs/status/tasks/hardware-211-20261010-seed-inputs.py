#!/usr/bin/env python3
"""Publish explicit management/first-API NIC seed inputs after proved firstboot.

No service activation, VPP application, revision creation or driver binding.
The API environment remains canonical; opt-in is a persistent systemd drop-in.
"""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import hashlib,json,os,pathlib,stat,subprocess
os.umask(0o077)
AGENT=pathlib.Path('/etc/ngfw/agent.env')
DIRECTORY=pathlib.Path('/etc/systemd/system/ngfw-api.service.d')
DROPIN=DIRECTORY/'10-hardware-seed.conf'
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-211-20261010-seed-inputs')
AGENT_BYTES=b'NGFW_MGMT_IF=enp4s0\nNGFW_MGMT_PCI=0000:04:00.0\n'
UNIT_BYTES=b'[Service]\nEnvironment=NGFW_SEED_DEFAULT_NICS=1\n'
def command(args):
 p=subprocess.run(args,capture_output=True,text=True,timeout=30);assert p.returncode==0,(args,p.returncode);return p.stdout
def value(unit,key):return command(['systemctl','show',unit,'-p',key,'--value']).strip()
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(command(['ip','-j','addr']))},'routes4':json.loads(command(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(command(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(command(['ip','-j','-4','rule'])),'rules6':json.loads(command(['ip','-j','-6','rule']))}
def directory(p):
 s=p.lstat();assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022 and (os.major(s.st_dev),os.minor(s.st_dev))==(8,2)
def new(p,raw,mode):
 directory(p.parent);fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,mode)
 os.fchmod(fd,mode)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
assert pathlib.Path('/var/lib/ngfw/firstboot-complete').read_bytes()==b'completed\n'
assert not os.path.lexists('/etc/ngfw/bootstrap.env')
assert set(x.split('=',1)[0] for x in pathlib.Path('/etc/ngfw/api.env').read_text().splitlines())=={'NGFW_DATABASE_URL','NGFW_SECRET_KEY_FILE','NGFW_JWT_SECRET'}
for unit in ['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']:assert value(unit,'ActiveState')=='inactive'
assert value('vpp.service','LoadState')=='masked' and os.readlink('/etc/systemd/system/vpp.service')=='/dev/null'
assert pathlib.Path('/usr/sbin/policy-rc.d').read_bytes()==b'#!/bin/sh\nexit 101\n'
assert hashlib.sha256(pathlib.Path('/var/lib/ngfw-install-recovery/hardware-211-20261010/state.json').read_bytes()).hexdigest()=='146f46835119cb7341fce3974a40378bf5feab8159e419f962ded1a28d4ec792'
dev=pathlib.Path('/sys/class/net/enp4s0/device');assert dev.resolve().name=='0000:04:00.0' and (dev/'driver').resolve().name=='igc' and (dev/'iommu_group').resolve().name=='28'
before=l3();assert before==NETWORK_BASELINE
assert all(not os.path.lexists(p) for p in [AGENT,DROPIN,RECORD])
assert value('ngfw-api.service','DropInPaths')=='' and 'NGFW_SEED_DEFAULT_NICS=' not in value('ngfw-api.service','Environment')
assert value('ngfw-api.service','EnvironmentFiles')=='/etc/ngfw/api.env (ignore_errors=no)'
assert value('ngfw-agent.service','EnvironmentFiles')=='/etc/ngfw/agent.env (ignore_errors=yes)'
directory(pathlib.Path('/etc/ngfw'));directory(pathlib.Path('/etc/systemd/system'));directory(RECORD.parent)
if os.path.lexists(DIRECTORY):directory(DIRECTORY);assert not list(DIRECTORY.iterdir())
state={'firstboot_proof_SHA':PROOF_SHA,'original_agent_env':'absent','original_API_dropin':'absent','original_API_dropin_directory':os.path.lexists(DIRECTORY),'network_before':before,'VPP_API_agent_nginx_inactive':True,'agent_owned_SHA':hashlib.sha256(AGENT_BYTES).hexdigest(),'dropin_owned_SHA':hashlib.sha256(UNIT_BYTES).hexdigest()}
if APPLY:
 RECORD.mkdir(mode=0o700);new(RECORD/'before.json',json.dumps(state,indent=2).encode(),0o600)
 fd=os.open(RECORD.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
 if not os.path.lexists(DIRECTORY):DIRECTORY.mkdir(mode=0o755);os.chmod(DIRECTORY,0o755);fd=os.open(DIRECTORY.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
 new(AGENT,AGENT_BYTES,0o600);new(DROPIN,UNIT_BYTES,0o644);command(['systemctl','daemon-reload'])
 assert AGENT.read_bytes()==AGENT_BYTES and DROPIN.read_bytes()==UNIT_BYTES
 assert stat.S_IMODE(AGENT.lstat().st_mode)==0o600 and AGENT.lstat().st_uid==0 and stat.S_IMODE(DROPIN.lstat().st_mode)==0o644 and DROPIN.lstat().st_uid==0
 assert 'NGFW_SEED_DEFAULT_NICS=1' in value('ngfw-api.service','Environment').split()
 assert value('ngfw-api.service','DropInPaths')==str(DROPIN)
after=l3();assert after==before
state.update(mode='prepare' if APPLY else 'inspect',network_equal=True,API_environment=value('ngfw-api.service','Environment'),agent_environment_file=value('ngfw-agent.service','EnvironmentFiles'),no_activation_revision_or_binding=True)
print(json.dumps(state,indent=2))
'''
def save(p,raw):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--proof',required=True);p.add_argument('--proof-sha256',required=True);p.add_argument('--prepare',action='store_true');a=p.parse_args()
 proof=pathlib.Path(a.proof).resolve();s=proof.stat();assert proof.parent==PRIVATE and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 raw=proof.read_bytes();assert hashlib.sha256(raw).hexdigest()==a.proof_sha256;d=json.loads(raw)
 assert d['firstboot']['exit']==0 and d['network_equal'] and d['owned_table_only'] and d['safe_initial_noPCI'] and d['bootstrap_removed'] and d['guard_error'] is None and d['sysctls_only_expected_nr_change'] and d['no_VPP_API_agent_nginx_activation_or_binding'] and d['new_storage_errors']==[]
 fields={'APPLY':a.prepare,'NETWORK_BASELINE':d['network_after'],'PROOF_SHA':a.proof_sha256};code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 r=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 out=save(PRIVATE/('seed-inputs-'+stamp+'.json'),r.stdout);err=save(PRIVATE/('seed-inputs-'+stamp+'.stderr'),r.stderr)
 print(json.dumps({'SSH_exit':r.returncode,'stdout':out,'stderr':err,'no_activation_revision_or_binding':True}));raise SystemExit(r.returncode)
if __name__=='__main__':main()
