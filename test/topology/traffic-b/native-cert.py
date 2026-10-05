#!/usr/bin/env python3
"""Use unchanged native certificate fixture inside the already private network."""
import os
from pathlib import Path
import subprocess
import sys
ROOT=Path(__file__).resolve().parents[3]
if os.environ.get('NGFW_DISPOSABLE_VPP')!='1':raise SystemExit('private network wrapper required')
values={}
for line in subprocess.check_output([str(ROOT/'tools/lab'),'env','8'],text=True).splitlines():
    name,sep,value=line.removeprefix('export ').partition('=')
    if not sep or not name.startswith('NGFW_'):raise SystemExit('malformed logical private slot8 environment')
    values[name]=value
# Fixed historical fixture names are local to this private network/mount namespace;
# they never reserve or create the host's slot8 namespaces, sockets or interfaces.
env=dict(os.environ,**values,NGFW_NATIVE_INITIATOR='1',NGFW_NATIVE_PEER_LOSS='0')
env.pop('NGFW_AGENT_VPP_API_SOCKET',None)
env.pop('NGFW_VPP_API_SOCKET',None)
sys.exit(subprocess.call([str(ROOT/'tools/heavy.sh'),'python3',str(ROOT/'test/topology/hardware-smoke/isolated-vpp.py'),
                          'python3',str(ROOT/'test/topology/ipsec/cert-peer/run.py')],env=env))
