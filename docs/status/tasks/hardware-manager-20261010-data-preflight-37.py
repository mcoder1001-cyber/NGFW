#!/usr/bin/env python3
"""ROOT read-only corrected native seven-port preflight; no physical mutation."""
import argparse,datetime,hashlib,json,os,pathlib,re,stat,subprocess
OUTPUT=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
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
def nft(d):
 if isinstance(d,dict):return {k:nft({a:b for a,b in v.items() if a not in ['packets','bytes']}) if k=='counter' and isinstance(v,dict) else nft(v) for k,v in d.items() if k!='metainfo'}
 if isinstance(d,list):return [nft(x) for x in d if not(isinstance(x,dict) and 'metainfo' in x)]
 return d
def l3():
 return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()=='c8d66ea9-afab-4228-a293-00c198745040'
assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service')
assert hashlib.sha256(pathlib.Path('/usr/sbin/ngfw-agent').read_bytes()).hexdigest()=='b3c7cc10671abcab6d11c7dce12323f284c64822a15cfda1c46ba78803283744'
assert hashlib.sha256(pathlib.Path('/usr/lib/ngfw/bin/ngfw-startupgen').read_bytes()).hexdigest()=='55e490bad1c302505231df105a1d516cb074678b38b85e28740c399020aac619'
packages=checked(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']);assert len(packages.splitlines())==4 and all(x.split('\t')[1:]==['0.1.0~dev+97ae88ee5b6a','installed'] for x in packages.splitlines())
ioerr_before=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();assert int(ioerr_before,16)==6
kernel_before=checked(['dmesg','--color=never'])
assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
before=l3();assert before==NETWORK_BASELINE
nft_before=json.loads(checked(['nft','-j','list','ruleset']));assert nft(nft_before)==nft(NFT_BASELINE)
assert {k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in SYSCTLS}==SYSCTLS
assert {p:{'SHA':hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest(),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in DNS}==DNS
protected=pathlib.Path('/sys/class/net/enp12s0/device')
assert protected.resolve().name=='0000:0c:00.0' and (protected/'driver').resolve().name=='igc' and (protected/'iommu_group').resolve().name=='58'
assert sorted(p.name for p in (protected/'iommu_group/devices').iterdir())==['0000:0c:00.0']
links={x['ifname']:x for x in json.loads(checked(['ip','-j','-d','link']))}
nics={}
for name,pci in EXPECTED.items():
 p=pathlib.Path('/sys/class/net')/name/'device';assert p.resolve().name==pci
 group=(p/'iommu_group').resolve();members=sorted(x.name for x in (group/'devices').iterdir());assert members==[pci] and group.name!='58'
 assert before['addresses'].get(name)==[] and all(x.get('dev')!=name for x in before['routes4']+before['routes6'])
 driver=(p/'driver').resolve().name;assert driver=='igc'
 master=(pathlib.Path('/sys/class/net')/name/'master')
 try:carrier={'value':(pathlib.Path('/sys/class/net')/name/'carrier').read_text().strip(),'read_error':None}
 except OSError as e:carrier={'value':None,'read_error':e.errno}
 nics[name]={'PCI':pci,'driver':driver,'IOMMU':group.name,'group_members':members,'link':links[name],'bridge_master':master.resolve().name if master.is_symlink() else None,'sysfs_override':(p/'driver_override').read_text(),'persistent_override':snapshot('/etc/driverctl.d/pci-'+pci),'carrier':carrier}
 for key in ['bridge_master']:
  if nics[name][key]:
   bridge=nics[name][key];assert before['addresses'].get(bridge)==[] and all(x.get('dev')!=bridge for x in before['routes4']+before['routes6'])
 assert 'LOWER_UP' not in links[name].get('flags',[]) and links[name].get('operstate')!='UP' and carrier['value'] in [None,'0'],'new active data link needs classification before binding'
assert len(nics)==7 and hashlib.sha256(pathlib.Path('/usr/sbin/driverctl').read_bytes()).hexdigest()=='dfbdd15cb664676b035fe30b1314604a5a0cd1ae1f994adf7c01803c341f47fe'
unsafe=pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode')
assert not unsafe.exists() or unsafe.read_text().strip()=='N'
module_config=checked(['modprobe','-c'])
vfio_options=[x for x in module_config.splitlines() if re.match(r'^(options|install)\s+vfio(?:[-_]|\s|$)',x)]
vfio_cmdline=[x for x in pathlib.Path('/proc/cmdline').read_text().split() if 'vfio' in x]
assert vfio_options==[] and vfio_cmdline==[],'global VFIO module ID/options/install configuration requires concrete review'
ids=pathlib.Path('/sys/module/vfio_pci/parameters/ids')
if ids.exists():
 ids_value=ids.read_text().strip();assert ids_value=='','exposed global VFIO IDs are unsafe';ids_observation='exposed-empty'
else:ids_observation='loaded-parameter-unexposed' if pathlib.Path('/sys/module/vfio_pci').exists() else 'module-not-loaded'
module_dryrun=run(['modprobe','--dry-run','--verbose','--ignore-install','vfio-pci','ids=']);assert module_dryrun['exit']==0
units={u:checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts']) for u in ['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']}
assert all('ActiveState=active\n' in units[u] and 'NRestarts=0\n' in units[u] for u in ['vpp.service','nginx.service'])
assert all('ActiveState=inactive\n' in units[u] and 'NRestarts=0\n' in units[u] for u in ['ngfw-agent.service','ngfw-api.service'])
with socket.create_connection(('172.30.126.195',22),timeout=5):pass
document=json.dumps(DOCUMENT,separators=(',',':')).encode()
render=run(['/usr/lib/ngfw/bin/ngfw-startupgen','--current','/etc/vpp/startup.conf','--mgmt-if','enp12s0','--mgmt-pci','0000:0c:00.0','-'],document.decode())
assert render['exit']==0,'actual startup generation failed'
rendered=render['stdout'].encode();render_sha=hashlib.sha256(rendered).hexdigest();live=pathlib.Path('/etc/vpp/startup.conf').read_bytes();live_sha=hashlib.sha256(live).hexdigest()
assert live_sha==LIVE_SHA,'startup changed after native resource proof'
assert re.findall(r'^\s*buffers-per-numa\s+(\d+)\s*$',render['stdout'],re.M)==[],'unexpected buffer override for seven NICs'
rows=re.findall(r'^\s*dev\s+([0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7])(?:\s|\{)',render['stdout'],re.M)
assert len(rows)==7 and set(rows)==set(EXPECTED.values()) and '0000:0c:00.0' not in rows and not re.search(r'^\s*blacklist\s',render['stdout'],re.M)
for plugin in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so']:assert 'plugin '+plugin+' { enable }' in render['stdout']
fd=os.memfd_create('ngfw-reviewed-seed-document',os.MFD_CLOEXEC);os.write(fd,document);os.lseek(fd,0,os.SEEK_SET)
try:
 # A seekable anonymous descriptor makes repeated render/diff reads possible
 # without placing a config file on the target. The parent owns it until exit.
 dry=run(['/usr/lib/ngfw/apply-startup.sh','--mode','product','--doc','/proc/'+str(os.getpid())+'/fd/'+str(fd),'--approve-rendering',render_sha,'--expect-sha256',live_sha,'--expect-new-sha256',render_sha,'--mgmt-if','enp12s0','--mgmt-peer','172.30.126.195','--mgmt-probe','tcp:172.30.126.195:22'])
finally:os.close(fd)
assert before==l3(),'network changed during read-only preflight'
unit_states={u:dict(x.split('=',1) for x in text.splitlines()) for u,text in units.items()}
assert all(checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts'])==text for u,text in units.items())
assert nft(json.loads(checked(['nft','-j','list','ruleset'])))==nft(nft_before)
assert {k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in SYSCTLS}==SYSCTLS
assert {p:{'SHA':hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest(),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in DNS}==DNS
result={'native_seed_proof_SHA':PROOF_SHA,'confirmed_native_revision':1,'buffers_per_numa_rendered':None,'vfio_ids_observation':ids_observation,'network_before':before,'network_equal':True,'protected_PCI':'0000:0c:00.0','protected_driver':'igc','protected_group':'58','data_nics':nics,'links':links,'driverctl_list_overrides':run(['driverctl','list-overrides']),'driverctl_list_persisted':run(['driverctl','list-persisted']),'unsafe_noiommu':unsafe.read_text().strip() if unsafe.exists() else 'module-not-loaded','vfio_module_options':vfio_options,'vfio_cmdline_options':vfio_cmdline,'vfio_module_readonly_dryrun':module_dryrun,'units':units,'unit_states':unit_states,'nft':nft_before,'live_SHA':live_sha,'document_SHA':hashlib.sha256(document).hexdigest(),'render_SHA':render_sha,'render':render,'dryrun':dry,'no_target_config_module_network_driver_service_or_startup_mutation':True}
kernel_after=checked(['dmesg','--color=never']);assert kernel_after.startswith(kernel_before);new=kernel_after[len(kernel_before):].splitlines();bad=[x for x in new if re.search(r'EXT4-fs error|I/O error|Buffer I/O|blk_update_request|\bUNC\b|hard resetting link|failed command|ata\d.*(?:error|reset)|sd\s+\S+.*(?:error|fail)',x,re.I)];assert not bad
ioerr_after=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();assert ioerr_after==ioerr_before and int(ioerr_after,16)==6
result.update(kernel_before=kernel_before,kernel_after=kernel_after,new_storage_errors=bad,storage_ioerr=ioerr_after,ioerr_before=ioerr_before,ioerr_after=ioerr_after,fixed_native_version='0.1.0~dev+97ae88ee5b6a',seeded7_exact=True)
print(json.dumps(result,indent=2));raise SystemExit(0 if dry['exit']==0 else 2)
'''

def private(p,sha):
 p=pathlib.Path(p);assert p.parent==OUTPUT and not p.is_symlink();s=p.stat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 b=p.read_bytes();assert re.fullmatch('[0-9a-f]{64}',sha) and hashlib.sha256(b).hexdigest()==sha;return json.loads(b)
def save(p,b):
 assert p.parent==OUTPUT;fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(OUTPUT,os.O_DIRECTORY);os.fsync(fd);os.close(fd);return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--seed-proof',required=True);p.add_argument('--seed-sha',required=True);a=p.parse_args()
 assert a.seed_sha=='ba5439fcb0a7b2ccce59c1cf3b09f177e76e8de8cb1fe4e75d12840743b659ea';d=private(a.seed_proof,a.seed_sha)
 assert d['native_seed7_PASS'] and d['failure'] is None and d['seeded7_exact'] and d['candidate_equal_running'] and d['no_pending_commit'] and d['network_equal'] and d['all7_still_kernel'] and d['sysctls_equal'] and d['DNS_network_files_equal'] and d['foreign_nft_unchanged'] and d['new_storage_errors']==[] and d['running']['revision']=='1'
 doc=d['running']['data'];assert doc['dataplane'].get('buffersPerNuma') is None and len(d['inventory_after'])==7
 fields={'PROOF_SHA':a.seed_sha,'LIVE_SHA':'c1b121e410961cb64869909a2cd82448984c0472ada11ab0a32234897b86b3c8','NFT_BASELINE':d['nft_after'],'SYSCTLS':d['sysctls_after'],'DNS':d['DNS_network_files_after'],'NETWORK_BASELINE':d['network_after'],'EXPECTED':{n:q['PCI'] for n,q in d['inventory_after'].items()},'DOCUMENT':doc}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 q=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37','python3 -'],input=code.encode(),capture_output=True)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');out={'SSH_exit':q.returncode,'read_only':True}
 for k,b in [('stdout',q.stdout),('stderr',q.stderr)]:out[k]=save(OUTPUT/('manager-data-preflight37-'+stamp+'.'+k),b)
 print(json.dumps(out));raise SystemExit(q.returncode)
if __name__=='__main__':main()
