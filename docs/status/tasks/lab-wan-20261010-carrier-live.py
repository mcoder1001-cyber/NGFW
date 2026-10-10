"""Finite current-carrier runtime exercise; exact source assets, owned slot20 only."""
import fcntl, hashlib, ipaddress, json, os, runpy, secrets, shutil, signal, subprocess, sys, tempfile, time
from pathlib import Path
ROOT=Path(os.environ.get('NGFW_WAN_NATIVE_ROOT','/root/ngfw-wt/lab-wan-20261010'))
BASE=Path(os.environ.get('NGFW_WAN_NATIVE_BASE','/tmp/ngfw-lab-wan-20261010'))
for owned in (ROOT,BASE):
 assert owned.is_absolute() and owned.is_dir() and not owned.is_symlink()
 info=owned.stat();assert info.st_uid==0 and info.st_mode&0o022==0
TOKEN='ngp-'+hashlib.sha256(b'w20\0w20ppp').hexdigest()[:12]
CARRIER_ROOT=Path('/var/lib/ngfw/agent/pppoe-carrier')/TOKEN
INVENTORY='ngp-'+hashlib.sha256(b'inventory\0w20').hexdigest()[:12]
def run(*args,check=True):return subprocess.run(args,check=check,capture_output=True,text=True)
def cleanup_owned_runtime():
 root=Path('/run/ngfw/pppoe')/TOKEN
 if not os.path.lexists(root):return
 assert os.geteuid()==0 and not Path('/run/netns',TOKEN).exists()
 assert run('systemctl','is-active','ngfw-pppoe-carrier@'+TOKEN+'.service',check=False).stdout.strip()=='inactive'
 for parent in (root, *root.parents):
  info=parent.lstat();assert parent.is_dir() and not parent.is_symlink() and info.st_uid==0 and info.st_mode&0o022==0
 info=root.stat();identity=(info.st_dev,info.st_ino)
 leaf='pw'+TOKEN.removeprefix('ngp-')
 names={leaf+suffix for suffix in ('.state','.state6','.ipv6.pid','.ipv6.lock','.ipv6.action.lock','.ipv6.admission','.ipv6.blocked')}
 names|={'resolv.conf','ipv6-transitions/'+leaf}
 snapshots=[]
 for path in root.rglob('*'):
  rel=str(path.relative_to(root));info=path.lstat()
  assert info.st_uid==0 and not path.is_symlink() and info.st_mode&0o022==0
  if path.is_dir():assert rel=='ipv6-transitions';digest=None
  else:
   assert path.is_file() and info.st_nlink==1 and rel in names
   content=path.read_bytes();digest=hashlib.sha256(content).hexdigest()
   if rel==leaf+'.ipv6.pid':
    record=json.loads(content);pid=record['pid'];assert type(pid)is int and pid>1
    assert type(record['start'])is str and record['start'].isdigit()
    argv=record['argv'];assert type(argv)is list and len(argv)==9 and all(type(arg)is str for arg in argv)
    assert argv[:3]==['/usr/bin/python3','/etc/ppp/ngfw-ipv6-'+leaf,'refresh']
    assert argv[3]==record['generation'] and len(argv[3])==32 and all(c in '0123456789abcdef' for c in argv[3])
    assert argv[4]=='ppp0' and ipaddress.IPv6Address(argv[5])==ipaddress.IPv6Address('fe80::2') and ipaddress.IPv6Address(argv[6])==ipaddress.IPv6Address('fe80::1')
    assert argv[7].isdigit() and int(argv[7])>1 and argv[8].isdigit() and int(argv[8])>0
    stat=Path('/proc')/str(pid)/'stat'
    if stat.exists():assert stat.read_text().split(') ',1)[1].split()[19]!=record['start'],'referenced helper still active'
    parent=record['argv'][-2];assert parent.isdigit() and not Path('/proc',parent).exists(),'referenced pppd still active'
  snapshots.append((path,info.st_dev,info.st_ino,digest))
 for path,device,inode,digest in sorted(snapshots,key=lambda item:(item[3] is None,-len(item[0].parts))):
  info=path.lstat();assert not path.is_symlink() and (info.st_dev,info.st_ino)==(device,inode)
  if digest is None:path.rmdir()
  else:assert hashlib.sha256(path.read_bytes()).hexdigest()==digest;path.unlink()
 assert (root.stat().st_dev,root.stat().st_ino)==identity;root.rmdir()
 print('OWNED_INACTIVE_RUNTIME_TOKEN_REMOVED PASS',flush=True)
