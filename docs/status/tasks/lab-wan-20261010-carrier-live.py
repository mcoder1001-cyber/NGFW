"""Finite current-carrier runtime exercise; exact source assets, owned slot20 only."""
import fcntl, hashlib, ipaddress, json, os, secrets, shutil, signal, subprocess, sys, tempfile, time
from pathlib import Path
ROOT=Path('/root/ngfw-wt/lab-wan-20261010')
BASE=Path('/tmp/ngfw-lab-wan-20261010')
TOKEN='ngp-'+hashlib.sha256(b'w20\0w20ppp').hexdigest()[:12]
INVENTORY='ngp-'+hashlib.sha256(b'inventory\0w20').hexdigest()[:12]
def run(*args,check=True):return subprocess.run(args,check=check,capture_output=True,text=True)
if len(sys.argv)>1:
 assert os.readlink('/proc/self/ns/net')!=os.environ['NGFW_WAN_HOST_NETNS']
 processes=[]
 with tempfile.TemporaryDirectory(prefix='native-carrier-',dir=BASE) as directory:
  d=Path(directory)
  secret=d/'secrets';secret.write_text('"w20" * "NGFW_TEST_PSK_w20" *\n');secret.chmod(0o600)
  for name in ('pap-secrets','chap-secrets'):run('mount','--bind',str(secret),'/etc/ppp/'+name)
  options=d/'options';options.write_text('auth\nrequire-pap\nnoipv6\nmtu 1492\nmru 1492\nlcp-echo-interval 1\nlcp-echo-failure 2\n')
  try:
   for ns in ('ns-w20-carrier-isp','ns-w20-carrier-lan'):
    run('ip','netns','add',ns);run('ip','-n',ns,'link','set','lo','up')
   for dev,peer,ns in [('w20raw','w20is','ns-w20-carrier-isp'),('w20lan','w20lp','ns-w20-carrier-lan')]:
    run('ip','link','add',dev,'type','veth','peer','name',peer);run('ip','link','set',peer,'netns',ns);run('ip','link','set',dev,'up');run('ip','-n',ns,'link','set',peer,'up')
   run('ip','-n','ns-w20-carrier-lan','addr','add','10.20.1.2/24','dev','w20lp');run('ip','-n','ns-w20-carrier-lan','route','add','default','via','10.20.1.1')
   log=(d/'server.log').open('w');p=subprocess.Popen(['ip','netns','exec','ns-w20-carrier-isp','pppoe-server','-F','-k','-I','w20is','-L','100.64.20.1','-R','100.64.20.10','-N','1','-O',str(options)],stdout=log,stderr=log,start_new_session=True);processes.append((p,log));time.sleep(.5)
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
exec /tmp/ngfw-lab-wan-20261010/bin/carrier-live.test -test.v -test.count=1 -test.timeout=3m -test.run=^TestWANCurrentCarrierLive$
''');wrapper.chmod(0o700)
   result=subprocess.call(['python3',str(ROOT/'test/topology/hardware-smoke/isolated-vpp.py'),str(wrapper)])
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
   for ns in ('ns-w20-carrier-isp','ns-w20-carrier-lan'):run('ip','netns','del',ns,check=False)
 sys.exit(result)
lock=open('/run/lock/ngfw-slot-20.lock','a');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
assets={Path('/usr/lib/ngfw/pppoe-carrier.py'):ROOT/'scripts/pppoe-kernel-carrier.py'}
for name in ('ngfw-pppoe-carrier@.service','ngfw-pppoe-broker@.service'):assets[Path('/run/systemd/system')/name]=ROOT/'scripts/pppoe-carrier-assets'/name
for name in ('ip-up','ip-down','ipv6-up','ipv6-down'):assets[Path('/usr/lib/ngfw/pppoe-carrier-hooks')/name]=ROOT/'scripts/pppoe-carrier-assets'/name
assert all(not p.exists() and not p.is_symlink() for p in assets),'refuse replacing any existing asset'
assert not Path('/run/netns',TOKEN).exists() and not Path('/run/ngfw-pppoe-carrier',TOKEN+'.json').exists(),'owned token already exists'
before=run('systemctl','show','vpp','-p','MainPID','-p','NRestarts').stdout
hashes={}
try:
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
 env=dict(os.environ,NGFW_WAN_HOST_NETNS=os.readlink('/proc/self/ns/net'),NGFW_WAN_NATIVE_CARRIER='1',NGFW_INTEGRATION='1',NGFW_OWNER='w20',NGFW_TEST_PREFIX='w20',NGFW_SLOT='20',NGFW_VPP_ID_RANGE='all')
 result=subprocess.call(['unshare','--net','--mount','--propagation','private',sys.executable,__file__,'child'],env=env)
finally:
 run('systemctl','stop','ngfw-pppoe-carrier@'+TOKEN+'.service',check=False)
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
 for dest,digest in hashes.items():
  assert hashlib.sha256(dest.read_bytes()).hexdigest()==digest;dest.unlink()
 run('systemctl','daemon-reload')
 after=run('systemctl','show','vpp','-p','MainPID','-p','NRestarts').stdout
 print('SHARED_VPP_BEFORE '+before.replace('\n',','));print('SHARED_VPP_AFTER '+after.replace('\n',','));assert before==after
 print('TEMP_ORIGINAL_ASSETS_REMOVED PASS')
raise SystemExit(result)
