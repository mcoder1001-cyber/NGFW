#!/usr/bin/env python3
"""Reviewed firstboot-only phase. Requires a separate manager execution release.

No VPP/agent/API/nginx start, driver binding, network or global sysctl application.
Credentials travel only through SSH stdin and private files, never argv/stdout.
"""
import argparse,datetime,hashlib,json,os,pathlib,secrets,stat,subprocess,time
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
BASELINE=PRIVATE/'firstboot-installed-inspect-20261010T113746Z.json'
BASELINE_SHA='30fb509abc3cc8b2e710385117dc8cb36182e469569670d87a1ddd25494949ce'
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import hashlib,json,os,pathlib,re,stat,subprocess,time
os.umask(0o077)
START_MONOTONIC=time.monotonic()
record=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-211-20261010/firstboot')
def run(args,input=None):
 p=subprocess.run(args,input=input,capture_output=True,text=True)
 return {'argv':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
def checked(args,input=None):
 p=run(args,input);assert p['exit']==0,(args,p['exit']);return p['stdout']
def value(unit,property):return checked(['systemctl','show',unit,'-p',property,'--value']).strip()
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
KEYS=['vm.nr_hugepages','vm.hugetlb_shm_group','kernel.shmmax','net.ipv4.ip_forward','net.ipv6.conf.all.forwarding','net.ipv4.conf.all.rp_filter','net.ipv4.conf.default.rp_filter','net.ipv4.conf.enp4s0.rp_filter','net.ipv4.conf.enp4s0.forwarding','net.ipv4.conf.all.accept_redirects','net.ipv4.conf.enp4s0.accept_redirects','net.ipv6.conf.all.disable_ipv6','net.ipv6.conf.default.disable_ipv6','net.ipv6.conf.enp4s0.disable_ipv6','net.ipv6.conf.all.accept_ra','net.ipv6.conf.enp4s0.accept_ra']
def sysctls():return {k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in KEYS}
def nft():return json.loads(checked(['nft','-j','list','ruleset']))
def objects(doc):return [x for x in doc['nftables'] if 'metainfo' not in x]
def write_new(p,raw,mode=0o600):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,mode)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
 try:os.fsync(fd)
 finally:os.close(fd)
def guards():
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 assert abs(time.time()-(CONTROLLER_EPOCH+time.monotonic()-START_MONOTONIC))<60
 fs=checked(['tune2fs','-l','/dev/sda2']).splitlines();assert [x.split(':',1)[1].strip() for x in fs if x.startswith('Filesystem state:')]==['clean']
 p=pathlib.Path('/usr/sbin/policy-rc.d');s=p.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o755 and p.read_bytes()==b'#!/bin/sh\nexit 101\n'
 m=pathlib.Path('/etc/systemd/system/vpp.service');assert m.is_symlink() and m.lstat().st_uid==0 and os.readlink(m)=='/dev/null'
 assert value('vpp.service','LoadState')=='masked' and value('vpp.service','ActiveState')=='inactive'
 marker=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-211-20261010/state.json');assert hashlib.sha256(marker.read_bytes()).hexdigest()=='146f46835119cb7341fce3974a40378bf5feab8159e419f962ded1a28d4ec792'
 dev=pathlib.Path('/sys/class/net/enp4s0/device');assert dev.resolve().name=='0000:04:00.0' and (dev/'driver').resolve().name=='igc' and (dev/'iommu_group').resolve().name=='28'
 route=json.loads(checked(['ip','-j','route','get','172.30.126.195']))[0];assert route.get('dev')=='enp4s0' and route.get('prefsrc')=='172.30.110.211'
guarded=['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service','frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','snmpd.service','keepalived.service','chrony.service','rsyslog.service','apply-executor.socket','ngfw-ra-openfile.socket','ngfw-ra-namespace-broker.socket']
guards();before=l3();assert before==L3_BASELINE
before_states={u:value(u,'ActiveState') for u in guarded+['ngfw-firstboot.service','ngfw-firewall-bootstrap.service','nftables.service','postgresql.service','postgresql@18-main.service','valkey-server.service']}
assert all(v=='inactive' for v in before_states.values())
sys_before=sysctls()
assert all(not os.path.lexists(p) for p in ['/etc/ngfw/bootstrap.env','/etc/ngfw/api.env','/etc/ngfw/base-policy.env','/var/lib/ngfw/firstboot-complete','/var/lib/ngfw/secret.key','/etc/ngfw/tls/server.key','/etc/ngfw/tls/server.crt',str(record)])
for path,digest in SCRIPT_HASHES.items():
 p=pathlib.Path(path);s=p.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022 and hashlib.sha256(p.read_bytes()).hexdigest()==digest