if len(sys.argv)>1:
 assert os.readlink('/proc/self/ns/net')!=os.environ['NGFW_WAN_HOST_NETNS']
 assert os.readlink('/proc/self/ns/mnt') not in (os.environ['NGFW_WAN_HOST_MNT'],os.readlink('/proc/1/ns/mnt'))
 processes=[]
 with tempfile.TemporaryDirectory(prefix='native-carrier-',dir=BASE) as directory:
  d=Path(directory);owned_mounts=[]
  def owned_mount(*args):
   target=args[-1];run('mount',*args)
   rows=[line.split() for line in Path('/proc/self/mountinfo').read_text().splitlines()]
   row=next(row for row in reversed(rows) if row[4]==target)
   st=os.stat(target);owned_mounts.append((target,row[0],st.st_dev,st.st_ino))
  try:
   if os.environ.get('NGFW_WAN_FULL_API')=='1':
    assert os.environ.get('NGFW_WAN_EXTENDED')=='1'
    child_uts=os.readlink('/proc/self/ns/uts')
    assert child_uts!=os.readlink('/proc/1/ns/uts') and child_uts!=os.environ['NGFW_WAN_HOST_UTS']
    print('CHILD_UTS_BEFORE '+json.dumps({'namespace':child_uts,'hostname':os.uname().nodename}),flush=True)
    run('ip','link','set','lo','up')
    private_etc=d/'etc';shutil.copytree('/etc',private_etc,symlinks=True)
    # No pre-existing host-service render record is admitted into this fixture.
    for name in ('unbound','chrony','rsyslog.d','snmp','kea','frr'):
     target=private_etc/name
     if target.is_symlink():target.unlink()
     elif target.exists():shutil.rmtree(target)
     target.mkdir(mode=0o755)
    info=private_etc.stat();assert info.st_uid==0 and info.st_mode&0o022==0
    owned_mount('--bind',str(private_etc),'/etc')
    carrier_alias=d/'carrier-root';carrier_alias.mkdir(mode=0o700)
    assert [CARRIER_ROOT.stat().st_dev,CARRIER_ROOT.stat().st_ino]==json.loads(os.environ['NGFW_WAN_CARRIER_ROOT_IDENTITY'])
    owned_mount('--bind',str(CARRIER_ROOT),str(carrier_alias))
    owned_mount('-t','tmpfs','-o','mode=755,size=8m','tmpfs','/var/lib')
    # Use the exact package provisioning implementation, within these private mounts.
    runpy.run_path(str(ROOT/'deploy/debian/ngfw/assets/provision-system-identity.py'))['provision']()
    assert (os.stat('/etc').st_dev,os.stat('/etc').st_ino)==(info.st_dev,info.st_ino)
    mounts=[line.split() for line in Path('/proc/self/mountinfo').read_text().splitlines()]
    assert any(row[4]=='/var/lib' and row[row.index('-')+1]=='tmpfs' for row in mounts)
    CARRIER_ROOT.mkdir(mode=0o700,parents=True)
    owned_mount('--bind',str(carrier_alias),str(CARRIER_ROOT))
    assert [CARRIER_ROOT.stat().st_dev,CARRIER_ROOT.stat().st_ino]==json.loads(os.environ['NGFW_WAN_CARRIER_ROOT_IDENTITY'])
    os.environ['NGFW_WAN_PRIVATE_ETC_IDENTITY']=json.dumps([info.st_dev,info.st_ino])
    print('PRIVATE_UTS_ETC_VARLIB_ORIGINAL_IDENTITY_PROVISION PASS',flush=True)
   peer_password=os.environ['NGFW_WAN_PEER_PASSWORD'];assert len(peer_password)==48 and all(c in '0123456789abcdef' for c in peer_password)
   secret=d/'secrets';secret.write_text('"w20" * "'+peer_password+'" *\n');secret.chmod(0o600)
   owned_mount('-t','tmpfs','-o','mode=700,size=1m','tmpfs','/etc/ppp')
   for name in ('pap-secrets','chap-secrets'):
    Path('/etc/ppp',name).touch(mode=0o600);owned_mount('--bind',str(secret),'/etc/ppp/'+name)
   options=d/'options';options.write_text('auth\nrequire-pap\n'+('ipv6 ::1,::2\n' if os.environ.get('NGFW_WAN_EXTENDED')=='1' else 'noipv6\n')+'mtu 1492\nmru 1492\nlcp-echo-interval 1\nlcp-echo-failure 2\n')
   try:
    for ns in ('ns-w20-carrier-isp','ns-w20-carrier-lan'):
     run('ip','netns','add',ns);run('ip','-n',ns,'link','set','lo','up')
    for dev,peer,ns in [('w20raw','w20is','ns-w20-carrier-isp'),('w20lan','w20lp','ns-w20-carrier-lan')]:
     run('ip','link','add',dev,'type','veth','peer','name',peer);run('ip','link','set',peer,'netns',ns);run('ip','link','set',dev,'up');run('ip','-n',ns,'link','set',peer,'up')
    run('ip','-n','ns-w20-carrier-lan','addr','add','10.20.1.2/24','dev','w20lp');run('ip','-n','ns-w20-carrier-lan','route','add','default','via','10.20.1.1')
    log=(d/'server.log').open('w');p=subprocess.Popen(['ip','netns','exec','ns-w20-carrier-isp','pppoe-server','-F','-k','-I','w20is','-L','100.64.20.1','-R','100.64.20.10','-N','1','-O',str(options)],stdout=log,stderr=log,start_new_session=True);processes.append((p,log));time.sleep(.5)
    if os.environ.get('NGFW_WAN_EXTENDED')=='1':
     ra=d/'peer-ra.py';ra.write_text('''import subprocess,time
for attempt in range(250):
 if subprocess.run(['ip','link','show','ppp0'],capture_output=True).returncode==0:break
 time.sleep(.2)
else:raise SystemExit('real peer PPP did not appear')
subprocess.run(['ip','-6','addr','add','2001:db8:20::1/64','dev','ppp0'],check=True)
subprocess.run(['sysctl','-w','net.ipv6.conf.all.forwarding=1'],check=True,capture_output=True)
raise SystemExit(subprocess.call(['dnsmasq','--no-daemon','--conf-file=/dev/null','--port=0','--interface=ppp0','--bind-interfaces','--enable-ra','--dhcp-range=2001:db8:20::,ra-only,64','--ra-param=ppp0,5,60']))
''')
     ralog=(d/'ra.log').open('w');rp=subprocess.Popen(['ip','netns','exec','ns-w20-carrier-isp','python3',str(ra)],stdout=ralog,stderr=ralog,start_new_session=True);processes.append((rp,ralog))
     run('ip','-n','ns-w20-carrier-lan','-6','addr','add','2001:db8:21::2/64','dev','w20lp');run('ip','-n','ns-w20-carrier-lan','-6','route','add','default','via','2001:db8:21::1')
    wrapper=d/'test.sh';wrapper.write_text('''#!/bin/sh
set -eu
vppctl create host-interface name w20raw
vppctl set ip classify intfc host-w20raw table-index -1
vppctl set ip6 classify intfc host-w20raw table-index -1
vppctl set interface tag host-w20raw w20:host-w20raw
vppctl create host-interface name w20lan
vppctl set ip classify intfc host-w20lan table-index -1
vppctl set ip6 classify intfc host-w20lan table-index -1
vppctl set interface tag host-w20lan w20:host-w20lan
vppctl set interface ip address host-w20lan 10.20.1.1/24
if [ "${NGFW_WAN_EXTENDED:-0}" = 1 ]; then vppctl set interface ip address host-w20lan 2001:db8:21::1/64; fi
exec /tmp/ngfw-lab-wan-20261010/bin/carrier-live.test -test.v -test.count=1 -test.timeout=3m -test.run=^TestWANCurrentCarrierLive$
''');wrapper.write_text(wrapper.read_text().replace('/tmp/ngfw-lab-wan-20261010',str(BASE)));wrapper.chmod(0o700)
    if os.environ.get('NGFW_WAN_FULL_API')=='1':
     native=wrapper.read_text()
     marker='exec '+str(BASE)+'/bin/carrier-live.test'
     assert native.count(marker)==1
     # Actual owned AFPacket ports precede Agent.Start, as physical NICs do.
     wrapper.write_text(native[:native.index(marker)]+'vppctl show interface\nvppctl show interface tag host-w20raw\nvppctl show interface tag host-w20lan\nexec python3 '+str(ROOT/'docs/status/tasks/lab-wan-20261010-carrier-api.py')+'\n');wrapper.chmod(0o700)
    result=subprocess.call(['python3',str(ROOT/'test/topology/hardware-smoke/isolated-vpp.py'),str(wrapper)])
    if os.environ.get('NGFW_WAN_FULL_API')=='1':print('CHILD_UTS_AFTER '+json.dumps({'namespace':os.readlink('/proc/self/ns/uts'),'hostname':os.uname().nodename}),flush=True)
   finally:
    # Original unit has the actual pppd process; stop before removing TAP/namespace names.
    run('systemctl','stop','ngfw-pppoe-carrier@'+TOKEN+'.service',check=False)
    for p,log in reversed(processes):
     if p.poll() is None:
      os.killpg(p.pid,signal.SIGTERM)
      try:p.wait(timeout=5)
      except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
     log.close()
    print('REAL_PEER_LOG\n'+(d/'server.log').read_text(),flush=True)
    if (d/'ra.log').exists():print('REAL_PEER_RA_LOG\n'+(d/'ra.log').read_text(),flush=True)
    for ns in ('ns-w20-carrier-isp','ns-w20-carrier-lan'):run('ip','netns','del',ns,check=False)
  finally:
   # Child-only mounts must be gone before TemporaryDirectory traverses the copied /etc.
   assert os.readlink('/proc/self/ns/net')!=os.environ['NGFW_WAN_HOST_NETNS']
   assert os.readlink('/proc/self/ns/mnt') not in (os.environ['NGFW_WAN_HOST_MNT'],os.readlink('/proc/1/ns/mnt'))
   for target,mount_id,device,inode in reversed(owned_mounts):
    rows=[line.split() for line in Path('/proc/self/mountinfo').read_text().splitlines()]
    row=next(row for row in reversed(rows) if row[4]==target)
    st=os.stat(target)
    assert row[0]==mount_id and (st.st_dev,st.st_ino)==(device,inode),'owned child mount replaced'
    run('umount',target)
   print('OWNED_CHILD_MOUNTS_REMOVED PASS',flush=True)
 sys.exit(result)
