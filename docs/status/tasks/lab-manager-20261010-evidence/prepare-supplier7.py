from pathlib import Path
import os,sys,shutil,hashlib,json,subprocess,importlib.util,re
sys.dont_write_bytecode=True
base=Path('/run/ngfw-lab-build-20261010');root=base/'guest7'
assert base.is_dir() and not base.is_symlink() and base.stat().st_uid==0 and base.stat().st_mode&0o777==0o700
assert any(r.split()[4]==str(base) and r.split()[r.split().index('-')+1]=='tmpfs' for r in Path('/proc/self/mountinfo').read_text().splitlines())
repo=Path(sys.argv[1]);artifact=Path(sys.argv[2]);source=sys.argv[3]
assert subprocess.check_output(['git','-C',str(repo),'rev-parse','HEAD'],text=True).strip()==source
assert not root.exists()
def sha(p):
 with Path(p).open('rb') as f:return hashlib.file_digest(f,'sha256').hexdigest()
shutil.copytree('/root/.cache/claude-ra-vm/root',root,symlinks=True)
shutil.copytree('/root/ngfw-ra-full-guest-20261005/overlay',root,symlinks=True,dirs_exist_ok=True)
asset_manifest=Path('/root/ngfw-ra-full-guest-20261005/assets-verification.json')
assert sha(asset_manifest)=='63ca6a9820dbf23c78bd6b5e8212c625bc9376980f8891488adb61e06e49bc44'
assets=json.loads(asset_manifest.read_text())
for manifest,prefix,expected in [('/root/rct/ra-v3-offline-assets-manifest.json','opt/ngfw-ra',119),('/root/r19/client-v1-receipt.json','dev/shm/ra-peer-engine',122)]:
 entries=json.loads(Path(manifest).read_text())['entries'];assert len(entries)==expected
 for entry in entries:
  target=root/prefix/entry['path']
  if 'sha256' in entry:assert target.is_file() and sha(target)==entry['sha256']
  else:assert target.is_symlink() and os.readlink(target)==entry['link']
assert sha(root/'var/lib/dpkg/status')==assets['package_status']['staged_sha256']
for path,digest in assets['external_files'].items():assert sha(root/path.lstrip('/'))==digest

receipt={'source':source,'scope':'Supplemental supplier-only guest. Historical firstboot/nft no-op fixture units remain; not full appliance acceptance.','binaries':{},'units':{},'external_vpp_files':{}}
def copy(src,dst):
 src=Path(src);dst=root/dst.lstrip('/');dst.parent.mkdir(parents=True,exist_ok=True)
 assert dst.parent.resolve().is_relative_to(root.resolve())
 if dst.is_symlink():dst.unlink()
 shutil.copy2(src,dst,follow_symlinks=True)
 assert sha(src)==sha(dst)
 return dst
for name,destination in [('ngfw-agent','/usr/sbin/ngfw-agent'),('ngfw-ra-namespace-broker','/usr/lib/ngfw/ngfw-ra-namespace-broker'),('ngfw-ra-daemon','/usr/lib/ngfw/ngfw-ra-daemon')]:
 src=artifact/name;info=src.stat();assert src.is_file() and not src.is_symlink() and info.st_uid==0 and not info.st_mode&0o022 and info.st_nlink==1
 dst=copy(src,destination);receipt['binaries'][name]=sha(dst)
 if name!='ngfw-agent':
  p=Path(str(dst).removesuffix(name))/ (name+'.sha256');p.write_text(sha(dst)+'\n');p.chmod(0o644)
units=list((repo/'deploy/systemd').glob('ngfw-ra*.service'))+list((repo/'deploy/systemd').glob('ngfw-ra*.socket'));assert len(units)==9
units+=[repo/'deploy/systemd/ngfw-agent.service']
for unit in units:
 dst=copy(unit,'/usr/lib/systemd/system/'+unit.name);receipt['units'][str(dst.relative_to(root))]=sha(dst)
hardening=copy(repo/'deploy/hardening/systemd/ngfw-agent.service.d/10-ngfw-hardening.conf','/usr/lib/systemd/system/ngfw-agent.service.d/10-ngfw-hardening.conf');receipt['units'][str(hardening.relative_to(root))]=sha(hardening)
for directory in ('etc/systemd/resolved.conf.d','etc/unbound','etc/chrony','etc/rsyslog.d','etc/snmp','etc/kea','etc/frr','etc/ngfw/rsyslog-tls','var/lib/ngfw/agent','var/lib/ngfw/captures','run/netns','dev/shm'):(root/directory).mkdir(parents=True,exist_ok=True)
for filename,line in [('etc/group','ngfw:x:984:\n'),('etc/passwd','ngfw:x:984:984:NGFW:/nonexistent:/usr/sbin/nologin\n')]:
 p=root/filename;old=p.read_text();assert not any(row.split(':')[0]=='ngfw' for row in old.splitlines());assert not any(row.split(':')[2]=='984' for row in old.splitlines() if len(row.split(':'))>2);p.write_text(old+line)
