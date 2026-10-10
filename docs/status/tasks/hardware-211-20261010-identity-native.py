#!/usr/bin/env python3
"""Canonicalize only the verified same-file timezone link, then native configure.

Inspect is read-only and fsyncs the exact original link/target backup offhost.
Apply never relaxes the product guard, changes effective timezone or starts services.
"""
import argparse,datetime,hashlib,json,os,pathlib,subprocess,time
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import hashlib,json,os,pathlib,secrets,stat,subprocess,time
os.umask(0o077)
LINK='../usr/share/zoneinfo/Asia/Tehran'
ZONE='/usr/share/zoneinfo/Asia/Tehran'
ZONE_SHA='2dbd87f410815edcfcd7d14be84de0040ef0d913a22203e0c7e7f4f17a6a915a'
FLAGS=os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC
NAMES=['hostname','issue','issue.net','motd','localtime']
UNITS=['vpp.service','ngfw-firstboot.service','ngfw-agent.service','ngfw-api.service','nginx.service','frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','chrony.service','postgresql.service','valkey-server.service','nftables.service','snmpd.service','keepalived.service','rsyslog.service','postgresql@18-main.service','ngfw-firewall-bootstrap.service','apply-executor.socket','ngfw-ra-openfile.socket','ngfw-ra-namespace-broker.socket']
KEYS=['vm.nr_hugepages','vm.hugetlb_shm_group','kernel.shmmax','net.ipv4.ip_forward','net.ipv6.conf.all.forwarding','net.ipv4.conf.all.rp_filter','net.ipv4.conf.default.rp_filter','net.ipv4.conf.enp4s0.rp_filter','net.ipv4.conf.enp4s0.forwarding','net.ipv4.conf.all.accept_redirects','net.ipv4.conf.enp4s0.accept_redirects','net.ipv6.conf.all.disable_ipv6','net.ipv6.conf.default.disable_ipv6','net.ipv6.conf.enp4s0.disable_ipv6','net.ipv6.conf.all.accept_ra','net.ipv6.conf.enp4s0.accept_ra']
def command(args):
 p=subprocess.run(args,capture_output=True,text=True,timeout=30)
 assert p.returncode==0,(args,p.returncode)
 return p.stdout
def trusted(s):assert s.st_uid==0 and not s.st_mode&0o022,'untrusted owner/mode'
def meta(s):return {'dev':s.st_dev,'inode':s.st_ino,'mode':stat.S_IMODE(s.st_mode),'uid':s.st_uid,'gid':s.st_gid,'bytes':s.st_size,'nlink':s.st_nlink}
def directory(path):
 fd=os.open('/',FLAGS);trusted(os.fstat(fd))
 try:
  for part in pathlib.PurePosixPath(path).parts[1:]:
   child=os.open(part,FLAGS,dir_fd=fd);trusted(os.fstat(child));os.close(fd);fd=child
  return fd
 except BaseException:os.close(fd);raise
def zone():
 fd=directory('/usr/share/zoneinfo/Asia')
 try:
  f=os.open('Tehran',os.O_RDONLY|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=fd)
  try:
   s=os.fstat(f);trusted(s);assert stat.S_ISREG(s.st_mode) and s.st_size==1248
   raw=os.read(f,1249);assert len(raw)==1248 and hashlib.sha256(raw).hexdigest()==ZONE_SHA
   assert meta(os.fstat(f))==meta(s)
   return dict(meta(s),sha256=ZONE_SHA,path=ZONE)
  finally:os.close(f)
 finally:os.close(fd)
