#!/usr/bin/env python3
"""Authenticate to the disposable slot API without logging its bearer token."""
import json
import os
from pathlib import Path
import subprocess
import urllib.request

prefix = os.environ['VRX_TEST_PREFIX']
base = 'http://127.0.0.1:' + os.environ['VRX_HTTP_PORT']
password = Path('/run/vrx-test/' + prefix + '/admin.pw').read_text().strip()
req = urllib.request.Request(base + '/api/v1/auth/login', data=json.dumps({'username':'admin','password':password}).encode(), headers={'Content-Type':'application/json'}, method='POST')
with urllib.request.urlopen(req, timeout=20) as response:
    token = json.load(response)['accessToken']
env = dict(os.environ, VRX_API_URL=base, VRX_API_TOKEN=token,
           VRX_REACH_LOOPBACK=str(int(os.environ['VRX_SLOT']) * 100 + 99))
raise SystemExit(subprocess.call(['go', '-C', 'test/integration/reachability', 'test', '-json', '-race', '-count=1', '-timeout', '5m', '-run', 'TestReachabilityLoopback', './...'], env=env))
