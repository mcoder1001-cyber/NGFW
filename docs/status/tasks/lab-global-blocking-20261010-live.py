#!/usr/bin/env python3
"""Finite real-agent/API global-blocking acceptance in the verified private VPP."""
import ipaddress,json,os,re,subprocess,sys,time,urllib.request,urllib.error
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
sys.path.insert(0,str(ROOT/'test/topology/traffic-b'))
from stack import product_stack
from tunnels import check_commit
from probe import stop
from scenario import private_identity
EVID=ROOT/'docs/status/tasks/lab-global-blocking-20261010-evidence'
BIN=Path('/tmp/ngfw-lab-nat46-20261010-bin')
events=[]
def record(case,**data):
 events.append(dict(case=case,atUTC=time.strftime("%Y-%m-%dT%H:%M:%SZ",time.gmtime()),**data));(EVID/'live.json').write_text(json.dumps(events,indent=2)+'\n');print(case,json.dumps(data),flush=True)
def run(*argv,timeout=40):return subprocess.check_output(argv,text=True,stderr=subprocess.STDOUT,timeout=timeout)
def peer(side,*argv):return run('ip','netns','exec','ns-w17-'+side,*argv)
def vpp(label,*argv):
 text=run('timeout','20','vppctl',*argv);(EVID/(label+'.txt')).write_text(text);return text

def request(api,method,path,body=None,ctype='application/json'):
 if body is not None and ctype!='text/plain':body=json.dumps(body)
 req=urllib.request.Request(api.base+path,method=method,data=None if body is None else body.encode(),headers={'Authorization':'Bearer '+api.token,'Content-Type':ctype})
 try:
  with urllib.request.urlopen(req,timeout=180) as response:return json.load(response)
 except urllib.error.HTTPError as error:
  problem=json.load(error);record('api-error',status=error.code,problem=problem);raise

def traffic(case,side,src,dst,allow,tcp=True):
 if allow:subprocess.run(['ip','netns','exec','ns-w17-'+side,'ping','-n','-I',src,'-c','1','-W','1',dst],capture_output=True,timeout=5)
 p=subprocess.run(['ip','netns','exec','ns-w17-'+side,'ping','-n','-I',src,'-c','2','-W','1',dst],text=True,capture_output=True,timeout=10)
 text=p.stdout+p.stderr;(EVID/(case+'-ping.txt')).write_text(text)
 assert re.search(r'2 packets transmitted, '+('2' if allow else '0')+' received',text),text
 if tcp:
  code='import socket; s=socket.socket(); s.settimeout(2); s.bind(("'+src+'",0)); s.connect(("'+dst+'",18443)); s.sendall(b"GB-exact-payload"); assert s.recv(99)==b"GB-exact-payload"'
  t=subprocess.run(['ip','netns','exec','ns-w17-'+side,'python3','-c',code],text=True,capture_output=True,timeout=6)
  (EVID/(case+'-tcp.txt')).write_text(t.stdout+t.stderr)
  assert (t.returncode==0)==allow,(case,t.stderr)
 record(case,allowed=allow,icmp=True,tcp=tcp)

def runtime(label):return vpp(label,'show','runtime','verbose')
def lookup(label):
 private_identity()
 vpp(label+'-runtime-clear','clear','runtime')
 before=runtime(label+'-runtime-before')
 started=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime())
 p=peer('lan','ping','-n','-q','-f','-c','2000','-I','10.17.1.3','10.17.2.2')
 assert '2000 packets transmitted, 2000 received' in p,p
 (EVID/(label+'-burst.txt')).write_text(p);after=runtime(label+'-runtime-after')
 def node(text,name):
  for line in text.splitlines():
   if line.strip().startswith(name+' '):
    cols=line.split();return {'calls':int(cols[2]),'vectors':int(cols[3]),'clocksPerVector':float(cols[5])}
  return None
 a,b=node(before,'acl-plugin-in-ip4-fa'),node(after,'acl-plugin-in-ip4-fa')
 ref=node(after,'ip4-lookup')
 delta=None if not a or not b else b['vectors']-a['vectors']
 if label!='lookup-0':assert a is not None and b is not None and delta>=2000,(label,a,b)
 else:assert b is not None and b['vectors']==0 and ref is not None and ref['vectors']>=2000,(b,ref)
 record(label,startedUTC=started,packetsTransmitted=2000,packetsReceived=2000,before=a,after=b,vectorsDelta=delta,ip4LookupReference=ref,aclCost=None if label=='lookup-0' else b['clocksPerVector'],scope='own runtime reset; bounded window clocks/vector, no throughput claim; zero-entry ACL feature absent/cost N/A')

