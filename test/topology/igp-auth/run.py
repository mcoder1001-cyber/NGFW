#!/usr/bin/env python3
"""Actual sealed OSPF MD5 lifecycle in owned private FRR/VPP namespaces."""
import hashlib,json,os,runpy,secrets,subprocess,sys,tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
def main():
 if '--child' in sys.argv:
  source=(ROOT/'test/topology/ospf/private-fib.py').read_text()
  source=source.replace("ROOT = Path(__file__).resolve().parents[3]",'ROOT = Path('+repr(str(ROOT))+')')
  needle="str(ROOT / 'test/topology/ospf/run.sh')"
  assert source.count(needle)==1
  source=source.replace(needle,"'go', '-C', str(ROOT/'apps/agent'), 'test', '-overlay', os.environ['NGFW_IGP_OVERLAY'], '-v', '-count=1', '-timeout', '8m', '-run', '^TestOSPFMD5PrivateLive$', './internal/agent'")
  os.environ['NGFW_OSPF_TOPOLOGY']='1'
  exec(compile(source,str(ROOT/'test/topology/ospf/private-fib.py'),'exec'),{'__name__':'__main__','__file__':str(ROOT/'test/topology/ospf/private-fib.py')})
  return
 env=dict(os.environ)
 for line in subprocess.check_output([ROOT/'tools/lab','env','6'],text=True).splitlines():
  if line.startswith('export '):
   import shlex
   key,value=line[7:].split('=',1);env[key]=shlex.split(value)[0]
 env.update(NGFW_INTEGRATION='1',NGFW_ISOLATED_TEST_RUN='1')
 env['NGFW_FIB_HOST_NETNS']=os.readlink('/proc/self/ns/net')
 env['NGFW_IGP_PASSWORD_ONE']=secrets.token_hex(6);env['NGFW_IGP_PASSWORD_TWO']=secrets.token_hex(6)
 original=ROOT/'apps/agent/internal/agent/ospf_topology_integration_test.go'
 source=original.read_text();old='func TestOSPFTopologyOnHost(t *testing.T)';assert source.count(old)==1
 source=source.replace(old,'func TestOSPFMD5PrivateLive(t *testing.T)',1)
 anchor='\te.evidence("after commit")';assert source.count(anchor)==1
 source=source.replace(anchor,anchor+'\n\tigpAuthLive(t,e,full,start)\n\treturn',1)
 source+=(Path(__file__).with_name('helper.go').read_text().split('package agent\n',1)[1])
 before=subprocess.check_output(['systemctl','show','vpp','-p','MainPID','-p','NRestarts'])
 (ROOT/'.scratch').mkdir(exist_ok=True)
 with tempfile.TemporaryDirectory(prefix='igp-auth-',dir=ROOT/'.scratch') as temp:
  staged=Path(temp)/'auth_test.go';staged.write_text(source)
  subprocess.run(['gofmt','-w',str(staged)],check=True)
  overlay=Path(temp)/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(original):str(staged)}}));env['NGFW_IGP_OVERLAY']=str(overlay)
  print('SOURCE_SHA='+subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),flush=True)
  print('ORIGINAL_FIXTURE_DIGEST='+hashlib.sha256(original.read_bytes()).hexdigest(),flush=True)
  print('OVERLAY_DIGEST='+hashlib.sha256(staged.read_bytes()).hexdigest(),flush=True)
  result=subprocess.run([ROOT/'tools/heavy.sh','unshare','--net','--mount','--propagation','private',sys.executable,str(Path(__file__).resolve()),'--child'],env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  output=result.stdout
  for value in (env['NGFW_IGP_PASSWORD_ONE'],env['NGFW_IGP_PASSWORD_TWO']):output=output.replace(value,'[REDACTED_EPHEMERAL_PASSWORD]')
  print(output,end='',flush=True)
 after=subprocess.check_output(['systemctl','show','vpp','-p','MainPID','-p','NRestarts'])
 if before!=after:raise SystemExit('shared VPP changed')
 print('SHARED_VPP_UNCHANGED='+after.decode().strip().replace('\n',','),flush=True)
 expected='DIAGNOSTIC_OSPF_MD5_NEW_REF_LIFECYCLE=PASS' if env.get('NGFW_IGP_DIAGNOSTIC_NEW_REF')=='1' else 'REAL_OSPF_MD5_SEALED_ROTATION_REMOVAL_RESTART_WITHDRAW_ROLLBACK=PASS'
 if result.returncode or expected not in output or '--- SKIP:' in output:raise SystemExit(result.returncode or 1)
if __name__=='__main__':main()