nft_before=nft();assert objects(nft_before)==NFT_BASELINE and not objects(nft_before),'only observed empty pre-firstboot ruleset supported'
assert value('nftables.service','DropInPaths')=='/usr/lib/systemd/system/nftables.service.d/nftables-ngfw.conf'
for key in ['ExecStart','ExecReload']:
 v=value('nftables.service',key);assert 'argv[]=/usr/sbin/nft -f /etc/nftables.d/ngfw-base.nft ;' in v and 'flush' not in v and '/etc/nftables.conf' not in v
assert value('nftables.service','ExecStop')=='' and 'ngfw-firewall-bootstrap.service' in value('nftables.service','Requires').split()
assert checked(['systemctl','cat','nftables.service','ngfw-firewall-bootstrap.service','ngfw-firstboot.service'])==UNIT_BASELINE
pg_listen=checked(['runuser','-u','postgres','--','/usr/lib/postgresql/18/bin/postgres','-D','/var/lib/postgresql/18/main','-c','config_file=/etc/postgresql/18/main/postgresql.conf','-C','listen_addresses']).strip();assert pg_listen=='localhost'
valkey=pathlib.Path('/etc/valkey/valkey.conf').read_text().splitlines();assert [x for x in valkey if x.startswith('bind ')]==['bind 127.0.0.1 -::1'] and [x for x in valkey if x.startswith('protected-mode ')]==['protected-mode yes']
rendered=checked(['python3','/usr/lib/ngfw/render-base-policy.py','--management-interface','enp4s0','--punt-interfaces','']);assert checked(['nft','-c','-f','-'],rendered)==''
mem={x.split(':',1)[0]:int(x.split()[1]) for x in pathlib.Path('/proc/meminfo').read_text().splitlines() if x.split(':',1)[0] in ['MemAvailable','Hugepagesize','HugePages_Total','HugePages_Free']}
assert mem['Hugepagesize']==2048 and mem['HugePages_Total']==0 and mem['MemAvailable']>=4194304 and pathlib.Path('/sys/devices/system/node/online').read_text().strip()=='0'
assert pathlib.Path('/proc/sys/vm/nr_hugepages').read_text().strip()=='0' and pathlib.Path('/proc/sys/vm/hugetlb_shm_group').read_text().strip()=='0'
active=[]
for directory in ['/etc/sysctl.d','/run/sysctl.d','/usr/local/lib/sysctl.d','/usr/lib/sysctl.d','/lib/sysctl.d']:
 p=pathlib.Path(directory)
 if p.is_dir():
  for f in sorted(p.glob('*.conf')):
   for line in f.read_text().splitlines():
    if re.match(r'\s*(vm\.(nr_hugepages|hugetlb_shm_group)|kernel\.shmmax)\s*=',line):active.append([str(f.resolve()),line.strip()])
if pathlib.Path('/etc/sysctl.conf').exists():
 for line in pathlib.Path('/etc/sysctl.conf').read_text().splitlines():
  if re.match(r'\s*(vm\.(nr_hugepages|hugetlb_shm_group)|kernel\.shmmax)\s*=',line):active.append(['/etc/sysctl.conf',line.strip()])
