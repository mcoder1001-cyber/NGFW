#!/usr/bin/env python3
"""Stage the existing ACL real-stack harness; add focused security acceptance."""
import os
import json
import sys
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile

ROOT=Path(__file__).resolve().parents[3]
PRODUCT=Path(os.environ.get('NGFW_SECURITY_PRODUCT_ROOT',str(ROOT)))
if '--dry-run' in sys.argv:
 print('slot8: real API/agent, isolated VPP; expiry/remove/restart/rollback; global block forward/reverse/unlisted/unselected/replay; private netns nft local-input/rollback')
 raise SystemExit(0)
env=dict(os.environ)
for line in subprocess.check_output([ROOT/'tools/lab','env','8'],text=True).splitlines():
 if line.startswith('export '):
  key,value=line[7:].split('=',1);env[key]=shlex.split(value)[0]
env['NGFW_INTEGRATION']='1'
for key,name in [('NGFW_ACL_AGENT_BIN','ngfw-agent'),('NGFW_ACL_AGENTCTL_BIN','ngfw-agentctl'),('NGFW_ACL_PREFLIGHT_BIN','ngfw-vpp-preflight')]:
 env[key]=str(PRODUCT/'apps/agent/bin'/name)
with tempfile.TemporaryDirectory(prefix='security-host-') as tmp:
 stage=Path(tmp)
 for path in (ROOT/'test/topology/acl').glob('*'):
  if path.suffix=='.go' or path.name in ('go.mod','go.sum'):shutil.copy2(path,stage/path.name)
 mod=stage/'go.mod';mod.write_text(mod.read_text().replace('../../../apps/agent',str(ROOT/'apps/agent')))
 stack=stage/'stack_test.go';source=stack.read_text();source=source.replace('dir, err := os.Getwd()',f'dir, err := os.Getwd()\n\tdir = {json.dumps(str(PRODUCT))}')
 stack.write_text(source)
 aclfile=stage/'acl_test.go';aclsource=aclfile.read_text()
 aclsource=aclsource.replace('st.startAgent(t)\n\tt.Cleanup', 'st.agentEnv = append(st.agentEnv, "NGFW_HOST_ACL_NETNS="+os.Getenv("NGFW_HOST_ACL_NETNS"))\n\tst.startAgent(t)\n\tt.Cleanup')
 aclfile.write_text(aclsource)
 shutil.copy2(Path(__file__).with_name('acceptance_test.go'),stage/'acceptance_test.go')
 command=[str(ROOT/'tools/heavy.sh'),'python3',str(ROOT/'test/topology/hardware-smoke/isolated-vpp.py'),'go','-C',tmp,'test','-v','-count=1','-timeout','5m','-run',os.environ.get('NGFW_SECURITY_TEST','Test(RuleExpiry|GlobalBlocking)RealAPI'),'.']
 raise SystemExit(subprocess.call(command,env=env))
