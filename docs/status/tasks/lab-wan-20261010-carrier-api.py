"""Real product agent/API consumer in the reviewed private carrier fixture."""
import hashlib, json, os, pathlib, sys, time, types, subprocess, signal
ROOT=pathlib.Path(os.environ.get('NGFW_WAN_NATIVE_ROOT','/root/ngfw-wt/lab-wan-20261010'))
BASE=pathlib.Path(os.environ.get('NGFW_WAN_NATIVE_BASE','/tmp/ngfw-lab-wan-20261010'))
sys.path.insert(0,str(ROOT/'test/topology/traffic-b'))
from scenario import private_identity, Refused
from tunnels import check_commit
private_identity()
assert os.readlink('/proc/self/ns/uts') != os.readlink('/proc/1/ns/uts')
assert [os.stat('/etc').st_dev,os.stat('/etc').st_ino]==json.loads(os.environ['NGFW_WAN_PRIVATE_ETC_IDENTITY'])
mounts=[line.split() for line in pathlib.Path('/proc/self/mountinfo').read_text().splitlines()]
assert any(row[4]=='/var/lib' and row[row.index('-')+1]=='tmpfs' for row in mounts)
# Configure the genuine product binary; no handler, runner or helper is replaced.
pristine=(ROOT/'test/topology/traffic-b/stack.py').read_text()
edits=(("ROOT=Path(__file__).resolve().parents[3]",'ROOT=Path('+repr(str(ROOT))+')'),
       ("NGFW_GLOBALS_OWNER='0'","NGFW_GLOBALS_OWNER='1'"),
       ("NGFW_KEA_MODE='off'","NGFW_KEA_MODE='off',NGFW_FRR='off'"))
if os.environ.get('NGFW_WAN_DIAG')=='1':
 edits+=(("NGFW_LOG_LEVEL='info'","NGFW_LOG_LEVEL='debug'"),)
source=pristine
for old,new in edits:
 assert source.count(old)==1,'original stack literal changed or ambiguous'
 source=source.replace(old,new)
roundtrip=source
for old,new in reversed(edits):
 assert roundtrip.count(new)==1,'unexpected stack replacement'
 roundtrip=roundtrip.replace(new,old)
assert roundtrip==pristine,'stack modified outside exact fixture edits'
module=types.ModuleType('wan_private_product_stack');module.__file__=str(ROOT/'test/topology/traffic-b/stack.py')
exec(compile(source,module.__file__,'exec'),module.__dict__)
namespace=pathlib.Path('/run/netns/ngp-'+hashlib.sha256(b'w20\0w20ppp').hexdigest()[:12])
namespace_identity=(namespace.stat().st_dev,namespace.stat().st_ino)
password=os.environ['NGFW_WAN_PEER_PASSWORD']
assert len(password)==48

def browser_password(runtime):
 found=[]
 for proc in pathlib.Path('/proc').iterdir():
  if not proc.name.isdigit():continue
  try:
   if pathlib.Path(os.readlink(proc/'exe')).name!='node':continue
   argv=(proc/'cmdline').read_bytes().split(b'\0')
   if argv[:2]!=[b'node',str(ROOT/'apps/api/dist/main.js').encode()]:continue
   if pathlib.Path(os.readlink(proc/'cwd'))!=ROOT:continue
   start=(proc/'stat').read_text().split(') ',1)[1].split()[19]
   env=dict(item.split(b'=',1) for item in (proc/'environ').read_bytes().split(b'\0') if b'=' in item)
   if env.get(b'NGFW_AGENT_SOCKET')!=str(runtime/'agent.sock').encode():continue
   assert env[b'NGFW_AGENT_OWNER']==b'w20' and env[b'NGFW_HTTP_PORT']==b'12000'
   value=env[b'NGFW_BOOTSTRAP_ADMIN_PASSWORD'].decode()
   assert (proc/'stat').read_text().split(') ',1)[1].split()[19]==start
   assert pathlib.Path(os.readlink(proc/'cwd'))==ROOT and pathlib.Path(os.readlink(proc/'exe')).name=='node'
   found.append(value)
  except (FileNotFoundError,ProcessLookupError,PermissionError):continue
 assert len(found)==1,'exact own stable API process required'
 return found[0]