lock=open('/run/lock/ngfw-slot-20.lock','a');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
assets={Path('/usr/lib/ngfw/pppoe-carrier.py'):ROOT/'scripts/pppoe-kernel-carrier.py'}
for name in ('ngfw-pppoe-carrier@.service','ngfw-pppoe-broker@.service'):assets[Path('/run/systemd/system')/name]=ROOT/'scripts/pppoe-carrier-assets'/name
for name in ('ip-up','ip-down','ipv6-up','ipv6-down'):assets[Path('/usr/lib/ngfw/pppoe-carrier-hooks')/name]=ROOT/'scripts/pppoe-carrier-assets'/name
assert all(not p.exists() and not p.is_symlink() for p in assets),'refuse replacing any existing asset'
assert not Path('/run/netns',TOKEN).exists() and not Path('/run/ngfw-pppoe-carrier',TOKEN+'.json').exists(),'owned token already exists'
before=run('systemctl','show','vpp','-p','MainPID','-p','NRestarts').stdout
hashes={}
relay=None
carrier_root_identity=None
identity_files=('/etc/hostname','/etc/issue','/etc/issue.net','/etc/motd')
host_identity={name:hashlib.sha256(Path(name).read_bytes()).hexdigest() for name in identity_files}
host_hostname=os.uname().nodename
host_uts=os.readlink('/proc/self/ns/uts')
print('ROOT_HOST_IDENTITY_BEFORE '+json.dumps({'uts':host_uts,'hostname':host_hostname,'public_file_sha256':host_identity}),flush=True)
try:
 if os.environ.get('NGFW_WAN_FULL_API')=='1':
  for parent in CARRIER_ROOT.parents:
   info=parent.lstat();assert parent.is_dir() and not parent.is_symlink() and info.st_uid==0 and info.st_mode&0o022==0
  assert not os.path.lexists(CARRIER_ROOT),'refuse existing carrier render token'
  CARRIER_ROOT.mkdir(mode=0o700)
  info=CARRIER_ROOT.stat();carrier_root_identity=[info.st_dev,info.st_ino]
  os.environ['NGFW_WAN_CARRIER_ROOT_IDENTITY']=json.dumps(carrier_root_identity)
 for dest,source in assets.items():
  dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(source,dest);dest.chmod(0o644 if dest.suffix=='.service' else 0o755);hashes[dest]=hashlib.sha256(dest.read_bytes()).hexdigest();print('TEMP_ORIGINAL_ASSET '+str(dest)+' sha256='+hashes[dest],flush=True)
 run('systemctl','daemon-reload')
 # Original broker creates the persistent namespace before the child mount view snapshots it.
 digest=hashlib.sha256(b'w20\0w20ppp').digest(); block=int.from_bytes(digest[6:8],'big')%16256
 base=(169<<24)|(254<<16)|256; base+=block*4
 raw6=bytearray(16);raw6[0]=0xfd;raw6[1:8]=digest[:7]
 raw6[-1]=2;host6=str(ipaddress.IPv6Address(bytes(raw6)))+'/126'
 raw6[-1]=1;peer6=str(ipaddress.IPv6Address(bytes(raw6)))
 spec=dict(owner='w20',logical='w20ppp',parent='host-w20raw',mtu=1492,host4=str(ipaddress.IPv4Address(base+2))+'/30',peer4=str(ipaddress.IPv4Address(base+1)),host6=host6,peer6=peer6)
 nonce=secrets.token_hex(16);request=dict(op='provision',owner='w20',logical='w20ppp',spec=spec)
 queued=json.loads(run('python3','-I','/usr/lib/ngfw/pppoe-carrier.py','broker-queue',TOKEN,nonce,'--request-json',json.dumps(request)).stdout)
 run('systemctl','start','ngfw-pppoe-broker@'+TOKEN+'.service')
 receipt=json.loads(run('python3','-I','/usr/lib/ngfw/pppoe-carrier.py','broker-result',TOKEN,nonce).stdout)
 assert receipt['ok'] and all(receipt[key]==queued[key] for key in ('token','nonce','boot','expires','request_sha256'))
 print('REAL_BROKER_PREPROVISION_BEFORE_PRIVATE_MOUNT PASS',flush=True)
 env=dict(os.environ,NGFW_WAN_HOST_MNT=os.readlink('/proc/self/ns/mnt'),NGFW_WAN_HOST_NETNS=os.readlink('/proc/self/ns/net'),NGFW_WAN_HOST_UTS=host_uts,NGFW_WAN_PEER_PASSWORD=secrets.token_hex(24),NGFW_WAN_NATIVE_CARRIER='1',NGFW_INTEGRATION='1',NGFW_OWNER='w20',NGFW_TEST_PREFIX='w20',NGFW_SLOT='20',NGFW_VPP_ID_RANGE='all')
 command=['unshare','--net','--mount','--propagation','private']
 if env.get('NGFW_WAN_FULL_API')=='1':
  sys.path.insert(0,str(ROOT/'test/topology/traffic-b'))
  from pgrelay import Relay
  relay=Relay(BASE/('carrier-api-pg-relay-'+str(os.getpid())))
  env.update(NGFW_TRAFFIC_B_REST='1',NGFW_TRAFFIC_PG_PROXY_DIR=str(relay.directory))
  command+=['--uts']
 child=subprocess.Popen(command+[sys.executable,__file__,'child'],env=env)
 if env.get('NGFW_WAN_DIAG')=='1':
  seen={}
  while child.poll() is None:
   for token in (TOKEN,INVENTORY):
    for area in ('requests','results'):
     path=Path('/run/ngfw/pppoe-broker')/area/(token+'.json')
     try:
      info=path.lstat();assert path.is_file() and not path.is_symlink() and info.st_uid==0 and info.st_nlink==1 and info.st_mode&0o077==0 and info.st_size<=1048576
      record=json.loads(path.read_bytes());assert record['token']==token
      signature=(info.st_ino,record['nonce'])
      if seen.get((token,area))==signature:continue
      seen[(token,area)]=signature
      if area=='requests':
       operation=record['request']['op'];assert operation in ('provision','list','prepare','verify','inspect','configure','withdraw','delete','probe')
       metadata={'op':operation}
      else:
       error=record.get('error','');assert error=='' or error.startswith('carrier operation failed: ')
       error_class=error.removeprefix('carrier operation failed: ')
       assert type(record['ok'])is bool and type(error_class)is str and len(error_class)<=64 and (not error_class or error_class.isascii() and error_class.isidentifier())
       metadata={'ok':record['ok'],'error_class':error_class}
      print('OWNED_BROKER_METADATA '+json.dumps({'observed_monotonic':time.monotonic(),'child_pid':child.pid,'token':token,'area':area,**metadata}),flush=True)
     except FileNotFoundError:pass
     except (AssertionError,ValueError,KeyError,TypeError):
      print('OWNED_BROKER_METADATA_REFUSED',flush=True)
   time.sleep(0.1)
 result=child.wait()
 if env.get('NGFW_WAN_DIAG')=='1':
  logs=Path('/run/ngfw-test/w20tb').glob('*.private.log')
  leak=any(env['NGFW_WAN_PEER_PASSWORD'].encode() in path.read_bytes() for path in logs)
  print('REAL_RAW_DIAGNOSTIC_LOG_PASSWORD_ABSENCE '+('FAIL' if leak else 'PASS'),flush=True)
  if leak:result=1