private_identity()
assert os.environ.get('NGFW_TEST_PREFIX')=='w17'
assert not any(n in run('ip','netns','list') for n in ['ns-w17-lan','ns-w17-wan']), 'slot occupied'
shared=run('systemctl','show','vpp','-p','MainPID','-p','NRestarts')
servers=[];rig=False
try:
 run(str(ROOT/'tools/lab'),'rig','up','w17');rig=True
 peer('lan','ip','addr','add','10.17.1.3/24','dev','w17l1')
 peer('wan','ip','addr','add','198.18.17.2/32','dev','w17w1')
 # Dispose only fixture peer checksum offload; no production NIC is targeted.
 for side,dev in [('lan','w17l1'),('wan','w17w1')]:
  peer(side,'ethtool','-K',dev,'tx','off');features=peer(side,'ethtool','-k',dev);assert 'tx-checksumming: off' in features
  (EVID/('fixture-'+side+'-features.txt')).write_text(features)
 for side,address in [('lan','10.17.1.2'),('lan','10.17.1.3'),('wan','10.17.2.2')]:
  code='import socket; s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1); s.bind(("'+address+'",18443)); s.listen(); print("ready",flush=True)\nwhile True:\n c,a=s.accept(); c.sendall(c.recv(99)); c.close()'
  server=subprocess.Popen(['ip','netns','exec','ns-w17-'+side,'python3','-u','-c',code],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True);servers.append(server);assert server.stdout.readline().strip()=='ready'
 run(str(BIN/'nat46-handoff'),'w17')
 print(run('/tmp/ngfw-lab-global-blocking-20261010-counters'),flush=True)
 vpp('counter-switch','show','acl-plugin','tables')
 with product_stack(17,agent_binary=str(BIN/'ngfw-agent')) as (api,owned,restart):
  api.call('PATCH','/config/vrfs',{'w17-gb-proof':{'id':17040}})
  first=api.call('POST','/config/commit?comment=gb-baseline');warnings=check_commit(first,changed_paths=('/vrfs',));revision=first['revision']['id']
  paths=('/acl/globalBlocking','/interfaces/host-w17l0','/interfaces/host-w17w0','/routing/static')
  def commit(label):
   start=time.monotonic();result=request(api,'POST','/config/commit?comment='+label,{})
   check_commit(result,baseline_warnings=warnings,changed_paths=paths);record(label,seconds=time.monotonic()-start,revision=result['revision']['id']);return result
  def list_entries(entries):request(api,'PATCH','/config/acl/globalBlocking',{'lists':{'proof':{'enabled':True,'source':{'kind':'upload'},'allInterfaces':False,'interfaces':['host-w17l0'],'direction':'both','protectHost':False,'log':False,'entries':entries}}},'application/merge-patch+json')
  try:
   api.call('PATCH','/config/interfaces',{'host-w17l0':{'enabled':True,'ipv4':['10.17.1.1/24']},'host-w17w0':{'enabled':True,'ipv4':['10.17.2.1/24']}})
   api.call('PATCH','/config/routing',{'static':[{'prefix':'198.18.17.2/32','nextHops':[{'address':'10.17.2.2','interface':'host-w17w0'}]}]})
   commit('gb-interfaces')
   for case,side,src,dst,tcp in [('baseline-selected-in','lan','10.17.1.2','10.17.2.2',True),('baseline-selected-out','wan','10.17.2.2','10.17.1.2',True),('baseline-unselected','wan','198.18.17.2','10.17.1.3',True),('baseline-local-in','lan','10.17.1.2','10.17.1.1',False)]:traffic(case,side,src,dst,True,tcp)
   vpp('acl-before','show','acl-plugin','acl');vpp('bindings-before','show','acl-plugin','interface');lookup('lookup-0')
   list_entries([])
   preview=request(api,'POST','/security/global-blocking/lists/proof/import','10.17.1.2\ninvalid-address\n198.18.17.2\n','text/plain')
   assert preview['invalidCount']==1 and preview['invalid'][0]['line']==2 and not preview['staged']
   assert not request(api,'GET','/security/global-blocking')['lists'][0]['runningEntries'];record('upload-preview',preview=preview)
   staged=request(api,'POST','/security/global-blocking/lists/proof/import?dryRun=false','10.17.1.2\ninvalid-address\n198.18.17.2\n','text/plain');assert staged['staged'] and staged['entries']==2
   commit('gb-upload-confirmed')
   state_before=request(api,'GET','/security/global-blocking');record('counter-before',state=state_before)
   for case,side,src,dst,allow,tcp in [('selected-in-drop','lan','10.17.1.2','10.17.2.2',False,True),('selected-out-drop','wan','10.17.2.2','10.17.1.2',False,True),('selected-local-in-drop','lan','10.17.1.2','10.17.1.1',False,False),('unlisted-pass','lan','10.17.1.3','10.17.2.2',True,True),('listed-unselected-pass','wan','198.18.17.2','10.17.1.3',True,True),('listed-unselected-local-pass','wan','198.18.17.2','10.17.2.1',True,False)]:traffic(case,side,src,dst,allow,tcp)
   state_after=request(api,'GET','/security/global-blocking');assert state_after['lists'][0]['hits']['dataplanePackets']>state_before['lists'][0]['hits']['dataplanePackets'];record('counter-after',state=state_after)
   vpp('acl-small-after','show','acl-plugin','acl');vpp('bindings-small-after','show','acl-plugin','interface')
   # Normal browser authentication reads only this owned API child's bootstrap secret.
   secret=None
   for child in Path('/proc/self/task/'+str(os.getpid())+'/children').read_text().split():
    if str(ROOT/'apps/api/dist/main.js').encode() in Path('/proc/'+child+'/cmdline').read_bytes().split(b'\0'):
     env=dict(p.split(b'=',1) for p in Path('/proc/'+child+'/environ').read_bytes().split(b'\0') if b'=' in p);secret=env[b'NGFW_BOOTSTRAP_ADMIN_PASSWORD'].decode()
   assert secret
   subprocess.run(['node',str(ROOT/'docs/status/tasks/lab-global-blocking-20261010-shots.mjs')],input=json.dumps({'password':secret}),text=True,check=True)
   base=int(ipaddress.IPv4Address('100.64.0.0'));entries=[str(ipaddress.IPv4Address(base+2*i))+'/32' for i in range(200000)]
   for n in [10000,200000]:
    list_entries(entries[:n]);commit('gb-'+str(n)+'-real-commit');status=request(api,'GET','/security/global-blocking');assert status['lists'][0]['runningEntries']==n
    lookup('lookup-'+str(n));vpp('bindings-'+str(n),'show','acl-plugin','interface')
   changed=entries.copy();changed[777]='100.127.255.254/32';list_entries(changed)
   candidate=request(api,'GET','/config/candidate');path=owned/'gb-incremental.json';path.write_text(json.dumps(candidate));path.chmod(0o600)
   plan=json.loads(run(str(BIN/'ngfw-agentctl'),'-s',str(owned/'agent.sock'),'dryrun',str(path),'-subsystems','acl',timeout=90));(EVID/'incremental-dryrun.json').write_text(json.dumps(plan,indent=2)+'\n')
   assert plan['ok'] and not plan.get('errors')
   assert plan.get('summary',{}).get('created',0)==0 and plan.get('summary',{}).get('deleted',0)==0 and 1<=plan['summary']['updated']<=5,plan
   assert not any('interface-binding' in item.get('key','') for item in plan.get('plan',[])),plan
   commit('gb-one-entry-real-commit');assert request(api,'GET','/security/global-blocking')['lists'][0]['runningEntries']==200000
   record('incremental-plan',summary=plan['summary'],keys=[item['key'] for item in plan['plan']])
  finally:
   api.call('POST','/config/discard');result=request(api,'POST',f'/config/rollback/{revision}?comment=gb-rollback',{});check_commit(result,baseline_warnings=warnings,changed_paths=paths)
   text=vpp('acl-rollback','show','acl-plugin','acl');assert '_gb.' not in text
   assert not request(api,'GET','/security/global-blocking')['lists'];record('rollback',noOwnedACL=True)
   api.call('PATCH','/config/vrfs',{'w17-gb-proof':None});check_commit(api.call('POST','/config/commit?comment=gb-cleanup'),baseline_warnings=warnings,changed_paths=('/vrfs',))
finally:
 for server in servers:stop(server);server.stdout.close()
 if rig:run(str(ROOT/'tools/lab'),'rig','down','w17')
 assert run('systemctl','show','vpp','-p','MainPID','-p','NRestarts')==shared
 record('cleanup',sharedIdentityUnchanged=True)
print('REAL_GLOBAL_BLOCKING_PASS')
