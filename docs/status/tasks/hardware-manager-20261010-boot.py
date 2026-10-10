#!/usr/bin/env python3
"""ROOT same-campaign boot preparation; inspect by default, no --now activation."""
import argparse,datetime,hashlib,json,os,pathlib,re,stat,subprocess
OUTPUT=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
REMOTE=r'''
import hashlib,json,os,pathlib,re,socket,stat,subprocess,time,datetime,zoneinfo
UNITS=['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service','ngfw-firstboot.service','ngfw-firewall-bootstrap.service','nftables.service','postgresql.service','postgresql@18-main.service','valkey-server.service','frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','snmpd.service','keepalived.service','chrony.service','rsyslog.service','apply-executor.socket','ngfw-ra-openfile.socket','ngfw-ra-namespace-broker.socket']
OWNED=['vpp.service','ngfw-agent.service','ngfw-api.service']
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery')/('hardware-manager-20261010-boot-'+HOST)
result={'mode':MODE,'host':HOST,'native_proof_SHA':NATIVE_SHA,'commands':[],'stage':'begin'}
def run(a,timeout=30):
 q=subprocess.run(a,capture_output=True,text=True,timeout=timeout);d={'argv':a,'exit':q.returncode,'stdout':q.stdout,'stderr':q.stderr};result['commands'].append(d);return d
def checked(a):
 q=run(a);assert q['exit']==0,'command failed '+repr(a);return q['stdout'].strip()
def state(u):
 d=dict(x.split('=',1) for x in checked(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts','-p','UnitFileState','-p','FragmentPath','-p','Requires','-p','After']).splitlines())
 for k in ['Requires','After']:d[k]=sorted(d[k].split())
 return d
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(checked(['ip','-j','addr']))},'routes4':json.loads(checked(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(checked(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(checked(['ip','-j','-4','rule'])),'rules6':json.loads(checked(['ip','-j','-6','rule']))}
def nft(d):
 if isinstance(d,dict):return {k:nft({a:b for a,b in v.items() if a not in ['packets','bytes']}) if k=='counter' and isinstance(v,dict) else nft(v) for k,v in d.items() if k not in ['metainfo','handle']}
 if isinstance(d,list):return [nft(x) for x in d if not(isinstance(x,dict) and 'metainfo' in x)]
 return d
def synced(p,b,mode=0o600):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,mode)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
def digest(p):
 p=pathlib.Path(p);s=p.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022;return hashlib.sha256(p.read_bytes()).hexdigest()
def protect(postboot=False):
 boot=pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip();assert re.fullmatch(r'[0-9a-f-]{36}',boot)
 assert (boot!=ORIGINAL_BOOT) if postboot else (boot==ORIGINAL_BOOT)
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2) and not os.path.lexists('/run/nextroot') and not os.path.exists('/run/ngfwrescue')
 assert not os.path.lexists('/usr/sbin/policy-rc.d') and not os.path.lexists('/etc/systemd/system/vpp.service')
 assert digest('/etc/vpp/startup.conf')==STARTUP and digest('/usr/sbin/ngfw-agent')=='b3c7cc10671abcab6d11c7dce12323f284c64822a15cfda1c46ba78803283744' and digest('/usr/lib/ngfw/bin/ngfw-startupgen')=='55e490bad1c302505231df105a1d516cb074678b38b85e28740c399020aac619'
 versions=checked(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']);assert len(versions.splitlines())==4 and all(x.split('\t')[1:]==['0.1.0~dev+97ae88ee5b6a','installed'] for x in versions.splitlines())
 network=l3();assert network==BASELINE['network'],'complete L3 differs from actual physical acceptance'
 p=pathlib.Path('/sys/class/net')/MGMT_IF/'device';g=(p/'iommu_group').resolve(strict=True);assert p.resolve().name==MGMT_PCI and (p/'driver').resolve().name=='igc' and g.name==MGMT_GROUP and sorted(x.name for x in (g/'devices').iterdir())==[MGMT_PCI]
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
 devices={}
 for name,q in INVENTORY.items():
  p=pathlib.Path('/sys/bus/pci/devices')/q['PCI'];g=(p/'iommu_group').resolve(strict=True);assert q['PCI']!=MGMT_PCI and (p/'driver').resolve().name=='vfio-pci' and (p/'driver_override').read_text().strip()=='vfio-pci' and g.name==q['group'] and sorted(x.name for x in (g/'devices').iterdir())==[q['PCI']] and stat.S_ISCHR(pathlib.Path('/dev/vfio/'+g.name).stat().st_mode);devices[name]={'PCI':q['PCI'],'group':g.name,'driver':'vfio-pci'}
 assert pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode').read_text().strip()=='N';ids=pathlib.Path('/sys/module/vfio_pci/parameters/ids');assert not ids.exists() or ids.read_text().strip()==''
 sysctls={k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in BASELINE['sysctls']};assert sysctls==BASELINE['sysctls']
 dns={p:{'SHA':hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest(),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in BASELINE['DNS']};assert dns==BASELINE['DNS']
 rules=json.loads(checked(['nft','-j','list','ruleset']));assert nft(rules)==nft(BASELINE['nft'])
 io=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();assert re.fullmatch(r'(?:0x)?[0-9a-fA-F]+',io)
 if not postboot:assert int(io,16)==6
 fs=checked(['tune2fs','-l','/dev/sda2']);assert [x.split(':',1)[1].strip() for x in fs.splitlines() if x.startswith('Filesystem state:')]==['clean']
 units={u:state(u) for u in UNITS};assert all(units[u]['ActiveState']=='active' and units[u]['NRestarts']=='0' and int(units[u]['MainPID'])>1 for u in ['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service'])
 assert all(units[u]['ActiveState']=='active' for u in ['ngfw-firstboot.service','ngfw-firewall-bootstrap.service','nftables.service','postgresql.service','postgresql@18-main.service','valkey-server.service'])
 assert all(units[u]['ActiveState']=='inactive' and units[u]['UnitFileState']=='disabled' for u in ['frr.service','kea-dhcp4-server.service','kea-dhcp6-server.service','unbound.service','snmpd.service','keepalived.service','apply-executor.socket','ngfw-ra-openfile.socket','ngfw-ra-namespace-broker.socket'])
 if not postboot:
  for u,q in NATIVE['unit_states_after'].items():assert {k:units[u][k] for k in q}==q
 else:assert all(units[u]['ActiveState']=='active' for u in ['chrony.service','rsyslog.service'])
 assert 'ngfw-hardware-'+HOST+'-bind.service' in units['vpp.service']['Requires'] and 'ngfw-firstboot.service' in units['vpp.service']['Requires']
 assert 'vpp.service' in units['ngfw-agent.service']['Requires'] and 'nftables.service' in units['ngfw-agent.service']['Requires'] and 'ngfw-agent.service' in units['ngfw-api.service']['Requires']
 binder=state('ngfw-hardware-'+HOST+'-bind.service');assert binder['ActiveState']=='active'
 desired=NATIVE['running']['data']['system']['timezone'];assert re.fullmatch(r'[A-Za-z0-9_+/-]+',desired) and '..' not in desired and not desired.startswith('/')
 expected_zone=pathlib.Path('/usr/share/zoneinfo')/desired;zone=pathlib.Path('/etc/localtime');assert zone.resolve(strict=True)==expected_zone.resolve(strict=True) and zone.read_bytes()==expected_zone.read_bytes();time.tzset();assert time.strftime('%z')==datetime.datetime.now(zoneinfo.ZoneInfo(desired)).strftime('%z')
 if postboot:
  label=checked(['timedatectl','show','-p','Timezone','--value']);assert re.fullmatch(r'[A-Za-z0-9_+/-]+',label) and '..' not in label and not label.startswith('/')
  assert (pathlib.Path('/usr/share/zoneinfo')/label).resolve(strict=True)==expected_zone.resolve(strict=True)
 plugins=checked(['vppctl','show','plugins']);assert all(re.search(r'\b'+re.escape(n)+r'\b',plugins) for n in ['linux_cp_plugin.so','linux_nl_plugin.so','npt66_plugin.so'])
 assert checked(['dpkg','--audit'])==''
 return {'boot_id':boot,'network':network,'sysctls':sysctls,'DNS':dns,'nft':rules,'storage_ioerr':io,'units':units,'binder':binder,'VFIO_devices':devices,'startup_SHA':STARTUP,'timezone':str(zone.resolve()),'offset':time.strftime('%z')}
def main():
 result['before']=protect(MODE=='observe-boot');kernel=checked(['dmesg','--color=never']);result['kernel_before']=kernel;result['stage']='protected'
 if MODE=='enable-boot':
  assert not os.path.lexists(RECORD);parent=RECORD.parent;s=parent.lstat();assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o700;assert (os.major(s.st_dev),os.minor(s.st_dev))==(8,2);RECORD.mkdir(mode=0o700);parentfd=os.open(parent,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(parentfd);os.close(parentfd)
  synced(RECORD/'before.json',json.dumps(result['before'],sort_keys=True).encode());synced(RECORD/'native-proof-sha',NATIVE_SHA.encode())
  for u in OWNED:
   assert result['before']['units'][u]['UnitFileState'] in ['disabled','enabled'];link=pathlib.Path('/etc/systemd/system/multi-user.target.wants')/u
   assert not os.path.lexists(link) or link.is_symlink() and link.resolve(strict=True)==pathlib.Path(result['before']['units'][u]['FragmentPath']).resolve(strict=True)
  checked(['systemctl','enable']+OWNED)
  result['after']=protect();assert all(result['after']['units'][u]['UnitFileState']=='enabled' for u in OWNED)
  for u in UNITS:
   a=dict(result['before']['units'][u]);b=dict(result['after']['units'][u])
   if u in OWNED:a.pop('UnitFileState');b.pop('UnitFileState')
   assert a==b,'unintended unit/enablement change'
  synced(RECORD/'enabled.json',json.dumps(result['after'],sort_keys=True).encode());result['owned_three_enabled_without_start']=True
 elif MODE=='reboot':
  assert ENABLE['owned_three_enabled_without_start'] and ENABLE['PASS'] and ENABLE['native_proof_SHA']==NATIVE_SHA
  assert all(result['before']['units'][u]['UnitFileState']=='enabled' for u in OWNED)
  assert result['before']['network']==ENABLE['after']['network'] and result['before']['startup_SHA']==ENABLE['after']['startup_SHA']
  # The prior controller-fsynced enable proof is durable before this request.
  assert INSPECT['PASS'] and INSPECT['mode']=='inspect' and INSPECT['native_proof_SHA']==NATIVE_SHA and INSPECT['before']['units']==result['before']['units']
  synced(RECORD/'reboot-request.json',json.dumps({'native_proof_SHA':NATIVE_SHA,'enable_proof_SHA':ENABLE_SHA,'inspect_proof_SHA':INSPECT_SHA,'protected_before':result['before']},sort_keys=True).encode())
  result['reboot_requested']=True
  q=run(['systemctl','reboot'],20);assert q['exit']==0;result['reboot_request_exit']=0
 elif MODE=='observe-boot':
  assert all(result['before']['units'][u]['UnitFileState']=='enabled' for u in OWNED)
  time.sleep(2);result['after']=protect(True);assert result['after']['boot_id']==result['before']['boot_id']
  assert result['before']['units']==result['after']['units']
  assert int(result['before']['storage_ioerr'],16)==int(result['after']['storage_ioerr'],16),'new-boot storage counter increased'
  result['storage_counter_epoch']='fresh-boot';result['new_boot_observed']=True
 else:result['after']=protect();assert result['before']['units']==result['after']['units']
 after=checked(['dmesg','--color=never']);result['kernel_after']=after
 scan=after.splitlines() if MODE=='observe-boot' else after[len(kernel):].splitlines() if after.startswith(kernel) else None;assert scan is not None
 bad=[x for x in scan if re.search(r'EXT4-fs error|I/O error|Buffer I/O|blk_update_request|\bUNC\b|hard resetting link|failed command|ata\d.*(?:error|reset)|sd\s+\S+.*(?:error|fail)',x,re.I)];assert not bad
 result.update(new_storage_errors=bad,PASS=True,stage='complete')
try:main()
except Exception as e:result.update(PASS=False,failure={'type':type(e).__name__,'message':str(e)})
finally:print(json.dumps(result,indent=2),flush=True)
raise SystemExit(0 if result.get('PASS') else 2)
'''
def private(p,sha):
 p=pathlib.Path(p);assert p.parent==OUTPUT and not p.is_symlink();s=p.stat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600;b=p.read_bytes();assert re.fullmatch('[0-9a-f]{64}',sha) and hashlib.sha256(b).hexdigest()==sha;return json.loads(b)