assert active==[['/etc/sysctl.d/80-vpp.conf','vm.nr_hugepages=1024'],['/etc/sysctl.d/80-vpp.conf','vm.hugetlb_shm_group=0']]
kernel_before=checked(['dmesg','--color=never']);ioerr_before=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip()
audit=run(['dpkg','--audit']);assert audit['exit']==0 and not audit['stdout'] and not audit['stderr']
startup=pathlib.Path('/etc/vpp/startup.conf');st=startup.lstat();assert stat.S_ISREG(st.st_mode) and st.st_uid==0 and hashlib.sha256(startup.read_bytes()).hexdigest()==STARTUP_SHA
pre={'states':before_states,'sysctls':sys_before,'network':before,'nft':nft_before,'VM_nr_hugepages':0,'VM_hugetlb_shm_group':0,'80_vpp_sha256':hashlib.sha256(pathlib.Path('/etc/sysctl.d/80-vpp.conf').read_bytes()).hexdigest(),'startup_conf_before':startup.read_text(),'startup_metadata':{'uid':st.st_uid,'gid':st.st_gid,'mode':stat.S_IMODE(st.st_mode),'sha256':STARTUP_SHA},'pg_listen':pg_listen,'ioerr':ioerr_before,'meminfo':mem,'unit_baseline_sha256':hashlib.sha256(UNIT_BASELINE.encode()).hexdigest()}
if not APPLY:print(json.dumps({'preflight':pre,'read_only':True},indent=2))
else:
 for directory in [record.parent,pathlib.Path('/etc/ngfw'),pathlib.Path('/etc/vpp')]:
  ds=directory.lstat();assert stat.S_ISDIR(ds.st_mode) and ds.st_uid==0 and not ds.st_mode&0o022 and (os.major(ds.st_dev),os.minor(ds.st_dev))==(8,2)
 record.mkdir(mode=0o700)
 fd=os.open(record.parent,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
 write_new(record/'before.json',json.dumps(pre,indent=2).encode());write_new(record/'startup.conf.before',startup.read_bytes())
 write_new(pathlib.Path('/etc/ngfw/bootstrap.env'),('NGFW_BOOTSTRAP_ADMIN_USER='+ADMIN_USER+'\nNGFW_BOOTSTRAP_ADMIN_PASSWORD='+ADMIN_PASSWORD+'\nNGFW_BOOTSTRAP_MGMT_IF=enp4s0\n').encode())
 # Existing packaged persistent1024 setting is verified above, never reapply --system.
 pathlib.Path('/proc/sys/vm/nr_hugepages').write_text('1024\n')
 assert pathlib.Path('/proc/sys/vm/nr_hugepages').read_text().strip()=='1024' and pathlib.Path('/proc/sys/vm/hugetlb_shm_group').read_text().strip()=='0'
 disable=run(['systemctl','disable','frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','snmpd.service','keepalived.service'])
 if disable['exit']!=0:print(json.dumps({'preflight':pre,'disable':disable,'partial':True},indent=2));raise SystemExit(disable['exit'])
 firstboot=run(['systemctl','start','ngfw-firstboot.service'])
 after=l3();nft_after=nft();sys_after=sysctls();sys_expected=dict(sys_before,**{'vm.nr_hugepages':'1024'});states={u:value(u,'ActiveState') for u in guarded+['ngfw-firstboot.service','ngfw-firewall-bootstrap.service','nftables.service','postgresql.service','postgresql@18-main.service','valkey-server.service']}
 files={}
 for name in ['/etc/ngfw/api.env','/var/lib/ngfw/secret.key','/etc/ngfw/tls/server.key','/etc/ngfw/tls/server.crt','/var/lib/ngfw/firstboot-complete','/etc/vpp/startup.conf']:
  p=pathlib.Path(name)
  if not p.exists():files[name]={'exists':False};continue
  s=p.lstat();files[name]={'exists':True,'regular':stat.S_ISREG(s.st_mode),'uid':s.st_uid,'gid':s.st_gid,'mode':stat.S_IMODE(s.st_mode),'bytes':s.st_size,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()}
 env=pathlib.Path('/etc/ngfw/api.env');keys=[line.split('=',1)[0] for line in env.read_text().splitlines()] if env.is_file() else []
 startup_text=startup.read_text();device_lines=[x for x in startup_text.splitlines() if re.match(r'\s*dev\s+(?!default(?:\s|\{))',x)];safe_no_pci=not device_lines and ('  no-pci\n' in startup_text or 'plugin dpdk_plugin.so { disable }' in startup_text) and ('blacklist 0000:04:00.0' in startup_text or 'plugin dpdk_plugin.so { disable }' in startup_text)
 other_objects=[x for x in objects(nft_after) if not any(isinstance(v,dict) and ((k=='table' and v.get('name')=='ngfw_base' and v.get('family')=='inet') or (k!='table' and v.get('table')=='ngfw_base' and v.get('family')=='inet')) for k,v in x.items())]
 table=[v['table'] for v in objects(nft_after) if 'table' in v];owned_table_only=len(table)==1 and table[0].get('family')=='inet' and table[0].get('name')=='ngfw_base' and other_objects==NFT_BASELINE
 guard_error=None
 try:guards()
 except BaseException as e:guard_error=type(e).__name__+': '+str(e)
 ioerr_after=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();kernel_after=checked(['dmesg','--color=never']);new_lines=kernel_after.splitlines()[len(kernel_before.splitlines()):] if kernel_after.startswith(kernel_before) else None
 bad=re.compile(r'(EXT4-fs error|Buffer I/O error|I/O error, dev sda|ata\d.*(hard resetting|failed command|error:.*(UNC|ICRC)))',re.I);new_storage=None if new_lines is None else [x for x in new_lines if bad.search(x)]
 result={'sysctls_after':sys_after,'sysctls_only_expected_nr_change':sys_after==sys_expected,'preflight':pre,'disable':disable,'firstboot':firstboot,'journal':run(['journalctl','--no-pager','-u','ngfw-firstboot.service','-u','ngfw-firewall-bootstrap.service','-n','200']),'network_before':before,'network_after':after,'network_equal':before==after,'nft_before':nft_before,'nft_after':nft_after,'owned_table_only':owned_table_only,'states':states,'files':files,'api_env_keys':keys,'safe_initial_noPCI':safe_no_pci,'startup_conf':startup_text,'bootstrap_removed':not os.path.lexists('/etc/ngfw/bootstrap.env'),'guard_error':guard_error,'VM_nr_hugepages':pathlib.Path('/proc/sys/vm/nr_hugepages').read_text().strip(),'ioerr_before':ioerr_before,'ioerr_after':ioerr_after,'kernel_before':kernel_before,'kernel_after':kernel_after,'new_storage_errors':new_storage,'no_VPP_API_agent_nginx_activation_or_binding':all(states[u]=='inactive' for u in guarded)}
 print(json.dumps(result,indent=2))
 success=firstboot['exit']==0 and sys_after==sys_expected and before==after and owned_table_only and safe_no_pci and guard_error is None and result['bootstrap_removed'] and set(keys)=={'NGFW_DATABASE_URL','NGFW_SECRET_KEY_FILE','NGFW_JWT_SECRET'} and len(keys)==3 and ioerr_before==ioerr_after and new_storage==[] and all(states[u]=='inactive' for u in guarded) and all(states[u]=='active' for u in ['ngfw-firstboot.service','ngfw-firewall-bootstrap.service','nftables.service','postgresql.service','postgresql@18-main.service','valkey-server.service']) and files['/etc/ngfw/api.env'].get('mode')==0o600 and files['/etc/ngfw/api.env'].get('uid')==0 and files['/var/lib/ngfw/secret.key'].get('bytes')==32 and files['/var/lib/ngfw/secret.key'].get('mode')==0o600
 raise SystemExit(0 if success else (firstboot['exit'] or 2))
'''
def save(path,raw):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
 return {'file':str(path),'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--firstboot',action='store_true');a=p.parse_args()
 raw=BASELINE.read_bytes();assert hashlib.sha256(raw).hexdigest()==BASELINE_SHA and stat.S_IMODE(BASELINE.stat().st_mode)==0o600
 base=json.loads(raw);commands=base['commands'];by_argv={tuple(x['argv']):x['stdout'] for x in commands}
 l3={'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(by_argv[('ip','-j','addr')])},'routes4':json.loads(by_argv[('ip','-j','-4','route','show','table','all')]),'routes6':json.loads(by_argv[('ip','-j','-6','route','show','table','all')]),'rules4':json.loads(by_argv[('ip','-j','-4','rule')]),'rules6':json.loads(by_argv[('ip','-j','-6','rule')])}
 scripts={x['path']:x['sha256'] for x in base['files'] if x['path'].startswith('/usr/lib/ngfw/') and x.get('sha256')};startup=next(x['sha256'] for x in base['files'] if x['path']=='/etc/vpp/startup.conf')
 user='lab-admin';password=secrets.token_hex(32) if a.firstboot else ''
 if a.firstboot:save(PRIVATE/'bootstrap-admin-211.json',json.dumps({'host':'172.30.110.211','username':user,'password':password}).encode())
 fields={'APPLY':a.firstboot,'CONTROLLER_EPOCH':time.time(),'L3_BASELINE':l3,'NFT_BASELINE':[x for x in json.loads(by_argv[('nft','-j','list','ruleset')])['nftables'] if 'metainfo' not in x],'UNIT_BASELINE':by_argv[('systemctl','cat','nftables.service','ngfw-firewall-bootstrap.service','ngfw-firstboot.service')],'SCRIPT_HASHES':scripts,'STARTUP_SHA':startup,'ADMIN_USER':user,'ADMIN_PASSWORD':password}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 run=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');mode='apply' if a.firstboot else 'preflight'
 out=save(PRIVATE/('firstboot-'+mode+'-'+stamp+'.json'),run.stdout);err=save(PRIVATE/('firstboot-'+mode+'-'+stamp+'.stderr'),run.stderr)
 print(json.dumps({'mode':mode,'SSH_exit':run.returncode,'stdout':out,'stderr':err,'no_VPP_API_binding':True}));raise SystemExit(run.returncode)
if __name__=='__main__':main()
