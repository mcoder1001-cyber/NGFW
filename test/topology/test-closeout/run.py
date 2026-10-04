#!/usr/bin/env python3
"""Run existing host acceptance on fresh private VPP instances; retain skips and failures."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import signal
import subprocess
import time

ROOT = Path(__file__).resolve().parents[3]
CASES = {
 'agent-baseline': ('^Test(AgentOnHost|AgentProcessOnHost|HostNicsOnHost|DataplaneRuntimeOnHost|UntaggedClaimsSurviveAgentRestartOnHost|WireguardOnHost)$', {}),
 'tunnels': ('^TestTunnelsOnDisposableVPP$', {}),
 'mpls': ('^TestMplsOnHost$', {'NGFW_DF7_GLOBALS': '1'}),
 'srv6': ('^TestSrv6(OnHost|GlobalsOnHost)$', {'NGFW_FSRV6_GLOBALS': '1'}),
 'lb': ('^TestLb(OnHost|GarbageCollectOnHost)$', {'NGFW_LB_HOST': '1', 'NGFW_LB_GLOBALS': '1'}),
 'wireguard-packets': ('^TestWireguardHandshakeOnHost$', {'NGFW_WG_HANDSHAKE': '1'}),
}

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--slot', type=int, default=10)
 p.add_argument('--out', type=Path, required=True)
 p.add_argument('--cases', default=','.join(CASES))
 p.add_argument('--build', action='store_true', help='build agent and race test binary, recording exact source/binary provenance')
 args=p.parse_args(); args.out=args.out.resolve()
 if args.out.exists() and any(args.out.iterdir()): raise SystemExit('evidence output directory must be empty; retain prior runs')
 args.out.mkdir(parents=True, exist_ok=True)
 env=dict(os.environ)
 for line in subprocess.check_output([ROOT/'tools/lab','env',str(args.slot)],text=True).splitlines():
  if line.startswith('export '):
   key,value=line[7:].split('=',1);env[key]=shlex.split(value)[0]
 env.update(NGFW_INTEGRATION='1',NGFW_ISOLATED_TEST_RUN='1',NGFW_TEST_AGENT_BIN=str(ROOT/'apps/agent/bin/ngfw-agent'))
 if args.build: env['NGFW_TEST_AGENT_BIN']=str(args.out/'ngfw-agent')
 source_sha=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
 dirty=subprocess.check_output(['git','status','--porcelain','--untracked-files=normal','--','apps/agent'],cwd=ROOT,text=True).strip()
 if dirty: raise SystemExit('tracked agent source is dirty; commit before building/running acceptance')
 binary=args.out/'agent-host.test' if args.build else ROOT/'.scratch/agent-host.test'
 if args.build:
  subprocess.run([ROOT/'tools/heavy.sh','go','-C',str(ROOT/'apps/agent'),'build','-o',env['NGFW_TEST_AGENT_BIN'],'./cmd/ngfw-agent'],check=True)
  subprocess.run([ROOT/'tools/heavy.sh','go','-C',str(ROOT/'apps/agent'),'test','-c','-race','-o',str(binary),'./internal/agent'],check=True)
  if subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()!=source_sha or subprocess.check_output(['git','status','--porcelain','--untracked-files=normal','--','apps/agent'],cwd=ROOT,text=True).strip():
   raise SystemExit('source changed during build')
  binary.with_suffix(binary.suffix+'.build.json').write_text(json.dumps({'source_sha':source_sha,'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'agent_sha256':hashlib.sha256(Path(env['NGFW_TEST_AGENT_BIN']).read_bytes()).hexdigest()},indent=2)+'\n')
 if not binary.is_file(): raise SystemExit('build .scratch/agent-host.test with go test -c -race ./internal/agent first')
 provenance_path=binary.with_suffix(binary.suffix+'.build.json')
 if not provenance_path.is_file(): raise SystemExit('missing build provenance sidecar; record source SHA and binary SHA256 after a successful build')
 provenance=json.loads(provenance_path.read_text())
 binary_sha=hashlib.sha256(binary.read_bytes()).hexdigest()
 if provenance.get('source_sha')!=source_sha or provenance.get('binary_sha256')!=binary_sha or provenance.get('agent_sha256')!=hashlib.sha256(Path(env['NGFW_TEST_AGENT_BIN']).read_bytes()).hexdigest():
  raise SystemExit('test binary provenance does not match current source and binary; rebuild and record provenance')
 if not args.build:
  snapshot=args.out/'agent-host.test'; shutil.copy2(binary,snapshot); binary=snapshot
  agent_snapshot=args.out/'ngfw-agent'; shutil.copy2(env['NGFW_TEST_AGENT_BIN'],agent_snapshot); env['NGFW_TEST_AGENT_BIN']=str(agent_snapshot)
  if hashlib.sha256(binary.read_bytes()).hexdigest()!=binary_sha or hashlib.sha256(agent_snapshot.read_bytes()).hexdigest()!=provenance['agent_sha256']:
   raise SystemExit('binary changed during snapshot; rebuild in isolated output directory')
 before=subprocess.check_output(['systemctl','show','vpp','-p','MainPID','-p','NRestarts'],text=True)
 results=[]
 for name in args.cases.split(','):
  pattern,extra=CASES[name];childenv=dict(env,**extra)
  argv=[str(ROOT/'tools/heavy.sh'),'python3',str(ROOT/'test/topology/hardware-smoke/isolated-vpp.py'),
        'go','tool','test2json','-t','-p','ngfw/agent/internal/agent',str(binary),
        '-test.v=test2json','-test.count=1','-test.timeout=8m','-test.run='+pattern]
  start=time.monotonic(); log=args.out/(name+'.jsonl')
  print('START',name,flush=True)
  interrupted=False
  with log.open('w') as f:
   proc=subprocess.Popen(argv,cwd=ROOT/'apps/agent/internal/agent',env=childenv,stdout=f,stderr=subprocess.STDOUT,start_new_session=True)
   try: rc=proc.wait(timeout=720)
   except (subprocess.TimeoutExpired, KeyboardInterrupt) as error:
    interrupted=isinstance(error,KeyboardInterrupt)
    os.killpg(proc.pid,signal.SIGTERM)
    try: proc.wait(timeout=20)
    except subprocess.TimeoutExpired:os.killpg(proc.pid,signal.SIGKILL);proc.wait()
    rc=124
  outcomes=[]
  for line in log.read_text(errors='replace').splitlines():
   try:event=json.loads(line)
   except ValueError:continue
   if event.get('Test') and event.get('Action') in ('pass','fail','skip'):
    outcomes.append({'test':event['Test'],'outcome':event['Action']})
  results.append({'case':name,'command':argv,'extra_env':extra,'exit':rc,'elapsed':round(time.monotonic()-start,2),'tests':outcomes})
  manifest={'source_sha':source_sha,'binary_sha256':binary_sha,'build_provenance':provenance,
            'slot':args.slot,'date_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'shared_vpp_before':before,'results':results}
  (args.out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
  print('END',name,'exit',rc,outcomes,flush=True)
  if interrupted: break
 after=subprocess.check_output(['systemctl','show','vpp','-p','MainPID','-p','NRestarts'],text=True)
 manifest['shared_vpp_after']=after;manifest['shared_vpp_unchanged']=before==after
 (args.out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
 if before!=after:raise SystemExit('shared VPP identity changed')
 if any(r['exit'] or not r['tests'] or any(t['outcome']!='pass' for t in r['tests']) for r in results):raise SystemExit(1)

if __name__=='__main__':main()
