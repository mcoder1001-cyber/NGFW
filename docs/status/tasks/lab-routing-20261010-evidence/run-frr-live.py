#!/usr/bin/env python3
"""Run current FRR-only OSPF acceptance in owned outer namespaces."""
import json
import os
from pathlib import Path
import subprocess
import sys
ROOT = Path(__file__).resolve().parents[4]
if '--child' not in sys.argv:
    if os.environ.get('NGFW_SLOT') != '6':
        raise SystemExit('assigned slot6 required')
    before = subprocess.check_output(['systemctl','show','vpp','-p','MainPID','-p','NRestarts'])
    env = dict(os.environ, NGFW_FRRLIVE_HOST_NS=os.readlink('/proc/self/ns/net'))
    rc = subprocess.call(['unshare','--net','--mount','--propagation','private',sys.executable,str(Path(__file__).resolve()),'--child'],env=env)
    after = subprocess.check_output(['systemctl','show','vpp','-p','MainPID','-p','NRestarts'])
    print('SHARED_BEFORE=' + before.decode().strip())
    print('SHARED_AFTER=' + after.decode().strip())
    raise SystemExit(rc if before == after else 1)
private = os.readlink('/proc/self/ns/net')
if private in {os.environ['NGFW_FRRLIVE_HOST_NS'], os.readlink('/proc/1/ns/net')}:
    raise SystemExit('private namespace required')
print('FRRLIVE_PRIVATE_NS=' + private, flush=True)
runtime = ROOT / '.scratch' / ('frr-live-' + str(os.getpid()))
for source_name, target in [('netns','/run/netns'),('frr','/run/frr'),('test-run','/run/ngfw-test')]:
    source = runtime / source_name
    source.mkdir(parents=True, mode=0o755)
    subprocess.run(['mount','--bind',str(source),target],check=True)
subprocess.run(['ip','link','set','lo','up'],check=True)
os.environ['NGFW_INTEGRATION']='1'
rc = subprocess.call([str(ROOT / 'tools/lab'),'lock','shared','go','test','-count=1','-v','-timeout','3m','-run','^TestOSPFLive$','./internal/renderers/frr/ospf/'],cwd=ROOT / 'apps/agent')
print('PRIVATE_NETNS_HANDLES=' + subprocess.check_output(['ip','netns','list']).decode(),flush=True)
print('PRIVATE_FRR_HANDLES=' + json.dumps([entry.name for entry in (runtime/'frr').iterdir()]),flush=True)
raise SystemExit(rc)