def save(p,b):
 assert p.parent==OUTPUT;fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(OUTPUT,os.O_DIRECTORY);os.fsync(fd);os.close(fd);return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--host',choices=['211','37'],required=True);p.add_argument('--native-proof',required=True);p.add_argument('--native-sha',required=True);p.add_argument('--enable-proof');p.add_argument('--enable-sha');p.add_argument('--inspect-proof');p.add_argument('--inspect-sha');g=p.add_mutually_exclusive_group();g.add_argument('--enable-boot',action='store_true');g.add_argument('--reboot',action='store_true');g.add_argument('--observe-boot',action='store_true');a=p.parse_args()
 n=private(a.native_proof,a.native_sha);assert n['PASS'] and n['stage']=='complete' and n['new_storage_errors']==[] and n['unit_identity_stable'] and n['physical_admin_converged'];assert n['native2_physical17_PASS'] if a.host=='211' else n['native1_physical7_PASS']
 mode='enable-boot' if a.enable_boot else 'reboot' if a.reboot else 'observe-boot' if a.observe_boot else 'inspect';enable=private(a.enable_proof,a.enable_sha) if a.reboot else None;inspection=private(a.inspect_proof,a.inspect_sha) if a.reboot else None
 inv=n['after']['VFIO_devices'];assert len(inv)==(17 if a.host=='211' else 7);assert n['actual_pool_total']>=n['total_RX_descriptor_requirement'] and n['expected_package_version']=='0.1.0~dev+97ae88ee5b6a'
 host='172.30.110.211' if a.host=='211' else '172.30.126.37';fields={'HOST':a.host,'MODE':mode,'NATIVE':n,'NATIVE_SHA':a.native_sha,'BASELINE':n['after'],'INVENTORY':inv,'STARTUP':n['expected_startup_SHA'],'ENABLE':enable,'ENABLE_SHA':a.enable_sha,'INSPECT':inspection,'INSPECT_SHA':a.inspect_sha,'ORIGINAL_BOOT':'3a609803-be4e-46eb-bfc6-7dcfe385cf50' if a.host=='211' else 'c8d66ea9-afab-4228-a293-00c198745040','MGMT_IF':'enp4s0' if a.host=='211' else 'enp12s0','MGMT_PCI':'0000:04:00.0' if a.host=='211' else '0000:0c:00.0','MGMT_GROUP':'28' if a.host=='211' else '58'}
 code='\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE;q=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@'+host,'python3 -'],input=code.encode(),capture_output=True);stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');print(json.dumps({'SSH_exit':q.returncode,'mode':mode,'stdout':save(OUTPUT/('manager-boot'+a.host+'-'+mode+'-'+stamp+'.json'),q.stdout),'stderr':save(OUTPUT/('manager-boot'+a.host+'-'+mode+'-'+stamp+'.stderr'),q.stderr)}));raise SystemExit(q.returncode)
if __name__=='__main__':main()
