#!/usr/bin/env python3
"""Read-only original boot/auth/network and RAM normal-return proof; no reboot."""
import hashlib
import importlib.util
import json
import os
import pathlib
import stat
import subprocess
import tarfile

HERE=pathlib.Path(__file__).resolve().parent
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
COMMANDS={'addresses':['ip','-j','-details','address','show'],
          'routes-ipv4-all':['ip','-j','-4','route','show','table','all'],
          'routes-ipv6-all':['ip','-j','-6','route','show','table','all'],
          'rules-ipv4':['ip','-j','-4','rule','show'],'rules-ipv6':['ip','-j','-6','rule','show']}
REMOTE=r'''
root=parent/'return-original-root';efi=parent/'return-original-efi'
report['selected']=[];report['runtime']=[];report['mounts']=[]
def command(args,timeout=180):
    p=run(args,timeout);report['commands'].append(entry(args,p));assert p.returncode==0
    return p.stdout.decode(errors='replace')
def readonly_mount(source,target,fstype,options,dev):
    if os.path.lexists(target):
        st=target.lstat()
        assert stat.S_ISDIR(st.st_mode) and st.st_uid==0 and st.st_gid==0 and stat.S_IMODE(st.st_mode)==0o700 and st.st_dev==51
        assert not list(target.iterdir()),'refuse nonempty existing owned mountpoint'
    else:target.mkdir(mode=0o700)
    assert os.stat(target).st_dev==51
    command(['/usr/bin/mount','-t',fstype,'-o',options,source,str(target)])
    report['mounts'].append(str(target))
    m=json.loads(command(['/usr/bin/findmnt','-J','-n','-T',str(target),'-o','TARGET,SOURCE,FSTYPE,OPTIONS']))['filesystems'][0]
    assert m['target']==str(target) and m['source']==source and m['fstype']==fstype
    assert {'ro','nosuid','nodev','noexec'}<=set(m['options'].split(',')) and os.stat(target).st_dev==dev
    assert os.statvfs(target).f_flag&os.ST_RDONLY
    if fstype=='ext4':assert {'noload','norecovery'}&set(m['options'].split(','))
def original_path(name,follow_final=True):
    # Resolve symlinks as inside the original root, never against RAM /boot or /etc.
    pending=name;links=0
    while True:
        pieces=pathlib.PurePosixPath(os.path.normpath(pending)).parts[1:]
        current=efi if pending.startswith('/boot/efi/') else root
        if current==efi:pieces=pieces[2:]
        logical='/boot/efi' if current==efi else ''
        for index,piece in enumerate(pieces):
            current=current/piece;logical+='/'+piece
            if current.is_symlink() and (follow_final or index<len(pieces)-1):
                links+=1;assert links<=40
                link=os.readlink(current)
                base=link if link.startswith('/') else str(pathlib.PurePosixPath(logical).parent/link)
                pending=base+'/'+('/'.join(pieces[index+1:]))
                break
        else:return current
try:
    audit();report['health_before']=health()
    assert int(pathlib.Path('/sys/class/block/sda1/size').read_text())*512==511705088
    assert os.stat('/dev/sda1').st_rdev==2049
    readonly_mount('/dev/sda2',root,'ext4','ro,noload,nosuid,nodev,noexec',2050)
    readonly_mount('/dev/sda1',efi,'vfat','ro,nosuid,nodev,noexec',2049)
    for item in EXPECTED:
        p=original_path(item['path'],item.get('follow_final',True))
        st=p.stat() if item.get('follow_final',True) else p.lstat()
        kind='file' if stat.S_ISREG(st.st_mode) else ('directory' if stat.S_ISDIR(st.st_mode) else ('symlink' if stat.S_ISLNK(st.st_mode) else 'other'))
        got={'path':item['path'],'kind':kind,'uid':st.st_uid,'gid':st.st_gid,'mode':stat.S_IMODE(st.st_mode)}
        if kind=='file':got.update({'bytes':st.st_size,'sha256':checksum(p)})
        if kind=='symlink':got['target']=os.readlink(p)
        got['matches']=all(got[key]==item[key] for key in ['kind','uid','gid','mode','bytes','sha256','target'] if key in item)
        report['selected'].append(got);assert got['matches'],'selected original-file or metadata mismatch'
    report['root_account_records']=[]
    for item in ROOT_RECORDS:
        p=original_path(item['path'],False);st=p.lstat()
        assert stat.S_ISREG(st.st_mode) and st.st_uid==0 and not st.st_mode&0o027
        lines=[line.rstrip(b'\r\n')+b'\n' for line in p.read_bytes().splitlines(keepends=True) if line.startswith(b'root:')]
        assert len(lines)==1
        got={'path':item['path'],'record_selector':'root:','bytes':len(lines[0]),'sha256':hashlib.sha256(lines[0]).hexdigest(),
             'current_file_uid':st.st_uid,'current_file_gid':st.st_gid,'current_file_mode':stat.S_IMODE(st.st_mode),
             'original_file_metadata_baseline_available':False}
        got['matches']=got['bytes']==item['bytes'] and got['sha256']==item['sha256']
        report['root_account_records'].append(got);assert got['matches'],'selected root-account record mismatch'
    # EFI had no pre-repair file baseline: record current boot artifacts, do not claim equality.
    report['efi_files']=[]
    for p in sorted(efi.rglob('*')):
        if p.is_file():
            assert not p.is_symlink()
            report['efi_files'].append({'path':str(p.relative_to(efi)),'bytes':p.stat().st_size,'sha256':checksum(p)})
    assert report['efi_files'],'EFI boot files absent'
    report['efi_pe_headers']=[]
    for p in sorted(efi.rglob('*')):
        if p.is_file() and p.suffix.lower()=='.efi':
            with p.open('rb') as f:header=f.read(2)
            assert header==b'MZ'
            report['efi_pe_headers'].append(str(p.relative_to(efi)))
    assert report['efi_pe_headers'],'EFI executable bootloader absent'
    for name in ['/etc/fstab','/etc/default/grub','/boot/grub/grub.cfg']:
        report.setdefault('boot_configuration',{})[name]=original_path(name).read_text()
    report['kernel_cmdline']=pathlib.Path('/proc/cmdline').read_text()
    report['root_superblock']=command(['/usr/sbin/dumpe2fs','-h','/dev/sda2'])
    assert re.search(r'^Filesystem state:\s+clean$',report['root_superblock'],re.M)
    root_uuid=re.search(r'^Filesystem UUID:\s+(\S+)$',report['root_superblock'],re.M).group(1)
    assert root_uuid in report['boot_configuration']['/etc/fstab']
    assert root_uuid in report['boot_configuration']['/boot/grub/grub.cfg']
    assert '7.0.0-22-generic' in report['boot_configuration']['/boot/grub/grub.cfg']
    report['root_uuid_fstab_grub_match']=True
    report['block_inventory']=command(['/usr/bin/lsblk','-J','-b','-o','NAME,MAJ:MIN,FSTYPE,UUID,SIZE'])
except Exception as exc:
    report['status']='REFUSE';report['failure']=str(exc);report['traceback']=traceback.format_exc()
finally:
    # Ordinary unmount only; no lazy/force detach, no persistent mount configuration.
    for target in reversed(report['mounts']):
        command(['/usr/bin/umount',target])
        assert os.stat(target).st_dev==51
    report['ordinary_unmounts_complete']=True
if report.get('status')=='REFUSE':
    print(json.dumps(report,indent=2));raise SystemExit(1)
try:
    report['network']={name:json.loads(command(args)) for name,args in COMMANDS.items()}
    nic=pathlib.Path('/sys/class/net/enp12s0');pci=(nic/'device').resolve()
    assert pci.name=='0000:0c:00.0' and (pci/'driver').resolve().name=='igc'
    assert (pci/'iommu_group').resolve().name=='58'
    assert (nic/'address').read_text().strip()=='00:04:e1:e0:00:3e'
    report['protected_management_identity']='enp12s0/0c:00.0/igc/group58/originalMAC'
    for item in RAM_EXPECTED:
        p=pathlib.Path(item['path']);assert p.stat().st_dev==51
        got={'path':str(p),'bytes':p.stat().st_size,'sha256':checksum(p)}
        got['matches']=got['bytes']==item['bytes'] and got['sha256']==item['sha256']
        report['runtime'].append(got);assert got['matches']
    report['loader_closures']={}
    for executable in ['/usr/lib/systemd/systemd','/usr/lib/systemd/systemd-executor','/usr/lib/systemd/systemd-shutdown']:
        out=command(['/lib64/ld-linux-x86-64.so.2','--list',executable]);assert 'not found' not in out
        paths=re.findall(r'(?:=>\s*)?(/\S+)\s+\(0x[0-9a-fA-F]+\)',out)
        assert paths and all(os.stat(p).st_dev==51 for p in paths)
        report['loader_closures'][executable]={'output':out,'all_paths_ram51':True,'paths':paths}
    assert os.stat('/proc/1/exe').st_dev==51 and os.stat('/proc/3940/root').st_dev==51
    report['services']=command(['/usr/bin/systemctl','show','ngfw-rescue.service','ngfw-rescue-runtime.service','-p','ActiveState','-p','SubState','-p','MainPID','-p','Result'])
    assert not os.path.lexists('/run/nextroot')
    report['health_after']=health()
    report['new_storage_error_lines']=[x for x in storage_errors(report['health_after']['kernel']) if x not in storage_errors(report['health_before']['kernel'])]
    assert report['health_before']['ioerr_counter']==report['health_after']['ioerr_counter']=='0x9'
    assert not report['new_storage_error_lines']
    audit();command(['/usr/bin/sync']);command(['/usr/bin/block-check','/dev/sda2','8','2'])
    report['status']='PASS_RETURN_READINESS_CAPTURE';report['normal_reboot_not_run']=True
except Exception as exc:
    report['status']='REFUSE';report['failure']=str(exc);report['traceback']=traceback.format_exc()
print(json.dumps(report,indent=2))
raise SystemExit(0 if report['status']=='PASS_RETURN_READINESS_CAPTURE' else 1)
'''