def command(*args):return subprocess.check_output(args,stderr=subprocess.STDOUT,text=True,timeout=15)
def state(api):
 for item in api.call('GET','/state/interfaces').get('items',[]):
  if item.get('name')=='w20ppp':return item.get('state',{}).get('pppoe') or {}
 return {}
def wait_up(api,budget=75):
 end=time.monotonic()+budget
 while time.monotonic()<end:
  item=state(api)
  if item.get('phase')=='up' and item.get('localIpv4')=='100.64.20.10/32' and time.monotonic()<=end:return item
  time.sleep(.4)
 print('REAL_API_PPP_WAIT_TIMEOUT_STATE '+json.dumps({key:value for key,value in state(api).items() if key in ('phase','localIpv4','ipv6','forwardingReady','defaultReady','memberReady')}),flush=True)
 raise Refused('real API/current Wiring PPP did not become up')
def commit(api,changed,baseline=None):
 validation=api.call('POST','/config/validate')
 if not validation.get('ok'):raise Refused('real candidate validation failed')
 plan=validation.get('plan',[])
 forbidden=[p.get('key') for p in plan if p.get('subsystem') not in ('interfaces','routing','vrfs','system','nat')]
 if forbidden:raise Refused('unrelated daemon/domain operation in real plan: '+str(forbidden))
 candidate=api.call('GET','/config/candidate')
 digest=hashlib.sha256(json.dumps(candidate,sort_keys=True,separators=(',',':')).encode()).hexdigest()
 result=api.call('POST','/config/commit?comment=w20-real-pppoe-native')
 warnings=check_commit(result,baseline_warnings=baseline,changed_paths=changed)
 running=api.call('GET','/config')
 if hashlib.sha256(json.dumps(running,sort_keys=True,separators=(',',':')).encode()).hexdigest()!=digest:raise Refused('real running/candidate mismatch')
 print('REAL_API_COMMIT '+json.dumps({'revision':result['revision']['id'],'candidate_sha256':digest,'plan':plan,'warnings':warnings}),flush=True)
 return result,warnings
def packets():
  command('ip','-n','ns-w20-carrier-isp','route','replace','10.20.1.0/24','dev','ppp0')
  command('ip','-n','ns-w20-carrier-isp','-6','route','replace','2001:db8:21::/64','dev','ppp0')
  for family,destination in (('-4','100.64.20.1'),('-6','2001:db8:20::1')):
   end=time.monotonic()+35
   while True:
    probe=subprocess.run(['ip','netns','exec','ns-w20-carrier-lan','ping',family,'-n','-c','3','-W','2',destination],capture_output=True,text=True,timeout=12)
    if probe.returncode==0 and '3 packets transmitted, 3 received, 0% packet loss' in probe.stdout:break
    if time.monotonic()>end:raise Refused('real API/current Wiring LAN PPP packet failed '+family)
    time.sleep(.4)
   print('REAL_API_CURRENT_WIRING_LAN_PACKET '+family+' PASS '+probe.stdout.splitlines()[-2],flush=True)
