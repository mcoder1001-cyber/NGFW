#!/usr/bin/env python3
"""Finite NAT46 real API/agent acceptance. No secret enters evidence."""
import json, os, subprocess, sys, urllib.request, urllib.error
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
sys.path.insert(0,str(ROOT/'test/topology/traffic-b'))
from stack import product_stack
from tunnels import check_commit
EVID=ROOT/'docs/status/tasks/lab-nat46-20261010-evidence'
events=[]
def save(): (EVID/'api.json').write_text(json.dumps(events,indent=2)+'\n')
def ctl(runtime,*args):
 return json.loads(subprocess.check_output(['/tmp/ngfw-lab-nat46-20261010-bin/ngfw-agentctl','-s',str(runtime/'agent.sock'),*args],text=True))
with product_stack(17,agent_binary='/tmp/ngfw-lab-nat46-20261010-bin/ngfw-agent') as (api,runtime,restart):
 baseline='w17-nat46-proof'
 api.call('PATCH','/config/vrfs',{baseline:{'id':17040}})
 first=api.call('POST','/config/commit?comment=nat46-baseline');warnings=check_commit(first,changed_paths=('/vrfs',))
 revision=first['revision']['id'];events.append({'case':'baseline','revision':revision})
 try:
  interfaces={'loop1746':{'enabled':True,'ipv4':['10.17.1.1/24']},'loop1747':{'enabled':True,'ipv6':['fd00:11:2::1/64']}}
  nat={'clientPrefix':'fd00:11:4646::/96','interfaces':['loop1746','loop1747'],'mappings':[{'name':'web','ipv4':'10.17.46.10','ipv6':'fd00:11:46::a11:2e0a'}]}
  api.call('PATCH','/config/interfaces',interfaces);api.call('PATCH','/config/nat',{'nat46':nat})
  diff=api.call('GET','/config/diff');assert diff['changes']
  result=api.call('POST','/config/commit?comment=nat46-real-api');check_commit(result,baseline_warnings=warnings,changed_paths=('/interfaces/loop1746','/interfaces/loop1747','/nat/nat46'))
  state=api.call('GET','/state/nat/nat46');assert len(state['mappings'])==1
  client=api.call('GET','/state/nat/nat46/client?ipv4=10.17.1.2');assert client['ipv6']=='fd00:11:4646::a11:102'
  running=api.call('GET','/config');assert running['nat']['nat46']['mappings']==nat['mappings']
  domains=subprocess.check_output(['vppctl','show','map','domain'],text=True);assert '10.17.46.10/32' in domains
  (EVID/'api-vpp-domains.txt').write_text(domains)
  events.append({'case':'real-commit','revision':result['revision']['id'],'state':state,'client':client,'retrieve':ctl(runtime,'retrieve','-subsystems','nat')['desiredState']['nat']});save()
  duplicate=dict(nat,mappings=[*nat['mappings'],{'name':'dup','ipv4':'10.17.46.10','ipv6':'fd00:11:46::a11:2e0b'}])
  req=urllib.request.Request(api.base+'/config/nat',method='PATCH',data=json.dumps({'nat46':duplicate}).encode(),headers={'Authorization':'Bearer '+api.token,'Content-Type':'application/merge-patch+json'})
  try: urllib.request.urlopen(req);raise AssertionError('duplicate accepted')
  except urllib.error.HTTPError as error:
   problem=json.load(error);assert error.code==400 and error.headers['Content-Type'].startswith('application/problem+json')
   assert any(e['pointer']=='/nat/nat46/mappings/1/ipv4' for e in problem['errors'])
   events.append({'case':'duplicate-http400','status':error.code,'contentType':error.headers['Content-Type'],'problem':problem})
  assert api.call('GET','/config')['nat']==running['nat']
  restart();assert api.call('GET','/state/nat/nat46')==state
  events.append({'case':'own-agent-restart','stateUnchanged':True});save()
  # Read only bootstrap secret of the API child this process spawned, for UI login.
  api_secret=None
  for child in Path('/proc/self/task/'+str(os.getpid())+'/children').read_text().split():
   cmd=Path('/proc/'+child+'/cmdline').read_bytes().split(b'\0')
   if str(ROOT/'apps/api/dist/main.js').encode() in cmd:
    env=dict(p.split(b'=',1) for p in Path('/proc/'+child+'/environ').read_bytes().split(b'\0') if b'=' in p)
    api_secret=env[b'NGFW_BOOTSTRAP_ADMIN_PASSWORD'].decode()
  assert api_secret
  browser=subprocess.run(['node',str(ROOT/'docs/status/tasks/lab-nat46-20261010-shots.mjs')],input=json.dumps({'password':api_secret}),text=True,check=True)
  events.append({'case':'browser-en-fa','screenshots':['nat46-en.png','nat46-fa.png']});save()
 finally:
  api.call('POST','/config/discard')
  rolled=api.call('POST',f'/config/rollback/{revision}?comment=nat46-real-api-rollback');check_commit(rolled,baseline_warnings=warnings,changed_paths=('/interfaces/loop1746','/interfaces/loop1747','/nat/nat46'))
  assert not api.call('GET','/config')['nat'].get('nat46')
  assert 'nat46-web' not in subprocess.check_output(['vppctl','show','map','domain'],text=True)
  events.append({'case':'api-revision-rollback','retrieve':ctl(runtime,'retrieve','-subsystems','nat')['desiredState'].get('nat',{})});save()
  api.call('PATCH','/config/vrfs',{baseline:None});check_commit(api.call('POST','/config/commit?comment=nat46-cleanup'),baseline_warnings=warnings,changed_paths=('/vrfs',))
print('REAL_API_NAT46_PASS')
