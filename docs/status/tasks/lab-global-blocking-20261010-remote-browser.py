#!/usr/bin/env python3
"""Real remote API authentication through an owned SSH tunnel; no secret files."""
import json,os,shlex,subprocess,sys,time
from pathlib import Path
root=Path(__file__).resolve().parents[3]
target=sys.argv[1]; remote_root=sys.argv[2]
assert target.startswith('root@') and remote_root.startswith('/tmp/ngfw-lab-global-blocking-')
ssh=['ssh','-o','BatchMode=yes','-o','ConnectTimeout=10',target]
tunnel=subprocess.Popen(['ssh','-o','BatchMode=yes','-o','ExitOnForwardFailure=yes','-N','-L','127.0.0.1:11790:127.0.0.1:11700',target],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
try:
 deadline=time.monotonic()+300
 transport=None
 while time.monotonic()<deadline:
  if tunnel.poll() is not None:raise RuntimeError('owned tunnel exited')
  # Namespace fixture path is random; the exact own worktree limits discovery.
  command=['python3','-c',"from pathlib import Path; p=Path("+repr(remote_root)+"); paths=list(p.glob('.scratch/isolated-vpp-*/test-run/w17tb/browser-transfer/input')); print(str(paths[-1]) if paths else '')"]
  found=subprocess.check_output(ssh+[shlex.join(command)],text=True).strip()
  if found:transport=found;break
  time.sleep(.5)
 if transport is None:raise TimeoutError('remote browser pipe deadline')
 # Read password and nonce directly into memory; never log or persist them.
 raw=subprocess.check_output(ssh+[shlex.join(['cat',transport])],timeout=180)
 payload=json.loads(raw)
 out=root/'docs/status/tasks/lab-global-blocking-20261010-evidence/250';out.mkdir(exist_ok=True)
 subprocess.run(['node',str(root/'docs/status/tasks/lab-global-blocking-20261010-shots.mjs')],input=json.dumps({'password':payload['password']}).encode(),check=True,cwd=root,timeout=150,env=dict(os.environ,NGFW_GB_BROWSER_API_PORT='11790',NGFW_GB_BROWSER_WEB_PORT='15790',NGFW_GB_BROWSER_OUTPUT=str(out)))
 result=str(Path(transport).with_name('result'))
 # Structured stdin avoids shell expansion and secret-bearing command arguments.
 script="import json,sys; x=json.load(sys.stdin); open(x['path'],'w').write(json.dumps(x['reply'])+'\\n')"
 subprocess.run(ssh+[shlex.join(['python3','-c',script])],input=json.dumps({'path':result,'reply':{'status':'pass','nonce':payload['nonce']}}).encode(),check=True,timeout=10)
 print('REMOTE_ACTUAL_BROWSER_EN_FA_PASS')
finally:
 tunnel.terminate()
 try:tunnel.wait(timeout=10)
 except subprocess.TimeoutExpired:tunnel.kill();tunnel.wait()
