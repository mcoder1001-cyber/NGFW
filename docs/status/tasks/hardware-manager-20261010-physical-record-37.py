#!/usr/bin/env python3
"""Prepare a finite private physical recovery record; never bind or start units."""
import argparse,datetime,hashlib,json,os,pathlib,stat,subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
WORKER_PRIVATE=PRIVATE
SOURCE=pathlib.Path('/root/ngfw-wt/hardware-manager-20261010/docs/status/tasks')
REMOTE=r'''
import hashlib,json,os,pathlib,socket,stat,subprocess
os.umask(0o077)
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery/hardware-manager-20261010-data-37')
def checked(a):
 q=subprocess.run(a,capture_output=True,text=True,timeout=30);assert q.returncode==0,(a,q.returncode);return q.stdout.strip()
def trusted(p):
 p=pathlib.Path(p);fd=os.open('/',os.O_DIRECTORY)
 try:
  for c in p.parts[1:]:
   child=os.open(c,os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd);os.close(fd);fd=child;s=os.fstat(fd);assert s.st_uid==0 and not s.st_mode&0o022
 finally:os.close(fd)
def fresh(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
def network():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def guard():
 assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()=='c8d66ea9-afab-4228-a293-00c198745040'
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2)
 assert not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 assert [x.split(':',1)[1].strip() for x in checked(['tune2fs','-l','/dev/sda2']).splitlines() if x.startswith('Filesystem state:')]==['clean']
 assert int(pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip(),16)==6
 assert network()==PREFLIGHT['network_before']
 m=pathlib.Path('/sys/class/net/enp12s0/device');g=(m/'iommu_group').resolve(strict=True)
 assert m.resolve().name=='0000:0c:00.0' and (m/'driver').resolve().name=='igc' and g.name=='58' and sorted(x.name for x in (g/'devices').iterdir())==['0000:0c:00.0']
 links={x['ifname']:x for x in json.loads(checked(['ip','-j','-d','link']))}
 for name,q in PREFLIGHT['data_nics'].items():
  assert links[name]==q['link']
  assert PREFLIGHT['network_before']['addresses'].get(name)==[] and not any(r.get('dev')==name for r in PREFLIGHT['network_before']['routes4']+PREFLIGHT['network_before']['routes6'])
  p=pathlib.Path('/sys/class/net')/name/'device';g=(p/'iommu_group').resolve(strict=True)
  assert p.resolve().name==q['PCI'] and (p/'driver').resolve().name==q['driver'] and g.name==q['IOMMU'] and sorted(x.name for x in (g/'devices').iterdir())==q['group_members']
  assert (p/'driver_override').read_text()==q['sysfs_override'] and q['sysfs_override'].strip() in ['','(null)'] and not os.path.lexists('/etc/driverctl.d/pci-'+q['PCI'])
  master=pathlib.Path('/sys/class/net')/name/'master';assert (master.resolve().name if master.is_symlink() else None)==q['bridge_master']
 for u,pid in EXPECTED_PIDS.items():
  assert checked(['systemctl','show',u,'-p','ActiveState','--value'])==('inactive' if pid=='0' else 'active') and checked(['systemctl','show',u,'-p','MainPID','--value'])==pid and checked(['systemctl','show',u,'-p','NRestarts','--value'])=='0'
 assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service')
 assert hashlib.sha256(pathlib.Path('/etc/vpp/startup.conf').read_bytes()).hexdigest()=='c1b121e410961cb64869909a2cd82448984c0472ada11ab0a32234897b86b3c8'
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
assert len(PREFLIGHT['data_nics'])==7 and PREFLIGHT['dryrun']['exit']==0 and PREFLIGHT['network_equal'] and PREFLIGHT['no_target_config_module_network_driver_service_or_startup_mutation']
assert hashlib.sha256(DOCUMENT).hexdigest()==PREFLIGHT['document_SHA']
render=PREFLIGHT['render']['stdout'].encode();assert hashlib.sha256(render).hexdigest()==PREFLIGHT['render_SHA'] and b'buffers-per-numa' not in render
q=json.loads(DOCUMENT);assert q['dataplane'].get('buffersPerNuma') is None and q['dataplane']['managementPci']==['0000:0c:00.0']
assert set(q['dataplane']['pciWhitelist'])=={x['PCI'] for x in PREFLIGHT['data_nics'].values()} and set(q['dataplane']['devices'])==set(q['dataplane']['pciWhitelist'])
for name,row in PREFLIGHT['data_nics'].items():assert q['interfaces'][name]['physical']=={'pci':row['PCI'],'owner':'dataplane','builtIn':True}
guard();assert not os.path.lexists(RECORD);trusted(RECORD.parent)
owned={}
for target,key,mode in [('/usr/local/libexec/ngfw-hardware-37-bind','binder',0o755),('/etc/systemd/system/ngfw-hardware-37-bind.service','unit',0o644),('/etc/systemd/system/vpp.service.d/20-hardware-37-bind.conf','dropin',0o644)]:
 assert not os.path.lexists(target);p=pathlib.Path(target).parent
 while not os.path.lexists(p):p=p.parent
 trusted(p);owned[target]={'before_absent':True,'SHA':hashlib.sha256(FILES[key]).hexdigest(),'mode':mode}
startup=pathlib.Path('/etc/vpp/startup.conf');st=startup.lstat();assert stat.S_ISREG(st.st_mode) and st.st_uid==st.st_gid==0 and stat.S_IMODE(st.st_mode)==0o644
manifest={'schema':1,'task':'hardware-37-20261010','root_dev':[8,2],'data_nics':PREFLIGHT['data_nics'],'network_before':PREFLIGHT['network_before'],'new_startup_SHA':PREFLIGHT['render_SHA'],'owned_files':owned}
record_before={'native_seed_proof_SHA':SEED_SHA,'expected_unit_PIDs':EXPECTED_PIDS,'preflight_SHA':PREFLIGHT_SHA,'document_SHA':PREFLIGHT['document_SHA'],'startup':{'uid':st.st_uid,'gid':st.st_gid,'mode':stat.S_IMODE(st.st_mode),'SHA':PREFLIGHT['live_SHA']},'units':{u:checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts','-p','UnitFileState','-p','ActiveEnterTimestampMonotonic']) for u in ['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']},'module_options':checked(['modprobe','-c']),'cmdline':pathlib.Path('/proc/cmdline').read_text(),'modules':{n:{'loaded':pathlib.Path('/sys/module/'+n).exists(),'parameters':{k:(pathlib.Path('/sys/module')/n/'parameters'/k).read_text().strip() if (pathlib.Path('/sys/module')/n/'parameters'/k).exists() else None for k in ['ids','enable_unsafe_noiommu_mode']}} for n in ['vfio','vfio_pci','i40e','igc']}}
RECORD.mkdir(mode=0o700);fd=os.open(RECORD.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
fresh(RECORD/'startup.before',startup.read_bytes());fresh(RECORD/'physical.doc.json',DOCUMENT);fresh(RECORD/'physical.rendered.conf',render)
for name,b in FILES.items():fresh(RECORD/(name+'.source'),b)
fresh(RECORD/'before.json',json.dumps(record_before,indent=2).encode());b=json.dumps(manifest,indent=2).encode();fresh(RECORD/'manifest.json',b);guard()
print(json.dumps({'record_created':True,'record':str(RECORD),'manifest':manifest,'manifest_SHA':hashlib.sha256(b).hexdigest(),'record_before':record_before,'sources_SHA':{k:hashlib.sha256(v).hexdigest() for k,v in FILES.items()},'network_equal':True,'no_service_driver_or_startup_mutation':True},indent=2))
'''
def private(p,parent):
 p=pathlib.Path(p);assert p.parent==parent and not p.is_symlink();s=p.stat();assert s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600;return p.read_bytes()
