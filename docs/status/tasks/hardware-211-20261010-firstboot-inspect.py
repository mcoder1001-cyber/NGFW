#!/usr/bin/env python3
"""Read-only installed firstboot, firewall, VM and identity evidence; private output."""
import datetime,hashlib,json,os,pathlib,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import hashlib,json,os,pathlib,stat,subprocess,time
def command(args):
 p=subprocess.run(args,capture_output=True,text=True,timeout=30)
 return {'argv':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
def metadata(path,contents=False):
 p=pathlib.Path(path)
 if not os.path.lexists(p):return {'path':path,'exists':False}
 s=p.lstat();d={'path':path,'exists':True,'mode':stat.S_IMODE(s.st_mode),'uid':s.st_uid,'gid':s.st_gid,'bytes':s.st_size,'dev':s.st_dev,'inode':s.st_ino}
 if stat.S_ISLNK(s.st_mode):d['link']=os.readlink(p)
 elif stat.S_ISREG(s.st_mode):
  raw=p.read_bytes();d['sha256']=hashlib.sha256(raw).hexdigest()
  if contents:d['contents']=raw.decode()
 return d
units=['nftables.service','ngfw-firewall-bootstrap.service','ngfw-firstboot.service','vpp.service','ngfw-agent.service','ngfw-api.service','postgresql.service','postgresql@18-main.service','valkey-server.service','nginx.service','frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','snmpd.service','keepalived.service']
paths=['/usr/lib/ngfw/firstboot.sh','/usr/lib/ngfw/firewall-bootstrap.sh','/usr/lib/ngfw/render-base-policy.py','/usr/lib/ngfw/tls-bootstrap.sh','/usr/lib/ngfw/bin/ngfw-startupgen','/usr/lib/ngfw/api/bootstrap-db.mjs','/usr/lib/systemd/system/nftables.service.d/nftables-ngfw.conf','/etc/vpp/startup.conf','/etc/nftables.conf','/etc/nftables.d/ngfw-base.nft','/etc/ngfw/base-policy.env','/etc/ngfw/bootstrap.env','/etc/ngfw/api.env','/etc/ngfw/agent.env','/var/lib/ngfw/firstboot-complete','/var/lib/ngfw/secret.key','/etc/ngfw/tls/server.crt','/etc/ngfw/tls/server.key']
result={'UTC_epoch':time.time(),'boot_id':pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip(),'commands':[]}
for args in [['ip','-j','addr'],['ip','-j','-4','route','show','table','all'],['ip','-j','-6','route','show','table','all'],['ip','-j','-4','rule'],['ip','-j','-6','rule'],['ip','-j','route','get','172.30.126.195'],['nft','-j','list','ruleset'],['dpkg','--audit'],['tune2fs','-l','/dev/sda2'],['dmesg','--color=never'],['systemctl','cat','nftables.service','ngfw-firewall-bootstrap.service','ngfw-firstboot.service'],['systemctl','show','nftables.service','-p','FragmentPath','-p','DropInPaths','-p','Requires','-p','After','-p','ExecStart','-p','ExecReload','-p','ExecStop'],['timedatectl','show','-p','Timezone','-p','LocalRTC','-p','NTP']]:result['commands'].append(command(args))
result['states']={u:command(['systemctl','show',u,'-p','ActiveState','-p','UnitFileState','-p','LoadState']) for u in units}
result['files']=[metadata(p) for p in paths]
result['sysctl_files']=[metadata(str(p),contents=True) for directory in ['/etc/sysctl.d','/run/sysctl.d','/usr/local/lib/sysctl.d','/usr/lib/sysctl.d','/lib/sysctl.d'] if pathlib.Path(directory).is_dir() for p in sorted(pathlib.Path(directory).glob('*.conf'))]
result['sysctl_conf']=metadata('/etc/sysctl.conf',contents=True)
result['VM']={key:(pathlib.Path('/proc/sys/vm')/key).read_text().strip() for key in ['nr_hugepages','hugetlb_shm_group']}
result['meminfo']=pathlib.Path('/proc/meminfo').read_text();result['NUMA_online']=pathlib.Path('/sys/devices/system/node/online').read_text().strip()
result['localtime_chain']=[metadata(p) for p in ['/etc/localtime','/var/lib/ngfw-system-identity/localtime','/usr/share/zoneinfo/Asia/Tehran']]
result['localtime_dereferenced_SHA']=hashlib.sha256(pathlib.Path('/etc/localtime').read_bytes()).hexdigest()
result['localtime_realpath']=str(pathlib.Path('/etc/localtime').resolve())
dev=pathlib.Path('/sys/class/net/enp4s0/device');result['protected']={'PCI':dev.resolve().name,'driver':(dev/'driver').resolve().name,'IOMMU_group':(dev/'iommu_group').resolve().name,'MAC':pathlib.Path('/sys/class/net/enp4s0/address').read_text().strip()}
result['root_dev']=[os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev)]
result['storage_ioerr']=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip()
result['guard_policy']=metadata('/usr/sbin/policy-rc.d');result['guard_mask']=metadata('/etc/systemd/system/vpp.service')
result['fresh_timedated_restart_performed']=False
print(json.dumps(result,indent=2))
raise SystemExit(0 if all(x['exit']==0 for x in result['commands']) and all(x['exit']==0 for x in result['states'].values()) else 2)
'''
def save(path,raw):
 fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
 return {'file':str(path),'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
def main():
 os.umask(0o077);run=subprocess.run(SSH+['python3 -'],input=REMOTE.encode(),capture_output=True)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 out=save(PRIVATE/('firstboot-installed-inspect-'+stamp+'.json'),run.stdout);err=save(PRIVATE/('firstboot-installed-inspect-'+stamp+'.stderr'),run.stderr)
 print(json.dumps({'SSH_exit':run.returncode,'stdout':out,'stderr':err,'read_only':True}));raise SystemExit(run.returncode)
if __name__=='__main__':main()