# Run the real package fixture injection only on the owned protected guest root.
identity_source=repo/'deploy/debian/ngfw/assets/provision-system-identity.py'
identity_spec=importlib.util.spec_from_file_location('owned_guest_identity',identity_source)
identity_module=importlib.util.module_from_spec(identity_spec);identity_spec.loader.exec_module(identity_module)
assert root.resolve().is_relative_to(base.resolve()) and root.name=='guest7'
identity_module.provision(root=str(root))
receipt['identity_provisioner_sha256']=sha(identity_source)
receipt['identity_public_links']={name:os.readlink(root/'etc'/name) for name in identity_module.FILES}
assert all(target==identity_module.STATE+'/'+name for name,target in receipt['identity_public_links'].items())
# Exact installed VPP executable/plugins plus actual resolved dependencies; no host writes.
vpp_sources=[Path('/usr/bin/vpp'),Path('/usr/bin/vppctl')]+sorted(Path('/usr/lib/x86_64-linux-gnu/vpp_plugins').glob('*.so'))
for src in vpp_sources:
 copy(src,str(src));receipt['external_vpp_files'][str(src)]=sha(src)
 out=subprocess.check_output(['ldd',str(src)],text=True);assert 'not found' not in out
 for name in re.findall(r'(?:=>\s*)?(/\S+)\s+\(',out):
  copy(name,name);receipt['external_vpp_files'][name]=sha(name)
p=root/'etc/vpp/startup.conf';p.write_text('unix { nodaemon cli-listen /run/vpp/cli.sock log /run/vpp/vpp.log }\napi-segment { prefix manager_supplier7 }\nsocksvr { socket-name /run/vpp/api.sock }\nstatseg { socket-name /run/vpp/stats.sock }\ncpu { main-core 0 }\nmemory { main-heap-size 512M main-heap-page-size default }\nbuffers { buffers-per-numa 4096 page-size default }\nplugins { plugin default { enable } }\n')
copy('/usr/bin/sleep','/usr/bin/sleep')
receipt['observation_sleep_sha256']=sha(root/'usr/bin/sleep')
copy('/tmp/ngfw-lab-manager-20261010/ra-readiness','/ra-readiness')
receipt['readiness_probe_sha256']=sha(root/'ra-readiness')
(root/'coordinator.sh').write_text('''set -u
/usr/bin/mount -t tmpfs tmpfs /dev/shm
/usr/bin/mkdir -p /run/vpp /run/netns
prerequisites=0
/sbin/modprobe xfrm_interface || prerequisites=1
/sbin/modprobe nf_tables || prerequisites=1
/sbin/modprobe nft_ct || prerequisites=1
test -c /dev/net/tun || prerequisites=1
test -d /sys/module/xfrm_interface || prerequisites=1
echo CURRENT_SUPPLIER_PREREQUISITES_EXIT=$prerequisites
/usr/bin/systemctl start systemd-journald.socket systemd-journald.service ngfw-ra-openfile.socket
/usr/bin/systemctl start ngfw-agent.service
# Fixed fixture observation delay; production deadlines and canonical units unchanged.
/usr/bin/sleep 30
/usr/bin/setpriv --reuid=984 --regid=984 --clear-groups /ra-readiness
result=$?
echo CURRENT_SUPPLIER_READINESS_EXIT=$result
/usr/bin/systemctl show ngfw-agent.service vpp.service ngfw-ra-openfile.service ngfw-ra-namespace-broker.socket --property=MainPID,NRestarts,ActiveState,SubState
/usr/bin/journalctl --sync
/usr/bin/journalctl -u ngfw-agent.service -u ngfw-ra-openfile.service -u 'ngfw-ra-targets@*.service' -u 'ngfw-ra-namespace-broker@*.service' --no-pager -o cat
/usr/bin/systemctl stop ngfw-agent.service vpp.service ngfw-ra-openfile.socket
/probe poweroff
''')
# Protect all copied regular files/directories for the unchanged bounded packer.
for p in [root,*root.rglob('*')]:
 if not p.is_symlink():p.chmod(p.stat().st_mode&0o777&~0o022)
spec=importlib.util.spec_from_file_location('owned_packer','/root/ngfw-ra-full-guest-20261005/pack_initramfs.py');module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);module.BASE=base
receipt['unpacked_bytes']=module.pack(root,base/'supplier7.gz');receipt['image_sha256']=sha(base/'supplier7.gz')
(base/'supplier7-receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
print('SUPPLEMENTAL_IMAGE_READY',receipt['image_sha256'],receipt['unpacked_bytes'])