def save(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
 return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--preflight',required=True);p.add_argument('--preflight-sha',required=True);p.add_argument('--document',required=True);p.add_argument('--document-sha',required=True);p.add_argument('--seed',required=True);p.add_argument('--seed-sha',required=True);a=p.parse_args()
 b=private(a.preflight,PRIVATE);assert hashlib.sha256(b).hexdigest()==a.preflight_sha;pre=json.loads(b)
 doc=private(a.document,PRIVATE);assert hashlib.sha256(doc).hexdigest()==a.document_sha==pre['document_SHA']
 sb=private(a.seed,PRIVATE);assert hashlib.sha256(sb).hexdigest()==a.seed_sha;seed=json.loads(sb);assert seed['seeded7_exact'] and seed['all7_still_kernel'] and seed['failure'] is None and seed['network_equal'] and seed['new_storage_errors']==[]
 assert json.loads(doc)==seed['running']['data']
 assert seed['network_after']==pre['network_before']
 states=pre['unit_states'];assert set(states)=={'vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service'}
 assert all(states[u]['ActiveState']=='active' and states[u]['NRestarts']=='0' and states[u]['MainPID'].isdigit() and int(states[u]['MainPID'])>1 for u in ['vpp.service','nginx.service'])
 assert all(states[u]=={'ActiveState':'inactive','MainPID':'0','NRestarts':'0'} for u in ['ngfw-agent.service','ngfw-api.service'])
 assert pre['fixed_native_version']=='0.1.0~dev+97ae88ee5b6a' and pre['new_storage_errors']==[]
 pids={u:x['MainPID'] for u,x in states.items()}
 files={}
 for key,name,h in [('binder','hardware-manager-20261010-boot-bind-37.py','0de00a7e5f34097fe530fb8ae1dab3b3e541826facda704eb7a4c7511b13086d'),('unit','hardware-manager-20261010-boot-bind-37.service','d537f6c4aa66d74a20e982a06b4c1003f8858367f3c14860df534a468d4dc153'),('rollback','hardware-manager-20261010-driver-rollback-37.py','00bed9a07c13f079eb56c384ce392a16b08083c4d101b872995139b50ae185dd')]:
  b=(SOURCE/name).read_bytes();assert hashlib.sha256(b).hexdigest()==h;files[key]=b
 files['dropin']=b'[Unit]\nRequires=ngfw-hardware-37-bind.service\nAfter=ngfw-hardware-37-bind.service\n'
 code='EXPECTED_PIDS='+repr(pids)+'\nSEED_SHA='+repr(a.seed_sha)+'\nPREFLIGHT='+repr(pre)+'\nPREFLIGHT_SHA='+repr(a.preflight_sha)+'\nDOCUMENT='+repr(doc)+'\nFILES='+repr(files)+'\n'+REMOTE
 r=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37','python3 -'],input=code.encode(),capture_output=True)
 stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');out={'SSH_exit':r.returncode}
 for k,b in [('stdout',r.stdout),('stderr',r.stderr)]:out[k]=save(PRIVATE/('manager-physical-record-37-'+stamp+'.'+k),b)
 print(json.dumps(out));raise SystemExit(r.returncode)
if __name__=='__main__':main()
