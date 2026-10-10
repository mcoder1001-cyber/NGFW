#!/usr/bin/env python3
"""Read-only canonical required-plugin rendering after actual seed refusal.

No stored config/candidate/revision edit. ROOT manager owns any actual startup
application; this helper only renders/dry-runs and saves private evidence.
"""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211')
SSH=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.110.211']
REMOTE=r'''
import hashlib,json,os,pathlib,re,socket,subprocess
def run(a,data=None):
 p=subprocess.run(a,input=data,capture_output=True,text=True,timeout=120);return {'argv':a,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
def checked(a):
 q=run(a);assert q['exit']==0,(a,q['exit']);return q['stdout']
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
assert l3()==NETWORK
d=pathlib.Path('/sys/class/net/enp4s0/device');assert d.resolve().name=='0000:04:00.0' and (d/'driver').resolve().name=='igc' and (d/'iommu_group').resolve().name=='28'
for name,q in INVENTORY.items():
 p=pathlib.Path('/sys/class/net')/name/'device';assert p.resolve().name==q['PCI'] and (p/'driver').resolve().name==q['driver'] and (p/'iommu_group').resolve().name==q['group']
for unit,q in UNITS.items():
 for k in ['ActiveState','MainPID','NRestarts']:assert checked(['systemctl','show',unit,'-p',k,'--value']).strip()==q[k]
live=pathlib.Path('/etc/vpp/startup.conf').read_bytes();assert hashlib.sha256(live).hexdigest()==LIVE_SHA
assert pathlib.Path('/proc/sys/vm/nr_hugepages').read_text().strip()=='1024'
with socket.create_connection(('172.30.126.195',22),timeout=5):pass
for plugin in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so']:assert (pathlib.Path('/usr/lib/x86_64-linux-gnu/vpp_plugins')/plugin).is_file()
kernel_before=checked(['dmesg','--color=never']);io_before=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip()
payload=json.dumps(DOCUMENT,separators=(',',':')).encode()
render=run(['/usr/lib/ngfw/bin/ngfw-startupgen','--current','/etc/vpp/startup.conf','--mgmt-if','enp4s0','--mgmt-pci','0000:04:00.0','-'],payload.decode());assert render['exit']==0
text=render['stdout'];assert '  no-pci\n' in text and 'blacklist 0000:04:00.0' in text and not any(re.match(r'\s*dev\s+(?!default(?:\s|\{))',x) for x in text.splitlines())
for plugin in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so']:assert 'plugin '+plugin+' { enable }' in text
render_sha=hashlib.sha256(text.encode()).hexdigest();fd=os.memfd_create('ngfw-required-plugins-noPCI',os.MFD_CLOEXEC);os.write(fd,payload);os.lseek(fd,0,os.SEEK_SET)
try:dry=run(['/usr/lib/ngfw/apply-startup.sh','--mode','product','--doc','/proc/'+str(os.getpid())+'/fd/'+str(fd),'--approve-rendering',render_sha,'--expect-sha256',LIVE_SHA,'--expect-new-sha256',render_sha,'--mgmt-if','enp4s0','--mgmt-peer','172.30.126.195','--mgmt-probe','tcp:172.30.126.195:22'])
finally:os.close(fd)
after=l3();io_after=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();kernel_after=checked(['dmesg','--color=never'])
new=kernel_after.splitlines()[len(kernel_before.splitlines()):] if kernel_after.startswith(kernel_before) else None
bad=re.compile(r'(EXT4-fs error|Buffer I/O error|I/O error, dev sda|ata\d.*(hard resetting|failed command|error:.*(UNC|ICRC)))',re.I)
errors=None if new is None else [x for x in new if bad.search(x)]
print(json.dumps({'failed_seed_proof_SHA':PROOF_SHA,'render_document':DOCUMENT,'document_SHA':hashlib.sha256(payload).hexdigest(),'live_SHA':LIVE_SHA,'render_SHA':render_sha,'render':render,'dryrun':dry,'network_equal':after==NETWORK,'ioerr_before':io_before,'ioerr_after':io_after,'new_storage_errors':errors,'kernel_before':kernel_before,'kernel_after':kernel_after,'no_PCI_binding_stored_document_revision_or_real_startup_apply':True},indent=2))
raise SystemExit(0 if dry['exit']==0 and after==NETWORK and io_before==io_after and errors==[] else 2)
'''
def save(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--proof',required=True);p.add_argument('--proof-sha256',required=True);a=p.parse_args()
 q=pathlib.Path(a.proof).resolve();assert q.parent==PRIVATE and q.stat().st_uid==0 and stat.S_IMODE(q.stat().st_mode)==0o600;b=q.read_bytes();assert hashlib.sha256(b).hexdigest()==a.proof_sha256;j=json.loads(b)
 assert j['failure']=='actual system.seed-defaults revision1 not observed' and j['last_observed_config']['revision']=='0' and j['network_equal'] and j['all17_still_kernel'] and j['new_storage_errors']==[] and j['ioerr_before']==j['ioerr_after']
 doc=j['last_observed_config']['data'];assert not any(x.get('physical') for x in doc.get('interfaces',{}).values()) and doc['dataplane']['devices']=={} and doc['dataplane']['pciWhitelist']==[]
 doc=json.loads(json.dumps(doc));doc['dataplane'].setdefault('plugins',{}).setdefault('switches',{}).update({p:True for p in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so']})
 fb_raw=(PRIVATE/'firstboot-apply-20261010T120628Z.json').read_bytes();assert hashlib.sha256(fb_raw).hexdigest()==j['firstboot_proof_SHA'];fb=json.loads(fb_raw);live_sha=fb['files']['/etc/vpp/startup.conf']['sha256']
 fields={'PROOF_SHA':a.proof_sha256,'NETWORK':j['network_after'],'INVENTORY':j['inventory_after'],'UNITS':j['unit_states'],'LIVE_SHA':live_sha,'DOCUMENT':doc};code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE
 r=subprocess.run(SSH+['python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
 print(json.dumps({'SSH_exit':r.returncode,'stdout':save(PRIVATE/('noPCI-plugin-preflight-'+stamp+'.json'),r.stdout),'stderr':save(PRIVATE/('noPCI-plugin-preflight-'+stamp+'.stderr'),r.stderr),'read_only':True}));raise SystemExit(r.returncode)
if __name__=='__main__':main()