with module.product_stack(20,agent_binary=BASE/'bin/ngfw-agent',target_owner='w20') as (api,runtime,restart):
 baseline=api.call('GET','/config')
 assert baseline['services']['dns']['resolvers']=={} and not baseline['services']['ntp']['enabled'] and baseline['management']['syslog']==[]
 secret=api.call('POST','/secrets?replace=true',{'kind':'password','name':'w20-carrier','value':password})
 assert secret.get('ref')=='password/w20-carrier'
 config={'host-w20raw':{'enabled':True},'host-w20lan':{'enabled':True,'ipv4':['10.20.1.1/24'],'ipv6':['2001:db8:21::1/64']},'w20ppp':{'enabled':True,'pppoe':{'enabled':True,'parent':'host-w20raw','username':'w20','passwordRef':'password/w20-carrier','mtu':1492,'mssClamp':True,'defaultRoute':True,'ipv6':'slaac','reconnect':{'holdoffSec':1,'maxFail':0}}}}
 api.call('PATCH','/config/interfaces',config)
 extended_api=os.environ.get('NGFW_WAN_API_EXTENDED')=='1'
 if extended_api:
  api.call('PATCH','/config/nat',{'enabled':True,'mode':'ed','sessionLimit':4096,'inside':['host-w20lan'],'outside':['w20ppp'],'pools':[{'name':'w20-ppp-native','range':'100.64.20.10'}]})
 paths=('/interfaces',)+tuple('/nat/'+field for field in ('enabled','mode','sessionLimit','inside','outside','pools')) if extended_api else ('/interfaces',)
 result,warnings=commit(api,paths)
 assert (namespace.stat().st_dev,namespace.stat().st_ino)==namespace_identity,'pre-existing carrier namespace was replaced'
 live=wait_up(api);print('REAL_AGENT_WIRING_API_PAP_IPCP_STATE '+json.dumps(live),flush=True)
 if os.environ.get('NGFW_WAN_BROWSER')=='1':
  admin_password=browser_password(runtime)
  browser=subprocess.Popen(['node',str(ROOT/'docs/status/tasks/lab-wan-20261010-pppoe-shots.mjs')],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,start_new_session=True,env=dict(os.environ,NGFW_WAN_BROWSER_OUTPUT=str(ROOT/'docs/status/tasks/lab-wan-20261010-evidence'/'195-pppoe-drawer')))
  try:out,err=browser.communicate(json.dumps({'password':admin_password}),timeout=90)
  except subprocess.TimeoutExpired:
   if browser.poll() is None:os.killpg(browser.pid,signal.SIGTERM)
   try:out,err=browser.communicate(timeout=5)
   except subprocess.TimeoutExpired:
    if browser.poll() is None:os.killpg(browser.pid,signal.SIGKILL)
    out,err=browser.communicate(timeout=5)
  redacted=(out+err).replace(admin_password,'[redacted]').replace(password,'[redacted]')
  print(redacted,flush=True);del admin_password
  if browser.returncode:raise Refused('real PPPoE browser proof failed')
 packets()
 if extended_api:
  receipt=subprocess.run([str(BASE/'bin/carrier-live.test'),'-test.v','-test.count=1','-test.timeout=25s','-test.run=^TestWANCurrentCarrierReadback$'],capture_output=True,text=True,timeout=30)
  if receipt.returncode or '=== RUN   TestWANCurrentCarrierReadback' not in receipt.stdout or '--- PASS: TestWANCurrentCarrierReadback' not in receipt.stdout or '--- SKIP:' in receipt.stdout:raise Refused('actual API MSS getter test failed')
  print(receipt.stdout,flush=True)
  sessions=command('vppctl','show','nat44','sessions')
  if '10.20.1.2' not in sessions or '100.64.20.10' not in sessions:raise Refused('actual API NAT session tuple missing')
  print('REAL_API_NAT_LAN_SESSION PASS '+sessions,flush=True)
 reconnect=api.call('POST','/actions/interfaces/w20ppp/pppoe/reconnect')
 assert reconnect.get('accepted') is True
 wait_up(api);print('REAL_API_CURRENT_WIRING_RECONNECT PASS',flush=True)
 for path in (runtime/'state').rglob('*'):
  if path.is_file() and password.encode() in path.read_bytes():raise Refused('plaintext in agent persisted state')
 for path in runtime.glob('*.private.log'):
  if password in path.read_text(errors='replace'):raise Refused('plaintext in product log')
 for route in ('/config','/config/candidate','/config/diff'):
  if password in json.dumps(api.call('GET',route)):raise Refused('plaintext in config response')
 print('REAL_AGENT_API_CONFIG_LOG_PASSWORD_ABSENCE PASS',flush=True)
 if extended_api:
  scope={};exec(compile((ROOT/'docs/status/tasks/lab-wan-20261010-api-lifecycle.py').read_text(),str(ROOT/'docs/status/tasks/lab-wan-20261010-api-lifecycle.py'),'exec'),scope)
  scope['lifecycle'](api,runtime,config,password,state,wait_up,commit,command,packets,Refused)
  api.call('PATCH','/config/nat',baseline['nat'])
 api.call('PATCH','/config/interfaces',{name:None for name in config})
 commit(api,paths,warnings)
 print('REAL_API_CURRENT_WIRING_SCOPED_CLEANUP PASS',flush=True)
