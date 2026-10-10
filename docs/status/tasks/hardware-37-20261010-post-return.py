#!/usr/bin/env python3
"""Read-only original-kernel return/storage/network/auth proof; never installs."""
import hashlib,importlib.util,json,os,pathlib,stat,subprocess
HERE=pathlib.Path(__file__).resolve().parent
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
REMOTE=r'''
import hashlib,json,os,pathlib,re,stat,subprocess
def run(args):
 p=subprocess.run(args,capture_output=True,text=True,timeout=60)
 report['commands'].append({'command':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr})
 assert p.returncode==0
 return p.stdout
def checksum(p):
 h=hashlib.sha256()
 with p.open('rb') as f:
  for chunk in iter(lambda:f.read(1048576),b''):h.update(chunk)
 return h.hexdigest()
report={'commands':[],'selected':[],'root_account_records':[],'readonly':True,'installation_not_run':True}
report['ioerr_before']=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip()
assert os.stat('/').st_dev==2050 and os.stat('/proc/1/root').st_dev==2050
assert not os.path.lexists('/run/nextroot') and not os.path.lexists('/run/ngfwrescue')
boot=pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip();assert boot!=OLD_BOOT
report['boot_id']=boot
superblock=run(['tune2fs','-l','/dev/sda2'])
assert re.search(r'^Filesystem state:\s+clean$',superblock,re.M)
props=run(['systemctl','show','systemd-fsck-root.service','-p','ActiveState','-p','SubState','-p','Result','-p','ExecMainStatus'])
assert 'Result=success' in props and 'ExecMainStatus=0' in props
report['boot_fsck']=props
for item in EXPECTED:
 p=pathlib.Path(item['path']);st=p.stat() if item['follow_final'] else p.lstat()
 kind='file' if stat.S_ISREG(st.st_mode) else ('directory' if stat.S_ISDIR(st.st_mode) else ('symlink' if stat.S_ISLNK(st.st_mode) else 'other'))
 got={'path':str(p),'kind':kind,'uid':st.st_uid,'gid':st.st_gid,'mode':stat.S_IMODE(st.st_mode)}
 if kind=='file':got.update({'bytes':st.st_size,'sha256':checksum(p)})
 if kind=='symlink':got['target']=os.readlink(p)
 got['matches']=all(got[k]==item[k] for k in ['kind','uid','gid','mode','bytes','sha256','target'] if k in item)
 report['selected'].append(got);assert got['matches'],'selected original-object changed'
for item in ROOT_RECORDS:
 p=pathlib.Path(item['path']);lines=[x.rstrip(b'\r\n')+b'\n' for x in p.read_bytes().splitlines(keepends=True) if x.startswith(b'root:')]
 assert len(lines)==1
 got={'path':str(p),'bytes':len(lines[0]),'sha256':hashlib.sha256(lines[0]).hexdigest()}
 got['matches']=got['bytes']==item['bytes'] and got['sha256']==item['sha256']
 report['root_account_records'].append(got);assert got['matches']
report['network']={name:json.loads(run(args)) for name,args in COMMANDS.items()}
p=pathlib.Path('/sys/class/net/enp12s0/device');assert p.resolve().name=='0000:0c:00.0' and (p/'driver').resolve().name=='igc' and (p/'iommu_group').resolve().name=='58'
assert pathlib.Path('/sys/class/net/enp12s0/address').read_text().strip()=='00:04:e1:e0:00:3e'
report['protected_management_identity']='enp12s0/0c/igc/group58/originalMAC'
report['ioerr_counter']=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();assert report['ioerr_counter']==report['ioerr_before']
report['ioerr_not_compared_across_kernel_reboot']=True
kernel=run(['dmesg']);pattern=r'(?i)(ata\d.*(error|failed|reset|timeout|unc)|scsi.*(error|failed|reset|timeout)|I/O error|Buffer I/O|blk_update_request|end_request|uncorrectable|critical medium error|EXT4-fs error|JBD2.*(error|failed))'
report['new_boot_storage_events']=[x for x in kernel.splitlines() if re.search(pattern,x)]
assert not report['new_boot_storage_events']
report['status']='PASS_NORMAL_RETURN_CAPTURE';print(json.dumps(report,indent=2))
'''
def main():
 os.umask(0o077)
 source=HERE/'hardware-37-20261010-offline-preserve.py'
 assert hashlib.sha256(source.read_bytes()).hexdigest()=='bdb2a28a5235142dc60c3ce72f2577acdee7ead4667f19b3733c22acef4c4301'
 spec=importlib.util.spec_from_file_location('preserve',source);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
 ready=PRIVATE/'post-repair-return-readiness-v2-20261010.json'
 assert hashlib.sha256(ready.read_bytes()).hexdigest()=='c147e7cc765582c8d974a1b8a3e5e6a0bd563b46a1e2ae429bab920ecf989b03'
 d=json.loads(ready.read_text());boot_paths={x['path'] for x in json.loads((PRIVATE/'readonly-boot-baseline-20261010.json').read_text())['boot_files']}
 expected=[dict(x,follow_final=x['path'] in boot_paths) for x in d['selected']]
 request=json.loads((PRIVATE/'normal-return-request-20261010.json').read_text())
 network_source=HERE/'hardware-37-20261010-post-transition.py'
 spec=importlib.util.spec_from_file_location('net',network_source);net=importlib.util.module_from_spec(spec);spec.loader.exec_module(net)
 fields={'OLD_BOOT':request['old_boot_id'],'EXPECTED':expected,'ROOT_RECORDS':d['root_account_records'],'COMMANDS':net.COMMANDS}
 ssh=['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37','LC_ALL=C python3 -B -']
 p=subprocess.run(ssh,input=m.code(fields,REMOTE).encode(),capture_output=True,timeout=240)
 r=m.save('post-normal-return-20261010.json',p.stdout);e=m.save('post-normal-return-20261010.stderr',p.stderr)
 print(json.dumps({'ssh_exit':p.returncode,'receipt':r,'stderr':e}),flush=True);assert p.returncode==0 and not p.stderr
 current=json.loads(p.stdout);comparisons={}
 for name,value in current['network'].items():
  original=json.loads((PRIVATE/(name+'.json')).read_text())
  comparisons[name]=net.addresses(value)==net.addresses(original) if name=='addresses' else value==original
 assert all(comparisons.values())
 public={'network_comparisons':comparisons,'boot_id':current['boot_id'],'root_clean':True,'boot_fsck_success0':True,'selected_count':len(current['selected']),'all_selected_match':True,'root_records_match':True,'protected_management_unchanged':True,'ioerr_counter':current['ioerr_counter'],'new_boot_storage_events':0,'installation_not_run':True}
 r=m.save('post-normal-return-conclusions-20261010.json',json.dumps(public,indent=2).encode());print(json.dumps({'conclusions':r,'facts':public}))
if __name__=='__main__':main()