def identity():
 d={};fd=directory('/etc')
 try:
  for name in NAMES:
   try:s=os.stat(name,dir_fd=fd,follow_symlinks=False)
   except FileNotFoundError:d[name]={'type':'absent'};continue
   if stat.S_ISLNK(s.st_mode):d[name]=dict(meta(s),type='link',link=os.readlink(name,dir_fd=fd));continue
   trusted(s);assert stat.S_ISREG(s.st_mode) and s.st_nlink==1
   f=os.open(name,os.O_RDONLY|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=fd)
   try:
    assert meta(os.fstat(f))==meta(s);raw=os.read(f,1048577);assert len(raw)<=1048576
    d[name]=dict(meta(s),type='file',sha256=hashlib.sha256(raw).hexdigest())
   finally:os.close(f)
 finally:os.close(fd)
 return d
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(command(['ip','-j','addr']))},'routes4':json.loads(command(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(command(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(command(['ip','-j','-4','rule'])),'rules6':json.loads(command(['ip','-j','-6','rule']))}
def sysctls():return {key:(pathlib.Path('/proc/sys')/key.replace('.','/')).read_text().strip() for key in KEYS}
def guards():
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 assert abs(time.time()-CONTROLLER_EPOCH)<60
 fs=command(['tune2fs','-l','/dev/sda2']).splitlines();assert [x.split(':',1)[1].strip() for x in fs if x.startswith('Filesystem state:')]==['clean']
 p=pathlib.Path('/usr/sbin/policy-rc.d');s=p.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o755 and p.read_bytes()==b'#!/bin/sh\nexit 101\n'
 m=pathlib.Path('/etc/systemd/system/vpp.service');assert m.is_symlink() and m.lstat().st_uid==0 and os.readlink(m)=='/dev/null'
 assert command(['systemctl','show','vpp.service','-p','LoadState','--value']).strip()=='masked'
 assert command(['systemctl','show','vpp.service','-p','ActiveState','--value']).strip()=='inactive'
 dev=pathlib.Path('/sys/class/net/enp4s0/device');assert dev.resolve().name=='0000:04:00.0' and (dev/'driver').resolve().name=='igc' and (dev/'iommu_group').resolve().name=='28'
 route=json.loads(command(['ip','-j','route','get','172.30.126.195']))[0];assert route.get('dev')=='enp4s0' and route.get('prefsrc')=='172.30.110.211'
 marker=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-211-20261010/state.json');assert hashlib.sha256(marker.read_bytes()).hexdigest()=='146f46835119cb7341fce3974a40378bf5feab8159e419f962ded1a28d4ec792'
guards();before=l3();sys_before=sysctls();ids=identity();tz=zone()
assert ids['motd']=={'type':'absent'} and all(ids[x]['type']=='file' for x in ['hostname','issue','issue.net'])
assert ids['localtime']['type']=='link' and ids['localtime']['link']==LINK and ids['localtime']['uid']==0 and ids['localtime']['gid']==0
assert pathlib.Path('/etc/localtime').resolve()==pathlib.Path(ZONE)
assert all(not os.path.lexists('/var/lib/ngfw-system-identity/'+name) for name in NAMES)
clock={'epoch':time.time(),'monotonic':time.monotonic(),'timezone':command(['timedatectl','show','-p','Timezone','--value']).strip(),'hostname':command(['hostnamectl','--static']).strip()}
backup={'identity':ids,'zone':tz,'network':before,'sysctls':sys_before,'clock':clock}
if MODE=='inspect':print(json.dumps(backup,indent=2))
else:
 assert BASELINE['identity']==ids and BASELINE['zone']==tz and BASELINE['network']==before and BASELINE['sysctls']==sys_before
 assert BASELINE['clock']['timezone']==clock['timezone'] and BASELINE['clock']['hostname']==clock['hostname']
 fd=directory('/etc');temporary='.ngfw-211-localtime-'+secrets.token_hex(16)
 try:
  assert dict(meta(os.stat('localtime',dir_fd=fd,follow_symlinks=False)),type='link',link=os.readlink('localtime',dir_fd=fd))==ids['localtime']
  assert zone()==tz;os.symlink(ZONE,temporary,dir_fd=fd);os.chown(temporary,0,0,dir_fd=fd,follow_symlinks=False)
  assert dict(meta(os.stat('localtime',dir_fd=fd,follow_symlinks=False)),type='link',link=os.readlink('localtime',dir_fd=fd))==ids['localtime']
  os.replace(temporary,'localtime',src_dir_fd=fd,dst_dir_fd=fd);os.fsync(fd)
 finally:
  try:os.unlink(temporary,dir_fd=fd)
  except FileNotFoundError:pass
  os.close(fd)
 assert os.readlink('/etc/localtime')==ZONE and pathlib.Path('/etc/localtime').read_bytes()==pathlib.Path(ZONE).read_bytes() and zone()==tz
 # Native package helper alone performs supported identity migration; its refusal stays intact.
 p=subprocess.run(['dpkg','--configure','ngfw-agent','ngfw-meta'],env=dict(os.environ,DEBIAN_FRONTEND='noninteractive',VPP_INSTALL_SKIP_SYSCTL='1'),capture_output=True,text=True)
 after=l3();sys_after=sysctls();guard_error=None
 try:guards()
 except BaseException as e:guard_error=type(e).__name__+': '+str(e)
 tz_after=command(['timedatectl','show','-p','Timezone','--value']).strip();host_after=command(['hostnamectl','--static']).strip()
 contents={name:hashlib.sha256(pathlib.Path('/etc/'+name).read_bytes()).hexdigest() for name in NAMES}
 expected={name:ids[name]['sha256'] for name in ['hostname','issue','issue.net']};expected.update(motd=hashlib.sha256(b'').hexdigest(),localtime=ZONE_SHA)
 states={unit:command(['systemctl','show',unit,'-p','ActiveState','--value']).strip() for unit in UNITS}
 audit=subprocess.run(['dpkg','--audit'],capture_output=True,text=True)
 packages=subprocess.run(['dpkg-query','-W','-f','${Package} ${Version} ${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta','vpp','libvppinfra','vpp-drivers','vpp-plugin-core','vpp-plugin-dpdk','vpp-crypto-engines','python3-vpp-api'],capture_output=True,text=True)
 result={'configure_exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr,'original_backup_sha256':BASELINE_SHA,'network_before':before,'network_after':after,'network_equal':before==after,'sysctls_before':sys_before,'sysctls_after':sys_after,'sysctls_equal':sys_before==sys_after,'guard_error':guard_error,'timezone_equal':tz_after==clock['timezone'],'hostname_equal':host_after==clock['hostname'],'identity_content_hashes':contents,'identity_contents_equal':contents==expected,'service_states':states,'all_services_suppressed':all(x in ['inactive','failed'] for x in states.values()),'dpkg_audit':{'exit':audit.returncode,'stdout':audit.stdout,'stderr':audit.stderr},'packages':{'exit':packages.returncode,'stdout':packages.stdout,'stderr':packages.stderr},'clock_step_seconds':(time.time()-clock['epoch'])-(time.monotonic()-clock['monotonic']),'no_firstboot_or_binding':True}
 rows=[line.split() for line in packages.stdout.splitlines()];products={'ngfw-agent','ngfw-api','ngfw-web','ngfw-meta'};vpps={'vpp','libvppinfra','vpp-drivers','vpp-plugin-core','vpp-plugin-dpdk','vpp-crypto-engines','python3-vpp-api'}
 result['package_identities_exact']=len(rows)==11 and {x[0] for x in rows}==products|vpps and all(len(x)==3 and x[2]=='installed' and x[1]==('0.1.0~dev+2045ab8b3d2f' if x[0] in products else '26.06-release+ngfw3') for x in rows)
 print(json.dumps(result,indent=2))
 passed=p.returncode==0 and result['network_equal'] and result['sysctls_equal'] and guard_error is None and result['timezone_equal'] and result['hostname_equal'] and result['identity_contents_equal'] and result['all_services_suppressed'] and audit.returncode==0 and not audit.stdout and packages.returncode==0 and result['package_identities_exact'] and abs(result['clock_step_seconds'])<1
 raise SystemExit(0 if passed else (p.returncode or 2))
'''
def save(path,raw):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
 return {'file':str(path),'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
def main():
 os.umask(0o077)
 p=argparse.ArgumentParser();p.add_argument('mode',choices=['inspect','canonicalize-configure']);p.add_argument('--baseline');a=p.parse_args();baseline=None;digest=None
 if a.mode!='inspect':
  assert a.baseline;path=pathlib.Path(a.baseline).resolve();assert path.parent==PRIVATE and path.stat().st_uid==0 and path.stat().st_mode&0o777==0o600
  raw=path.read_bytes();baseline=json.loads(raw);digest=hashlib.sha256(raw).hexdigest()
 fields={'MODE':a.mode,'BASELINE':baseline,'BASELINE_SHA':digest,'CONTROLLER_EPOCH':time.time()}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 run=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 out=save(PRIVATE/('identity-native-'+a.mode+'-'+stamp+'.json'),run.stdout);err=save(PRIVATE/('identity-native-'+a.mode+'-'+stamp+'.stderr'),run.stderr)
 print(json.dumps({'mode':a.mode,'SSH_exit':run.returncode,'stdout':out,'stderr':err,'no_firstboot_or_binding':True}))
 raise SystemExit(run.returncode)
if __name__=='__main__':main()
