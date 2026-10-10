#!/usr/bin/env python3
"""Read-only manager binding/startup preflight after actual native 17-NIC seed.

Only controller-private evidence files are written. Real binding/--apply is
manager-owned and is not implemented or invoked by this worker helper.
"""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import hashlib,json,os,pathlib,re,socket,stat,subprocess
def run(a,data=None):
 p=subprocess.run(a,input=data,capture_output=True,text=True,timeout=150)
 return {'argv':a,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
def checked(a):
 q=run(a);assert q['exit']==0,(a,q['exit']);return q['stdout']
def snapshot(p):
 p=pathlib.Path(p)
 if not os.path.lexists(p):return {'type':'absent'}
 s=p.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,(str(p),'untrusted override')
 b=p.read_bytes();return {'type':'regular','mode':stat.S_IMODE(s.st_mode),'uid':s.st_uid,'gid':s.st_gid,'inode':s.st_ino,'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest(),'text':b.decode()}
def l3():
 return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
before=l3();assert before==NETWORK_BASELINE
protected=pathlib.Path('/sys/class/net/enp4s0/device')
assert protected.resolve().name=='0000:04:00.0' and (protected/'driver').resolve().name=='igc' and (protected/'iommu_group').resolve().name=='28'
assert sorted(p.name for p in (protected/'iommu_group/devices').iterdir())==['0000:04:00.0']
links={x['ifname']:x for x in json.loads(checked(['ip','-j','-d','link']))}
nics={}
for name,pci in EXPECTED.items():
 p=pathlib.Path('/sys/class/net')/name/'device';assert p.resolve().name==pci
 group=(p/'iommu_group').resolve();members=sorted(x.name for x in (group/'devices').iterdir());assert members==[pci] and group.name!='28'
 assert before['addresses'].get(name)==[] and all(x.get('dev')!=name for x in before['routes4']+before['routes6'])
 driver=(p/'driver').resolve().name;assert driver in ['i40e','igc']
 master=(pathlib.Path('/sys/class/net')/name/'master')
 try:carrier={'value':(pathlib.Path('/sys/class/net')/name/'carrier').read_text().strip(),'read_error':None}
 except OSError as e:carrier={'value':None,'read_error':e.errno}
 nics[name]={'PCI':pci,'driver':driver,'IOMMU':group.name,'group_members':members,'link':links[name],'bridge_master':master.resolve().name if master.is_symlink() else None,'sysfs_override':(p/'driver_override').read_text(),'persistent_override':snapshot('/etc/driverctl.d/pci-'+pci),'carrier':carrier}
 for key in ['bridge_master']:
  if nics[name][key]:
   bridge=nics[name][key];assert before['addresses'].get(bridge)==[] and all(x.get('dev')!=bridge for x in before['routes4']+before['routes6'])
 assert 'LOWER_UP' not in links[name].get('flags',[]) and links[name].get('operstate')!='UP' and carrier['value'] in [None,'0'],'new active data link needs classification before binding'
assert len(nics)==17 and hashlib.sha256(pathlib.Path('/usr/sbin/driverctl').read_bytes()).hexdigest()=='dfbdd15cb664676b035fe30b1314604a5a0cd1ae1f994adf7c01803c341f47fe'
unsafe=pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode')
assert not unsafe.exists() or unsafe.read_text().strip()=='N'
module_config=checked(['modprobe','-c'])
vfio_options=[x for x in module_config.splitlines() if re.match(r'^(options|install)\s+vfio(?:[-_]|\s|$)',x)]
vfio_cmdline=[x for x in pathlib.Path('/proc/cmdline').read_text().split() if 'vfio' in x]
assert vfio_options==[] and vfio_cmdline==[],'global VFIO module ID/options/install configuration requires concrete review'
module_dryrun=run(['modprobe','--dry-run','--verbose','--ignore-install','vfio-pci','ids=']);assert module_dryrun['exit']==0
units={u:checked(['systemctl','show',u,'-p','ActiveState','-p','NRestarts']) for u in ['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']}
assert all('ActiveState=active\n' in x and 'NRestarts=0\n' in x for x in units.values())
with socket.create_connection(('172.30.126.195',22),timeout=5):pass
document=json.dumps(DOCUMENT,separators=(',',':')).encode()
render=run(['/usr/lib/ngfw/bin/ngfw-startupgen','--current','/etc/vpp/startup.conf','--mgmt-if','enp4s0','--mgmt-pci','0000:04:00.0','-'],document.decode())
assert render['exit']==0,'actual startup generation failed'
rendered=render['stdout'].encode();render_sha=hashlib.sha256(rendered).hexdigest();live=pathlib.Path('/etc/vpp/startup.conf').read_bytes();live_sha=hashlib.sha256(live).hexdigest()
rows=re.findall(r'^\s*dev\s+([0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7])(?:\s|\{)',render['stdout'],re.M)
assert len(rows)==17 and set(rows)==set(EXPECTED.values()) and 'blacklist 0000:04:00.0' in render['stdout']
for plugin in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so']:assert 'plugin '+plugin+' { enable }' in render['stdout']
fd=os.memfd_create('ngfw-reviewed-seed-document',os.MFD_CLOEXEC);os.write(fd,document);os.lseek(fd,0,os.SEEK_SET)
try:
 # A seekable anonymous descriptor makes repeated render/diff reads possible
 # without placing a config file on the target. The parent owns it until exit.
 dry=run(['/usr/lib/ngfw/apply-startup.sh','--mode','product','--doc','/proc/'+str(os.getpid())+'/fd/'+str(fd),'--approve-rendering',render_sha,'--expect-sha256',live_sha,'--expect-new-sha256',render_sha,'--mgmt-if','enp4s0','--mgmt-peer','172.30.126.195','--mgmt-probe','tcp:172.30.126.195:22'])
finally:os.close(fd)
assert before==l3(),'network changed during read-only preflight'
result={'native_seed_proof_SHA':PROOF_SHA,'network_before':before,'network_equal':True,'protected_PCI':'0000:04:00.0','protected_driver':'igc','protected_group':'28','data_nics':nics,'links':links,'driverctl_list_overrides':run(['driverctl','list-overrides']),'driverctl_list_persisted':run(['driverctl','list-persisted']),'unsafe_noiommu':unsafe.read_text().strip() if unsafe.exists() else 'module-not-loaded','vfio_module_options':vfio_options,'vfio_cmdline_options':vfio_cmdline,'vfio_module_readonly_dryrun':module_dryrun,'units':units,'live_SHA':live_sha,'document_SHA':hashlib.sha256(document).hexdigest(),'render_SHA':render_sha,'render':render,'dryrun':dry,'no_target_config_module_network_driver_service_or_startup_mutation':True}
print(json.dumps(result,indent=2));raise SystemExit(0 if dry['exit']==0 else 2)
'''
def save(p,raw):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(raw),'SHA':hashlib.sha256(raw).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--proof',required=True);p.add_argument('--proof-sha256',required=True);a=p.parse_args()
 original=pathlib.Path(a.proof);assert not original.is_symlink();proof=original.resolve();assert proof.parent==PRIVATE and proof.stat().st_uid==0 and stat.S_IMODE(proof.stat().st_mode)==0o600
 raw=proof.read_bytes();assert hashlib.sha256(raw).hexdigest()==a.proof_sha256;d=json.loads(raw)
 assert d['mode']=='after' and d['observation_PASS'] and d['revision_expectation_PASS'] and d['required_plugins_loaded'] and d['manager_runtime_replaced'] and d['seeded17_exact'] and d['candidate_equal_running'] and d['no_pending_commit'] and d['state_RPC_HTTP_PASS'] and d['network_equal'] and d['all17_still_kernel'] and d['sysctls_equal'] and d['DNS_equal'] and d['foreign_nft_unchanged'] and d['new_storage_errors']==[] and d['ioerr_before']==d['ioerr_after']
 fields={'PROOF_SHA':a.proof_sha256,'NETWORK_BASELINE':d['network_after'],'EXPECTED':{name:q['PCI'] for name,q in d['inventory_after'].items()},'DOCUMENT':d['running']['data']}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 r=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 print(json.dumps({'SSH_exit':r.returncode,'stdout':save(PRIVATE/('data-preflight-'+stamp+'.json'),r.stdout),'stderr':save(PRIVATE/('data-preflight-'+stamp+'.stderr'),r.stderr),'no_real_bind_or_apply':True}));raise SystemExit(r.returncode)
if __name__=='__main__':main()