def addresses(data):
    return sorted((iface['ifname'],tuple(sorted(tuple(sorted((k,json.dumps(v,sort_keys=True))
                  for k,v in a.items() if k not in ['valid_life_time','preferred_life_time']))
                  for a in iface.get('addr_info',[])))) for iface in data)

def main():
    os.umask(0o077)
    assert os.geteuid()==0 and stat.S_IMODE(PRIVATE.stat().st_mode)==0o700
    source=HERE/'hardware-37-20261010-offline-preserve.py'
    assert hashlib.sha256(source.read_bytes()).hexdigest()=='bdb2a28a5235142dc60c3ce72f2577acdee7ead4667f19b3733c22acef4c4301'
    for name,digest in [('readonly-boot-baseline-20261010.json','c76678897876dcd36b5d5254a9af0d399b6c9efaf80451e1e5ebbb09656a141d'),('ram-selected-integrity-capacity-20261010.json','ef0459d164ebd1d83a91787d85617767ef50688fd05f725afcf375bea58c0bc5')]:
        assert hashlib.sha256((PRIVATE/name).read_bytes()).hexdigest()==digest
    spec=importlib.util.spec_from_file_location('preserve',source)
    m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
    expected={};root_records=[]
    for x in json.loads((PRIVATE/'readonly-boot-baseline-20261010.json').read_text())['boot_files']:
        expected[x['path']]={'path':x['path'],'kind':'file','bytes':x['bytes'],'sha256':x['sha256'],'mode':int(x['mode'],0),'follow_final':True}
    for filename,digest in [('auth-return-private-20261010.tar','2a57558ae6a6c7c2694b2034249e301d209bb0b5b8bc69076e77710725cd45c9'),('network-config.tar','ddfcd92c7df9dc4f67f85445f1793b6280984d1f7acda89cbcbce5c1e4cbce1f')]:
        archive=PRIVATE/filename;assert hashlib.sha256(archive.read_bytes()).hexdigest()==digest
        with tarfile.open(archive,'r:') as tar:
            for member in tar.getmembers():
                assert not pathlib.PurePosixPath(member.name).is_absolute() and '..' not in pathlib.PurePosixPath(member.name).parts
                if member.name in ['return/root-shadow.record','return/root-gshadow.record']:
                    assert member.isfile()
                    data=tar.extractfile(member).read();assert data.startswith(b'root:') and data.count(b'\n')==1
                    target='/etc/shadow' if member.name=='return/root-shadow.record' else '/etc/gshadow'
                    root_records.append({'path':target,'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest()})
                    continue
                assert not member.name.startswith('return/'),'unknown generated archive selector'
                path='/'+member.name.lstrip('./')
                kind='file' if member.isfile() else ('directory' if member.isdir() else ('symlink' if member.issym() else 'other'))
                assert kind!='other'
                item={'path':path,'kind':kind,'uid':member.uid,'gid':member.gid,'mode':member.mode,'follow_final':False}
                if member.isfile():
                    data=tar.extractfile(member).read();item.update({'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest()})
                if member.issym():item['target']=member.linkname
                expected[path]=item
    assert {x['path'] for x in root_records}=={'/etc/shadow','/etc/gshadow'}
    ram=json.loads((PRIVATE/'ram-selected-integrity-capacity-20261010.json').read_text())['selected_files']
    fields={'REMOTE_PARENT':m.REMOTE_PARENT,'EXPECTED':list(expected.values()),'ROOT_RECORDS':root_records,'RAM_EXPECTED':ram,'COMMANDS':COMMANDS}
    p=subprocess.run(m.SSH+['LC_ALL=C /usr/bin/python3 -B -'],input=m.code(fields,m.IDENTITY+REMOTE).encode(),capture_output=True,timeout=900)
    receipt=m.save('post-repair-return-readiness-v2-20261010.json',p.stdout);error=m.save('post-repair-return-readiness-v2-20261010.stderr',p.stderr)
    print(json.dumps({'ssh_exit':p.returncode,'receipt':receipt,'stderr':error}),flush=True)
    assert p.returncode==0 and not p.stderr
    d=json.loads(p.stdout);comparisons={}
    for name,current in d['network'].items():
        baseline=json.loads((PRIVATE/(name+'.json')).read_text())
        comparisons[name]=addresses(current)==addresses(baseline) if name=='addresses' else current==baseline
    assert all(comparisons.values())
    public={'root_account_records_match':all(x['matches'] for x in d['root_account_records']),'all_selected_match':all(x['matches'] for x in d['selected']),'selected_count':len(d['selected']),
            'all47_ram_runtime_match':len(d['runtime'])==47 and all(x['matches'] for x in d['runtime']),
            'network_comparisons':comparisons,'normal_reboot_not_run':True}
    m.save('post-repair-return-conclusions-v2-20261010.json',json.dumps(public,indent=2).encode());print(json.dumps(public))

if __name__=='__main__':main()