finally:
 if relay:relay.close()
 run('systemctl','stop','ngfw-pppoe-carrier@'+TOKEN+'.service',check=False)
 unit='ngfw-pppoe-carrier@'+TOKEN+'.service'
 status=dict(line.split('=',1) for line in run('systemctl','show',unit,'-p','Id','-p','MainPID','-p','ActiveState').stdout.splitlines())
 assert status['Id']==unit and status['MainPID']=='0' and status['ActiveState'] in ('inactive','failed')
 assert run('systemctl','show',unit,'-p','Id','-p','MainPID','-p','ActiveState').stdout==''.join(k+'='+v+'\n' for k,v in status.items())
 run('systemctl','reset-failed',unit,check=False)
 ledger=Path('/run/ngfw-pppoe-carrier')/(TOKEN+'.json')
 if ledger.exists():
  record=json.loads(ledger.read_text());assert record['owner']=='w20' and record['logical']=='w20ppp'
  run('python3','-I','/usr/lib/ngfw/pppoe-carrier.py','delete',TOKEN,record['generation'])
 for token in (TOKEN,INVENTORY):
  assert run('systemctl','is-active','ngfw-pppoe-broker@'+token+'.service',check=False).stdout.strip()!='active'
  run('systemctl','reset-failed','ngfw-pppoe-broker@'+token+'.service',check=False)
  for area in ('requests','results'):
   p=Path('/run/ngfw/pppoe-broker')/area/(token+'.json')
   if p.exists():assert json.loads(p.read_text())['token']==token;p.unlink()
 cleanup_owned_runtime()
 if carrier_root_identity is not None:
  assert run('systemctl','is-active','ngfw-pppoe-carrier@'+TOKEN+'.service',check=False).stdout.strip()!='active'
  assert [CARRIER_ROOT.stat().st_dev,CARRIER_ROOT.stat().st_ino]==carrier_root_identity
  snapshots=[]
  for path in CARRIER_ROOT.rglob('*'):
   info=path.lstat();assert info.st_uid==0 and not path.is_symlink()
   assert path.is_dir() or (path.is_file() and info.st_nlink==1)
   digest=hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else None
   snapshots.append((path,info.st_dev,info.st_ino,digest))
  for path,device,inode,digest in sorted(snapshots,key=lambda item:(item[3] is None,-len(item[0].parts))):
   info=path.lstat();assert (info.st_dev,info.st_ino)==(device,inode) and not path.is_symlink()
   if digest is None:path.rmdir()
   else:assert hashlib.sha256(path.read_bytes()).hexdigest()==digest;path.unlink()
  assert [CARRIER_ROOT.stat().st_dev,CARRIER_ROOT.stat().st_ino]==carrier_root_identity
  CARRIER_ROOT.rmdir();print('OWNED_RENDER_TOKEN_REMOVED PASS',flush=True)
 for dest,digest in hashes.items():
  assert hashlib.sha256(dest.read_bytes()).hexdigest()==digest;dest.unlink()
 run('systemctl','daemon-reload')
 after=run('systemctl','show','vpp','-p','MainPID','-p','NRestarts').stdout
 print('SHARED_VPP_BEFORE '+before.replace('\n',','));print('SHARED_VPP_AFTER '+after.replace('\n',','));assert before==after
 print('TEMP_ORIGINAL_ASSETS_REMOVED PASS')
 assert os.uname().nodename==host_hostname and all(hashlib.sha256(Path(name).read_bytes()).hexdigest()==digest for name,digest in host_identity.items()),'shared host identity changed'
 print('ROOT_HOST_IDENTITY_AFTER '+json.dumps({'uts':os.readlink('/proc/self/ns/uts'),'hostname':os.uname().nodename,'public_file_sha256':{name:hashlib.sha256(Path(name).read_bytes()).hexdigest() for name in identity_files}}),flush=True)
 assert os.readlink('/proc/self/ns/uts')==host_uts
 print('SHARED_HOST_IDENTITY_UNCHANGED PASS',flush=True)
raise SystemExit(result)
